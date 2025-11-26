package openai

import (
	"context"
	"fmt"
	"sync"
	"time"

	contractProviders "xiaozhi-server-go/internal/contracts/providers"
	"xiaozhi-server-go/src/core/utils"

	"github.com/sashabaranov/go-openai"
)

// OpenAILLMProvider OpenAI LLM提供者的新架构实现
// 实现统一的LLMProvider接口
type OpenAILLMProvider struct {
	sessionID      string
	providerType   string
	isInitialized  bool
	logger         *utils.Logger
	identityType   string
	identityFlag   string

	// OpenAI特有配置
	client    *openai.Client
	apiKey    string
	baseURL   string
	model     string
	maxTokens int
	timeout   time.Duration

	// 性能优化相关
	connectionPool  *ConnectionPool
	cache           *ResponseCache
	circuitBreaker  *CircuitBreaker
	requestTimeout  time.Duration
	rateLimiter     *RateLimiter

	// 配置参数
	temperature float32

	// 状态跟踪
	lastActivity time.Time
	totalRequests int64
	errorCount    int64
	mutex         sync.RWMutex
}

// Config OpenAI LLM配置
type Config struct {
	APIKey      string        `json:"api_key" yaml:"api_key"`
	BaseURL     string        `json:"base_url" yaml:"base_url"`
	Model       string        `json:"model" yaml:"model"`
	MaxTokens   int           `json:"max_tokens" yaml:"max_tokens"`
	Temperature float32       `json:"temperature" yaml:"temperature"`
	Timeout     time.Duration `json:"timeout" yaml:"timeout"`
}

// ConnectionPool HTTP连接池实现
type ConnectionPool struct {
	clients    map[string]*openai.Client
	mutex      sync.RWMutex
	maxClients int
}

// ResponseCache 响应缓存实现
type ResponseCache struct {
	cache    map[string]*CacheEntry
	mutex    sync.RWMutex
	maxSize  int
	ttl      time.Duration
}

// CacheEntry 缓存条目
type CacheEntry struct {
	response  contractProviders.ResponseChunk
	timestamp time.Time
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

// RateLimiter 速率限制器
type RateLimiter struct {
	requests   []time.Time
	mutex      sync.Mutex
	maxRPS     int // 每秒最大请求数
	windowSize time.Duration
}

// NewOpenAILLMProvider 创建新的OpenAI LLM提供者
func NewOpenAILLMProvider(config Config, logger *utils.Logger) *OpenAILLMProvider {
	if logger == nil {
		logger = utils.DefaultLogger
	}

	provider := &OpenAILLMProvider{
		sessionID:    fmt.Sprintf("openai-llm-%d", time.Now().UnixNano()),
		providerType: "openai",
		logger:       logger,
		isInitialized: false,

		// 配置参数
		apiKey:    config.APIKey,
		baseURL:   config.BaseURL,
		model:     config.Model,
		maxTokens: config.MaxTokens,
		timeout:   config.Timeout,
		temperature: config.Temperature,

		// 设置默认值
		requestTimeout: 30 * time.Second,
	}

	// 初始化性能优化组件
	provider.connectionPool = NewConnectionPool(10)
	provider.cache = NewResponseCache(1000, 10*time.Minute)
	provider.circuitBreaker = &CircuitBreaker{
		maxFailures: 5,
		retryAfter:  30 * time.Second,
	}
	provider.rateLimiter = NewRateLimiter(60, time.Minute) // 60 requests per minute

	return provider
}

// Initialize 初始化提供者
func (p *OpenAILLMProvider) Initialize() error {
	p.logger.InfoTag("OpenAILLM", "初始化OpenAI LLM提供者，SessionID: %s", p.sessionID)

	// 验证必要配置
	if p.apiKey == "" {
		return fmt.Errorf("api_key is required")
	}

	// 设置默认值
	if p.model == "" {
		p.model = "gpt-3.5-turbo"
	}
	if p.maxTokens <= 0 {
		p.maxTokens = 500
	}
	if p.timeout <= 0 {
		p.timeout = 30 * time.Second
	}
	if p.temperature < 0 || p.temperature > 2 {
		p.temperature = 0.7
	}

	// 创建OpenAI客户端
	clientConfig := openai.DefaultConfig(p.apiKey)
	if p.baseURL != "" {
		clientConfig.BaseURL = p.baseURL
	}

	p.client = openai.NewClientWithConfig(clientConfig)
	p.isInitialized = true
	p.lastActivity = time.Now()

	p.logger.InfoTag("OpenAILLM", "OpenAI LLM提供者初始化完成，Model: %s, SessionID: %s", p.model, p.sessionID)
	return nil
}

// Cleanup 清理提供者资源
func (p *OpenAILLMProvider) Cleanup() error {
	p.logger.InfoTag("OpenAILLM", "清理OpenAI LLM提供者，SessionID: %s", p.sessionID)

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.connectionPool != nil {
		p.connectionPool.Close()
	}

	if p.cache != nil {
		p.cache.Clear()
	}

	p.isInitialized = false
	return nil
}

// HealthCheck 健康检查
func (p *OpenAILLMProvider) HealthCheck(ctx context.Context) error {
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

	// 执行简单的API测试
	_, err := p.client.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("API health check failed: %w", err)
	}

	return nil
}

// GetProviderType 获取提供者类型
func (p *OpenAILLMProvider) GetProviderType() string {
	return p.providerType
}

// GetSessionID 获取会话ID
func (p *OpenAILLMProvider) GetSessionID() string {
	return p.sessionID
}

// Response 生成回复（基础模式）
func (p *OpenAILLMProvider) Response(ctx context.Context, sessionID string, messages []contractProviders.Message, tools []contractProviders.Tool) (<-chan contractProviders.ResponseChunk, error) {
	if !p.isInitialized {
		return nil, fmt.Errorf("provider not initialized")
	}

	// 检查熔断器
	if p.circuitBreaker.isOpen() {
		return nil, fmt.Errorf("circuit breaker is open")
	}

	// 检查速率限制
	if !p.rateLimiter.AllowRequest() {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	p.logger.InfoTag("OpenAILLM", "开始生成回复，SessionID: %s, Messages: %d", sessionID, len(messages))

	// 生成缓存键
	cacheKey := p.generateCacheKey(messages, tools)

	// 检查缓存
	if cachedResponse := p.cache.Get(cacheKey); cachedResponse != nil {
		p.logger.DebugTag("OpenAILLM", "使用缓存响应，SessionID: %s", sessionID)
		return p.singleChunkChannel(cachedResponse), nil
	}

	// 创建响应通道
	responseChan := make(chan contractProviders.ResponseChunk, 1)

	// 异步处理请求
	go p.handleResponseRequest(ctx, sessionID, messages, tools, responseChan, cacheKey)

	return responseChan, nil
}

// ResponseWithFunctions 生成带函数调用的回复
func (p *OpenAILLMProvider) ResponseWithFunctions(ctx context.Context, sessionID string, messages []contractProviders.Message, tools []contractProviders.Tool) (<-chan contractProviders.ResponseChunk, error) {
	return p.Response(ctx, sessionID, messages, tools)
}

// ResponseWithTools 生成带工具的回复
func (p *OpenAILLMProvider) ResponseWithTools(ctx context.Context, sessionID string, messages []contractProviders.Message, tools []contractProviders.Tool) (<-chan contractProviders.ResponseChunk, error) {
	return p.Response(ctx, sessionID, messages, tools)
}

// GetCapabilities 获取提供者能力
func (p *OpenAILLMProvider) GetCapabilities() contractProviders.LLMCapabilities {
	return contractProviders.LLMCapabilities{
		SupportStreaming: true,
		SupportFunctions: true,
		SupportVision:    true, // OpenAI支持视觉输入
		MaxTokens:        p.maxTokens,
		SupportedModels: []string{
			"gpt-3.5-turbo",
			"gpt-4",
			"gpt-4-turbo",
			"gpt-4-vision-preview",
		},
	}
}

// SetIdentityFlag 设置身份标识
func (p *OpenAILLMProvider) SetIdentityFlag(idType string, flag string) {
	p.identityType = idType
	p.identityFlag = flag
	p.logger.InfoTag("OpenAILLM", "设置身份标识，Type: %s, Flag: %s, SessionID: %s", idType, flag, p.sessionID)
}

// GetConfig 获取LLM配置
func (p *OpenAILLMProvider) GetConfig() contractProviders.LLMConfig {
	return contractProviders.LLMConfig{
		Provider:    p.providerType,
		Model:       p.model,
		APIKey:      p.apiKey, // 注意：生产环境中不应暴露API密钥
		BaseURL:     p.baseURL,
		Temperature: p.temperature,
		MaxTokens:   p.maxTokens,
		Timeout:     int(p.timeout.Seconds()),
		Extra: map[string]interface{}{
			"identity_type": p.identityType,
			"identity_flag": p.identityFlag,
		},
	}
}

// Close 关闭LLM资源
func (p *OpenAILLMProvider) Close() error {
	return p.Cleanup()
}

// handleResponseRequest 处理响应请求
func (p *OpenAILLMProvider) handleResponseRequest(ctx context.Context, sessionID string, messages []contractProviders.Message, tools []contractProviders.Tool, responseChan chan<- contractProviders.ResponseChunk, cacheKey string) {
	defer close(responseChan)

	p.mutex.Lock()
	p.totalRequests++
	p.lastActivity = time.Now()
	p.mutex.Unlock()

	// 转换消息格式
	openaiMessages := p.convertMessages(messages)
	openaiTools := p.convertTools(tools)

	// 创建请求
	request := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    openaiMessages,
		MaxTokens:   p.maxTokens,
		Temperature: p.temperature,
		Stream:      false,
	}

	if len(openaiTools) > 0 {
		request.Tools = openaiTools
	}

	// 设置超时上下文
	requestCtx, cancel := context.WithTimeout(ctx, p.requestTimeout)
	defer cancel()

	// 调用OpenAI API
	response, err := p.client.CreateChatCompletion(requestCtx, request)
	if err != nil {
		p.handleRequestError(err, cacheKey)
		responseChan <- contractProviders.ResponseChunk{
			Error:  err,
			IsDone: true,
		}
		return
	}

	// 记录成功
	p.circuitBreaker.recordSuccess()

	// 转换响应
	if len(response.Choices) > 0 {
		choice := response.Choices[0]
		responseChunk := contractProviders.ResponseChunk{
			Content: choice.Message.Content,
			IsDone:  true,
			Usage: &contractProviders.Usage{
				PromptTokens:     response.Usage.PromptTokens,
				CompletionTokens: response.Usage.CompletionTokens,
				TotalTokens:      response.Usage.TotalTokens,
			},
		}

		// 缓存响应
		p.cache.Set(cacheKey, &responseChunk)

		responseChan <- responseChunk
	} else {
		responseChan <- contractProviders.ResponseChunk{
			Error:  fmt.Errorf("no response from OpenAI"),
			IsDone: true,
		}
	}
}

// convertMessages 转换消息格式
func (p *OpenAILLMProvider) convertMessages(messages []contractProviders.Message) []openai.ChatCompletionMessage {
	openaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		openaiMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return openaiMessages
}

// convertTools 转换工具格式
func (p *OpenAILLMProvider) convertTools(tools []contractProviders.Tool) []openai.Tool {
	openaiTools := make([]openai.Tool, len(tools))
	for i, tool := range tools {
		openaiTools[i] = openai.Tool{
			Type: tool.Type,
			Function: openai.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		}
	}
	return openaiTools
}

// generateCacheKey 生成缓存键
func (p *OpenAILLMProvider) generateCacheKey(messages []contractProviders.Message, tools []contractProviders.Tool) string {
	// 简单的缓存键生成，实际应用中应该使用更复杂的哈希算法
	key := fmt.Sprintf("%s-%d", p.model, len(messages))
	for _, msg := range messages {
		key += fmt.Sprintf("-%s:%s", msg.Role, msg.Content)
	}
	return key
}

// handleRequestError 处理请求错误
func (p *OpenAILLMProvider) handleRequestError(err error, cacheKey string) {
	p.mutex.Lock()
	p.errorCount++
	p.mutex.Unlock()

	p.circuitBreaker.recordFailure()
	p.logger.ErrorTag("OpenAILLM", "请求失败: %v, SessionID: %s", err, p.sessionID)
}

// singleChunkChannel 创建单块响应通道
func (p *OpenAILLMProvider) singleChunkChannel(chunk *contractProviders.ResponseChunk) <-chan contractProviders.ResponseChunk {
	responseChan := make(chan contractProviders.ResponseChunk, 1)
	go func() {
		responseChan <- *chunk
		close(responseChan)
	}()
	return responseChan
}

// NewConnectionPool 创建连接池
func NewConnectionPool(maxClients int) *ConnectionPool {
	return &ConnectionPool{
		clients:    make(map[string]*openai.Client),
		maxClients: maxClients,
	}
}

// Close 关闭连接池
func (cp *ConnectionPool) Close() {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()
	// 清理所有客户端连接
	cp.clients = make(map[string]*openai.Client)
}

// NewResponseCache 创建响应缓存
func NewResponseCache(maxSize int, ttl time.Duration) *ResponseCache {
	return &ResponseCache{
		cache:   make(map[string]*CacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get 获取缓存
func (rc *ResponseCache) Get(key string) *contractProviders.ResponseChunk {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	if entry, exists := rc.cache[key]; exists {
		if time.Since(entry.timestamp) < rc.ttl {
			return &entry.response
		}
		delete(rc.cache, key)
	}
	return nil
}

// Set 设置缓存
func (rc *ResponseCache) Set(key string, response *contractProviders.ResponseChunk) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	// 简单的LRU：如果缓存满了，删除最老的条目
	if len(rc.cache) >= rc.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range rc.cache {
			if oldestKey == "" || v.timestamp.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.timestamp
			}
		}
		if oldestKey != "" {
			delete(rc.cache, oldestKey)
		}
	}

	rc.cache[key] = &CacheEntry{
		response:  *response,
		timestamp: time.Now(),
	}
}

// Clear 清空缓存
func (rc *ResponseCache) Clear() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.cache = make(map[string]*CacheEntry)
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

// NewRateLimiter 创建速率限制器
func NewRateLimiter(maxRPS int, windowSize time.Duration) *RateLimiter {
	return &RateLimiter{
		requests:   make([]time.Time, 0),
		maxRPS:     maxRPS,
		windowSize: windowSize,
	}
}

// AllowRequest 检查是否允许请求
func (rl *RateLimiter) AllowRequest() bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()

	// 清理过期的请求记录
	cutoff := now.Add(-rl.windowSize)
	validRequests := make([]time.Time, 0)
	for _, reqTime := range rl.requests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}
	rl.requests = validRequests

	// 检查是否超过速率限制
	if len(rl.requests) >= rl.maxRPS {
		return false
	}

	// 记录新请求
	rl.requests = append(rl.requests, now)
	return true
}