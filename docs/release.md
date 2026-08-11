# Release 包、升级与回滚

每个正式标签会自动生成 Windows、Linux（AMD64/ARM64）和 macOS（Intel/Apple Silicon）发行包。包内包含可执行程序、`config.yaml` 模板、Web 管理页静态资源、启动脚本、快速开始和本说明；运行包不需要安装 Go、CGO 或系统 Opus 库。

## 选择与启动

| 系统 | 文件 | 启动方式 |
| --- | --- | --- |
| Windows x64 | `xiaozhi-server-go-<版本>-windows-amd64.zip` | 解压后双击 `start.bat` |
| Linux x64 / ARM64 | `xiaozhi-server-go-<版本>-linux-*.tar.gz` | 解压后执行 `./start.sh` |
| macOS Intel / Apple Silicon | `xiaozhi-server-go-<版本>-darwin-*.tar.gz` | 解压后执行 `./start.sh` |

首次运行若不存在 `.config.yaml`，服务会从 `config.yaml` 创建该私有配置文件并停止。填写 ASR、LLM、TTS 及 `web.websocket` 后，再运行启动脚本。首次正常启动后，浏览器访问：

```text
http://127.0.0.1:8080/api/ota/
```

能获得正常响应即表示 HTTP 服务已启动；管理页为 `http://127.0.0.1:8080`。

## 升级

1. 停止旧服务。
2. 完整备份旧运行目录，至少保留 `.config.yaml`、`config.db`、`.mcp_server_settings.json`、`ota_bin/` 和 `logs/`。这些文件可能包含密钥、设备状态或个人数据，不要上传到公开位置。
3. 将新版本解压到**新的空目录**，不要直接覆盖旧目录。
4. 从备份中复制 `.config.yaml`、`config.db`、可选的 `.mcp_server_settings.json` 与 `ota_bin/` 到新目录；不要用旧版 `web/` 覆盖新包的 `web/`。
5. 启动新版本，检查 OTA 健康检查、管理页和一台设备的语音会话。

## 回滚

若健康检查、管理页或设备会话异常：

1. 停止新版本；
2. 回到升级前完整备份的运行目录；
3. 使用旧版本启动脚本重新启动；
4. 确认 `http://127.0.0.1:8080/api/ota/` 与设备语音会话恢复。

保留旧目录的方式避免配置数据库或固件文件被新版本覆盖，回滚时无需手工编辑数据库。
