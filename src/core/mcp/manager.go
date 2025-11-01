package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/domain/llm"
	"xiaozhi-server-go/src/core/utils"

	go_openai "github.com/sashabaranov/go-openai"
)

// Conn 是与连接相关的接口，用于发送消息
type Conn interface {
	WriteMessage(messageType int, data []byte) error
}

// Manager MCP客户端管理器
type Manager struct {
	logger                *utils.Logger
	funcHandler           llm.FunctionRegistryInterface
	conn                  Conn
	configPath            string
	clients               map[string]MCPClient
	localClient           *LocalClient // 本地MCP客户端
	tools                 []string
	XiaoZhiMCPClient      *XiaoZhiMCPClient // XiaoZhiMCPClient用于处理小智MCP相关逻辑
	bRegisteredXiaoZhiMCP bool              // 是否已注册小智MCP工具
	isInitialized         bool              // 添加初始化状态标记
	systemCfg             *config.Config
	mu                    sync.RWMutex

	// 缓存外部工具列表，避免每次连接重新注册
	cachedExternalTools []go_openai.Tool
	toolsCacheValid     bool

	AutoReturnToPool bool // 是否自动归还到资源池
}

// NewManagerForPool 创建用于资源池的MCP管理器
func NewManagerForPool(lg *utils.Logger, cfg *config.Config) *Manager {
	lg.Debug("创建MCP Manager用于资源池")
	projectDir := utils.GetProjectDir()
	configPath := filepath.Join(projectDir, "data", ".mcp_server_settings.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = ""
	}

	mgr := &Manager{
		logger:                lg,
		funcHandler:           nil, // 将在绑定连接时设置
		conn:                  nil, // 将在绑定连接时设置
		configPath:            configPath,
		clients:               make(map[string]MCPClient),
		tools:                 make([]string, 0),
		bRegisteredXiaoZhiMCP: false,
		systemCfg:             cfg,
	}
	// 预先初始化非连接相关的MCP服务器
	if err := mgr.preInitializeServers(); err != nil {
		if err.Error() == "no valid MCP server configuration found" {
			lg.Warn("没有找到有效的MCP服务器配置，跳过预初始化，如需使用外部MCP功能，请提供配置文件")
		} else {
			lg.Error("预初始化MCP服务器失败: %v", err)
		}
	}

	// 预创建XiaoZhiMCPClient（不绑定连接）
	mgr.XiaoZhiMCPClient = NewXiaoZhiMCPClientWithoutConn(lg)

	return mgr
}

// preInitializeServers 预初始化不依赖连接的MCP服务器
func (m *Manager) preInitializeServers() error {
	// 异步初始化外部MCP服务器
	go m.initializeExternalServers()

	// 同步初始化本地客户端（因为本地客户端启动很快）
	m.localClient, _ = NewLocalClient(m.logger, m.systemCfg)
	m.localClient.Start(context.Background())
	m.clients["local"] = m.localClient

	// 预先初始化XiaoZhiMCPClient（不绑定连接）
	m.XiaoZhiMCPClient = NewXiaoZhiMCPClientWithoutConn(m.logger)
	// 预先启动XiaoZhiMCPClient，让它在后台准备就绪
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("预初始化XiaoZhiMCPClient时发生panic: %v", r)
			}
		}()
		// 创建一个虚拟连接用于初始化
		mockConn := &mockConnection{}
		xiaoZhiClient := NewXiaoZhiMCPClient(m.logger, mockConn, "preinit-session")
		xiaoZhiClient.SetVisionURL("") // 预初始化时不需要vision URL
		xiaoZhiClient.SetID("preinit", "preinit")
		xiaoZhiClient.SetToken("preinit")

		// 尝试启动，如果失败则记录但不影响主流程
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := xiaoZhiClient.Start(ctx); err != nil {
			m.logger.Warn("预初始化XiaoZhiMCPClient失败，将在连接时重新初始化: %v", err)
			return
		}

		// 如果启动成功，替换预创建的客户端
		m.mu.Lock()
		m.XiaoZhiMCPClient = xiaoZhiClient
		m.mu.Unlock()
		m.logger.Info("XiaoZhiMCPClient预初始化成功")
	}()

	// 检查是否有配置文件，如果没有则直接标记为已初始化
	config := m.LoadConfig()
	if config == nil {
		m.logger.Debug("未找到MCP服务器配置文件，跳过外部MCP服务器初始化")
		m.isInitialized = true
		return nil
	}

	// 如果有配置文件，异步初始化会处理
	return nil
}

// initializeExternalServers 异步初始化外部MCP服务器
func (m *Manager) initializeExternalServers() {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error("异步初始化MCP服务器时发生panic: %v", r)
		}
	}()

	config := m.LoadConfig()
	if config == nil {
		m.logger.Debug("异步初始化：未找到MCP服务器配置文件")
		m.isInitialized = true
		return
	}

	m.logger.Info("开始异步初始化外部MCP服务器")

	for name, srvConfig := range config {
		// 只初始化不需要连接的外部MCP服务器
		srvConfigMap, ok := srvConfig.(map[string]interface{})

		if !ok {
			m.logger.Warn("异步初始化：Invalid configuration format for server %s", name)
			continue
		}

		// 创建并启动外部MCP客户端
		clientConfig, err := convertConfig(srvConfigMap)
		if err != nil {
			m.logger.Error("异步初始化：Failed to convert config for server %s: %v", name, err)
			continue
		}

		if !clientConfig.Enabled {
			m.logger.Debug("异步初始化：MCP client %s is disabled", name)
			continue
		}

		// 为每个客户端启动添加超时控制
		go func(name string, clientConfig *Config) {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("初始化MCP客户端 %s 时发生panic: %v", name, r)
				}
			}()

			client, err := NewClient(clientConfig, m.logger)
			if err != nil {
				m.logger.Error("异步初始化：Failed to create MCP client for server %s: %v", name, err)
				return
			}

			// 添加超时控制，避免单个客户端阻塞整个初始化过程
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			startDone := make(chan error, 1)
			go func() {
				startDone <- client.Start(ctx)
			}()

			select {
			case err := <-startDone:
				if err != nil {
					m.logger.Error("异步初始化：Failed to start MCP client %s: %v", name, err)
					return
				}

				m.mu.Lock()
				m.clients[name] = client
				m.mu.Unlock()

				m.logger.Info("异步初始化：外部MCP客户端 %s 初始化完成", name)
			case <-ctx.Done():
				m.logger.Warn("异步初始化：MCP客户端 %s 启动超时", name)
				// 尝试停止客户端（如果客户端有Stop方法）
				if client != nil {
					// 客户端可能有Stop方法，尝试调用
					defer func() {
						if r := recover(); r != nil {
							// 忽略停止时的错误
						}
					}()
					// 这里可以添加停止逻辑，如果客户端支持的话
				}
			}
		}(name, clientConfig)
	}

	// 不等待所有客户端初始化完成，立即标记为已初始化
	// 这样可以避免阻塞服务启动
	m.mu.Lock()
	m.isInitialized = true
	m.mu.Unlock()

	m.logger.Info("外部MCP服务器异步初始化已启动（不阻塞服务启动）")
}

func (m *Manager) GetAllToolsNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, len(m.tools))
	copy(names, m.tools)
	return names
}

// BindConnection 绑定连接到MCP Manager
func (m *Manager) BindConnection(
	conn Conn,
	fh llm.FunctionRegistryInterface,
	params interface{},
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.conn = conn
	m.funcHandler = fh
	paramsMap := params.(map[string]interface{})
	sessionID := paramsMap["session_id"].(string)
	visionURL := paramsMap["vision_url"].(string)
	deviceID := paramsMap["device_id"].(string)
	clientID := paramsMap["client_id"].(string)
	token := paramsMap["token"].(string)
	m.logger.Debug("绑定连接到MCP Manager, sessionID: %s, visionURL: %s", sessionID, visionURL)

	// 异步处理MCP初始化和绑定，不阻塞连接建立
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("MCP绑定连接时发生panic: %v", r)
			}
		}()

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

		// 优化：检查XiaoZhiMCPClient是否需要重新启动
		if m.XiaoZhiMCPClient == nil {
			m.XiaoZhiMCPClient = NewXiaoZhiMCPClient(m.logger, conn, sessionID)
			m.XiaoZhiMCPClient.SetVisionURL(visionURL)
			m.XiaoZhiMCPClient.SetID(deviceID, clientID)
			m.XiaoZhiMCPClient.SetToken(token)

			if err := m.XiaoZhiMCPClient.Start(context.Background()); err != nil {
				m.logger.Error("启动XiaoZhi MCP客户端失败: %v", err)
				return
			}
		} else {
			// 重新绑定连接而不是重新创建
			m.XiaoZhiMCPClient.SetConnection(conn)
			m.XiaoZhiMCPClient.SetSessionID(sessionID)
			m.XiaoZhiMCPClient.SetVisionURL(visionURL)
			m.XiaoZhiMCPClient.SetID(deviceID, clientID)
			m.XiaoZhiMCPClient.SetToken(token)
			if !m.XiaoZhiMCPClient.IsReady() {
				m.logger.DebugTag("MCP", "XiaoZhi MCP客户端未完全初始化，将重新启动")
				if err := m.XiaoZhiMCPClient.Start(context.Background()); err != nil {
					m.logger.Error("重启XiaoZhi MCP客户端失败: %v", err)
					return
				}
			}
		}
		m.clients["xiaozhi"] = m.XiaoZhiMCPClient

		// 重新注册工具（只注册尚未注册的）
		m.registerAllToolsIfNeeded()
	}()

	return nil
}

// 新增方法：只在需要时注册工具
func (m *Manager) registerAllToolsIfNeeded() {
	if m.funcHandler == nil {
		return
	}

	// 检查是否已注册，避免重复注册
	if m.XiaoZhiMCPClient != nil && m.XiaoZhiMCPClient.IsReady() {
		tools := m.XiaoZhiMCPClient.GetAvailableTools()
		for _, tool := range tools {
			toolName := tool.Function.Name
			m.funcHandler.RegisterFunction(toolName, tool)
		}
		m.bRegisteredXiaoZhiMCP = true
	}

	// 注册其他外部MCP客户端工具（使用缓存）
	m.registerExternalToolsIfNeeded()
}

// 注册外部工具（带缓存）
func (m *Manager) registerExternalToolsIfNeeded() {
	// 如果缓存有效，直接使用缓存的工具
	if m.toolsCacheValid && len(m.cachedExternalTools) > 0 {
		for _, tool := range m.cachedExternalTools {
			toolName := tool.Function.Name
			if !m.isToolRegistered(toolName) {
				m.funcHandler.RegisterFunction(toolName, tool)
				m.tools = append(m.tools, toolName)
				m.logger.Info("Registered cached external MCP tool: [%s] %s", toolName, tool.Function.Description)
			}
		}
		return
	}

	// 重新收集外部工具并缓存
	var allExternalTools []go_openai.Tool
	for name, client := range m.clients {
		if name != "xiaozhi" && client.IsReady() {
			tools := client.GetAvailableTools()
			allExternalTools = append(allExternalTools, tools...)
		}
	}

	// 更新缓存
	m.cachedExternalTools = allExternalTools
	m.toolsCacheValid = true

	// 注册工具
	for _, tool := range allExternalTools {
		toolName := tool.Function.Name
		if !m.isToolRegistered(toolName) {
			m.funcHandler.RegisterFunction(toolName, tool)
			m.tools = append(m.tools, toolName)
			m.logger.Info("Registered external MCP tool: [%s] %s", toolName, tool.Function.Description)
		}
	}
}

// 新增辅助方法
func (m *Manager) isToolRegistered(toolName string) bool {
	for _, tool := range m.tools {
		if tool == toolName {
			return true
		}
	}
	return false
}

// 改进Reset方法
func (m *Manager) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 重置连接相关状态但保留可复用的客户端结构
	m.conn = nil
	m.funcHandler = nil
	m.bRegisteredXiaoZhiMCP = false
	m.tools = make([]string, 0)

	// 清除工具缓存（连接断开时需要重新注册）
	m.cachedExternalTools = nil
	m.toolsCacheValid = false

	// 对xiaozhi客户端进行连接重置而不是完全销毁
	if m.XiaoZhiMCPClient != nil {
		m.XiaoZhiMCPClient.ResetConnection() // 新增方法
	}

	// 对外部MCP客户端进行连接重置
	/*
		for name, client := range m.clients {
			if name != "xiaozhi" {
				if resetter, ok := client.(interface{ ResetConnection() error }); ok {
					resetter.ResetConnection()
				}
			}
		}
	*/

	return nil
}

// Cleanup 实现Provider接口的Cleanup方法
func (m *Manager) Cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m.CleanupAll(ctx)
	return m.Reset()
}

// LoadConfig 加载MCP服务配置
func (m *Manager) LoadConfig() map[string]interface{} {
	if m.configPath == "" {
		return nil
	}

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		m.logger.Error("Error loading MCP config from %s: %v", m.configPath, err)
		return nil
	}

	var config struct {
		MCPServers map[string]interface{} `json:"mcpServers"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		m.logger.Error("Error parsing MCP config: %v", err)
		return nil
	}

	return config.MCPServers
}

func (m *Manager) HandleXiaoZhiMCPMessage(msgMap map[string]interface{}) error {
	// 处理小智MCP消息
	if m.XiaoZhiMCPClient == nil {
		return fmt.Errorf("XiaoZhiMCPClient is not initialized")
	}
	err := m.XiaoZhiMCPClient.HandleMCPMessage(msgMap)
	if err != nil {
		m.logger.Error("处理小智MCP消息失败: %v", err)
		return err
	}
	if m.XiaoZhiMCPClient.IsReady() && !m.bRegisteredXiaoZhiMCP {
		// 注册小智MCP工具, 小智MCP工具交互时间长，之前可能未注册
		m.registerTools(m.XiaoZhiMCPClient.GetAvailableTools())
		m.bRegisteredXiaoZhiMCP = true
	}
	return nil
}

// convertConfig 将map配置转换为Config结构
func convertConfig(cfg map[string]interface{}) (*Config, error) {
	// 实现从map到Config结构的转换
	config := &Config{
		Enabled: true, // 默认启用
	}

	// 服务器地址
	if addr, ok := cfg["server_address"].(string); ok {
		config.ServerAddress = addr
	}

	// 服务器端口
	if port, ok := cfg["server_port"].(float64); ok {
		config.ServerPort = int(port)
	}

	// 命名空间
	if ns, ok := cfg["namespace"].(string); ok {
		config.Namespace = ns
	}

	// 节点ID
	if nodeID, ok := cfg["node_id"].(string); ok {
		config.NodeID = nodeID
	}

	// 命令行连接方式
	if cmd, ok := cfg["command"].(string); ok {
		config.Command = cmd
	}

	// 命令行参数
	if args, ok := cfg["args"].([]interface{}); ok {
		for _, arg := range args {
			if argStr, ok := arg.(string); ok {
				config.Args = append(config.Args, argStr)
			}
		}
	}

	// enabled参数
	if enabled, ok := cfg["enabled"].(bool); ok {
		config.Enabled = enabled
	}

	// 环境变量
	if env, ok := cfg["env"].(map[string]interface{}); ok {
		config.Env = make([]string, 0)
		for k, v := range env {
			if vStr, ok := v.(string); ok {
				config.Env = append(config.Env, fmt.Sprintf("%s=%s", k, vStr))
			}
		}
	}

	// SSE连接URL
	if url, ok := cfg["url"].(string); ok {
		config.URL = url
	}

	return config, nil
}

// registerTools 注册工具
func (m *Manager) registerTools(tools []go_openai.Tool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, tool := range tools {
		toolName := tool.Function.Name

		// 检查工具是否已注册
		if m.isToolRegistered(toolName) {
			continue // 跳过已注册的工具
		}

		m.tools = append(m.tools, toolName)
		if m.funcHandler != nil {
			if err := m.funcHandler.RegisterFunction(toolName, tool); err != nil {
				m.logger.Error("注册工具失败: %s, 错误: %v", toolName, err)
				continue
			}
			// m.logger.Info("Registered tool: [%s] %s", toolName, tool.Function.Description)
		}
	}
}

// IsMCPTool 检查是否是MCP工具
func (m *Manager) IsMCPTool(toolName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, tool := range m.tools {
		if tool == toolName {
			return true
		}
	}

	return false
}

// ExecuteTool 执行工具调用
func (m *Manager) ExecuteTool(
	ctx context.Context,
	toolName string,
	arguments map[string]interface{},
) (interface{}, error) {
	m.logger.Info("Executing tool %s with arguments: %v", toolName, arguments)

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, client := range m.clients {
		if client.HasTool(toolName) {
			return client.CallTool(ctx, toolName, arguments)
		}
	}

	clientNames := make([]string, 0, len(m.clients))
	for name := range m.clients {
		clientNames = append(clientNames, name)
	}

	return nil, fmt.Errorf("Tool %s not found in any MCP server， %v", toolName, clientNames)
}

// CleanupAll 依次关闭所有MCPClient
func (m *Manager) CleanupAll(ctx context.Context) {
	m.mu.Lock()
	clients := make(map[string]MCPClient, len(m.clients))
	for name, client := range m.clients {
		clients[name] = client
	}
	m.mu.Unlock()

	for name, client := range clients {
		func() {
			// 设置一个超时上下文
			ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()

			done := make(chan struct{})
			go func() {
				client.Stop()
				close(done)
			}()

			select {
			case <-done:
				m.logger.Info("MCP client closed: %s", name)
			case <-ctx.Done():
				m.logger.Error("Timeout closing MCP client %s", name)
			}
		}()

		m.mu.Lock()
		delete(m.clients, name)
		m.mu.Unlock()
	}
	m.isInitialized = false
}

// mockConnection 用于预初始化的虚拟连接
type mockConnection struct{}

func (m *mockConnection) WriteMessage(messageType int, data []byte) error {
	// 预初始化时不需要实际发送消息，只需要不报错即可
	return nil
}
