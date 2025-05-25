package asr

import (
	"bytes"
	"fmt"
	"math"
	"time"

	"xiaozhi-server-go/src/core/providers"
)

// Config ASR配置结构
type Config struct {
	Type string
	Data map[string]interface{}
}

// Provider ASR提供者接口
type Provider interface {
	providers.Provider

}

// BaseProvider ASR基础实现
type BaseProvider struct {
	config     *Config
	deleteFile bool

	// 音频处理相关
	lastChunkTime time.Time
	audioBuffer   *bytes.Buffer

	// 静音检测配置
	silenceThreshold float64 // 能量阈值
	silenceDuration  int     // 静音持续时间(ms)

	listener providers.AsrEventListener
}

// SetListener 设置事件监听器
func (p *BaseProvider) SetListener(listener providers.AsrEventListener) {
	p.listener = listener
}

// GetListener 获取事件监听器
func (p *BaseProvider) GetListener() providers.AsrEventListener {
	return p.listener
}

// Config 获取配置
func (p *BaseProvider) Config() *Config {
	return p.config
}

// GetAudioBuffer 获取音频缓冲区
func (p *BaseProvider) GetAudioBuffer() *bytes.Buffer {
	return p.audioBuffer
}

// GetLastChunkTime 获取最后一个音频块的时间
func (p *BaseProvider) GetLastChunkTime() time.Time {
	return p.lastChunkTime
}

// SetLastChunkTime 设置最后一个音频块的时间
func (p *BaseProvider) SetLastChunkTime(t time.Time) {
	p.lastChunkTime = t
}

// DeleteFile 获取是否删除文件标志
func (p *BaseProvider) DeleteFile() bool {
	return p.deleteFile
}

// NewBaseProvider 创建ASR基础提供者
func NewBaseProvider(config *Config, deleteFile bool) *BaseProvider {
	return &BaseProvider{
		config:     config,
		deleteFile: deleteFile,
	}
}

// Initialize 初始化提供者
func (p *BaseProvider) Initialize() error {
	return nil
}

// Cleanup 清理资源
func (p *BaseProvider) Cleanup() error {
	return nil
}

// Factory ASR工厂函数类型
type Factory func(config *Config, deleteFile bool) (Provider, error)

var (
	factories = make(map[string]Factory)
)

// Register 注册ASR提供者工厂
func Register(name string, factory Factory) {
	factories[name] = factory
}

// Create 创建ASR提供者实例
func Create(name string, config *Config, deleteFile bool) (Provider, error) {
	factory, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("未知的ASR提供者: %s", name)
	}

	provider, err := factory(config, deleteFile)
	if err != nil {
		return nil, fmt.Errorf("创建ASR提供者失败: %v", err)
	}

	return provider, nil
}

// 初始化音频处理
func (p *BaseProvider) InitAudioProcessing() {
	p.audioBuffer = new(bytes.Buffer)
	p.silenceThreshold = 0.01 // 默认能量阈值
	p.silenceDuration = 800   // 默认静音判断时长(ms)
}

// 计算音频能量
func (p *BaseProvider) calculateEnergy(data []byte) float64 {
	if len(data) < 2 {
		return 0
	}

	var sum float64
	samples := len(data) / 2 // 16位音频，每个样本2字节

	for i := 0; i < len(data); i += 2 {
		// 将两个字节转换为16位整数
		sample := int16(data[i]) | int16(data[i+1])<<8
		// 计算平方和
		amplitude := float64(sample) / 32768.0 // 归一化到[-1,1]
		sum += amplitude * amplitude
	}

	// 返回RMS能量
	return math.Sqrt(sum / float64(samples))
}

// IsSilence 检测数据片段是否是静音
func (p *BaseProvider) IsSilence(data []byte) bool {
	energy := p.calculateEnergy(data)
	return energy < p.silenceThreshold
}

// IsEndOfSpeech 检测说话是否结束
func (p *BaseProvider) IsEndOfSpeech() bool {
	if p.audioBuffer == nil || p.audioBuffer.Len() == 0 {
		return false
	}

	if time.Since(p.lastChunkTime) > time.Duration(p.silenceDuration)*time.Millisecond {
		return p.IsSilence(p.audioBuffer.Bytes())
	}
	return false
}
