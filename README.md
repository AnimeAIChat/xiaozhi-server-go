# 小智语音助手服务端（Go）

`xiaozhi-server-go` 是面向个人和家庭场景的小智语音助手服务端。它通过 WebSocket 与兼容的小智客户端通信，将语音识别、对话模型和语音合成串成一段可本地部署的语音会话。

> 想快速跑通一台设备？请直接阅读 [快速开始](docs/quick-start.md)。

<p align="center">
  <img src="https://github.com/user-attachments/assets/aa1e2f26-92d3-4d16-a74a-68232f34cca3" alt="小智服务端架构示意图" width="600">
</p>

本项目兼容小智 WebSocket 协议，可配合 ESP32、Android、Python 等兼容客户端使用。

## 已实现功能

| 分类 | 功能 |
| --- | --- |
| 设备连接 | WebSocket 连接；PCM / Opus 语音编解码；兼容小智协议客户端 |
| 语音对话 | 自动、手动、实时三种对话模式；支持语音打断 |
| 模型接入 | ASR、LLM、TTS、视觉模型均可按 `config.yaml` 选择；内置 OpenAI 兼容接口、Ollama、豆包、Coze、Edge TTS、Deepgram、GoSherpa、讯飞、阶跃等接入实现 |
| 视觉能力 | 客户端可通过语音触发摄像头图像识别 |
| 角色与声音 | 预设角色、预设声音与语音指令切换 |
| 短期记忆 | 已绑定设备重连后可延续短期上下文；按设备与智能体隔离，支持在设备管理页清空 |
| 扩展能力 | 本地 MCP、设备 MCP 与外部 Stdio MCP；可配置天气、地图等工具 |
| 设备接入 | OTA 接口、固件文件下载接口与 WebSocket 地址下发 |
| 运维与数据 | 本地 SQLite 配置存储、日志、Swagger API 文档、Docker 运行方式 |

上述能力以当前代码和配置模板为准；尚在规划或开发中的功能会通过 [Issues](https://github.com/AnimeAIChat/xiaozhi-server-go/issues) 跟踪。

## 5 分钟快速开始

完整说明见 [docs/quick-start.md](docs/quick-start.md)。以下为最短路径：

1. 从 [Releases](https://github.com/AnimeAIChat/xiaozhi-server-go/releases) 下载与系统匹配的服务端程序，并将 `config.yaml` 放到同一目录。
2. 复制为私有配置文件并填写至少一组可用的 ASR、LLM、TTS 配置：

   ```powershell
   Copy-Item config.yaml .config.yaml
   ```

   ```bash
   cp config.yaml .config.yaml
   ```

3. 将 `web.websocket` 配置为设备能够访问的地址，例如 `ws://192.168.1.10:8000`；不要使用 `localhost` 作为设备地址。
4. 启动下载的程序，浏览器打开 `http://127.0.0.1:8080`。局域网设备使用 `http://<服务器局域网 IP>:8080/api/ota/` 作为 OTA 地址。
5. 让设备联网并发起一次语音对话；服务端日志会依次显示连接、识别、模型回答和播报过程。

如果目录中没有 `.config.yaml`，首次启动会从 `config.yaml` 生成私有配置文件后停止；填写模型配置后重新启动即可。成功启动后，配置会保存到同目录的 `config.db`。后续修改请通过管理接口保存，或在备份后重新创建运行目录。详情见 [配置与排错](docs/quick-start.md#配置与排错)。

## 配置要点

配置模板：[`config.yaml`](config.yaml)。建议始终保留原有字段，仅替换示例值。

```yaml
server:
  port: 8000

web:
  port: 8080
  # 供局域网设备连接的地址，替换成服务器局域网 IP 或域名
  websocket: ws://192.168.1.10:8000

selected_module:
  ASR: DoubaoASR
  TTS: EdgeTTS
  LLM: OllamaLLM
  VLLLM: ChatGLMVLLM
```

- `selected_module` 中的名称必须与下方 `ASR`、`TTS`、`LLM`、`VLLLM` 分组中的配置名称一致。
- 凭据只保存在本机的 `.config.yaml` 或配置数据库中。请勿将 `.config.yaml`、`config.db`、日志或 MCP 密钥提交到仓库。
- 设备与服务端不在同一台机器时，需要允许 TCP `8000` 和 `8080` 通过系统防火墙；设备与服务端应能相互访问。
- 本项目当前使用 WebSocket 接入设备，`transport.websocket.enabled` 应保持为 `true`。

## 源码运行

源码运行适合开发与调试。需要 Go `1.24.2`（项目指定的 toolchain）以及可用的 C 编译器供 SQLite 驱动构建；Opus 依赖已随仓库提供，无需单独安装系统 Opus 库。

```bash
git clone https://github.com/AnimeAIChat/xiaozhi-server-go.git
cd xiaozhi-server-go
cp config.yaml .config.yaml
go run ./src/main.go
```

Windows PowerShell：

```powershell
git clone https://github.com/AnimeAIChat/xiaozhi-server-go.git
Set-Location xiaozhi-server-go
Copy-Item config.yaml .config.yaml
go run ./src/main.go
```

如果 Windows 上提示找不到 C 编译器，请安装 MSYS2 的 UCRT64 或 MINGW64 工具链，并确保 `gcc` 位于 `PATH` 后重新执行。不要在配置文件中填写真实密钥后提交或分享该文件。

编译程序：

```bash
go build -o xiaozhi-server ./src/main.go
```

## Docker 运行

仓库提供 [`docker-compose.yml`](docker-compose.yml)，用于运行已下载的 Linux 版本程序。将下列文件放入同一目录后，按实际文件名修改 `command`：

- Linux 服务端程序；
- `.config.yaml`；
- `docker-compose.yml`。

然后执行：

```bash
docker compose up -d
docker compose logs -f
```

需要将 `8000` 与 `8080` 映射到宿主机，并把 `.config.yaml` 中的 `web.websocket` 写为设备能够访问的宿主机 IP 或域名。

## MCP 配置

MCP 使用方式、外部 Stdio MCP 示例和设备 MCP 说明见 [src/core/mcp/README.md](src/core/mcp/README.md)。外部 MCP 配置文件为 `.mcp_server_settings.json`，其中可能含有服务密钥，应只保存在本机。

## 接口与排错

- Web 控制台：`http://127.0.0.1:8080`
- Swagger：`http://127.0.0.1:8080/swagger/index.html`
- OTA 健康检查：`http://127.0.0.1:8080/api/ota/`
- 运行日志：`logs/server.log`
- 常见问题与一台设备的验证流程：[docs/quick-start.md](docs/quick-start.md)
- 升级、回滚与 Release 包内容：[docs/release.md](docs/release.md)
- 短期记忆的边界与清理方式：[docs/short-term-memory.md](docs/short-term-memory.md)

## 更多功能

小智商业版在相同协议与模型接入基础上，提供更多设备管理、知识库、声音复刻、定制音色、工作流和部署能力。若希望了解这些功能，可通过下方微信二维码联系。

## 社区与反馈

欢迎提交 Issue、PR 或功能建议。

<img src="https://github.com/Eric0308/assert/blob/main/xiaozhi/qr.jpg" width="450" alt="微信交流群二维码">
<img src="https://github.com/user-attachments/assets/074c6aec-cfb5-4a68-8fc2-2d08679e366b" width="450" alt="QQ群二维码">

## License

本仓库遵循 `Xiaozhi-server-go Open Source License`（基于 Apache 2.0 增强版）。
