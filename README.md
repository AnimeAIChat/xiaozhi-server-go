# 小智服务商业版

本项目目标为 **小智AI提供商业版后端解决方案**，提供 **高并发，低成本，功能全面，开箱即用的高性能服务**，助力企业快速搭建小智后端服务。

项目大模型调用全部采用api方式调用，不在本地部署模型，以保证服务的精简和便捷部署。
如需要本地部署模型，可单独部署并开放api供本项目调用即可。

**核心优势**

✔ **高并发**——单台服务支持3000以上用户同时在线，分布式部署支持百万用户同时在线

✔ **用户管理**——提供完整的用户注册、登录、管理等功能

✔ **便捷收费**——提供完善的支付系统，助力企业快速实现盈利

✔ **专业运维保障**——资深技术团队提供7×24小时监控、故障响应及性能优化支持

# 功能清单

* [x] 支持PCM格式的语音对话
* [x] 支持Opus格式的语音对话
* [x] 支持的模型 ASR(豆包流式）LLM（OpenAi API）TTS（EdgeTTS，豆包TTS）
* [x] 识图解说（智谱)
* [x] OTA功能
* [x] 支持服务端mcp
* [x] 支持小智客户端mcp调用
* [ ] 管理后台
* [ ] 支持mqtt连接
* [ ] IOT功能

# 安装和使用

## 下载Release版

推荐下载[Releases ](https://github.com/AnimeAIChat/xiaozhi-server-go/releases)版体验，避免配置开发环境，尽快体验服务效果

选择对应平台的版本，以windows为例，可以下载windows-amd64-server.exe，也可以下载windows-amd64-server-upx.exe（经过upx压缩，体积更小，效果相同，方便远程部署，其他平台均有upx版本）

下载后放到一个目录，尽量使用英文路径

## 配置config

在同目录下拷贝一份config.yaml文件，推荐改名为.config.yaml

### 配置WS地址

修改配置下web选项的websocket ，将其设为你的ip，格式为ws://xxx.xxx.xxx.xxx:8000，端口8000和server配置的端口保持一致

此地址通过ota下发给客户端，最新版本的esp32小智不能配置ws地址，只能通过ota下发

### 配置ota地址

esp32硬件编码，将ota地址写入到硬件；有ota配置的功能的设备可以直接配置地址，地址为

http://xxx.x.x.x:8080/api/ota/

8080端口为ota服务配置的地址，如果修改了配置文件，此处做相应修改

### 配置ASR，LLM，TTS

根据配置文件的格式，配置好相关模型服务，尽量不要增减字段

## 开始体验

启动服务，重启小智，如果正确连接到ota服务，服务日志会有打印

... POST     "/api/ota/"

表示客户端已经连接到ota服务，并获取了ws地址，后面请尽情体验

## MCP配置使用

参考MCP目录下的[README文件](https://github.com/AnimeAIChat/xiaozhi-server-go/blob/main/src/core/mcp/README.md)

# 源码安装和部署

## 前置条件

本项目采用go语言编写，windows建议使用g维护go版本，mac可以使用gvm维护go版本

* go 1.24.2

## 下载源码

git clone https://github.com/AnimeAIChat/xiaozhi-server-go.git

## 修改配置文件

复制一份config.yaml到.config.yaml，并在配置中填好相应的key等信息，避免密钥泄漏

## windows安装opus库

由于使用了cgo链接opus库，在windows上配置稍微复杂，需要安装cgo编译环境

首先，下载msys2，安装 GCC 工具链：
```
pacman -Syu

pacman -S mingw-w64-x86_64-gcc

pacman -S mingw-w64-x86_64-go mingw-w64-x86_64-opus

pacman -S mingw-w64-x86_64-pkg-config
```
可以在msys2 MINGW64的环境下运行 go run ./src/main.go

如果想要在windows的powershell中运行，可以把mingw64\bin加入环境变量path中，若还有报错，请检查设置
```
set PKG_CONFIG_PATH=C:\msys64\mingw64\lib\pkgconfig #使用你自己的pkgconfig路径
set CGO_ENABLED=1
```
再执行
go run ./src/main.go

## 源码运行

```
go mod tidy

 go run ./src/main.go
```

## 编译

```
go build -o xiaozhi-server.exe src/main.go
```

## 运行

```
.\xiaozhi-server.exe
```

## Centos系统下源码部署安装指南

- [Centos 8 安装指南](Centos_Guide.md)

# 贡献指南
欢迎任何形式的贡献！如果您有好的想法或发现问题，请通过以下方式联系我们：

<img src="https://github.com/user-attachments/assets/1fac0c06-5db5-4156-bc98-f17b7b46bea6" width="450" alt="微信群二维码">


## 定制开发
我们接受各种定制化开发项目，如果您有特定需求，欢迎通过微信联系洽谈。
