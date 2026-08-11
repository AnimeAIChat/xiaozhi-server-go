# 快速开始：启动并接入一台设备

本文以局域网中的一台兼容小智协议设备为例，完成一次“说话—识别—回答—播报”验证。

## 开始前

准备以下内容：

- 一台 Windows、Linux 或 macOS 电脑；
- 一台与电脑处在可互访网络中的兼容小智协议设备；
- 至少一组可用的 ASR、LLM、TTS 服务配置；
- 服务端电脑的局域网 IP，例如 `192.168.1.10`。

设备通过 WebSocket 连接服务端，因此 `127.0.0.1`、`localhost` 只能用于电脑本机浏览器，不能写入设备使用的 WebSocket 或 OTA 地址。

## 方式一：运行 Release 程序

1. 从 [Releases](https://github.com/AnimeAIChat/xiaozhi-server-go/releases) 下载与系统匹配的程序，解压到一个专用目录。
2. 从仓库获取当前 [`config.yaml`](../config.yaml)，放到该目录，复制为 `.config.yaml`。
3. 用编辑器打开 `.config.yaml`，完成下面三项最小配置：

   ```yaml
   web:
     websocket: ws://192.168.1.10:8000

   selected_module:
     ASR: DoubaoASR
     TTS: EdgeTTS
     LLM: OllamaLLM
   ```

   - `web.websocket` 中的 IP 或域名必须能被设备访问。
   - `selected_module` 的名称必须对应各 Provider 分组中一个已填写可用凭据的配置。
   - 示例中的 `你的api_key`、`你的access_token` 等占位文字必须替换为实际值；未使用的 Provider 可以保留原样，但不要将其选为默认模块。

4. 启动程序。Windows 直接运行下载的 `.exe`；Linux/macOS 为程序增加执行权限后运行：

   ```bash
   chmod +x ./linux-amd64-server-upx
   ./linux-amd64-server-upx
   ```

5. 在服务端电脑上打开 `http://127.0.0.1:8080/api/ota/`。返回正常状态和 WebSocket 地址即表示 HTTP 服务已启动。

如果首次直接启动时目录里没有 `.config.yaml`，服务会先从 `config.yaml` 创建私有配置文件并停止。这是正常的初始化步骤：填写新生成的 `.config.yaml` 中的模型配置后，再次启动即可。首次正常启动还会在日志中显示一次随机生成的本地管理员临时密码，请立即登录并修改；不要把该密码或日志发布到公开位置。

## 方式二：从源码运行

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

项目使用 Go `1.24.2` toolchain。SQLite 驱动需要 C 编译器；Windows 使用者可安装 MSYS2 UCRT64 或 MINGW64 工具链，并确保 `gcc` 可从 `PATH` 调用。仓库已经包含 Opus 的 Go 实现，不需要单独安装系统 Opus 库。

## 配置设备

1. 将设备的 OTA 地址填写为：

   ```text
   http://192.168.1.10:8080/api/ota/
   ```

2. 设备请求 OTA 接口后会获得 `web.websocket` 中的 WebSocket 地址。
3. 确认服务端电脑防火墙允许设备访问 TCP `8000` 和 `8080`。
4. 让设备联网，观察控制台或 `logs/server.log` 中的连接日志。
5. 对设备说一句简短的话。一次正常会话通常会依次出现设备连接、ASR 识别、LLM 回答、TTS 播报相关日志。

## 验证入口

| 用途 | 地址 |
| --- | --- |
| Web 控制台 | `http://127.0.0.1:8080` |
| Swagger API | `http://127.0.0.1:8080/swagger/index.html` |
| OTA 健康检查 | `http://127.0.0.1:8080/api/ota/` |
| 局域网设备 OTA 地址 | `http://<服务器 IP>:8080/api/ota/` |

## 配置与排错

### 修改 `.config.yaml` 后没有生效

服务首次正常启动时会将配置写入同目录的 `config.db`，后续启动优先读取该数据库。若缺少 `.config.yaml`，服务会先生成它并停止，填写配置后再启动。已经运行过的目录需要通过管理接口保存配置；如果是全新调试环境，请先备份后重新创建独立的运行目录，避免覆盖仍在使用的配置与数据。

### 设备连不上服务端

检查以下内容：

1. `web.websocket` 是否填写为服务器的局域网 IP 或可访问域名，而不是 `localhost`；
2. 设备、服务端是否可互相访问；
3. TCP `8000` 与 `8080` 是否被系统防火墙、路由器或容器端口映射拦截；
4. `transport.websocket.enabled` 是否为 `true`；
5. `http://<服务器 IP>:8080/api/ota/` 是否能从局域网的另一台设备打开。

### 能连接但没有语音回复

1. 检查 `selected_module` 指向的 ASR、LLM、TTS Provider 名称是否存在；
2. 检查所选 Provider 的凭据、地址、模型名称是否有效；
3. 查看 `logs/server.log` 中最早出现的错误；
4. 先以简短中文语句测试，再逐一切换 ASR、LLM、TTS 配置定位问题。

### 配置文件或日志能否分享

不建议直接公开分享 `.config.yaml`、`config.db`、`.mcp_server_settings.json` 或完整日志。这些文件可能包含模型服务密钥、访问令牌、设备标识或 MCP 凭据。提交 Issue 时请删除敏感字段，只保留错误信息和必要的脱敏配置片段。

## 下一步

- 配置本地、设备或外部 MCP 工具：[MCP 使用说明](../src/core/mcp/README.md)
- 查看 OTA 接口细节：[OTA 模块说明](../src/httpsvr/ota/README.md)
- 提交问题或建议：[GitHub Issues](https://github.com/AnimeAIChat/xiaozhi-server-go/issues)
