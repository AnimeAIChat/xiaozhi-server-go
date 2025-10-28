package silero_vad

import (
	"fmt"
	"sync"
	"xiaozhi-server-go/internal/domain/vad"
	"xiaozhi-server-go/internal/domain/vad/inter"
)

// SileroVAD Silero VAD实现
type SileroVAD struct {
	mu         sync.Mutex
	config     inter.VADConfig
	isActive   bool
	speechCount int
	silenceCount int
	// TODO: 添加Silero VAD模型相关字段
}

// NewSileroVAD 创建Silero VAD实例
func NewSileroVAD(config inter.VADConfig) (*SileroVAD, error) {
	if err := vad.ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	vad := &SileroVAD{
		config: config,
	}

	// TODO: 初始化Silero VAD模型
	// 这里需要加载ONNX模型或使用相应的库

	return vad, nil
}

// ProcessAudio 处理音频数据
func (v *SileroVAD) ProcessAudio(audioData []byte) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// TODO: 使用Silero VAD模型进行推理
	// 目前使用简化的能量检测作为占位符

	energy := calculateRMSEnergy(audioData)
	isSpeech := energy > v.config.Sensitivity

	if isSpeech {
		v.speechCount++
		v.silenceCount = 0
		v.isActive = true
	} else {
		v.silenceCount++
		if v.silenceCount > v.config.MaxSilenceLength / v.config.FrameDuration {
			v.isActive = false
			v.speechCount = 0
		}
	}

	return v.isActive && v.speechCount >= v.config.MinSpeechLength / v.config.FrameDuration, nil
}

// Reset 重置VAD状态
func (v *SileroVAD) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.isActive = false
	v.speechCount = 0
	v.silenceCount = 0
	// TODO: 重置模型状态
}

// Close 关闭VAD资源
func (v *SileroVAD) Close() error {
	v.Reset()
	// TODO: 释放模型资源
	return nil
}

// GetConfig 获取VAD配置
func (v *SileroVAD) GetConfig() inter.VADConfig {
	return v.config
}

// calculateRMSEnergy 计算RMS能量（临时实现）
func calculateRMSEnergy(audioData []byte) float32 {
	if len(audioData) == 0 {
		return 0
	}

	var sum float64
	sampleCount := len(audioData) / 2

	for i := 0; i < len(audioData); i += 2 {
		if i+1 >= len(audioData) {
			break
		}
		sample := int16(audioData[i]) | (int16(audioData[i+1]) << 8)
		sum += float64(sample) * float64(sample)
	}

	if sampleCount == 0 {
		return 0
	}

	rms := sum / float64(sampleCount)
	return float32(rms) / 32768.0
}

// AcquireVAD 创建Silero VAD实例（工厂方法）
func AcquireVAD(config map[string]interface{}) (inter.VADProvider, error) {
	vadConfig := inter.VADConfig{
		SampleRate:      16000,
		Channels:        1,
		FrameDuration:   30,
		Sensitivity:     0.5,
		MinSpeechLength: 100,
		MaxSilenceLength: 500,
	}

	// 从config中读取配置
	if sampleRate, ok := config["sample_rate"].(float64); ok {
		vadConfig.SampleRate = int(sampleRate)
	}
	if channels, ok := config["channels"].(float64); ok {
		vadConfig.Channels = int(channels)
	}
	if frameDuration, ok := config["frame_duration"].(float64); ok {
		vadConfig.FrameDuration = int(frameDuration)
	}
	if sensitivity, ok := config["sensitivity"].(float64); ok {
		vadConfig.Sensitivity = float32(sensitivity)
	}
	if minSpeechLength, ok := config["min_speech_length"].(float64); ok {
		vadConfig.MinSpeechLength = int(minSpeechLength)
	}
	if maxSilenceLength, ok := config["max_silence_length"].(float64); ok {
		vadConfig.MaxSilenceLength = int(maxSilenceLength)
	}

	// TODO: 检查是否有模型文件路径等配置
	if modelPath, ok := config["model_path"].(string); ok {
		// 使用模型路径
		_ = modelPath
	}

	return NewSileroVAD(vadConfig)
}