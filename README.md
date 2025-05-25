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

* [X]  支持PCM格式的语音对话
* [X]  支持Opus格式的语音对话
* [X]  支持的模型 ASR(豆包流式）LLM（OpenAi API）TTS（EdgeTTS，豆包TTS）
* [ ]  识图解说（智谱)
* [ ]  IOT功能
* [ ]  OTA功能
* [ ]  支持mqtt连接
* [ ]  管理后台
* [ ]  支持function call和mcp

# 安装和使用

## 前置条件

本项目采用go语言编写，windows建议使用g维护go版本，mac可以使用gvm维护go版本

* go 1.24.2

## 下载源码

git clone https://github.com/AnimeAIChat/xiaozhi-server-go.git

## 修改配置文件

复制一份config.yaml到.config.yaml，并在配置中填好相应的key等信息，避免密钥泄漏

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
