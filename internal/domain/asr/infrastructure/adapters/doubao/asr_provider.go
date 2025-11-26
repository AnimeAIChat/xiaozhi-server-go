package doubao

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	contractProviders "xiaozhi-server-go/internal/contracts/providers"
	"xiaozhi-server-go/internal/utils"
	"xiaozhi-server-go/internal/transport/ws"

	"github.com/gorilla/websocket"
)

// Protocol constants
const (
	clientFullRequest   = 0x1
	clientAudioRequest  = 0x2
	serverFullResponse  = 0x9
	serverAck           = 0xB
	serverErrorResponse = 0xF
)

// Sequence types
const (
	noSequence  = 0x0
	negSequence = 0x2
)

// Serialization methods
const (
	noSerialization   = 0x0
	jsonFormat        = 0x1
	thriftFormat      = 0x3
	gzipCompression   = 0x1
	customCompression = 0xF

	// 超时设置
	idleTimeout = 30 * time.Second // 没有新数据就结束识别
)

// DoubaoASRProvider 豆包ASR提供者的新架构实现
// 实现统一的ASRProvider接口
type DoubaoASRProvider struct {
	sessionID      string
	providerType   string
	isInitialized  bool
	isListening    bool
	eventListener  contractProviders.ASREventListener
	logger         *utils.Logger

	// 豆包特有配置
	appID         string
	accessToken   string
	outputDir     string
	host          string
	wsURL         string
	chunkDuration int
	connectID     string
	session       *ws.Session

	// 语音识别配置
	modelName     string
	endWindowSize int
	enablePunc    bool
	enableITN     bool
	enableDDC     bool

	// 流式识别相关字段
	conn        *websocket.Conn
	isStreaming bool
	reqID       string
	result      string
	err         error
	connMutex   sync.Mutex

	sendDataCnt int
	ticker      *time.Ticker
	tickerDone  chan struct{}

	// 异步初始化相关字段
	initDone     chan struct{}
	initErr      error
	isReady      bool
	initMutex    sync.RWMutex

	// 预连接相关字段
	preConn        *websocket.Conn
	preConnReady   bool
	preConnMutex   sync.RWMutex
	preConnCtx     context.Context
	preConnCancel  context.CancelFunc

	// 性能优化相关
	lastActivity   time.Time
	connectionPool  sync.Pool // 对象池优化
	cacheEnabled   bool
	circuitBreaker *CircuitBreaker
}

// Config Doubao ASR配置
type Config struct {
	AppID         string `json:"app_id" yaml:"app_id"`
	AccessToken   string `json:"access_token" yaml:"access_token"`
	Host          string `json:"host" yaml:"host"`
	WSURL         string `json:"ws_url" yaml:"ws_url"`
	ChunkDuration int    `json:"chunk_duration" yaml:"chunk_duration"`
	ModelName     string `json:"model_name" yaml:"model_name"`
	EndWindowSize int    `json:"end_window_size" yaml:"end_window_size"`
	EnablePunc    bool   `json:"enable_punc" yaml:"enable_punc"`
	EnableITN     bool   `json:"enable_itn" yaml:"enable_itn"`
	EnableDDC     bool   `json:"enable_ddc" yaml:"enable_ddc"`
	OutputDir     string `json:"output_dir" yaml:"output_dir"`
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

// NewDoubaoASRProvider 创建新的Doubao ASR提供者
func NewDoubaoASRProvider(config Config, logger *utils.Logger) *DoubaoASRProvider {
	if logger == nil {
		logger = utils.DefaultLogger
	}

	provider := &DoubaoASRProvider{
		sessionID:    fmt.Sprintf("doubao-asr-%d", time.Now().UnixNano()),
		providerType: "doubao",
		logger:       logger,
		initDone:     make(chan struct{}),
		tickerDone:   make(chan struct{}),
		cacheEnabled: true,

		// 配置参数
		appID:         config.AppID,
		accessToken:   config.AccessToken,
		outputDir:     config.OutputDir,
		host:          config.Host,
		wsURL:         config.WSURL,
		chunkDuration: config.ChunkDuration,
		modelName:     config.ModelName,
		endWindowSize: config.EndWindowSize,
		enablePunc:    config.EnablePunc,
		enableITN:     config.EnableITN,
		enableDDC:     config.EnableDDC,

		// 初始化熔断器
		circuitBreaker: &CircuitBreaker{
			maxFailures: 5,
			retryAfter:  30 * time.Second,
		},
	}

	// 初始化连接池
	provider.connectionPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 4096) // 4KB缓冲区
		},
	}

	return provider
}

// Initialize 初始化提供者
func (p *DoubaoASRProvider) Initialize() error {
	p.logger.InfoTag("DoubaoASR", "初始化Doubao ASR提供者，SessionID: %s", p.sessionID)

	// 验证必要配置
	if p.appID == "" {
		return fmt.Errorf("app_id is required")
	}
	if p.accessToken == "" {
		return fmt.Errorf("access_token is required")
	}

	// 设置默认值
	if p.modelName == "" {
		p.modelName = "volc_asr_general"
	}
	if p.chunkDuration <= 0 {
		p.chunkDuration = 1000 // 默认1秒
	}
	if p.endWindowSize <= 0 {
		p.endWindowSize = 6000 // 默认6秒
	}

	// 异步初始化
	go p.initializeAsync()

	p.isInitialized = true
	p.lastActivity = time.Now()

	return nil
}

// Cleanup 清理提供者资源
func (p *DoubaoASRProvider) Cleanup() error {
	p.logger.InfoTag("DoubaoASR", "清理Doubao ASR提供者，SessionID: %s", p.sessionID)

	if p.isListening {
		if err := p.StopListening(); err != nil {
			p.logger.WarnTag("DoubaoASR", "停止监听时出错: %v", err)
		}
	}

	if p.ticker != nil {
		p.ticker.Stop()
		close(p.tickerDone)
	}

	// 清理预连接
	if p.preConn != nil {
		p.preConn.Close()
		p.preConn = nil
	}

	p.isInitialized = false
	return nil
}

// HealthCheck 健康检查
func (p *DoubaoASRProvider) HealthCheck(ctx context.Context) error {
	if !p.isInitialized {
		return fmt.Errorf("provider not initialized")
	}

	// 检查熔断器状态
	if p.circuitBreaker.isOpen() {
		return fmt.Errorf("circuit breaker is open")
	}

	// 检查最近活动时间
	if time.Since(p.lastActivity) > 5*time.Minute {
		return fmt.Errorf("provider inactive for too long")
	}

	return nil
}

// GetProviderType 获取提供者类型
func (p *DoubaoASRProvider) GetProviderType() string {
	return p.providerType
}

// GetSessionID 获取会话ID
func (p *DoubaoASRProvider) GetSessionID() string {
	return p.sessionID
}

// StartListening 开始监听音频输入
func (p *DoubaoASRProvider) StartListening() error {
	if !p.isInitialized {
		return fmt.Errorf("provider not initialized")
	}

	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	if p.isListening {
		return fmt.Errorf("already listening")
	}

	p.logger.InfoTag("DoubaoASR", "开始监听音频输入，SessionID: %s", p.sessionID)
	p.isListening = true
	p.lastActivity = time.Now()

	return nil
}

// StopListening 停止监听音频输入
func (p *DoubaoASRProvider) StopListening() error {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	if !p.isListening {
		return nil
	}

	p.logger.InfoTag("DoubaoASR", "停止监听音频输入，SessionID: %s", p.sessionID)
	p.isListening = false
	p.lastActivity = time.Now()

	// 发送结束信号
	if p.conn != nil {
		// TODO: 实现WebSocket结束信号发送
	}

	return nil
}

// ProcessAudioData 处理音频数据
func (p *DoubaoASRProvider) ProcessAudioData(audioData []byte) error {
	if !p.isInitialized || !p.isListening {
		return fmt.Errorf("provider not ready")
	}

	// 更新活动时间
	p.lastActivity = time.Now()

	// 从对象池获取缓冲区
	buffer := p.connectionPool.Get().([]byte)
	defer p.connectionPool.Put(buffer[:0]) // 重置长度但保留容量

	// 处理音频数据
	buffer = append(buffer, audioData...)

	p.logger.DebugTag("DoubaoASR", "处理音频数据，大小: %d字节，SessionID: %s", len(audioData), p.sessionID)

	// TODO: 实现实际的音频数据处理和WebSocket发送
	// 这里需要根据豆包的协议格式处理音频数据

	return nil
}

// Transcribe 直接识别音频数据（同步模式）
func (p *DoubaoASRProvider) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	if !p.isInitialized {
		return "", fmt.Errorf("provider not initialized")
	}

	// 检查熔断器
	if p.circuitBreaker.isOpen() {
		return "", fmt.Errorf("circuit breaker is open")
	}

	p.logger.InfoTag("DoubaoASR", "开始音频转录，大小: %d字节，SessionID: %s", len(audioData), p.sessionID)

	// TODO: 实现同步音频转录逻辑
	// 这里需要实现完整的音频处理和结果返回流程

	return "转录结果占位符", nil
}

// SetEventListener 设置事件监听器
func (p *DoubaoASRProvider) SetEventListener(listener contractProviders.ASREventListener) error {
	p.eventListener = listener
	p.logger.InfoTag("DoubaoASR", "设置事件监听器，SessionID: %s", p.sessionID)
	return nil
}

// Reset 重置ASR状态
func (p *DoubaoASRProvider) Reset() {
	p.logger.InfoTag("DoubaoASR", "重置ASR状态，SessionID: %s", p.sessionID)

	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	p.result = ""
	p.err = nil
	p.sendDataCnt = 0
	p.lastActivity = time.Now()
}

// Close 关闭ASR资源
func (p *DoubaoASRProvider) Close() error {
	return p.Cleanup()
}

// SetUserPreferences 设置用户偏好
func (p *DoubaoASRProvider) SetUserPreferences(preferences map[string]interface{}) error {
	if preferences == nil {
		return fmt.Errorf("preferences cannot be nil")
	}

	p.logger.InfoTag("DoubaoASR", "设置用户偏好，SessionID: %s", p.sessionID)

	// 处理用户偏好设置
	if model, ok := preferences["model"].(string); ok {
		p.modelName = model
	}

	if enablePunc, ok := preferences["enable_punctuation"].(bool); ok {
		p.enablePunc = enablePunc
	}

	if enableITN, ok := preferences["enable_itn"].(bool); ok {
		p.enableITN = enableITN
	}

	return nil
}

// EnableSilenceDetection 启用静音检测
func (p *DoubaoASRProvider) EnableSilenceDetection(bEnable bool) {
	p.logger.InfoTag("DoubaoASR", "设置静音检测: %v，SessionID: %s", bEnable, p.sessionID)
	// TODO: 实现静音检测逻辑
}

// GetSilenceCount 获取当前静音计数
func (p *DoubaoASRProvider) GetSilenceCount() int {
	// TODO: 实现静音计数逻辑
	return 0
}

// ResetSilenceCount 重置静音计数
func (p *DoubaoASRProvider) ResetSilenceCount() {
	p.logger.DebugTag("DoubaoASR", "重置静音计数，SessionID: %s", p.sessionID)
}

// ResetStartListenTime 重置开始监听时间
func (p *DoubaoASRProvider) ResetStartListenTime() {
	p.logger.DebugTag("DoubaoASR", "重置开始监听时间，SessionID: %s", p.sessionID)
	p.lastActivity = time.Now()
}

// initializeAsync 异步初始化
func (p *DoubaoASRProvider) initializeAsync() {
	defer close(p.initDone)

	p.initMutex.Lock()
	p.isReady = true
	p.initMutex.Unlock()

	p.logger.InfoTag("DoubaoASR", "异步初始化完成，SessionID: %s", p.sessionID)
}

// CircuitBreaker 熔断器方法
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

func (cb *CircuitBreaker) recordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures = 0
	cb.state = 0 // closed
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.maxFailures {
		cb.state = 1 // open
	}
}