# 小智协议（本项目实现版）接入文档

本文档描述 `xiaozhi-server-go` 当前实现的“小智协议”接入方式，目标是让你实现客户端时可以顺利接入（语音/文本对话、TTS 音频回放、MCP 工具、Vision/OTA）。

重要说明：
- 协议以服务端实现为准：`src/core/connection_handlemsg.go`、`src/core/connection_sendmsg.go`、`src/core/transport/websocket/*`、`src/core/mcp/xiaozhi_client.go`。
- WebSocket 文本帧为 JSON；二进制帧用于音频数据。
- 本协议与标准 MCP(JSON-RPC) 的关系：服务端把 MCP(JSON-RPC) 封装在 `type:"mcp"` 的消息里，在同一条 WebSocket 上与设备交互。

## 1. 术语

- `DeviceId`：设备唯一标识（通常是 MAC 地址字符串），用于服务端识别设备与绑定 Agent。
- `ClientId`：客户端实例标识（可选，但强烈建议提供）。
- `SessionId`：会话标识，服务端会根据 header 或 DeviceId 生成。
- `Text Frame`：WebSocket opcode=1 的文本帧，内容是 JSON。
- `Binary Frame`：WebSocket opcode=2 的二进制帧，内容是音频数据。

## 2. 传输与握手

### 2.1 WebSocket 连接地址

服务端 WebSocket 监听地址（由配置决定）：
- Host：你的服务器 IP/域名
- Port：`config.Transport.WebSocket.Port`（常见为 `8000`）
- Path：`/`

示例：
- `ws://<host>:8000/`
- `wss://<host>/`（取决于你是否在网关层做 TLS 终止）

### 2.2 建议的连接参数（Header / Query）

服务端读取的 header（大小写不敏感）：
- `Device-Id`：建议必填
- `Client-Id`：建议必填
- `Session-Id`：可选
- `Transport-Type`：可选（用于标记来源传输层类型）

兼容：如果 header 没有 `Device-Id`/`Client-Id`，服务端会尝试从 URL query 中读取：
- `?device-id=...&client-id=...`

### 2.3 连接后第一条消息：`hello`

客户端连接成功后，建议立刻发送 `type:"hello"`，向服务端声明你上传音频的参数（格式/采样率/声道/帧时长）。

客户端 → 服务端：
```json
{
  "type": "hello",
  "audio_params": {
    "format": "opus",
    "sample_rate": 24000,
    "channels": 1,
    "frame_duration": 60
  }
}
```

服务端 → 客户端（返回服务端下行音频参数与 `session_id`）：
```json
{
  "type": "hello",
  "version": 1,
  "transport": "websocket",
  "session_id": "device-xx_xx_xx_xx_xx_xx",
  "audio_params": {
    "format": "opus",
    "sample_rate": 24000,
    "channels": 1,
    "frame_duration": 60
  }
}
```

说明：
- 如果客户端上行声明 `format:"pcm"`，服务端会把下行 `audio_params.format` 切换为 `pcm`（便于客户端不解码 opus）。
- 下行音频帧（Binary Frame）编码方式以 `hello.audio_params.format` 为准。

## 3. WebSocket 帧类型

### 3.1 文本帧（JSON）

文本帧（opcode=1）必须是 JSON。
服务端目前会按 `msg.type` 分发：
- `hello`：握手音频参数
- `listen`：语音拾音控制（start/stop/detect）
- `abort`：打断当前对话/停止服务端说话
- `image`：图像对话（发送图片 URL/base64）
- `mcp`：MCP(JSON-RPC) 消息封装
- `iot`/`vision` 等：存在分支但实现程度不一

### 3.2 二进制帧（音频）

二进制帧（opcode=2）用作音频传输：
- 客户端 → 服务端：上传用户语音（PCM 或 Opus，取决于 hello 中声明的 format）。
- 服务端 → 客户端：下发 TTS 音频帧（PCM 或 Opus，取决于服务端协商结果）。

## 4. 对话时序（语音模式）

最小可用语音对话流程（推荐 `auto`）：
1. 连接 WebSocket（带 `Device-Id`、`Client-Id`）。
2. 发送 `hello`（声明上行音频参数）。
3. 发送 `listen`：`state:"start"`，开始说话。
4. 语音数据以 Binary Frame 连续发送。
5. 发送 `listen`：`state:"stop"`，结束本轮说话。
6. 服务端进行 ASR/LLM/TTS：
   - 下发 `stt`（识别文本）
   - 下发 `tts` 状态（start/sentence_start/sentence_end/stop）
   - 下发二进制音频帧（TTS）

一个典型的服务端下发序列（概念示意）：
1. `{"type":"stt","text":"..."}`
2. `{"type":"tts","state":"start"}`
3. `{"type":"llm","emotion":"thinking"}`（可选）
4. `{"type":"tts","state":"sentence_start","index":1,"text":"第一句"}`
5. Binary Frame × N（TTS 音频帧）
6. `{"type":"tts","state":"sentence_end","index":1}`
7. ...（第二句/第三句重复）
8. `{"type":"tts","state":"stop"}`

## 5. 客户端 → 服务端消息定义

### 5.1 `listen`（语音拾音控制）

```json
{
  "type": "listen",
  "mode": "auto",
  "state": "start"
}
```

```json
{
  "type": "listen",
  "mode": "auto",
  "state": "stop"
}
```

`mode` 取值：
- `auto`：推荐。服务端在 ASR 得到结果后自动触发对话。
- `manual`：服务端会拼接流式 ASR 片段，直到 final 才触发对话。
- `realtime`：每次识别到片段会打断服务端说话并立刻触发对话（更激进）。

`state` 取值：
- `start`：开始一轮拾音
- `stop`：停止拾音并提交最后音频（服务端会调用 ASR 的 `SendLastAudio` 结束本轮）
- `detect`：纯文本触发（不需要音频）

`detect`（纯文本对话）示例：
```json
{
  "type": "listen",
  "state": "detect",
  "text": "你好，帮我讲个笑话"
}
```

说明：
- 当前服务端对纯文本最稳定的入口是 `listen.state="detect"`。

### 5.2 `abort`（打断）

```json
{ "type": "abort" }
```

效果：
- 服务端停止下发语音，并发送 `tts.state="stop"`。

### 5.3 `image`（图像对话）

用于把图片（URL 或 base64）交给服务端调用 VLLLM。

```json
{
  "type": "image",
  "text": "这张图里有什么？",
  "image_data": {
    "url": "https://example.com/a.jpg",
    "data": "",
    "format": "jpg"
  }
}
```

说明：
- `url` 与 `data` 二选一，至少提供一个。
- 若服务端未配置 VLLLM，会返回文本提示“不支持图片处理”。

### 5.4 `mcp`（MCP JSON-RPC 封装）

所有 MCP 消息统一包在：
```json
{
  "type": "mcp",
  "session_id": "<session_id>",
  "payload": { "jsonrpc": "2.0", "...": "..." }
}
```

服务端会在连接后主动向客户端发送 initialize/tools/list 请求；客户端需要按 JSON-RPC 返回对应 `result` 或 `error`（见第 8 节）。
如果你的客户端暂时不打算实现 MCP，也可以选择忽略 `type:"mcp"` 的消息，但将失去“设备能力工具调用”（音量/亮度/拍照等）以及部分高级功能。

## 6. 服务端 → 客户端消息定义

### 6.1 `stt`（服务端识别文本）

```json
{
  "type": "stt",
  "text": "用户说的话（识别结果）",
  "session_id": "<session_id>"
}
```

### 6.2 `tts`（TTS 状态）

服务端在一轮对话中会发送：
- `start`：开始 TTS 输出
- `sentence_start`：某段文本对应的语音开始（带 index）
- `sentence_end`：某段文本对应的语音结束（带 index）
- `stop`：本轮 TTS 完成

示例：
```json
{
  "type": "tts",
  "state": "sentence_start",
  "session_id": "<session_id>",
  "text": "第一句话",
  "index": 1,
  "audio_codec": "opus"
}
```

说明：
- `index` 从 1 开始递增，对应 LLM 分句输出的段落顺序。
- 音频帧（Binary Frame）会在 `sentence_start` 与 `sentence_end` 之间下发。
- `audio_codec` 当前实现固定写 `opus`，客户端应以 `hello.audio_params.format` 为准做解码/播放。

### 6.3 `llm`（情绪/状态）

```json
{
  "type": "llm",
  "text": "（表情或状态文本）",
  "emotion": "thinking",
  "session_id": "<session_id>"
}
```

## 7. 音频数据约定

### 7.1 客户端 → 服务端（用户语音）

- 发送方式：WebSocket Binary Frame（opcode=2）
- 编码：由 `hello.audio_params.format` 声明（`pcm` 或 `opus`）
- 时序：通常在 `listen.state="start"` 之后持续发送，`listen.state="stop"` 结束本轮

### 7.2 服务端 → 客户端（TTS 音频）

- 发送方式：WebSocket Binary Frame（opcode=2）
- 编码：以服务端 `hello.audio_params.format` 为准（多数情况下为 `opus`）
- 流控：服务端会按帧时长分时发送，避免撑爆客户端缓冲区（见 `src/core/connection_sendmsg.go`）。

客户端播放建议：
- `opus`：按每个二进制帧作为一个 Opus packet 送入解码器，顺序播放。
- `pcm`：按帧顺序拼接或流式喂给播放设备（采样率/声道见 hello）。

## 8. MCP 子协议（JSON-RPC over WebSocket）

本项目的“设备侧 MCP”由服务端发起握手，客户端需要实现一个 MCP Server 的最小子集：
- `initialize`
- `tools/list`（支持分页 cursor）
- `tools/call`

### 8.1 initialize（服务端 → 客户端）

服务端发送：
```json
{
  "type": "mcp",
  "session_id": "<session_id>",
  "payload": {
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {
        "roots": { "listChanged": true },
        "sampling": {},
        "vision": {
          "url": "http://<host>:8080/api/vision",
          "token": "<vision_token>"
        }
      },
      "clientInfo": { "name": "XiaozhiClient", "version": "1.0.0" }
    }
  }
}
```

客户端必须回：
```json
{
  "type": "mcp",
  "session_id": "<session_id>",
  "payload": {
    "jsonrpc": "2.0",
    "id": 1,
    "result": {
      "serverInfo": { "name": "your-device-mcp", "version": "1.0.0" }
    }
  }
}
```

说明：
- `vision.url/token` 用于设备侧工具（例如拍照）调用服务端 Vision HTTP API。

### 8.2 tools/list（服务端 → 客户端）

服务端发送：
```json
{
  "type": "mcp",
  "session_id": "<session_id>",
  "payload": { "jsonrpc": "2.0", "id": 2, "method": "tools/list" }
}
```

客户端返回工具列表：
```json
{
  "type": "mcp",
  "session_id": "<session_id>",
  "payload": {
    "jsonrpc": "2.0",
    "id": 2,
    "result": {
      "tools": [
        {
          "name": "self.get_device_status",
          "description": "获取设备状态",
          "inputSchema": {
            "type": "object",
            "properties": {},
            "required": []
          }
        }
      ],
      "nextCursor": ""
    }
  }
}
```

分页：
- 如果 `result.nextCursor` 非空，服务端会继续请求：
```json
{
  "type": "mcp",
  "session_id": "<session_id>",
  "payload": {
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": { "cursor": "<nextCursor>" }
  }
}
```

工具命名注意：
- 客户端上报的工具名允许包含点号 `.`（例如 `self.audio_speaker.set_volume`）。
- 服务端注册到 LLM 时会把 `.` 替换成 `_`（例如 `self_audio_speaker_set_volume`），但在 `tools/call` 下发给客户端时仍使用原始含点号的名称。

### 8.3 tools/call（服务端 → 客户端）

当 LLM 触发工具调用时，服务端发送：
```json
{
  "type": "mcp",
  "session_id": "<session_id>",
  "payload": {
    "jsonrpc": "2.0",
    "id": 10,
    "method": "tools/call",
    "params": {
      "name": "self.get_device_status",
      "arguments": {}
    }
  }
}
```

客户端返回（成功）：
```json
{
  "type": "mcp",
  "session_id": "<session_id>",
  "payload": {
    "jsonrpc": "2.0",
    "id": 10,
    "result": {
      "isError": false,
      "content": [
        { "type": "text", "text": "设备当前音量 10，WiFi 信号 weak" }
      ]
    }
  }
}
```

客户端返回（失败）：
```json
{
  "type": "mcp",
  "session_id": "<session_id>",
  "payload": {
    "jsonrpc": "2.0",
    "id": 10,
    "result": {
      "isError": true,
      "error": "设备忙，请稍后重试"
    }
  }
}
```

说明：
- 服务端会优先读取 `result.isError`；成功时会取 `result.content[0].text` 作为工具返回文本。
- 工具返回文本通常会被追加到对话上下文中，再次请求 LLM 生成最终回答。

### 8.4 特例：`self.camera.take_photo`

服务端对工具名包含 `self.camera.take_photo` 的返回做了特殊处理：
- 期望 `result.content[0].text` 是一个 **VisionResponse JSON 字符串**；
- 服务端会解析该 JSON，并将识别结果用 TTS 播报。

因此如果你在设备侧实现拍照工具，建议流程：
1. 设备拍照得到图片；
2. 使用 `initialize` 下发的 `vision.url` 和 `vision.token` 调用 Vision HTTP API；
3. 把 Vision API 返回的 JSON 原样作为 `text` 返回给服务端。

## 9. OTA 与 Vision（用于客户端落地接入）

### 9.1 OTA（设备注册 + 获取 WebSocket 地址）

HTTP：`POST http://<host>:8080/api/ota/`
Headers：
- `device-id: <mac>`
- `client-id: <client_id>`（可选）

返回中包含：
- `websocket.url`：设备应连接的 WS 地址
- `firmware.url/version`：若存在固件文件则提供下载信息

### 9.2 Vision（设备侧调用）

HTTP：`POST http://<host>:8080/api/vision`
Headers：
- `Authorization: Bearer <vision_token>`（来自 MCP initialize）
- `Device-Id: <device_id>`
- `Client-Id: <client_id>`（可选）

FormData：
- `question`：问题
- `file`：图片文件

## 10. 已知兼容性说明（以当前实现为准）

- 纯文本对话：当前最稳定方式是 `listen.state="detect"` + `text`。`type:"chat"` 在服务端实现上存在参数传递不一致的风险，不建议作为唯一入口。
- `tts.audio_codec` 字段当前固定为 `opus`；客户端应以 `hello.audio_params.format` 为准决定解码方式。

## 11. 最小可用客户端接入清单（Checklist）

1. OTA 注册（可选但建议）：调用 `/api/ota/` 让服务端创建 Device 记录，并返回 WS URL。
2. 建立 WS：`ws://<host>:8000/`，携带 `Device-Id`、`Client-Id`。
3. 发送 `hello`：声明上行音频参数（建议 `opus/24000/1/60`）。
4. 语音对话：
   - `listen start`
   - 连续发送二进制音频帧
   - `listen stop`
   - 处理服务端 `stt`、`tts` 状态与二进制 TTS 音频帧
5. 纯文本对话：发送 `listen detect` + `text`
6. MCP（可选但强烈建议）：
   - 处理服务端 `mcp.initialize` / `mcp.tools/list` / `mcp.tools/call`
   - 上报设备支持的工具（至少可先实现 `self.get_device_status`）
   - 返回工具执行结果（建议返回 `content[0].text`）

## 12. 协议实现索引（服务端）

用于你在改协议/排障时快速定位代码：
- WebSocket 握手与 header/query 兼容：`src/core/transport/websocket/transport.go`
- hello：
  - 接收：`src/core/connection_handlemsg.go`（`handleHelloMessage`）
  - 下发：`src/core/connection_sendmsg.go`（`sendHelloMessage`）
- listen：
  - 接收：`src/core/connection_handlemsg.go`（`handleListenMessage`）
  - ASR 回调触发：`src/core/connection.go`（`OnAsrResult`）
- abort：
  - 接收：`src/core/connection_handlemsg.go`（`clientAbortChat`）
- stt/tts/情绪：
  - 下发：`src/core/connection_sendmsg.go`（`sendSTTMessage`/`sendTTSMessage`/`sendEmotionMessage`）
  - 音频帧发送：`src/core/connection_sendmsg.go`（`sendAudioFrames`）
- image：
  - 接收：`src/core/connection_handlemsg.go`（`handleImageMessage`）
  - VLLLM：`src/core/connection.go`（`genResponseByVLLM`）
- mcp：
  - 设备侧 MCP 客户端（服务端发起 initialize/tools/list/tools/call）：`src/core/mcp/xiaozhi_client.go`
  - MCP 聚合与工具执行：`src/core/mcp/manager.go`（`BindConnection`/`ExecuteTool`）
