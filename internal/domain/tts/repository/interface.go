package repository

import (
	"context"
	"xiaozhi-server-go/internal/domain/tts/inter"
)

// TTSRepository TTS领域数据访问接口
type TTSRepository interface {
	// SynthesizeText 将文本合成为语音
	SynthesizeText(ctx context.Context, req SynthesizeRequest) (*SynthesizeResult, error)

	// StreamSynthesize 流式合成语音
	StreamSynthesize(ctx context.Context, req SynthesizeRequest) (<-chan AudioChunk, error)

	// ValidateProvider 验证提供商连接性
	ValidateProvider(ctx context.Context, config inter.TTSConfig) error

	// GetProviderInfo 获取提供商信息
	GetProviderInfo(provider string) (*ProviderInfo, error)
}

// SynthesizeRequest 语音合成请求
type SynthesizeRequest struct {
	Text   string
	Config inter.TTSConfig
	SessionID string
}

// SynthesizeResult 语音合成结果
type SynthesizeResult struct {
	AudioData []byte
	FilePath  string
	Format    string
	SampleRate int
	Duration  float32
}

// AudioChunk 音频数据块
type AudioChunk struct {
	AudioData []byte
	IsFinal   bool
	Done      bool
}

// ProviderInfo 提供商信息
type ProviderInfo struct {
	Name         string
	SupportedVoices []string
	SupportedFormats []string
	MaxTextLength int
	Features     []string
}