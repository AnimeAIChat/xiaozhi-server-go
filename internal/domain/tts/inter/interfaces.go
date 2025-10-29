package inter

// TTSProvider TTS提供者接口
type TTSProvider interface {
	// ToTTS 将文本转换为语音
	ToTTS(text string) (string, error)

	// ToTTSWithConfig 使用指定配置转换文本
	ToTTSWithConfig(text string, config TTSConfig) (string, error)

	// GetConfig 获取TTS配置
	GetConfig() TTSConfig

	// Close 关闭TTS资源
	Close() error
}

// TTSConfig TTS配置
type TTSConfig struct {
	Provider        string        `json:"provider"`         // 提供者类型 (doubao, edge, etc.)
	Voice           string        `json:"voice"`            // 语音名称
	Speed           float32       `json:"speed"`            // 语速 (0.5-2.0)
	Pitch           float32       `json:"pitch"`            // 音调 (0.5-2.0)
	Volume          float32       `json:"volume"`           // 音量 (0.0-1.0)
	SampleRate      int           `json:"sample_rate"`      // 采样率
	Format          string        `json:"format"`           // 音频格式 (wav, mp3, opus)
	Language        string        `json:"language"`         // 语言
}

// VoiceInfo 语音信息
type VoiceInfo struct {
	Name        string `json:"name"`         // 语音名称
	Language    string `json:"language"`     // 语言
	DisplayName string `json:"display_name"` // 显示名称
	Sex         string `json:"sex"`          // 性别
	Description string `json:"description"`  // 描述
	AudioURL    string `json:"audio_url"`    // 音频URL
}

// TTSResult TTS转换结果
type TTSResult struct {
	FilePath    string `json:"file_path"`    // 音频文件路径
	Duration    float64 `json:"duration"`     // 音频时长(秒)
	Size        int64   `json:"size"`         // 文件大小(字节)
	Format      string `json:"format"`       // 音频格式
	SampleRate  int    `json:"sample_rate"`  // 采样率
}