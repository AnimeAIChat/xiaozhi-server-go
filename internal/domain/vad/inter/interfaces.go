package inter

// VADProvider VAD提供者接口
type VADProvider interface {
	// ProcessAudio 处理音频数据，返回是否检测到语音活动
	ProcessAudio(audioData []byte) (bool, error)

	// Reset 重置VAD状态
	Reset()

	// Close 关闭VAD资源
	Close() error

	// GetConfig 获取VAD配置
	GetConfig() VADConfig
}

// VADConfig VAD配置
type VADConfig struct {
	SampleRate      int     `json:"sample_rate"`       // 采样率
	Channels        int     `json:"channels"`          // 声道数
	FrameDuration   int     `json:"frame_duration"`    // 帧持续时间(ms)
	Sensitivity     float32 `json:"sensitivity"`       // 灵敏度 (0.0-1.0)
	MinSpeechLength int     `json:"min_speech_length"` // 最短语音长度(ms)
	MaxSilenceLength int    `json:"max_silence_length"` // 最长静音长度(ms)
}

// VADResult VAD检测结果
type VADResult struct {
	IsSpeech    bool    `json:"is_speech"`     // 是否检测到语音
	Confidence  float32 `json:"confidence"`    // 置信度
	SpeechStart int64   `json:"speech_start"`  // 语音开始时间戳
	SpeechEnd   int64   `json:"speech_end"`    // 语音结束时间戳
}