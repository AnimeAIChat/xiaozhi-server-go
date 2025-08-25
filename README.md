# ✨ 小智 AI 聊天机器人后端服务（商业版）

小智 AI 是一个语音交互机器人，结合 Qwen、DeepSeek 等强大大模型，通过 MCP 协议连接多端设备（ESP32、Android、Python 等），实现高效自然的人机对话。

本项目是其后端服务，旨在提供一套 **商业级部署方案** —— 高并发、低成本、功能完整、开箱即用。

<p align="center">
  <img src="https://github.com/user-attachments/assets/aa1e2f26-92d3-4d16-a74a-68232f34cca3" alt="Xiaozhi Architecture" width="600">
</p>

项目初始基于 [虾哥的 ESP32 开源项目](https://github.com/78/xiaozhi-esp32?tab=readme-ov-file)，目前已形成完整生态，支持多种客户端协议兼容接入。

---

## ✨ 核心优势

| 优势         | 说明                                                   |
| ---------- | ---------------------------------------------------- |
| 🚀 高并发     | 单机支持 3000+ 在线，分布式可扩展至百万用户                            |
| 👥 用户系统    | 完整的用户注册、登录、权限管理能力                                    |
| 💰 支付集成    | 接入支付系统，助力商业闭环                                        |
| 🛠️ 模型接入灵活 | 支持通过 API 调用多种大模型，简化部署，支持定制本地部署                       |
| 📈 商业支持    | 提供 7×24 技术支持与运维保障                                    |
| 🧠 模型兼容    | 支持 ASR（豆包）、TTS（EdgeTTS）、LLM（OpenAI、Ollama）、图文解说（智谱）等 |

---

## ✅ 功能清单

* [x] 支持 websocket 连接
* [x] 支持 PCM / Opus 格式语音对话
* [x] 支持大模型：ASR（豆包流式）、TTS（EdgeTTS/豆包）、LLM（OpenAI API、Ollama）
* [x] 支持语音控制调用摄像头识别图像（智谱 API）
* [x] 支持 auto/manual/realtime 三种对话模式，支持对话实时打断
* [x] 支持 ESP32 小智客户端、Python 客户端、Android 客户端连入，无需校验
* [x] OTA 固件下发
* [x] 支持 MCP 协议（客户端 / 本地 / 服务器），可接入高德地图、天气查询等
* [x] 支持语音控制切换角色声音
* [x] 支持语音控制切换预设角色
* [x] 支持语音控制播放音乐
* [x] 支持单机部署服务
* [x] 支持本地数据库 sqlite
* [x] 支持coze工作流 
* [x] 支持Docker部署
* [x] 支持MySQL,PostgreSQL（商务版功能）
* [x] 支持 MQTT 连接（商务版功能）
* [x] 支持dify工作流 （商务版功能）
* [x] 管理后台(商务版已完成：设备绑定，用户、智能体管理)


---

## 🚀 快速开始

### 1. 下载 Release 版

> 推荐直接下载 Release 版本，无需配置开发环境：

👉 [点击前往 Releases 页面](https://github.com/AnimeAIChat/xiaozhi-server-go/releases)

* 选择你平台对应的版本（如 Windows: `windows-amd64-server.exe`）
* `.upx.exe` 是压缩版本，功能一致，体积更小，适合远程部署

---


### 2. 配置 `.config.yaml`

* 推荐复制一份 `config.yaml` 改名为 `.config.yaml`
* 按需求配置模型、WebSocket、OTA 地址等字段
* 不建议自行删减字段结构

#### WebSocket 地址配置（必配）

```yaml
web:
  websocket: ws://your-server-ip:8000
```

用于 OTA 服务下发给客户端的连接地址，ESP32 客户端会自动从此地址连接 WS，不再手动配置。

注：如果是局域网调试，your-server-ip要配置为**电脑在局域网中的IP**，且终端设备和电脑在同一网段，设备才能通过这个IP地址连到电脑上的服务。

#### OTA 地址配置（必配）

```text
http://your-server-ip:8080/api/ota/
```

> ESP32 固件内置 OTA 地址，确保该服务地址可用，**服务运行后可以在浏览器中输出此地址，确认服务可以访问**。

ESP32设备可以在联网界面修改OTA地址，从而在不重新刷固件的情况下，切换后端服务。

#### 配置ASR，LLM，TTS

根据配置文件的格式，配置好相关模型服务，尽量不要增减字段

---

## 💬 MCP 协议配置

参考：`src/core/mcp/README.md`

---

## 🧪 源码安装与运行

### 前置条件

* Go 1.24.2+
* Windows 用户需安装 CGO 和 Opus 库（见下文）

```bash
git clone https://github.com/AnimeAIChat/xiaozhi-server-go.git
cd xiaozhi-server-go
cp config.yaml .config.yaml
```

---

### Windows 安装 Opus 编译环境

安装 [MSYS2](https://www.msys2.org/)，打开MYSY2 MINGW64控制台，然后输入以下命令：

```bash
pacman -Syu
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-go mingw-w64-x86_64-opus
pacman -S mingw-w64-x86_64-pkg-config
```

设置环境变量（用于 PowerShell 或系统变量）：

```bash
set PKG_CONFIG_PATH=C:\msys64\mingw64\lib\pkgconfig
set CGO_ENABLED=1
```

尽量在MINGW64环境下运行一次 “go run ./src/main.go” 命令，确保服务正常运行

GO mod如果更新较慢，可以考虑设置go代理，切换国内镜像源。

---

### 运行项目

```bash
go mod tidy
go run ./src/main.go
```

### 编译发布版本

```bash
go build -o xiaozhi-server.exe src/main.go
```

---

## 📚 Swagger 文档

* 打开浏览器访问：`http://localhost:8080/swagger/index.html`

### 更新 Swagger 文档（每次修改 API 后都要运行）

```bash
cd src
swag init -g main.go
```

---

## ☁️ CentOS 源码部署指南

> 文档见：[Centos 8 安装指南](Centos_Guide.md)

---

## Docker 环境部署

1. 准备`docker-compose.yml`,`.config.yaml`,二进制程序文件

👉 [点击前往 Releases 页面](https://github.com/AnimeAIChat/xiaozhi-server-go/releases)下载二进制程序文件

* 选择你平台对应的版本（默认使用 Liunx: `linux-amd64-server-upx`，如使用其他版本，需要修改docker-compose.yml）

2. 三个文件放到同一目录下，配置`docker-compose.yml`,`.config.yaml`

3. 运行`docker compose up -d`

---

## 💬 社区支持


欢迎提交 Issue、PR 或新功能建议！

<img src="https://github.com/user-attachments/assets/ceacc885-0f8f-4e35-b3b3-25b49f5ee146" width="450" alt="微信群二维码">

---

## 🛠️ 定制开发

我们接受各种定制化开发项目，如果您有特定需求，欢迎通过微信联系洽谈。

<img src="https://github.com/user-attachments/assets/e2639bc3-a58a-472f-9e72-b9363f9e79a3" width="450" alt="群主二维码">

## 📄 License

本仓库遵循 `Xiaozhi-server-go Open Source License`（基于 Apache 2.0 增强版）
