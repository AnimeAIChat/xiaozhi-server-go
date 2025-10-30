# 设备注册API文档

本文档描述了设备注册、激活和查询相关的API端点。

## 概述

设备注册系统支持两种模式：
1. **自动激活模式**：设备注册后自动激活，无需额外步骤
2. **手动激活模式**：设备注册后需要使用激活码进行激活

## 配置

通过数据库配置控制激活模式：
- `Server.Device.RequireActivationCode`: 是否需要激活码 (true/false)
- `Server.Device.DefaultAdminUserID`: 默认管理员用户ID

## API端点

### 1. 设备注册

**端点**: `POST /api/ota/register`

**请求体**:
```json
{
  "deviceId": "string",     // 设备唯一标识 (必需)
  "clientId": "string",     // 客户端唯一标识 (必需)
  "name": "string",         // 设备名称 (可选)
  "version": "string",      // 设备版本 (可选)
  "ipAddress": "string",    // IP地址 (可选, 默认使用客户端IP)
  "appInfo": "string"       // 应用信息 (可选)
}
```

**响应**:
```json
{
  "success": true,
  "message": "Device registered successfully",
  "deviceId": "device-001",
  "clientId": "client-001",
  "requiresAuth": false
}
```

或 (需要激活码时):
```json
{
  "success": true,
  "message": "Device registered, activation code required",
  "deviceId": "device-002",
  "clientId": "client-002",
  "authCode": "123456",
  "requiresAuth": true
}
```

### 2. 设备激活

**端点**: `POST /api/ota/activate`

**请求体**:
```json
{
  "deviceId": "string",   // 设备ID (必需)
  "authCode": "string"    // 激活码 (必需)
}
```

**响应**:
```json
{
  "success": true,
  "message": "Device activated successfully",
  "deviceId": "device-002"
}
```

### 3. 设备查询

**端点**: `GET /api/ota/device/{deviceId}`

**响应**:
```json
{
  "success": true,
  "message": "Device found",
  "device": {
    "id": 1,
    "userId": 1,
    "name": "Test Device",
    "deviceId": "device-001",
    "clientId": "client-001",
    "version": "1.0.0",
    "registerTime": 1761833203,
    "lastActiveTime": 1761833203,
    "online": false,
    "authStatus": "approved",
    "boardType": "",
    "chipModelName": "",
    "channel": 0,
    "ssid": "",
    "application": "test app",
    "language": "zh-CN",
    "deviceCode": "",
    "lastIp": "192.168.1.100",
    "stats": "",
    "totalTokens": 0,
    "usedTokens": 0,
    "conversationId": "",
    "mode": ""
  }
}
```

## 错误响应

所有API在出错时都会返回以下格式：

```json
{
  "success": false,
  "message": "Error description",
  "code": 400
}
```

## 使用示例

### 自动激活模式

```bash
# 注册设备 (自动激活)
curl -X POST http://localhost:8080/api/ota/register \
  -H "Content-Type: application/json" \
  -d '{
    "deviceId": "esp32-001",
    "clientId": "client-001",
    "name": "ESP32 Device",
    "version": "1.0.0"
  }'

# 查询设备状态
curl http://localhost:8080/api/ota/device/esp32-001
```

### 手动激活模式

```bash
# 注册设备 (需要激活码)
curl -X POST http://localhost:8080/api/ota/register \
  -H "Content-Type: application/json" \
  -d '{
    "deviceId": "esp32-002",
    "clientId": "client-002",
    "name": "ESP32 Device 2",
    "version": "2.0.0"
  }'
# 返回: {"authCode": "123456", "requiresAuth": true}

# 激活设备
curl -X POST http://localhost:8080/api/ota/activate \
  -H "Content-Type: application/json" \
  -d '{
    "deviceId": "esp32-002",
    "authCode": "123456"
  }'
```

## 设备状态

- `pending`: 待激活状态
- `approved`: 已激活状态
- `rejected`: 已拒绝状态

## 安全考虑

- 激活码具有过期时间 (默认24小时)
- 每个激活码只能使用一次
- 设备ID在系统中必须唯一