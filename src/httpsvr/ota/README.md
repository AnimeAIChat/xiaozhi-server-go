# OTA模块

## 目录结构
- `ota.go`：OTA服务默认实现
- `interfaces.go`：OTA服务接口定义
- `server.go`：OTA HTTP服务实现
- `README.md`：模块说明文档

## 用法说明
1. 在主程序中引入OTA服务模块。
2. 初始化OTA服务。
3. 配置`UpdateURL`参数，指定WebSocket地址。

## OTA接口说明
- `GET /api/ota/`：返回OTA接口运行状态及WebSocket地址。
- `POST /api/ota/`：接收设备请求，返回服务器时间、固件信息和WebSocket地址。

## OTA接口测试（Apifox）

你可以使用 [Apifox](https://apifox.com/) 对OTA接口进行测试。

### 1. 测试GET接口
- 方法：GET
- URL：`http://localhost:8080/api/ota/`
- 预期返回：
```json
{
  "status": "ok",
  "ws": "ws://localhost:8080/ws"
}
```

### 2. 测试POST接口
- 方法：POST
- URL：`http://localhost:8080/api/ota/`
- Body类型：JSON
- 示例请求体：
```json
{
  "device_id": "your_device_id"
}
```
- 预期返回：
```json
{
  "server_time": "2024-01-01T12:00:00Z",
  "firmware": "v1.0.0",
  "ws": "ws://localhost:8080/ws"
}
```

### 3. 下载 
- URL：`http://localhost:8080/ota_bin/{*.bin}`

### 4. 跨域说明
OTA服务已支持CORS，便于前端或第三方工具直接调用。

如需进一步定制接口或返回内容，请根据实际需求修改`server.go`。