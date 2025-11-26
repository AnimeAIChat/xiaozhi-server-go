package providers

import (
	"context"
)

// BaseProvider 所有提供者的基础接口
type BaseProvider interface {
	// Initialize 初始化提供者
	Initialize() error

	// Cleanup 清理提供者资源
	Cleanup() error

	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error

	// GetProviderType 获取提供者类型
	GetProviderType() string

	// GetSessionID 获取会话ID（用于事件发布和追踪）
	GetSessionID() string
}

// ASREventListener ASR事件监听器接口
type ASREventListener interface {
	// OnAsrResult 接收ASR识别结果
	// result: 识别文本
	// isFinal: 是否为最终结果
	// 返回true表示继续识别，false表示停止识别
	OnAsrResult(result string, isFinal bool) bool
}

// ASRProvider 语音识别提供者统一接口
type ASRProvider interface {
	BaseProvider

	// StartListening 开始监听音频输入
	StartListening() error

	// StopListening 停止监听音频输入
	StopListening() error

	// ProcessAudioData 处理音频数据
	ProcessAudioData(audioData []byte) error

	// Transcribe 直接识别音频数据（同步模式）
	Transcribe(ctx context.Context, audioData []byte) (string, error)

	// SetEventListener 设置事件监听器
	SetEventListener(listener ASREventListener) error

	// Reset 重置ASR状态
	Reset()

	// Close 关闭ASR资源
	Close() error

	// SetUserPreferences 设置用户偏好，例如语言等
	SetUserPreferences(preferences map[string]interface{}) error

	// EnableSilenceDetection 启用静音检测
	EnableSilenceDetection(bEnable bool)

	// GetSilenceCount 获取当前静音计数
	GetSilenceCount() int

	// ResetSilenceCount 重置静音计数
	ResetSilenceCount()

	// ResetStartListenTime 重置开始监听时间
	ResetStartListenTime()
}