package inter

// VAD 语音活动检测接口
type VAD interface {
	// IsVAD 检测音频数据中的语音活动
	IsVAD(pcmData []float32) (bool, error)

	IsVADExt(pcmData []float32, sampleRate int, frameSize int) (bool, error)
	// Reset 重置检测器状态
	Reset() error
	// Close 关闭并释放资源
	Close() error
}