package providers

import (
	"context"
	"xiaozhi-server-go/internal/domain/llm/inter"
)

// Provider 所有提供者的基础接口
type Provider interface {
	Initialize() error
	Cleanup() error
}

type AsrEventListener interface {
	OnAsrResult(result string, isFinalResult bool) bool
}

// ASRProvider 语音识别提供者接口
type ASRProvider interface {
	Provider
	// 直接识别音频数据
	Transcribe(ctx context.Context, audioData []byte) (string, error)
	// 添加音频数据到缓冲区
	AddAudio(data []byte) error

	// 发送最后一块音频数据并标记为结束
	SendLastAudio(data []byte) error

	SetListener(listener AsrEventListener)

	// 设置用户偏好，例如语言等
	SetUserPreferences(preferences map[string]interface{}) error

	// 复位ASR状态
	Reset() error

	// 长连接的asr断开连接
	CloseConnection() error

	// 获取当前静音计数
	GetSilenceCount() int

	ResetSilenceCount()

	ResetStartListenTime()

	EnableSilenceDetection(bEnable bool)

	// 获取会话ID（用于事件发布）
	GetSessionID() string
}

// TTSProvider 语音合成提供者接口
type TTSProvider interface {
	Provider

	// 合成音频并返回文件路径
	ToTTS(text string) (string, error)

	SetVoice(voice string) (error, string)

	// 获取会话ID（用于事件发布）
	GetSessionID() string
}

// LLMProvider 大语言模型提供者接口
type LLMProvider interface {
	Provider
	Response(ctx context.Context, sessionID string, messages []Message) (<-chan string, error)
	ResponseWithFunctions(
		ctx context.Context,
		sessionID string,
		messages []Message,
		tools []Tool,
	) (<-chan Response, error)
	GetSessionID() string                       // 获取会话ID
	SetIdentityFlag(idType string, flag string) // 设置身份标识
}

// Message 对话消息
type Message = inter.Message

// Tool LLM工具
type Tool = inter.Tool

// Response LLM响应
type Response = inter.ResponseChunk
