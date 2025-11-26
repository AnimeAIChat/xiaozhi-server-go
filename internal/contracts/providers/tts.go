package providers

import (
	"context"
)

// TTSProvider 语音合成提供者统一接口
type TTSProvider interface {
	BaseProvider

	// Synthesize 合成音频（同步模式，返回音频数据）
	Synthesize(ctx context.Context, text string, options SynthesisOptions) ([]byte, error)

	// SynthesizeToFile 合成音频到文件（兼容旧接口）
	SynthesizeToFile(text string) (string, error)

	// SetVoice 设置语音类型
	SetVoice(voice string) error

	// GetAvailableVoices 获取可用的语音列表
	GetAvailableVoices() ([]Voice, error)

	// GetConfig 获取TTS配置
	GetConfig() TTSConfig
}

// SynthesisOptions 语音合成选项
type SynthesisOptions struct {
	// Voice 语音类型
	Voice string `json:"voice,omitempty"`

	// Speed 语速（0.1-2.0）
	Speed float32 `json:"speed,omitempty"`

	// Pitch 音调（-20.0到20.0）
	Pitch float32 `json:"pitch,omitempty"`

	// Volume 音量（0.0-1.0）
	Volume float32 `json:"volume,omitempty"`

	// Format 音频格式（mp3, wav, ogg等）
	Format string `json:"format,omitempty"`

	// SampleRate 采样率
	SampleRate int `json:"sample_rate,omitempty"`

	// Language 语言代码
	Language string `json:"language,omitempty"`

	// Emotion 情感（如果支持）
	Emotion string `json:"emotion,omitempty"`
}

// Voice 语音信息
type Voice struct {
	ID          string `json:"id"`           // 语音ID
	Name        string `json:"name"`         // 语音名称
	Language    string `json:"language"`     // 语言代码
	Gender      string `json:"gender"`       // 性别
	Age         string `json:"age,omitempty"` // 年龄段
	Description string `json:"description"`  // 描述
}

// TTSConfig TTS配置
type TTSConfig struct {
	Provider    string  `json:"provider"`      // 提供者类型 (edge, doubao, etc.)
	Voice       string  `json:"voice"`         // 默认语音
	Speed       float32 `json:"speed"`         // 默认语速
	Pitch       float32 `json:"pitch"`         // 默认音调
	Volume      float32 `json:"volume"`        // 默认音量
	Format      string  `json:"format"`        // 默认音频格式
	SampleRate  int     `json:"sample_rate"`   // 默认采样率
	Language    string  `json:"language"`      // 默认语言
	APIKey      string  `json:"api_key"`       // API密钥
	BaseURL     string  `json:"base_url"`      // 基础URL
	Timeout     int     `json:"timeout"`       // 超时时间(秒)

	// 扩展配置
	Extra map[string]interface{} `json:"extra,omitempty"`
}