package inter

// ASRProvider ASR提供者接口
type ASRProvider interface {
	// StartListening 开始监听音频输入
	StartListening() error

	// StopListening 停止监听音频输入
	StopListening() error

	// ProcessAudioData 处理音频数据
	ProcessAudioData(audioData []byte) error

	// SetEventListener 设置事件监听器
	SetEventListener(listener ASREventListener) error

	// Reset 重置ASR状态
	Reset()

	// GetConfig 获取ASR配置
	GetConfig() ASRConfig

	// Close 关闭ASR资源
	Close() error
}

// ASREventListener ASR事件监听器接口
type ASREventListener interface {
	// OnAsrResult 接收ASR识别结果
	// 返回true表示继续识别，false表示停止识别
	OnAsrResult(result string, isFinal bool) bool
}

// ASRConfig ASR配置
type ASRConfig struct {
	Provider     string  `json:"provider"`      // 提供者类型 (funasr, doubao, etc.)
	SampleRate   int     `json:"sample_rate"`   // 采样率
	Channels     int     `json:"channels"`      // 声道数
	Language     string  `json:"language"`      // 语言
	SilenceTimeout int    `json:"silence_timeout"` // 静音超时时间(毫秒)
	MaxSpeechLength int   `json:"max_speech_length"` // 最大语音长度(毫秒)
	Sensitivity  float32 `json:"sensitivity"`   // 灵敏度 (0.0-1.0)
}

// ASRResult ASR识别结果
type ASRResult struct {
	Text      string  `json:"text"`       // 识别文本
	IsFinal   bool    `json:"is_final"`   // 是否为最终结果
	Confidence float32 `json:"confidence"` // 置信度
	StartTime int64   `json:"start_time"` // 开始时间戳
	EndTime   int64   `json:"end_time"`   // 结束时间戳
}