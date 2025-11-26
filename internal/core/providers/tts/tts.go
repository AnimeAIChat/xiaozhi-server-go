package tts

import (
	"fmt"
	"os"
	"path/filepath"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/core/providers"
	internalutils "xiaozhi-server-go/internal/utils"
	"xiaozhi-server-go/internal/domain/eventbus"
)

// Config TTS配置结构
type Config struct {
	Name            string              `yaml:"name"` // TTS提供者名称
	Type            string              `yaml:"type"`
	OutputDir       string              `yaml:"output_dir"`
	Voice           string              `yaml:"voice,omitempty"`
	Format          string              `yaml:"format,omitempty"`
	SampleRate      int                 `yaml:"sample_rate,omitempty"`
	AppID           string              `yaml:"appid"`
	Token           string              `yaml:"token"`
	Cluster         string              `yaml:"cluster"`
	SupportedVoices []config.VoiceInfo `yaml:"supported_voices"` // 支持的语音列表
}

// Provider TTS提供者接口
type Provider interface {
	providers.TTSProvider
}

// BaseProvider TTS基础实现
type BaseProvider struct {
	config     *Config
	deleteFile bool
	sessionID  string // 会话ID，用于事件发布
}

// Config 获取配置
func (p *BaseProvider) Config() *Config {
	return p.config
}

// GetSessionID 获取会话ID
func (p *BaseProvider) GetSessionID() string {
	return p.sessionID
}

// SetSessionID 设置会话ID
func (p *BaseProvider) SetSessionID(sessionID string) {
	p.sessionID = sessionID
}

// PublishTTSSpeak 发布TTS说话事件
func (p *BaseProvider) PublishTTSSpeak(text string, textIndex int, round int) {
	eventData := eventbus.TTSEventData{
		SessionID: p.sessionID,
		Text:      text,
		TextIndex: textIndex,
		Round:     round,
	}
	eventbus.Publish(eventbus.EventTTSSpeak, eventData)
}

// PublishTTSCompleted 发布TTS完成事件
func (p *BaseProvider) PublishTTSCompleted(text string, textIndex int, round int, filePath string) {
	eventData := eventbus.TTSEventData{
		SessionID: p.sessionID,
		Text:      text,
		TextIndex: textIndex,
		Round:     round,
		FilePath:  filePath,
	}
	eventbus.Publish(eventbus.EventTTSCompleted, eventData)
}

// PublishTTSError 发布TTS错误事件
func (p *BaseProvider) PublishTTSError(err error, text string, textIndex int, round int) {
	eventData := eventbus.SystemEventData{
		Level:   "error",
		Message: fmt.Sprintf("TTS error: %v", err),
		Data: map[string]interface{}{
			"session_id": p.sessionID,
			"text":       text,
			"text_index": textIndex,
			"round":      round,
			"error":      err.Error(),
		},
	}
	eventbus.Publish(eventbus.EventTTSError, eventData)
}

// NewBaseProvider 创建TTS基础提供者
func NewBaseProvider(config *Config, deleteFile bool) *BaseProvider {
	return &BaseProvider{
		config:     config,
		deleteFile: deleteFile,
	}
}

// Initialize 初始化提供者
func (p *BaseProvider) Initialize() error {
	if err := os.MkdirAll(p.config.OutputDir, 0o755); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}
	return nil
}

func IsSupportedVoice(voice string, supportedVoices []config.VoiceInfo) (bool, string, error) {
	if voice == "" {
		return false, "", fmt.Errorf("声音不能为空")
	}
	cnNames := map[string]string{}
	enNames := map[string]string{}
	voiceNames := []string{}
	for _, v := range supportedVoices {
		cnNames[v.DisplayName] = v.Name // 中文名
		enNames[v.Name] = v.Name        // 英文名（实际是音色名）
		voiceNames = append(voiceNames, v.Name)
	}

	// 如果是中文名，则转换为音色名称
	if enVoice, ok := cnNames[voice]; ok {
		voice = enVoice
	}

	// 如果是英文名，则转换为音色名称
	if enVoice, ok := enNames[voice]; ok {
		voice = enVoice
	}

	// 检查声音是否在支持的列表中
	if !internalutils.IsInArray(voice, voiceNames) {
		return false, "", fmt.Errorf("不支持的声音: %s, 可用声音: %v", voice, voiceNames)
	}

	return true, voice, nil
}

func (p *BaseProvider) SetVoice(voice string) (error, string) {
	p.Config().Voice = voice
	return nil, voice
}

// Cleanup 清理资源
func (p *BaseProvider) Cleanup() error {
	if p.deleteFile {
		// 清理输出目录中的临时文件
		pattern := filepath.Join(p.config.OutputDir, "*.{wav,mp3,opus}")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("查找临时文件失败: %v", err)
		}
		for _, file := range matches {
			if err := os.Remove(file); err != nil {
				return fmt.Errorf("删除临时文件失败: %v", err)
			}
		}
	}
	return nil
}

// EventPublisher 事件发布接口
type EventPublisher interface {
	SetSessionID(sessionID string)
	PublishTTSSpeak(text string, textIndex int, round int)
	PublishTTSCompleted(text string, textIndex int, round int, filePath string)
	PublishTTSError(err error, text string, textIndex int, round int)
}

// GetEventPublisher 获取事件发布器
func GetEventPublisher(provider Provider) EventPublisher {
	if p, ok := provider.(EventPublisher); ok {
		return p
	}
	return nil
}

// Factory TTS工厂函数类型
type Factory func(config *Config, deleteFile bool) (Provider, error)

var factories = make(map[string]Factory)

// Register 注册TTS提供者工厂
func Register(name string, factory Factory) {
	factories[name] = factory
}

// Create 创建TTS提供者实例
func Create(name string, config *Config, deleteFile bool) (Provider, error) {
	factory, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("未知的TTS提供者: %s", name)
	}

	provider, err := factory(config, deleteFile)
	if err != nil {
		return nil, fmt.Errorf("创建TTS提供者失败: %v", err)
	}

	return provider, nil
}
