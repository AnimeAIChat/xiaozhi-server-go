package eventbus

// 事件类型定义
const (
	// ASR相关事件
	EventASRResult     = "asr:result"
	EventASRStarted    = "asr:started"
	EventASRStopped    = "asr:stopped"
	EventASRError      = "asr:error"

	// LLM相关事件
	EventLLMResponse   = "llm:response"
	EventLLMStarted    = "llm:started"
	EventLLMCompleted  = "llm:completed"
	EventLLMError      = "llm:error"

	// TTS相关事件
	EventTTSSpeak      = "tts:speak"
	EventTTSCompleted  = "tts:completed"
	EventTTSError      = "tts:error"

	// 对话相关事件
	EventChatMessage   = "chat:message"
	EventChatStarted   = "chat:started"
	EventChatCompleted = "chat:completed"

	// 连接相关事件
	EventConnectionHello    = "connection:hello"
	EventConnectionClosed   = "connection:closed"
	EventConnectionError    = "connection:error"

	// 系统事件
	EventSystemError   = "system:error"
	EventSystemInfo    = "system:info"
)

// 事件数据结构
type ASREventData struct {
	SessionID    string `json:"session_id"`
	Text         string `json:"text"`
	IsFinal      bool   `json:"is_final"`
	Confidence   float64 `json:"confidence,omitempty"`
}

type LLMEventData struct {
	SessionID string      `json:"session_id"`
	Round     int         `json:"round"`
	Content   string      `json:"content"`
	IsFinal   bool        `json:"is_final"`
	ToolCalls interface{} `json:"tool_calls,omitempty"`
}

type TTSEventData struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
	TextIndex int    `json:"text_index"`
	Round     int    `json:"round"`
	FilePath  string `json:"file_path,omitempty"`
}

type ChatEventData struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id,omitempty"`
	Message   string `json:"message"`
	Round     int    `json:"round"`
}

type ConnectionEventData struct {
	SessionID string                 `json:"session_id"`
	UserID    string                 `json:"user_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

type SystemEventData struct {
	Level   string `json:"level"` // error, warn, info
	Message string `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}