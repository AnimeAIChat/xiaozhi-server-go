package webrtc_vad

import (
	"fmt"
	"sync"
	"xiaozhi-server-go/internal/domain/vad"
	"xiaozhi-server-go/internal/domain/vad/inter"
)

// WebRTCVAD WebRTC VAD实现
type WebRTCVAD struct {
	mu         sync.Mutex
	config     inter.VADConfig
	isActive   bool
	speechCount int
	silenceCount int
}

// NewWebRTCVAD 创建WebRTC VAD实例
func NewWebRTCVAD(config inter.VADConfig) (*WebRTCVAD, error) {
	if err := vad.ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &WebRTCVAD{
		config: config,
	}, nil
}

// ProcessAudio 处理音频数据
func (v *WebRTCVAD) ProcessAudio(audioData []byte) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// 简单的能量检测作为VAD实现
	// 计算音频数据的RMS能量
	energy := calculateRMSEnergy(audioData)

	// 根据能量阈值判断是否为语音
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

	// 如果检测到足够长的语音，返回true
	return v.isActive && v.speechCount >= v.config.MinSpeechLength / v.config.FrameDuration, nil
}

// Reset 重置VAD状态
func (v *WebRTCVAD) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.isActive = false
	v.speechCount = 0
	v.silenceCount = 0
}

// Close 关闭VAD资源
func (v *WebRTCVAD) Close() error {
	v.Reset()
	return nil
}

// GetConfig 获取VAD配置
func (v *WebRTCVAD) GetConfig() inter.VADConfig {
	return v.config
}

// calculateRMSEnergy 计算RMS能量
func calculateRMSEnergy(audioData []byte) float32 {
	if len(audioData) == 0 {
		return 0
	}

	var sum float64
	sampleCount := len(audioData) / 2 // 假设16位采样

	for i := 0; i < len(audioData); i += 2 {
		if i+1 >= len(audioData) {
			break
		}
		// 转换为16位有符号整数
		sample := int16(audioData[i]) | (int16(audioData[i+1]) << 8)
		sum += float64(sample) * float64(sample)
	}

	if sampleCount == 0 {
		return 0
	}

	rms := sum / float64(sampleCount)
	return float32(rms) / 32768.0 // 归一化到0-1范围
}

// AcquireVAD 创建WebRTC VAD实例（工厂方法）
func AcquireVAD(config map[string]interface{}) (inter.VADProvider, error) {
	vadConfig := inter.VADConfig{
		SampleRate:      16000,
		Channels:        1,
		FrameDuration:   30,
		Sensitivity:     0.3,
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

	return NewWebRTCVAD(vadConfig)
}