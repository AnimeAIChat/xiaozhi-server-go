package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"

	"xiaozhi-server-go/internal/domain/llm"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/mcp"
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

	autoReturn bool
	isInitialized bool

	// Legacy manager for backward compatibility
	legacyManager *mcp.Manager
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

	// Pre-initialize external servers asynchronously
	if err := manager.preInitializeServers(); err != nil {
		if err.Error() == "no valid MCP server configuration found" {
			opts.Logger.Warn("没有找到有效的MCP服务器配置，跳过预初始化，如需使用外部MCP功能，请提供配置文件")
		} else {
			opts.Logger.Error("预初始化MCP服务器失败: %v", err)
		}
	}

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
	// If we have a legacy manager, delegate to it
	if m.legacyManager != nil {
		m.logger.Debug("Delegating ExecuteTool to legacy MCP manager: %s (legacyManager: %v, type: %T)", name, m.legacyManager, m.legacyManager)
		return m.legacyManager.ExecuteTool(ctx, name, args)
	}

	if name == "" {
		return nil, errors.New("tool name cannot be empty")
	}

	m.logger.Info("Executing tool %s with arguments: %v", name, args)

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
	// If we have a legacy manager, delegate to it
	if m.legacyManager != nil {
		m.logger.Info("Delegating BindConnection to legacy MCP manager (legacyManager: %v, type: %T, pointer: %p)", m.legacyManager, m.legacyManager, m.legacyManager)
		return m.legacyManager.BindConnection(conn, fh, params)
	}

	m.logger.Info("No legacy manager, using domain manager logic")

	paramsMap := params.(map[string]interface{})
	sessionID := paramsMap["session_id"].(string)
	visionURL := paramsMap["vision_url"].(string)

	m.logger.Debug("绑定连接到MCP Manager, sessionID: %s, visionURL: %s", sessionID, visionURL)

	// 异步处理MCP初始化和绑定，不阻塞连接建立
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("MCP绑定连接时发生panic: %v", r)
			}
		}()

		m.logger.Info("BindConnection: Starting async binding process")

		// 如果MCP管理器还未初始化完成，等待一下但不阻塞
		if !m.isInitialized {
			m.logger.Info("BindConnection, MCP Manager未初始化，等待异步初始化完成")
			// 最多等待10秒，避免阻塞连接建立
			timeout := time.After(10 * time.Second)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for !m.isInitialized {
				select {
				case <-timeout:
					m.logger.Warn("等待MCP异步初始化超时，继续处理连接（MCP功能可能受限）")
					goto continueBinding
				case <-ticker.C:
					// 继续等待
				}
			}
		}

	continueBinding:
		m.logger.Info("BindConnection: Proceeding with client binding")

		// 绑定 XiaoZhi 客户端
		if m.xiaozhiClient != nil {
			m.logger.Info("BindConnection: XiaoZhi client exists, attempting to bind")
			// Get the underlying WebSocket connection
			wsConn := conn.GetWebSocketConn()
			m.logger.Info("BindConnection: GetWebSocketConn returned: %v (type: %T)", wsConn, wsConn)
			if wsConn == nil {
				m.logger.Warn("BindConnection: connection does not provide WebSocket access, skipping XiaoZhi client binding")
			} else {
				m.logger.Info("BindConnection: successfully obtained WebSocket connection, proceeding with XiaoZhi client binding")
				if err := m.xiaozhiClient.BindConnection(wsConn); err != nil {
					m.logger.Error("绑定XiaoZhi MCP客户端失败: %v", err)
					return
				}

				// 等待客户端准备就绪
				if err := m.xiaozhiClient.WaitForReady(context.Background()); err != nil {
					m.logger.Error("等待XiaoZhi MCP客户端准备就绪失败: %v", err)
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
			}
		} else {
			m.logger.Warn("BindConnection: XiaoZhi client is nil, skipping binding")
		}

		// 注册其他外部MCP客户端工具
		m.registerExternalTools(fh)
	}()

	return nil
}

// registerExternalTools registers external MCP client tools
func (m *Manager) registerExternalTools(fh llm.FunctionRegistryInterface) {
	m.clientsMu.RLock()
	clients := maps.Clone(m.clients)
	m.clientsMu.RUnlock()

	for name, client := range clients {
		if name == "xiaozhi" || name == "local" || !client.IsReady() {
			continue
		}

		tools := client.GetAvailableTools()
		for _, tool := range tools {
			toolName := tool.Function.Name
			if err := fh.RegisterFunction(toolName, tool); err != nil {
				m.logger.Error("注册外部MCP工具失败: %s, 错误: %v", toolName, err)
				continue
			}
			m.logger.Info("Registered external MCP tool: [%s] %s", toolName, tool.Function.Description)
		}

		// Also register tools in the internal registry for IsMCPTool to work
		if err := m.registry.register(tools); err != nil {
			m.logger.Error("注册外部MCP工具到内部注册表失败: %v", err)
		}
	}
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
	// If we have a legacy manager, delegate to it
	if m.legacyManager != nil {
		m.logger.Debug("Delegating Reset to legacy MCP manager (legacyManager: %v, type: %T)", m.legacyManager, m.legacyManager)
		return m.legacyManager.Reset()
	}

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

	return nil
}

// IsMCPTool reports whether the tool comes from any MCP client.
func (m *Manager) IsMCPTool(name string) bool {
	// If we have a legacy manager, delegate to it
	if m.legacyManager != nil {
		m.logger.Debug("Delegating IsMCPTool to legacy MCP manager (legacyManager: %v, type: %T)", m.legacyManager, m.legacyManager)
		return m.legacyManager.IsMCPTool(name)
	}

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

func (m *Manager) refreshToolRegistry() {
	// Tool registry is maintained automatically when clients are registered
}

// HandleXiaoZhiMCPMessage delegates message handling to the XiaoZhi client.
func (m *Manager) HandleXiaoZhiMCPMessage(msg map[string]interface{}) error {
	// If we have a legacy manager, delegate to it
	if m.legacyManager != nil {
		m.logger.Debug("Delegating HandleXiaoZhiMCPMessage to legacy MCP manager (legacyManager: %v, type: %T)", m.legacyManager, m.legacyManager)
		return m.legacyManager.HandleXiaoZhiMCPMessage(msg)
	}

	if m.xiaozhiClient == nil {
		return errors.New("XiaoZhi MCP client not configured")
	}
	return m.xiaozhiClient.HandleMCPMessage(msg)
}

// preInitializeServers pre-initializes MCP servers that don't require connections
func (m *Manager) preInitializeServers() error {
	// Start local client
	if err := m.localClient.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start local client: %w", err)
	}
	m.clients["local"] = m.localClient

	// Check if there's a configuration file
	configs, err := m.configLoader.LoadConfig()
	if err != nil {
		return err
	}

	if configs == nil {
		m.logger.Debug("No MCP server configuration file found")
		m.isInitialized = true
		return nil
	}

	// Asynchronously initialize external MCP servers
	go m.initializeExternalServers(configs)

	return nil
}

// initializeExternalServers asynchronously initializes external MCP servers
func (m *Manager) initializeExternalServers(configs map[string]*Config) {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error("Panic during external server initialization: %v", r)
		}
	}()

	m.logger.Info("Starting asynchronous initialization of external MCP servers")

	for name, config := range configs {
		// Only initialize external MCP servers
		go func(name string, config *Config) {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("Panic initializing MCP client %s: %v", name, r)
				}
			}()

			client, err := NewExternalClient(config, m.logger)
			if err != nil {
				m.logger.Error("Failed to create MCP client for server %s: %v", name, err)
				return
			}

			if !config.Enabled {
				m.logger.Debug("MCP client %s is disabled", name)
				return
			}

			// Add timeout control for each client startup
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			startDone := make(chan error, 1)
			go func() {
				startDone <- client.Start(ctx)
			}()

			select {
			case err := <-startDone:
				if err != nil {
					m.logger.Error("Failed to start MCP client %s: %v", name, err)
					return
				}

				m.clientsMu.Lock()
				m.clients[name] = client
				m.clientsMu.Unlock()

				m.logger.Info("External MCP client %s initialized successfully", name)
			case <-ctx.Done():
				m.logger.Warn("MCP client %s startup timeout", name)
			}
		}(name, config)
	}

	// Mark as initialized (don't wait for all clients to complete)
	m.clientsMu.Lock()
	m.isInitialized = true
	m.clientsMu.Unlock()

	m.logger.Info("External MCP server asynchronous initialization started (non-blocking)")
}

// NewFromManager creates a new domain MCP Manager from an existing legacy MCP Manager.
// This function provides backward compatibility during the migration process.
func NewFromManager(legacyManager interface{}, logger Logger) (*Manager, error) {
	logger.Debug("NewFromManager called with legacyManager: %v, type: %T, pointer: %p", legacyManager, legacyManager, legacyManager)

	// For migration compatibility, check if it's a legacy manager
	if legacy, ok := legacyManager.(*mcp.Manager); ok && legacy != nil {
		logger.Debug("Successfully cast to legacy MCP manager, wrapping it - legacy: %v, type: %T", legacy, legacy)
		// Create a wrapper that delegates to the legacy manager
		return &Manager{
			logger:       logger,
			registry:     newToolRegistry(),
			clients:      make(map[string]Client),
			configLoader: NewConfigLoader(logger),
			autoReturn:   false, // Legacy managers don't auto-return
			isInitialized: true, // Assume legacy manager is initialized
			// Store the legacy manager for delegation
			legacyManager: legacy,
		}, nil
	}

	logger.Info("Failed to cast legacyManager, using default config - legacyManager: %v, type: %T", legacyManager, legacyManager)
	// Fallback to default config if not a legacy manager
	cfg := config.DefaultConfig()
	return NewManager(Options{
		Logger: logger,
		Config: cfg,
	})
}

// NewFromConfig creates a new domain MCP Manager from configuration.
// This function provides backward compatibility during the migration process.
func NewFromConfig(cfg *config.Config, logger Logger) (*Manager, error) {
	// Create a new manager using the standard constructor
	return NewManager(Options{
		Logger: logger,
		Config: cfg,
	})
}
