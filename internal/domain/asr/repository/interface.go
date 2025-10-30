package repository

import (
	"context"
	"xiaozhi-server-go/internal/domain/asr/inter"
)

// ASRRepository ASR领域数据访问接口
type ASRRepository interface {
	// ProcessAudio 处理音频数据并返回识别结果
	ProcessAudio(ctx context.Context, req ProcessAudioRequest) (*ProcessAudioResult, error)

	// StreamAudio 流式处理音频数据
	StreamAudio(ctx context.Context, req ProcessAudioRequest) (<-chan AudioChunk, error)

	// ValidateProvider 验证提供商连接性
	ValidateProvider(ctx context.Context, config inter.ASRConfig) error

	// GetProviderInfo 获取提供商信息
	GetProviderInfo(provider string) (*ProviderInfo, error)
}

// ProcessAudioRequest 音频处理请求
type ProcessAudioRequest struct {
	AudioData []byte
	Config    inter.ASRConfig
	SessionID string
}

// ProcessAudioResult 音频处理结果
type ProcessAudioResult struct {
	Text       string
	IsFinal    bool
	Confidence float32
	StartTime  int64
	EndTime    int64
}

// AudioChunk 音频数据块
type AudioChunk struct {
	Text       string
	IsFinal    bool
	Confidence float32
	StartTime  int64
	EndTime    int64
	Done       bool
}

// ProviderInfo 提供商信息
type ProviderInfo struct {
	Name         string
	SupportedFormats []string
	MaxAudioLength int
	Features     []string
}