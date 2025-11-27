package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"

	"xiaozhi-server-go/internal/domain/llm"
	"xiaozhi-server-go/internal/platform/config"
)


// Options configures the manager instance.
type Options struct {
	Logger     Logger
	Config     *config.Config
	AutoStart  bool
	AutoReturn bool
}

// Conn captures the minimal connection behaviour required by MCP clients.
type Conn interface {
	WriteMessage(messageType int, data []byte) error
	// GetWebSocketConn returns the underlying WebSocket connection if available
	GetWebSocketConn() *websocket.Conn
}

// Manager coordinates MCP clients and tool execution.
type Manager struct {
	logger Logger

	registry *toolRegistry

	clientsMu sync.RWMutex
	clients   map[string]Client

	configLoader *ConfigLoader

	// XiaoZhi client
	xiaozhiClient *XiaoZhiMCPClient

	// Local client
	localClient *LocalClient

	autoReturn    bool
	isInitialized bool

	// 工具调用缓存，防止短时间内重复调用相同参数的工具
	callCache    map[string]interface{}
	cacheMu      sync.RWMutex
	cacheExpiry  time.Time
}

// NewManager constructs a new manager instance.
func NewManager(opts Options) (*Manager, error) {
	if opts.Logger == nil {
		return nil, errors.New("mcp manager requires logger")
	}
	if opts.Config == nil {
		return nil, errors.New("mcp manager requires config")
	}

	manager := &Manager{
		logger:       opts.Logger,
		registry:     newToolRegistry(),
		clients:      make(map[string]Client),
		configLoader: NewConfigLoader(opts.Logger),
		autoReturn:   opts.AutoReturn,
		callCache:    make(map[string]interface{}),
	}

	// Initialize local client
	localClient, err := NewLocalClient(opts.Logger, opts.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create local client: %w", err)
	}
	manager.localClient = localClient

	// Pre-create XiaoZhi client (will be bound to connection later)
	xiaozhiClient, err := NewXiaoZhiMCPClient(opts.Logger, opts.Config, nil) // auth will be set during binding
	if err != nil {
		return nil, fmt.Errorf("failed to create XiaoZhi client: %w", err)
	}
	manager.xiaozhiClient = xiaozhiClient

	// 启动本地客户端
	if err := localClient.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start local client: %w", err)
	}
	manager.clients["local"] = localClient

	// 标记为已初始化
	manager.isInitialized = true

	opts.Logger.Info("MCP管理器初始化完成")
	return manager, nil
}


func (m *Manager) addClients(clients map[string]Client, autoStart bool) error {
	if len(clients) == 0 {
		return nil
	}

	for name, client := range maps.Clone(clients) {
		if err := m.registerClient(name, client, autoStart); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) registerClient(name string, client Client, autoStart bool) error {
	if name == "" {
		return fmt.Errorf("client name cannot be empty")
	}
	if client == nil {
		return fmt.Errorf("client %s is nil", name)
	}

	if autoStart {
		if err := client.Start(context.Background()); err != nil {
			return fmt.Errorf("start client %s: %w", name, err)
		}
	}

	if err := m.registry.register(client.GetAvailableTools()); err != nil {
		return err
	}

	m.clientsMu.Lock()
	m.clients[name] = client
	m.clientsMu.Unlock()

	m.logger.InfoTag("MCP", "注册客户端 %s（工具数量=%d）", name, len(client.GetAvailableTools()))
	m.refreshToolRegistry()
	return nil
}

// RegisterClient attaches a new client to the manager.
func (m *Manager) RegisterClient(name string, client Client, autoStart bool) error {
	return m.registerClient(name, client, autoStart)
}

// RemoveClient detaches a client and stops it.
func (m *Manager) RemoveClient(name string) {
	m.clientsMu.Lock()
	client, ok := m.clients[name]
	if ok {
		delete(m.clients, name)
	}
	m.clientsMu.Unlock()

	if ok && client != nil {
		client.Stop()
	}
}

// ListClients returns the registered client names.
func (m *Manager) ListClients() []string {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()
	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// RegisterTools merges the supplied tools into the registry.
func (m *Manager) RegisterTools(tools []Tool) error {
	if len(tools) == 0 {
		return nil
	}
	openAITools := make([]openai.Tool, 0, len(tools))
	for _, tool := range tools {
		openAITools = append(openAITools, tool.toOpenAITool())
	}
	return m.registry.register(openAITools)
}

// ExecuteTool executes a tool by name across known clients.
func (m *Manager) ExecuteTool(ctx context.Context, name string, args map[string]any) (any, error) {
	if name == "" {
		return nil, errors.New("tool name cannot be empty")
	}

	m.logger.Info("Executing tool %s with arguments: %v", name, args)

	// 生成缓存键
	cacheKey := m.generateCacheKey(name, args)
	m.logger.DebugTag("MCP", "生成缓存键: %s", cacheKey)

	// 检查缓存，避免重复调用
	m.cacheMu.RLock()
	cachedResult, exists := m.callCache[cacheKey]
	cacheValid := exists && time.Now().Before(m.cacheExpiry)
	m.cacheMu.RUnlock()

	if cacheValid {
		m.logger.InfoTag("MCP", "使用缓存结果: %s (10秒内)", name)
		return cachedResult, nil
	}

	if exists {
		// 缓存过期，清理旧缓存
		m.cacheMu.Lock()
		delete(m.callCache, cacheKey)
		m.cacheMu.Unlock()
		m.logger.DebugTag("MCP", "清理过期缓存: %s", cacheKey)
	}

	m.clientsMu.RLock()
	clients := maps.Clone(m.clients)
	m.clientsMu.RUnlock()

	if len(clients) == 0 {
		return nil, errors.New("no MCP clients registered")
	}

	for clientName, client := range clients {
		if client == nil || !client.HasTool(name) {
			continue
		}
		m.logger.DebugTag("MCP", "执行工具 %s，来自客户端 %s", name, clientName)
		result, err := client.CallTool(ctx, name, args)
		if err != nil {
			return nil, fmt.Errorf("client %s failed: %w", clientName, err)
		}

		// 缓存成功的结果，对话场景下使用较短缓存时间
		m.cacheMu.Lock()
		m.callCache[cacheKey] = result
		m.cacheExpiry = time.Now().Add(10 * time.Second) // 缓存10秒，适合对话场景
		m.cacheMu.Unlock()
		m.logger.DebugTag("MCP", "缓存结果: %s", cacheKey)

		return result, nil
	}

	return nil, fmt.Errorf("tool %s not found in clients %v", name, maps.Keys(clients))
}

// ToolNames returns the registered tool names sorted alphabetically.
func (m *Manager) ToolNames() []string {
	if m.registry != nil {
		return m.registry.list()
	}
	return nil
}

// GetAvailableTools returns all registered tools with complete definitions.
func (m *Manager) GetAvailableTools() []openai.Tool {
	if m.registry != nil {
		tools := m.registry.clone()
		result := make([]openai.Tool, 0, len(tools))
		for _, tool := range tools {
			result = append(result, tool)
		}
		return result
	}
	return nil
}

// Close stops every registered client.
func (m *Manager) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	var errs []error
	m.clientsMu.Lock()
	for name, client := range m.clients {
		if client == nil {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					errs = append(errs, fmt.Errorf("panic closing client %s: %v", name, r))
				}
			}()

			client.Stop()
		}()
		delete(m.clients, name)
	}
	m.clientsMu.Unlock()

	return errors.Join(errs...)
}

// AutoReturn reports whether the manager should be returned to its pool automatically.
func (m *Manager) AutoReturn() bool {
	return m.autoReturn
}

// BindConnection attaches the websocket connection to the MCP clients.
func (m *Manager) BindConnection(
	conn Conn,
	fh llm.FunctionRegistryInterface,
	params any,
) error {
	paramsMap := params.(map[string]interface{})
	sessionID := paramsMap["session_id"].(string)
	visionURL := paramsMap["vision_url"].(string)

	m.logger.Debug("绑定连接到MCP Manager, sessionID: %s, visionURL: %s", sessionID, visionURL)

	// Ensure local MCP tools are registered before handling requests.
	m.registerLocalTools(fh)

	// 获取全局MCP管理器并注册外部工具
	globalMCPManager := GetGlobalMCPManager()
	if globalMCPManager.IsReady() {
		m.registerGlobalToolsToConnection(fh, globalMCPManager)
	}

	// 完全异步处理MCP初始化和绑定，不阻塞连接建立
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("MCP绑定连接时发生panic: %v", r)
			}
		}()

		m.logger.Info("BindConnection: Starting async binding process")

		// 等待全局MCP管理器初始化完成（如果还未完成）
		for !globalMCPManager.IsReady() {
			time.Sleep(10 * time.Millisecond)
		}

		m.logger.Debug("BindConnection: Global MCP Manager initialized, registering external tools")

		// 注册全局外部工具到连接
		m.registerGlobalToolsToConnection(fh, globalMCPManager)

		// 绑定 XiaoZhi 客户端
		if m.xiaozhiClient != nil {
			m.logger.Debug("BindConnection: XiaoZhi client exists, attempting to bind")
			// Get the underlying WebSocket connection
			wsConn := conn.GetWebSocketConn()
			m.logger.Debug("BindConnection: GetWebSocketConn returned: %v (type: %T)", wsConn, wsConn)
			if wsConn == nil {
				m.logger.Warn("BindConnection: connection does not provide WebSocket access, skipping XiaoZhi client binding")
				return
			}

			m.logger.Debug("BindConnection: successfully obtained WebSocket connection, proceeding with XiaoZhi client binding")
			if err := m.xiaozhiClient.BindConnection(wsConn); err != nil {
				m.logger.Error("绑定XiaoZhi MCP客户端失败: %v", err)
				return
			}

			// 异步等待客户端准备就绪，避免阻塞
			go func() {
				m.logger.Debug("BindConnection: waiting for XiaoZhi client to be ready")
				// 使用较短的超时时间，避免长时间阻塞
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				if err := m.xiaozhiClient.WaitForReady(ctx); err != nil {
					m.logger.Warn("等待XiaoZhi MCP客户端准备就绪失败或超时: %v", err)
					return
				}

				// 注册 XiaoZhi 客户端工具
				m.clientsMu.Lock()
				m.clients["xiaozhi"] = m.xiaozhiClient
				m.clientsMu.Unlock()

				tools := m.xiaozhiClient.GetAvailableTools()
				for _, tool := range tools {
					toolName := tool.Function.Name
					fh.RegisterFunction(toolName, tool)
				}
				m.logger.Info("Registered XiaoZhi MCP tools: %d", len(tools))

				// Also register XiaoZhi tools in the internal registry
				if err := m.registry.register(tools); err != nil {
					m.logger.Error("注册XiaoZhi MCP工具到内部注册表失败: %v", err)
				}

				m.logger.Info("XiaoZhi MCP client binding completed successfully")
			}()
		} else {
			m.logger.Warn("BindConnection: XiaoZhi client is nil, skipping binding")
		}
	}()

	return nil
}

// registerLocalTools ensures local MCP tools are registered with the current function registry.
func (m *Manager) registerLocalTools(fh llm.FunctionRegistryInterface) {
	if fh == nil {
		return
	}
	if m.localClient == nil {
		m.logger.Warn("registerLocalTools: local MCP client is not initialized")
		return
	}

	tools := m.localClient.GetAvailableTools()
	if len(tools) == 0 {
		m.logger.Debug("registerLocalTools: no local MCP tools to register")
		return
	}

	registered := make([]openai.Tool, 0, len(tools))
	for _, tool := range tools {
		toolName := tool.Function.Name
		if toolName == "" {
			continue
		}
		if err := fh.RegisterFunction(toolName, tool); err != nil {
			m.logger.Error("注册本地MCP工具失败: %s, 错误: %v", toolName, err)
			continue
		}
		registered = append(registered, tool)
	}

	if len(registered) == 0 {
		m.logger.Warn("registerLocalTools: no local MCP tools registered")
		return
	}

	if err := m.registry.register(registered); err != nil {
		m.logger.Error("注册本地MCP工具到内部注册表失败: %v", err)
	}

	m.logger.Info("[MCP] 已注册的本地 MCP 工具数量: %d", len(registered))
}

// registerGlobalToolsToConnection 注册全局外部工具到连接
func (m *Manager) registerGlobalToolsToConnection(fh llm.FunctionRegistryInterface, globalManager *GlobalMCPManager) {
	if fh == nil || !globalManager.IsReady() {
		return
	}

	// 获取全局外部工具
	globalClients := globalManager.GetAllClients()
	registeredCount := 0

	for name, client := range globalClients {
		if name == "local" || !client.IsReady() {
			m.logger.Debug("registerGlobalToolsToConnection: 跳过本地客户端 %s", name)
			continue
		}

		tools := client.GetAvailableTools()
		m.logger.Info("registerGlobalToolsToConnection: 客户端 %s 有 %d 个工具", name, len(tools))

		for _, tool := range tools {
			toolName := tool.Function.Name
			if err := fh.RegisterFunction(toolName, tool); err != nil {
				m.logger.Error("注册全局MCP工具失败: %s, 错误: %v", toolName, err)
				continue
			}
			m.logger.Info("Registered external MCP tool: [%s] %s", toolName, tool.Function.Description)
			registeredCount++
		}

		// 将外部客户端也注册到当前管理器以便工具执行
		m.clientsMu.Lock()
		m.clients[name] = client
		m.clientsMu.Unlock()

		// 同时注册到内部注册表用于IsMCPTool检查
		if err := m.registry.register(tools); err != nil {
			m.logger.Error("注册外部MCP工具到内部注册表失败: %v", err)
		}
	}

	m.logger.Info("registerGlobalToolsToConnection: 共注册了 %d 个外部MCP工具", registeredCount)
}

// registerExternalTools registers external MCP client tools
func (m *Manager) registerExternalTools(fh llm.FunctionRegistryInterface) {
	m.clientsMu.RLock()
	clients := maps.Clone(m.clients)
	m.clientsMu.RUnlock()

	m.logger.Info("registerExternalTools: 开始注册外部MCP工具，客户端数量: %d", len(clients))

	registeredCount := 0
	for name, client := range clients {
		if name == "xiaozhi" || name == "local" || !client.IsReady() {
			m.logger.Debug("registerExternalTools: 跳过客户端 %s (xiaozhi/local/not ready)", name)
			continue
		}

		tools := client.GetAvailableTools()
		m.logger.Info("registerExternalTools: 客户端 %s 有 %d 个工具", name, len(tools))

		for _, tool := range tools {
			toolName := tool.Function.Name
			if err := fh.RegisterFunction(toolName, tool); err != nil {
				m.logger.Error("注册外部MCP工具失败: %s, 错误: %v", toolName, err)
				continue
			}
			m.logger.Info("Registered external MCP tool: [%s] %s", toolName, tool.Function.Description)
			registeredCount++
		}

		// Also register tools in the internal registry for IsMCPTool to work
		if err := m.registry.register(tools); err != nil {
			m.logger.Error("注册外部MCP工具到内部注册表失败: %v", err)
		}
	}

	m.logger.Info("registerExternalTools: 共注册了 %d 个外部MCP工具", registeredCount)
}

// Cleanup calls the underlying cleanup routine.
func (m *Manager) Cleanup() error {
	return m.Reset()
}

// CleanupAll closes all MCP clients.
func (m *Manager) CleanupAll(ctx context.Context) {
	m.Close(ctx)
}

// Reset clears internal state for reuse.
func (m *Manager) Reset() error {
	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()

	// Reset XiaoZhi client connection state
	if m.xiaozhiClient != nil {
		m.xiaozhiClient.ResetConnection()
	}

	// Keep local client, just reset external clients
	for name, client := range m.clients {
		if name != "local" {
			delete(m.clients, name)
			if client != nil {
				client.Stop()
			}
		}
	}

	if m.registry != nil {
		m.registry = newToolRegistry()
	}

	// 清理缓存
	m.cacheMu.Lock()
	m.callCache = make(map[string]interface{})
	m.cacheExpiry = time.Time{}
	m.cacheMu.Unlock()

	return nil
}

// IsMCPTool reports whether the tool comes from any MCP client.
func (m *Manager) IsMCPTool(name string) bool {
	if name == "" {
		return false
	}
	if m.registry != nil {
		if _, ok := m.registry.get(name); ok {
			return true
		}
	}
	return false
}

// printAllAvailableMCPFunctions prints all currently available MCP functions
func (m *Manager) printAllAvailableMCPFunctions() {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()
	clients := maps.Clone(m.clients)

	// Group tools by client
	toolsByClient := make(map[string][]string)
	for clientName, client := range clients {
		if client == nil {
			continue
		}
		tools := client.GetAvailableTools()
		for _, tool := range tools {
			toolsByClient[clientName] = append(toolsByClient[clientName], tool.Function.Name)
		}
	}

	if len(toolsByClient) == 0 {
		m.logger.InfoTag("MCP", "当前没有可用的MCP函数")
		return
	}

	m.logger.InfoTag("MCP", "当前所有可用的MCP函数:")
	for clientName, toolNames := range toolsByClient {
		toolsStr := strings.Join(toolNames, "、")
		m.logger.InfoTag("MCP", "[%s] %s", clientName, toolsStr)
	}
}

func (m *Manager) refreshToolRegistry() {
	// Tool registry is maintained automatically when clients are registered
}

// generateCacheKey 生成工具调用的缓存键
func (m *Manager) generateCacheKey(name string, args map[string]any) string {
	// 创建一个简单的基于参数的哈希键
	key := name
	// 对参数进行排序以确保一致的缓存键
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}

	for _, k := range keys {
		v := args[k]
		key += fmt.Sprintf(":%s=%v", k, v)
	}
	return key
}

// HandleXiaoZhiMCPMessage delegates message handling to the XiaoZhi client.
func (m *Manager) HandleXiaoZhiMCPMessage(msg map[string]interface{}) error {
	if m.xiaozhiClient == nil {
		return errors.New("XiaoZhi MCP client not configured")
	}
	return m.xiaozhiClient.HandleMCPMessage(msg)
}

// NewFromConfig creates a new domain MCP Manager from configuration.
func NewFromConfig(cfg *config.Config, logger Logger) (*Manager, error) {
	// Create a new manager using the standard constructor
	return NewManager(Options{
		Logger: logger,
		Config: cfg,
	})
}
