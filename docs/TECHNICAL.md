# xiaozhi-server-go 技术文档（面向二次开发）

本文档用于快速理解 `xiaozhi-server-go` 的整体架构、关键模块边界、启动链路、配置/数据库机制，以及你后续做功能修改时应从哪里切入。

说明：
- 文档以当前代码实现为准（以 `src/` 为核心）。
- 所有路径/函数名可直接在 IDE 中跳转定位。

## 1. 项目定位与组成

这是一个“小智”语音交互机器人后端服务：
- 设备/客户端通过 WebSocket 接入（当前主要实现），上传音频或文本。
- 服务端负责 ASR（语音识别）→ LLM（大模型对话）→ TTS（语音合成）并把音频回推给客户端。
- 支持 Vision（图像问答）与 OTA（固件下发/设备注册）。
- 支持 MCP（Model Context Protocol）工具体系：本地工具、外部 MCP 服务器、以及“客户端设备侧 MCP 工具”。

核心目录：
- `src/main.go`：进程入口、服务编排（WS 传输 + HTTP）。
- `src/core/`：连接/会话、对话链路、providers、MCP、资源池。
- `src/configs/`：配置结构、加载策略（DB 优先）、默认配置初始化。
- `src/configs/database/`：SQLite 初始化、迁移、业务 CRUD（User/Agent/Device/Provider…）。
- `src/httpsvr/`：HTTP 服务（OTA/Vision/WebAPI）。
- `web/`：前端静态产物（已构建文件）。

## 2. 运行与构建

本地运行（README 给出的方式）：
```bash
go run ./src/main.go
```

构建二进制：
```bash
go build -o xiaozhi-server ./src/main.go
```

Makefile：
- `Makefile` 提供 `make run/build`（内部就是 `go build` + 运行）。

Docker：
- `docker-compose.yml` 以镜像方式启动并映射 `8080/8000`，并把当前目录挂载到容器 `/app`。

## 3. 服务与端口

### 3.1 WebSocket 传输层（设备接入）
- 监听：`config.Transport.WebSocket.(IP/Port)`，默认端口 `8000`。
- 实现：`src/core/transport/websocket/transport.go`。
- WebSocket 路径：`/`（`http.ServeMux` 只注册了根路径）。

### 3.2 HTTP 服务（管理后台/OTA/Vision/静态站点）
- 监听：`config.Web.Port`，默认端口 `8080`。
- 实现：`src/main.go` 的 `StartHttpServer`。
- 路由前缀：API 统一挂载在 `/api`（`src/main.go`）。
- 静态前端：`./web` 目录被托管到站点根路径 `/`。
- Swagger：`/swagger/index.html`。

## 4. 启动链路（主流程）

入口：`src/main.go:main`
1. `LoadConfigAndLogger()`：
   - 加载 `.env`（可选）。
   - 初始化 DB：`src/configs/database/init.go:InitDB`（默认 `./config.db` SQLite）。
   - 加载配置：`src/configs/config.go:LoadConfig`（DB 优先，空才回落文件）。
   - 初始化日志：`src/core/utils/logger.go:NewLogger`。
   - 写入默认配置/默认用户/默认 Provider：`database.InsertDefaultConfigIfNeeded(...)`。
2. 初始化认证管理器：`src/core/auth/manager.go`（用于 WebAPI/Token 等）。
3. `StartTransportServer(...)`：
   - 初始化资源池：`src/core/pool/manager.go:NewPoolManager`。
   - 初始化任务管理器：`src/task/manager.go`（当前对核心对话链路依赖不强，更多是通用异步任务能力）。
   - 注册并启动 WebSocketTransport：`src/core/transport/websocket/transport.go:Start`。
4. `StartHttpServer(...)`：
   - Gin 路由：OTA/Vision/WebAPI + 静态站点 + Swagger。
5. `GracefulShutdown(...)`：SIGINT/SIGTERM 优雅退出。

### 4.1 关键入口索引（建议收藏）

- 服务编排与入口：`src/main.go:62`（LoadConfigAndLogger）、`src/main.go:114`（StartTransportServer）、`src/main.go:192`（StartHttpServer）、`src/main.go:380`（main）
- 配置结构与加载：`src/configs/config.go:12`（Config struct）、`src/configs/config.go:193`（LoadConfig）、`src/configs/config.go:293`（CheckAndModifyConfig）
- 连接主循环与对话：`src/core/connection.go:536`（Handle）、`src/core/connection.go:714`（handleChatMessage）、`src/core/connection.go:766`（genResponseByLLM）、`src/core/connection.go:1242`（Close）、`src/core/connection.go:1264`（genResponseByVLLM）
- WebSocket 传输：`src/core/transport/websocket/transport.go:39`（Start）、`src/core/transport/websocket/transport.go:105`（handleWebSocket）
- 资源池：`src/core/pool/manager.go:33`（NewPoolManager）、`src/core/pool/manager.go:140`（GetProviderSet）、`src/core/pool/manager.go:231`（ReturnProviderSet）
- MCP 管理：`src/core/mcp/manager.go:42`（NewManagerForPool）、`src/core/mcp/manager.go:135`（BindConnection）、`src/core/mcp/manager.go:411`（ExecuteTool）

## 5. 配置系统（非常重要）

### 5.1 加载优先级与落盘位置
配置加载：`src/configs/config.go:LoadConfig`
- 默认优先从数据库读取 `models.ServerConfig(cfg_str)`：路径表现为 `database:serverConfig`。
- DB 中没有配置时才会回落读取 `.config.yaml`，若不存在再读 `config.yaml`。
- 一旦配置写入 DB，后续你再改 `config.yaml` 通常不会生效，除非：
  - 你通过管理后台接口更新 DB 配置；
  - 或清空/替换 `config.db` 里的 `server_configs`（慎重）。

### 5.2 模块选择（ASR/LLM/TTS/VLLLM）
`config.SelectedModule` 用于选择当前启用的具体 Provider 名称（不是 provider type）。
资源池创建时会读取这些选择：`src/core/pool/manager.go:NewPoolManager`。

和设备接入强相关的两个 URL：
- `config.Web.Websocket`：OTA 下发给设备的“应该连接的 WebSocket 地址”（字符串），必须配置为设备可达的地址，例如 `ws://<公网域名>:8000` 或 `wss://<域名>/ws`。
- `config.Web.VisionURL`：MCP initialize 会下发给设备，用于设备侧调用 Vision HTTP API（必须设备可达）。

配置在 `LoadConfig` 后还会做“缺失修复/默认补齐”：
- `src/configs/config.go:CheckAndModifyConfig`

### 5.3 Provider 配置存储
Provider 既存在于配置（`config.ASR/TTS/LLM/VLLLM`），也会初始化/写入 DB 的 provider 表：
- `src/configs/database/providers.go`
- 表：`models.ASRConfig/LLMConfig/TTSConfig/VLLLMConfig`

注意：Web API 返回 provider 时会移除部分敏感字段（`RemoveSensitiveFields`）。

## 6. 数据库与模型

DB：`./config.db`（SQLite）
- 初始化/迁移：`src/configs/database/init.go`
- 自动迁移模型：User/Agent/Device/ProviderConfig/ServerConfig/ServerStatus 等。

关键模型（`src/models/models.go`、`src/models/providers.go`）：
- `User`：管理后台登录用户（角色：admin/observer/user）。
- `Agent`：智能体（Prompt/Voice/LLM/Tools…）。
- `Device`：设备（DeviceID/MAC、ClientID、AgentID、状态、软删除）。
- `ServerConfig`：服务端配置 YAML 字符串（DB 中保存一份）。
- `ServerStatus`：CPU/内存/在线数量等状态（定时更新）。

默认 admin：
- 初始化：`src/configs/database/user.go:InitAdminUser`
- 默认账号：`admin`，默认密码：`123456`（首次启动会打印提示）。

## 7. 设备接入与会话链路（WebSocket）

连接入口：
- `src/core/transport/websocket/transport.go:handleWebSocket`
- `src/core/transport/connection_adapter.go`：把传输层连接适配为 `core.ConnectionHandler` 的工作方式，并负责 providerSet 归还资源池。

连接处理器核心：
- `src/core/connection.go:Handle`

对话关键点：
- 文本消息解析/分发：`src/core/connection_handlemsg.go:processClientTextMessage`
- 二进制音频入队与 ASR：`src/core/connection_handlemsg.go:handleMessage` + `src/core/connection.go:processClientAudioMessagesCoroutine`
- ASR 回调触发对话：`src/core/connection.go:OnAsrResult`
- LLM 流式输出与工具调用：`src/core/connection.go:genResponseByLLM`
- TTS 任务队列与音频帧发送：`src/core/connection.go:processTTSQueueCoroutine` + `src/core/connection_sendmsg.go:sendAudioFrames`

## 8. MCP（工具系统）工作方式

MCP 在本项目中有三类来源：
1. 本地工具（Local MCP）：
   - 配置：`config.LocalMCPFun`
   - 实现：`src/core/mcp/local_client.go`、`src/core/mcp/local_mcp_tools.go`
   - 对 LLM 暴露为 `local_*` 工具名（例如 `local_exit`、`local_play_music`）。
2. 服务端外部 MCP（stdio 方式）：
   - 配置文件：`.mcp_server_settings.json`（二进制同目录/项目根目录）
   - 说明：`src/core/mcp/README.md`
   - 实现：`src/core/mcp/client.go`
   - 对 LLM 暴露为 `mcp_*` 工具名。
3. “客户端设备侧 MCP 工具”（小智 MCP）：
   - 服务端通过 WebSocket 向设备发起 MCP initialize/tools/list/tools/call。
   - 实现：`src/core/mcp/xiaozhi_client.go`
   - 对 LLM 暴露为“点号替换为下划线”的工具名（例如 `self.get_device_status` → `self_get_device_status`）。

聚合管理：
- `src/core/mcp/manager.go`（池化，连接绑定时注册工具到 FunctionRegistry）。

## 9. HTTP 子系统概览

### 9.1 OTA
- 注册：`src/httpsvr/ota/server.go`
- 处理：`src/httpsvr/ota/handler.go`
- 用途：
  - 设备上报信息/注册设备；
  - 返回固件下载 URL 与 WebSocket 接入地址；
  - 提供固件下载 `/api/ota_bin/*`。

### 9.2 Vision
- `src/httpsvr/vision/server.go`
- 用途：图片上传与视觉模型分析（需要 Bearer token + Device-Id）。

### 9.3 WebAPI（后台）
- 用户：`src/httpsvr/webapi/user.go`（login/profile/agent/device/providers…）
- 管理员：`src/httpsvr/webapi/system.go`（系统配置、providers 管理）
- 系统配置：`src/httpsvr/webapi/system_config.go`（拆分的配置项管理）
- 鉴权：`src/httpsvr/webapi/auth.go`（JWT + AdminMiddleware）

## 10. 日志与排障

日志实现：`src/core/utils/logger.go`
- 控制台：彩色文本（带时间/级别/标签）。
- 文件：JSON（默认 `logs/server.log`）。
- 支持按天轮转（保留天数硬编码）。

运行时关键日志标签：
- `[ASR]`、`[LLM]`、`[TTS]`、`[MCP]` 等。

## 11. 常见二次开发切入点

协议/消息扩展：
- `src/core/connection_handlemsg.go`（新增 `type` 分支）
- `src/core/connection_sendmsg.go`（新增服务端下发消息）

对话策略/分句/工具调用策略：
- `src/core/connection.go:genResponseByLLM`

新增/替换模型 Provider：
- `src/core/providers/*`（具体实现）
- `src/core/pool/factory.go`（工厂创建）
- `src/configs/config.go`（配置结构/默认值/校验）

管理后台接口：
- `src/httpsvr/webapi/*`

## 12. 已知“坑”和行为差异（建议你后续优先处理）

这些点会影响你做客户端接入或后续重构：
1. 配置 DB 优先：`config.yaml` 改动可能不生效（见 5.1）。
2. `go test ./...` 当前会被 `go vet` 的格式化检查卡住（大量 logger 调用风格混用导致），但 `go build ./src/main.go` 可通过。
3. 系统配置写回键名疑似不一致：`src/httpsvr/webapi/system.go` 写入 `SelectedModule["VLLM"]`，而其他地方使用 `VLLLM`。
4. 小智协议中 `type:"chat"` 的实现与直觉不一致（服务端传参方式容易导致把整段 JSON 当文本）。客户端接入建议优先使用 `listen.state="detect"` 携带 `text` 走纯文本对话（协议文档中会明确）。

## 13. Prompt / Roles / Agent 的关系（人格与系统提示词）

这一节回答你在二次开发中最常遇到的问题：**“默认 prompt、Agent prompt、roles 角色 prompt，谁覆盖谁？什么时候生效？能不能运行时切换？”**

### 13.1 概念澄清：两种“role”

代码里同时存在两类容易混淆的概念：
- **对话消息的 `role`**：`src/core/types/llm.go:64` 的 `types.Message.Role`，常见值是 `system/user/assistant/tool`，用于组织“发给 LLM 的消息列表”。
- **业务“角色”(Role)**：`src/configs/config.go:94` 的 `configs.Role`（`Name/Description/Enabled`），用于表示一组可切换的人设/职责。它本质上就是一段可替换的 **system prompt 文本**（`Role.Description`）。

### 13.2 system prompt 的三种来源与优先级

本项目最终会把“当前要生效的 prompt”写入到 `DialogueManager` 的第 0 条消息（`role=system`），即 system prompt：
- 写入点：`src/core/connection.go:239-240` → `src/core/chat/dialogue_manager.go:28`

system prompt 的来源分三类（按连接初始化阶段的优先级）：
1. **Agent Prompt（最高，若设备绑定 Agent）**
   - 存储：`src/models/models.go:11-33` 的 `models.Agent.Prompt`
   - 选择逻辑：`src/core/connection.go:247-283` 的 `InitWithAgent()`
   - 特殊处理：
     - 支持把 `{{assistant_name}}` 替换成 `agent.Name`（或追加一行助手名称）：`src/core/connection.go:258-266`
     - `agent.Language` 非中文时追加语言要求：`src/core/connection.go:268-270`
     - 读取 `agent.EnabledTools` 写入 `h.enabledTools`：`src/core/connection.go:272-276`（注意：当前代码里 **只写不读**，不会真正过滤工具）
2. **DefaultPrompt（没有绑定 Agent 时）**
   - 存储：`src/configs/config.go:66` 的 `config.DefaultPrompt`（yaml 字段名 `prompt`）
   - 默认值：`src/configs/config_default_init.go:124-132`
   - 管理端修改：`src/httpsvr/webapi/system.go:88-172`（`/api/admin/system` 写回 DB）
3. **Role Prompt（运行时切换，不参与初始化优先级）**
   - 存储：`src/configs/config.go:67` 的 `config.Roles[].Description`（yaml 字段名 `roles[].description`）
   - 默认值：`src/configs/config_default_init.go:133-146`（示例三种角色）
   - 运行时切换逻辑见 13.4。

总结成一句话：
- **连接初始化：`Agent.Prompt` 覆盖 `DefaultPrompt`。**
- **运行时切换：`Role.Description` 会覆盖当前 system prompt，但不落库、不持久化。**

### 13.3 注入点与生效范围（连接级）

连接建立后，`ConnectionHandler` 会初始化 `DialogueManager` 并设置 system prompt：
- `src/core/connection.go:232-240`：
  - `checkDeviceInfo()` 决定设备绑定哪个 `agentID`（来自 DB 的 `Device.AgentID`，若为空会绑定“第一个 agent”）：`src/core/connection.go:444-480`
  - `InitWithAgent()` 选择最终 prompt（`Agent.Prompt` 或 `DefaultPrompt`）
  - `dialogueManager.SetSystemMessage(prompt)` 写入对话上下文

`DialogueManager.SetSystemMessage()` 的行为是“覆盖第 0 条 system 消息内容”，而不是不断追加：
- `src/core/chat/dialogue_manager.go:28-43`

角色/智能体切换时，为了避免上下文混乱，会裁剪历史消息：
- `DialogueManager.KeepRecentMessages(n)`：保留 system + 最近 `n` 条消息：`src/core/chat/dialogue_manager.go:54-69`
- 切换角色：保留最近 5 条：`src/core/connection_handlemcp.go:184`
- 切换智能体：保留最近 1 条：`src/core/connection_handlemcp.go:97`

### 13.4 roles 如何参与运行时切换（通过 Local MCP 工具）

roles 并不是协议层字段，它是通过“本地 MCP 工具”暴露给 LLM 的：
1. 启动时注册本地工具（池化、一次性）：
   - `src/core/mcp/manager.go:74-78` → `NewLocalClient(... cfg ...)` → `LocalClient.Start()` → `RegisterTools()`
2. 如果 `config.LocalMCPFun` 中 `change_role` 启用，则注册 `change_role` 工具：
   - `src/core/mcp/local_client.go:52-76`
3. 工具实现会读取 `cfg.Roles`，构建 `roleName -> role.Description` 的映射，并把工具暴露给 LLM（工具名会被加前缀 `local_`）：
   - `src/core/mcp/local_mcp_tools.go:66-113`（`AddToolChangeRole()`）
4. LLM 调用工具后，最终会触发连接处理器的 handler：
   - `src/core/connection_handlemcp.go:177-203`（`mcp_handler_change_role()`）
   - 行为：
     - `dialogueManager.SetSystemMessage(prompt)`：把 system prompt 切到选中角色的 prompt
     - `KeepRecentMessages(5)`：裁剪上下文
     - 若 TTS provider 是 `edge`，会按角色名硬编码切换部分音色：`src/core/connection_handlemcp.go:185-195`

### 13.5 管理端入口（修改后何时生效）

- DefaultPrompt：
  - 读写接口：`/api/admin/system`（`src/httpsvr/webapi/system.go`）
  - 生效：新连接会使用新 prompt；已建立连接不会自动替换 system prompt（除非你主动触发切换逻辑或重连）。
- Roles：
  - CRUD 接口：`/api/admin/config/roles`（`src/httpsvr/webapi/system_config.go:54-58` + `handleGetRoleConfigs/...`）
  - 注意：roles 的变更不一定会立刻反映到 LLM 可见的工具描述，见 13.6。
- Agent Prompt：
  - 读写接口：`/api/user/agent/*`（`src/httpsvr/webapi/agent.go`）
  - 生效：设备绑定到该 agent 后，新连接会用 `Agent.Prompt`（或运行时调用 `switch_agent` 工具后重置）。

### 13.6 当前实现的“坑”（读这段能省很多时间）

1. `Role.Enabled` 目前没有被 `change_role` 工具过滤使用：
   - `AddToolChangeRole()` 会把 `cfg.Roles` 全量加入映射：`src/core/mcp/local_mcp_tools.go:66-81`
2. roles 的热更新不完整：
   - `AddToolChangeRole()` 在注册工具时把 `roles -> prompts` 映射固化进闭包。
   - 因为本地工具是在 `MCP Manager` 预初始化阶段一次性注册的（`src/core/mcp/manager.go:74-78`），所以你在后台更新 roles 后，**LLM 侧看到的可选角色列表/对应 prompt 可能不会更新**，通常需要重启进程（或你后续做成“重新注册工具”的热更新机制）。
3. `Agent.EnabledTools` 当前不会真正过滤工具（只记录到 `h.enabledTools`，但没看到后续使用点）：
   - 写入点：`src/core/connection.go:272-276`
