package asr

import (
	"bytes"
	"fmt"
	"time"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/utils"
)

// Config ASR配置结构
type Config struct {
	Name string `yaml:"name"` // ASR提供者名称
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

	BEnableSilenceDetection bool      // 是否启用静音检测
	StartListenTime         time.Time // 最后一次ASR处理时间
	SilenceCount            int       // 连续静音计数

	listener providers.AsrEventListener
}

func (p *BaseProvider) ResetStartListenTime() {
	p.StartListenTime = time.Now()
}

func (p *BaseProvider) SilenceTime() time.Duration {
	if !p.BEnableSilenceDetection {
		return 0
	}
	if p.StartListenTime.IsZero() {
		return 0
	}
	return time.Since(p.StartListenTime)
}

func (p *BaseProvider) EnableSilenceDetection(bEnable bool) {
	p.BEnableSilenceDetection = bEnable
}

func (p *BaseProvider) GetSilenceCount() int {
	return p.SilenceCount
}

func (p *BaseProvider) ResetSilenceCount() {
	p.SilenceCount = 0
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
type Factory func(config *Config, deleteFile bool, logger *utils.Logger) (Provider, error)

var factories = make(map[string]Factory)

// Register 注册ASR提供者工厂
func Register(name string, factory Factory) {
	factories[name] = factory
}

// Create 创建ASR提供者实例
func Create(name string, config *Config, deleteFile bool, logger *utils.Logger) (Provider, error) {
	factory, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("未知的ASR提供者: %s", name)
	}

	provider, err := factory(config, deleteFile, logger)
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
