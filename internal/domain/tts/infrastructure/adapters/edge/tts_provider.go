package edge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	contractProviders "xiaozhi-server-go/internal/contracts/providers"
	"xiaozhi-server-go/src/core/utils"

	"github.com/wujunwei928/edge-tts-go/edge_tts"
)

// EdgeTTSProvider Edge TTS提供者的新架构实现
// 实现统一的TTSProvider接口
type EdgeTTSProvider struct {
	sessionID     string
	providerType  string
	isInitialized bool
	logger        *utils.Logger

	// Edge TTS特有配置
	voice       string
	outputDir   string
	deleteFile  bool
	sampleRate  int
	format      string
	speed       float32
	pitch       float32
	volume      float32

	// 性能优化相关
	audioCache     *AudioCache
	requestPool    sync.Pool
	fileCleanup    *FileCleanupManager
	circuitBreaker *CircuitBreaker

	// 状态跟踪
	lastActivity  time.Time
	totalRequests int64
	errorCount    int64
	mutex         sync.RWMutex
}

// Config Edge TTS配置
type Config struct {
	Voice      string  `json:"voice" yaml:"voice"`
	OutputDir  string  `json:"output_dir" yaml:"output_dir"`
	DeleteFile bool    `json:"delete_file" yaml:"delete_file"`
	SampleRate int     `json:"sample_rate" yaml:"sample_rate"`
	Format     string  `json:"format" yaml:"format"`
	Speed      float32 `json:"speed" yaml:"speed"`
	Pitch      float32 `json:"pitch" yaml:"pitch"`
	Volume     float32 `json:"volume" yaml:"volume"`
}

// AudioCache 音频缓存实现
type AudioCache struct {
	cache   map[string]*AudioEntry
	mutex   sync.RWMutex
	maxSize int
	ttl     time.Duration
}

// AudioEntry 音频缓存条目
type AudioEntry struct {
	audioData []byte
	timestamp time.Time
	filePath  string
}

// FileCleanupManager 文件清理管理器
type FileCleanupManager struct {
	filesToCleanup []string
	mutex          sync.Mutex
}

// CircuitBreaker 熔断器实现
type CircuitBreaker struct {
	maxFailures int
	failures    int
	lastFailure time.Time
	state       int // 0=closed, 1=open, 2=half-open
	mutex       sync.RWMutex
	retryAfter  time.Duration
}

// NewEdgeTTSProvider 创建新的Edge TTS提供者
func NewEdgeTTSProvider(config Config, logger *utils.Logger) *EdgeTTSProvider {
	if logger == nil {
		logger = utils.DefaultLogger
	}

	provider := &EdgeTTSProvider{
		sessionID:    fmt.Sprintf("edge-tts-%d", time.Now().UnixNano()),
		providerType: "edge",
		logger:       logger,
		isInitialized: false,

		// 配置参数
		voice:      config.Voice,
		outputDir:  config.OutputDir,
		deleteFile: config.DeleteFile,
		sampleRate: config.SampleRate,
		format:     config.Format,
		speed:      config.Speed,
		pitch:      config.Pitch,
		volume:     config.Volume,

		// 初始化性能优化组件
		audioCache:     NewAudioCache(500, 30*time.Minute),
		circuitBreaker: &CircuitBreaker{
			maxFailures: 5,
			retryAfter:  30 * time.Second,
		},
		fileCleanup: &FileCleanupManager{},
	}

	// 初始化对象池
	provider.requestPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 8192) // 8KB缓冲区
		},
	}

	return provider
}

// Initialize 初始化提供者
func (p *EdgeTTSProvider) Initialize() error {
	p.logger.InfoTag("EdgeTTS", "初始化Edge TTS提供者，SessionID: %s", p.sessionID)

	// 设置默认值
	if p.voice == "" {
		p.voice = "zh-CN-XiaoxiaoNeural" // 默认中文女声
	}
	if p.outputDir == "" {
		p.outputDir = os.TempDir()
	}
	if p.sampleRate <= 0 {
		p.sampleRate = 24000 // 默认24kHz
	}
	if p.format == "" {
		p.format = "mp3" // 默认MP3格式
	}
	if p.speed <= 0 {
		p.speed = 1.0 // 默认正常速度
	}
	if p.volume <= 0 {
		p.volume = 1.0 // 默认正常音量
	}

	// 确保输出目录存在
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	p.isInitialized = true
	p.lastActivity = time.Now()

	p.logger.InfoTag("EdgeTTS", "Edge TTS提供者初始化完成，Voice: %s, SessionID: %s", p.voice, p.sessionID)
	return nil
}

// Cleanup 清理提供者资源
func (p *EdgeTTSProvider) Cleanup() error {
	p.logger.InfoTag("EdgeTTS", "清理Edge TTS提供者，SessionID: %s", p.sessionID)

	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 清理音频缓存
	if p.audioCache != nil {
		p.audioCache.Clear()
	}

	// 清理临时文件
	if p.fileCleanup != nil {
		p.fileCleanup.CleanupAll()
	}

	p.isInitialized = false
	return nil
}

// HealthCheck 健康检查
func (p *EdgeTTSProvider) HealthCheck(ctx context.Context) error {
	if !p.isInitialized {
		return fmt.Errorf("provider not initialized")
	}

	// 检查熔断器状态
	if p.circuitBreaker.isOpen() {
		return fmt.Errorf("circuit breaker is open")
	}

	// 检查最近活动时间
	if time.Since(p.lastActivity) > 10*time.Minute {
		return fmt.Errorf("provider inactive for too long")
	}

	// 检查输出目录是否可写
	testFile := filepath.Join(p.outputDir, fmt.Sprintf("test_%d.mp3", time.Now().UnixNano()))
	if _, err := os.Create(testFile); err != nil {
		return fmt.Errorf("output directory not writable: %w", err)
	}
	os.Remove(testFile)

	return nil
}

// GetProviderType 获取提供者类型
func (p *EdgeTTSProvider) GetProviderType() string {
	return p.providerType
}

// GetSessionID 获取会话ID
func (p *EdgeTTSProvider) GetSessionID() string {
	return p.sessionID
}

// Synthesize 合成音频（同步模式，返回音频数据）
func (p *EdgeTTSProvider) Synthesize(ctx context.Context, text string, options contractProviders.SynthesisOptions) ([]byte, error) {
	if !p.isInitialized {
		return nil, fmt.Errorf("provider not initialized")
	}

	// 检查熔断器
	if p.circuitBreaker.isOpen() {
		return nil, fmt.Errorf("circuit breaker is open")
	}

	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	p.logger.InfoTag("EdgeTTS", "开始合成音频，文本长度: %d, SessionID: %s", len(text), p.sessionID)

	// 生成缓存键
	cacheKey := p.generateCacheKey(text, options)

	// 检查缓存
	if cachedAudio := p.audioCache.Get(cacheKey); cachedAudio != nil {
		p.logger.DebugTag("EdgeTTS", "使用缓存音频，SessionID: %s", p.sessionID)
		return cachedAudio, nil
	}

	// 合并配置选项
	synthOptions := p.mergeOptions(options)

	// 执行语音合成
	audioData, err := p.performSynthesis(ctx, text, synthOptions)
	if err != nil {
		p.circuitBreaker.recordFailure()
		p.handleSynthesisError(err)
		return nil, err
	}

	// 记录成功
	p.circuitBreaker.recordSuccess()

	// 缓存音频数据
	p.audioCache.Set(cacheKey, audioData)

	p.logger.InfoTag("EdgeTTS", "音频合成完成，大小: %d字节，SessionID: %s", len(audioData), p.sessionID)
	return audioData, nil
}

// SynthesizeToFile 合成音频到文件（兼容旧接口）
func (p *EdgeTTSProvider) SynthesizeToFile(text string) (string, error) {
	if !p.isInitialized {
		return "", fmt.Errorf("provider not initialized")
	}

	ctx := context.Background()
	options := contractProviders.SynthesisOptions{}

	// 合成音频
	audioData, err := p.Synthesize(ctx, text, options)
	if err != nil {
		return "", err
	}

	// 保存到文件
	filePath := filepath.Join(p.outputDir, fmt.Sprintf("edge_tts_%d.mp3", time.Now().UnixNano()))
	if err := os.WriteFile(filePath, audioData, 0644); err != nil {
		return "", fmt.Errorf("failed to write audio file: %w", err)
	}

	// 如果配置了自动删除，添加到清理列表
	if p.deleteFile {
		p.fileCleanup.AddFile(filePath)
	}

	p.logger.DebugTag("EdgeTTS", "音频文件已保存: %s, SessionID: %s", filePath, p.sessionID)
	return filePath, nil
}

// SetVoice 设置语音类型
func (p *EdgeTTSProvider) SetVoice(voice string) error {
	if voice == "" {
		return fmt.Errorf("voice cannot be empty")
	}

	p.voice = voice
	p.logger.InfoTag("EdgeTTS", "设置语音类型: %s, SessionID: %s", voice, p.sessionID)
	return nil
}

// GetAvailableVoices 获取可用的语音列表
func (p *EdgeTTSProvider) GetAvailableVoices() ([]contractProviders.Voice, error) {
	// 返回Edge TTS支持的部分常用语音
	voices := []contractProviders.Voice{
		{
			ID:          "zh-CN-XiaoxiaoNeural",
			Name:        "晓晓",
			Language:    "zh-CN",
			Gender:      "Female",
			Description: "中文女声 - 自然流畅",
		},
		{
			ID:          "zh-CN-YunyangNeural",
			Name:        "云扬",
			Language:    "zh-CN",
			Gender:      "Male",
			Description: "中文男声 - 成熟稳重",
		},
		{
			ID:          "zh-CN-XiaoyiNeural",
			Name:        "晓伊",
			Language:    "zh-CN",
			Gender:      "Female",
			Description: "中文女声 - 温柔亲切",
		},
		{
			ID:          "en-US-AriaNeural",
			Name:        "Aria",
			Language:    "en-US",
			Gender:      "Female",
			Description: "English female voice - Natural",
		},
		{
			ID:          "en-US-GuyNeural",
			Name:        "Guy",
			Language:    "en-US",
			Gender:      "Male",
			Description: "English male voice - Friendly",
		},
	}

	return voices, nil
}

// GetConfig 获取TTS配置
func (p *EdgeTTSProvider) GetConfig() contractProviders.TTSConfig {
	return contractProviders.TTSConfig{
		Provider:    p.providerType,
		Voice:       p.voice,
		Speed:       p.speed,
		Pitch:       p.pitch,
		Volume:      p.volume,
		Format:      p.format,
		SampleRate:  p.sampleRate,
		Language:    p.extractLanguageFromVoice(p.voice),
		Extra: map[string]interface{}{
			"output_dir":  p.outputDir,
			"delete_file": p.deleteFile,
		},
	}
}

// performSynthesis 执行语音合成
func (p *EdgeTTSProvider) performSynthesis(ctx context.Context, text string, options contractProviders.SynthesisOptions) ([]byte, error) {
	// 更新活动时间
	p.mutex.Lock()
	p.totalRequests++
	p.lastActivity = time.Now()
	p.mutex.Unlock()

	// 配置Edge TTS参数
	voice := options.Voice
	if voice == "" {
		voice = p.voice
	}

	// 创建Edge TTS通信
	communicate, err := edge_tts.New(voice)
	if err != nil {
		return nil, fmt.Errorf("failed to create Edge TTS communicator: %w", err)
	}
	defer communicate.Close()

	// 执行语音合成
	startTime := time.Now()
	audioData, err := communicate.Output(text)
	if err != nil {
		return nil, fmt.Errorf("Edge TTS synthesis failed: %w", err)
	}

	duration := time.Since(startTime)
	p.logger.DebugTag("EdgeTTS", "语音合成耗时: %v, SessionID: %s", duration, p.sessionID)

	return audioData, nil
}

// mergeOptions 合并选项
func (p *EdgeTTSProvider) mergeOptions(options contractProviders.SynthesisOptions) contractProviders.SynthesisOptions {
	merged := options

	// 应用默认值
	if merged.Voice == "" {
		merged.Voice = p.voice
	}
	if merged.Speed == 0 {
		merged.Speed = p.speed
	}
	if merged.Pitch == 0 {
		merged.Pitch = p.pitch
	}
	if merged.Volume == 0 {
		merged.Volume = p.volume
	}
	if merged.Format == "" {
		merged.Format = p.format
	}
	if merged.SampleRate == 0 {
		merged.SampleRate = p.sampleRate
	}

	return merged
}

// generateCacheKey 生成缓存键
func (p *EdgeTTSProvider) generateCacheKey(text string, options contractProviders.SynthesisOptions) string {
	return fmt.Sprintf("%s-%s-%s-%.1f-%.1f", p.voice, text, options.Language, options.Speed, options.Pitch)
}

// extractLanguageFromVoice 从语音ID提取语言
func (p *EdgeTTSProvider) extractLanguageFromVoice(voice string) string {
	if len(voice) >= 5 {
		return voice[:5] // 例如: zh-CN
	}
	return "zh-CN" // 默认中文
}

// handleSynthesisError 处理合成错误
func (p *EdgeTTSProvider) handleSynthesisError(err error) {
	p.mutex.Lock()
	p.errorCount++
	p.mutex.Unlock()

	p.logger.ErrorTag("EdgeTTS", "语音合成失败: %v, SessionID: %s", err, p.sessionID)
}

// NewAudioCache 创建音频缓存
func NewAudioCache(maxSize int, ttl time.Duration) *AudioCache {
	return &AudioCache{
		cache:   make(map[string]*AudioEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get 获取缓存
func (ac *AudioCache) Get(key string) []byte {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()

	if entry, exists := ac.cache[key]; exists {
		if time.Since(entry.timestamp) < ac.ttl {
			return entry.audioData
		}
		delete(ac.cache, key)
	}
	return nil
}

// Set 设置缓存
func (ac *AudioCache) Set(key string, audioData []byte) {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()

	// 简单的LRU：如果缓存满了，删除最老的条目
	if len(ac.cache) >= ac.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range ac.cache {
			if oldestKey == "" || v.timestamp.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.timestamp
			}
		}
		if oldestKey != "" {
			delete(ac.cache, oldestKey)
		}
	}

	ac.cache[key] = &AudioEntry{
		audioData: audioData,
		timestamp: time.Now(),
	}
}

// Clear 清空缓存
func (ac *AudioCache) Clear() {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()
	ac.cache = make(map[string]*AudioEntry)
}

// AddFile 添加文件到清理列表
func (fcm *FileCleanupManager) AddFile(filePath string) {
	fcm.mutex.Lock()
	defer fcm.mutex.Unlock()
	fcm.filesToCleanup = append(fcm.filesToCleanup, filePath)
}

// CleanupAll 清理所有文件
func (fcm *FileCleanupManager) CleanupAll() {
	fcm.mutex.Lock()
	defer fcm.mutex.Unlock()

	for _, filePath := range fcm.filesToCleanup {
		if err := os.Remove(filePath); err != nil {
			// 记录错误但继续清理其他文件
			continue
		}
	}
	fcm.filesToCleanup = make([]string, 0)
}

// isOpen 检查熔断器是否打开
func (cb *CircuitBreaker) isOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	if cb.state == 1 { // open
		if time.Since(cb.lastFailure) > cb.retryAfter {
			cb.state = 2 // half-open
			return false
		}
		return true
	}
	return false
}

// recordSuccess 记录成功
func (cb *CircuitBreaker) recordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures = 0
	cb.state = 0 // closed
}

// recordFailure 记录失败
func (cb *CircuitBreaker) recordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.maxFailures {
		cb.state = 1 // open
	}
}