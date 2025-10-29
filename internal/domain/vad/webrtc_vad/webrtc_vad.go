package webrtc_vad

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"xiaozhi-server-go/internal/domain/vad/inter"

	"github.com/baabaaox/go-webrtcvad"
)

const (
	// DefaultSampleRate WebRTC VAD 支持的采样率 (8000, 16000, 32000, 48000)
	DefaultSampleRate = 16000
	// DefaultMode VAD 敏感度模式 (0: 最不敏感, 3: 最敏感)
	DefaultMode = 2
	// FrameDuration 帧持续时间 (ms)，WebRTC VAD 支持 10ms, 20ms, 30ms
	FrameDuration = 20
)

// WebRTCVAD WebRTC VAD 实现
type WebRTCVAD struct {
	vadInst     webrtcvad.VadInst
	sampleRate  int          // 采样率
	mode        int          // VAD 模式
	frameSize   int          // 每帧采样数
	frameSizeBytes int       // 每帧字节数
	initialized bool         // 是否已初始化
	lastUsed    time.Time    // 最后使用时间
	mu          sync.RWMutex // 读写锁
	config      inter.VADConfig
}

// NewWebRTCVAD 创建新的 WebRTC VAD 实例
func NewWebRTCVAD(config inter.VADConfig) (inter.VADProvider, error) {
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	vad := &WebRTCVAD{
		sampleRate: config.SampleRate,
		mode:       DefaultMode, // 使用默认模式，可以后续扩展配置
		lastUsed:   time.Now(),
		config:     config,
	}

	err := vad.init()
	if err != nil {
		return nil, err
	}

	return vad, nil
}

// init 初始化 WebRTC VAD
func (w *WebRTCVAD) init() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.initialized {
		return nil
	}

	// 计算帧大小
	w.frameSize = w.sampleRate / 1000 * FrameDuration
	w.frameSizeBytes = w.frameSize * 2 // 16-bit PCM

	// 创建 VAD 实例
	w.vadInst = webrtcvad.Create()
	if w.vadInst == nil {
		return fmt.Errorf("failed to create WebRTC VAD instance")
	}

	// 初始化 VAD
	err := webrtcvad.Init(w.vadInst)
	if err != nil {
		webrtcvad.Free(w.vadInst)
		return fmt.Errorf("failed to initialize WebRTC VAD: %w", err)
	}

	// 设置模式
	err = webrtcvad.SetMode(w.vadInst, w.mode)
	if err != nil {
		webrtcvad.Free(w.vadInst)
		return fmt.Errorf("failed to set WebRTC VAD mode: %w", err)
	}

	w.initialized = true
	w.lastUsed = time.Now()
	return nil
}

// ProcessAudio 处理音频数据
func (w *WebRTCVAD) ProcessAudio(audioData []byte) (bool, error) {
	if len(audioData) == 0 {
		return false, nil
	}

	// 更新最后使用时间
	w.lastUsed = time.Now()

	// 将字节数据转换为 float32 (假设是 16-bit PCM)
	pcmData := w.pcmBytesToFloat32(audioData)

	// 如果数据长度不够一帧，返回 false
	if len(pcmData) < w.frameSize {
		return false, nil
	}

	// 将 float32 数据转换为 int16 PCM 数据
	pcmBytes := w.float32ToPCMBytes(pcmData)

	// 处理多帧数据，取最后一帧的结果
	var isActive bool
	var err error

	activityCount := 0
	for i := 0; i+w.frameSizeBytes <= len(pcmBytes); i += w.frameSizeBytes {
		frameData := pcmBytes[i : i+w.frameSizeBytes]

		isActive, err = webrtcvad.Process(w.vadInst, w.sampleRate, frameData, w.frameSize)
		if err != nil {
			return false, fmt.Errorf("WebRTC VAD process error: %w", err)
		}
		if isActive {
			activityCount++
		}
	}

	frameCount := len(pcmBytes) / w.frameSizeBytes
	isActive = activityCount >= frameCount/2

	return isActive, nil
}

// Reset 重置检测器状态
func (w *WebRTCVAD) Reset() {
	// WebRTC VAD 不需要重置状态
}

// Close 关闭并释放资源
func (w *WebRTCVAD) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.initialized && w.vadInst != nil {
		webrtcvad.Free(w.vadInst)
		w.initialized = false
	}
	return nil
}

// GetConfig 获取VAD配置
func (w *WebRTCVAD) GetConfig() inter.VADConfig {
	return w.config
}

// float32ToPCMBytes 将 float32 数组转换为 16-bit PCM 字节数组
func (w *WebRTCVAD) float32ToPCMBytes(samples []float32) []byte {
	pcmBytes := make([]byte, len(samples)*2)

	for i, sample := range samples {
		// 将 float32 (-1.0 到 1.0) 转换为 int16 (-32768 到 32767)
		var intSample int16
		if sample > 1.0 {
			intSample = 32767
		} else if sample < -1.0 {
			intSample = -32768
		} else {
			intSample = int16(sample * 32767)
		}

		// 小端序写入字节数组
		binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(intSample))
	}

	return pcmBytes
}

// pcmBytesToFloat32 将 16-bit PCM 字节数组转换为 float32 数组
func (w *WebRTCVAD) pcmBytesToFloat32(pcmBytes []byte) []float32 {
	samples := make([]float32, len(pcmBytes)/2)

	for i := 0; i < len(pcmBytes); i += 2 {
		if i+1 >= len(pcmBytes) {
			break
		}
		// 从小端序字节读取 int16
		intSample := int16(binary.LittleEndian.Uint16(pcmBytes[i:]))
		// 转换为 float32 (-1.0 到 1.0)
		samples[i/2] = float32(intSample) / 32767.0
	}

	return samples
}

// validateConfig 验证配置
func validateConfig(config inter.VADConfig) error {
	if config.SampleRate <= 0 {
		return fmt.Errorf("invalid sample rate: %d", config.SampleRate)
	}
	validRates := []int{8000, 16000, 32000, 48000}
	isValid := false
	for _, rate := range validRates {
		if rate == config.SampleRate {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("unsupported sample rate: %d, supported rates: 8000, 16000, 32000, 48000", config.SampleRate)
	}
	if config.Channels != 1 {
		return fmt.Errorf("WebRTC VAD only supports mono audio, got channels: %d", config.Channels)
	}
	return nil
}

var vadPool *WebRTCVADPool
var once sync.Once

// AcquireVAD 创建WebRTC VAD实例（工厂方法，支持池化）
func AcquireVAD(config map[string]interface{}) (inter.VADProvider, error) {
	vadConfig := inter.VADConfig{
		SampleRate:      DefaultSampleRate,
		Channels:        1,
		FrameDuration:   FrameDuration,
		Sensitivity:     0.5, // WebRTC VAD 使用固定模式，不使用灵敏度
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
	if minSpeechLength, ok := config["min_speech_length"].(float64); ok {
		vadConfig.MinSpeechLength = int(minSpeechLength)
	}
	if maxSilenceLength, ok := config["max_silence_length"].(float64); ok {
		vadConfig.MaxSilenceLength = int(maxSilenceLength)
	}

	// 检查是否需要池化
	if poolSize, ok := config["pool_size"].(float64); ok && poolSize > 0 {
		var err error
		once.Do(func() {
			poolConfig := DefaultPoolConfig()
			if maxSize, ok := config["pool_max_size"].(float64); ok {
				poolConfig.MaxSize = int(maxSize)
			}
			if minSize, ok := config["pool_min_size"].(float64); ok {
				poolConfig.MinSize = int(minSize)
			}
			if maxIdleTime, ok := config["pool_max_idle_time"].(float64); ok {
				poolConfig.MaxIdleTime = time.Duration(maxIdleTime) * time.Second
			}
			vadPool, err = NewWebRTCVADPool(vadConfig, poolConfig)
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create VAD pool: %w", err)
		}
		if vadPool != nil {
			return vadPool.AcquireVAD()
		}
	}

	// 不使用池化，直接创建实例
	return NewWebRTCVAD(vadConfig)
}

// ReleaseVAD 释放 VAD 实例（如果使用池化）
func ReleaseVAD(vad inter.VADProvider) error {
	if vadPool != nil {
		return vadPool.ReleaseVAD(vad)
	}
	return nil
}