package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"

	go_openai "github.com/sashabaranov/go-openai"
)

// Conn 是与连接相关的接口，用于发送消息
type Conn interface {
	WriteMessage(messageType int, data []byte) error
}

// Manager MCP服务管理器
type Manager struct {
	logger                *utils.Logger
	conn                  Conn
	funcHandler           types.FunctionRegistryInterface
	configPath            string
	clients               map[string]MCPClient
	localClient           *LocalClient // 本地MCP客户端
	tools                 []string
	XiaoZhiMCPClient      *XiaoZhiMCPClient // XiaoZhiMCPClient用于处理小智MCP相关逻辑
	bRegisteredXiaoZhiMCP bool              // 是否已注册小智MCP工具
	isInitialized         bool              // 添加初始化状态标记
	systemCfg             *configs.Config
	mu                    sync.RWMutex
}

// NewManagerForPool 创建用于资源池的MCP管理器
func NewManagerForPool(lg *utils.Logger, cfg *configs.Config) *Manager {
	lg.Info("创建MCP Manager用于资源池")
	projectDir := utils.GetProjectDir()
	configPath := filepath.Join(projectDir, ".mcp_server_settings.json")

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
		lg.Error("预初始化MCP服务器失败: %v", err)
	}

	return mgr
}

// preInitializeServers 预初始化不依赖连接的MCP服务器
func (m *Manager) preInitializeServers() error {
	m.localClient, _ = NewLocalClient(m.logger, m.systemCfg)
	m.localClient.Start(context.Background())
	m.clients["local"] = m.localClient

	config := m.LoadConfig()
	if config == nil {
		return fmt.Errorf("no valid MCP server configuration found")
	}

	for name, srvConfig := range config {
		// 只初始化不需要连接的外部MCP服务器
		srvConfigMap, ok := srvConfig.(map[string]interface{})

		if !ok {
			m.logger.Warn("Invalid configuration format for server %s", name)
			continue
		}

		// 创建并启动外部MCP客户端
		clientConfig, err := convertConfig(srvConfigMap)
		if err != nil {
			m.logger.Error("Failed to convert config for server %s: %v", name, err)
			continue
		}

		client, err := NewClient(clientConfig, m.logger)
		if err != nil {
			m.logger.Error("Failed to create MCP client for server %s: %v", name, err)
			continue
		}

		if err := client.Start(context.Background()); err != nil {
			m.logger.Error("Failed to start MCP client %s: %v", name, err)
			continue
		}
		m.clients[name] = client
	}

	m.isInitialized = true
	return nil
}

// BindConnection 绑定连接到MCP Manager
func (m *Manager) BindConnection(
	conn Conn,
	fh types.FunctionRegistryInterface,
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

	// 优化：检查XiaoZhiMCPClient是否需要重新启动
	if m.XiaoZhiMCPClient == nil {
		m.XiaoZhiMCPClient = NewXiaoZhiMCPClient(m.logger, conn, sessionID)
		m.clients["xiaozhi"] = m.XiaoZhiMCPClient
		m.XiaoZhiMCPClient.SetVisionURL(visionURL)
		m.XiaoZhiMCPClient.SetID(deviceID, clientID)
		m.XiaoZhiMCPClient.SetToken(token)

		if err := m.XiaoZhiMCPClient.Start(context.Background()); err != nil {
			return fmt.Errorf("启动XiaoZhi MCP客户端失败: %v", err)
		}
	} else {
		// 重新绑定连接而不是重新创建
		m.XiaoZhiMCPClient.SetConnection(conn)
		m.XiaoZhiMCPClient.SetID(deviceID, clientID)
		m.XiaoZhiMCPClient.SetToken(token)
		if !m.XiaoZhiMCPClient.IsReady() {
			if err := m.XiaoZhiMCPClient.Start(context.Background()); err != nil {
				return fmt.Errorf("重启XiaoZhi MCP客户端失败: %v", err)
			}
		}
	}

	// 重新注册工具（只注册尚未注册的）
	m.registerAllToolsIfNeeded()
	return nil
}

// 新增方法：只在需要时注册工具
func (m *Manager) registerAllToolsIfNeeded() {
	if m.funcHandler == nil {
		return
	}

	// 检查是否已注册，避免重复注册
	if !m.bRegisteredXiaoZhiMCP && m.XiaoZhiMCPClient != nil && m.XiaoZhiMCPClient.IsReady() {
		tools := m.XiaoZhiMCPClient.GetAvailableTools()
		for _, tool := range tools {
			toolName := tool.Function.Name
			m.funcHandler.RegisterFunction(toolName, tool)
		}
		m.bRegisteredXiaoZhiMCP = true
	}

	// 注册其他外部MCP客户端工具
	for name, client := range m.clients {
		if name != "xiaozhi" && client.IsReady() {
			tools := client.GetAvailableTools()
			for _, tool := range tools {
				toolName := tool.Function.Name
				if !m.isToolRegistered(toolName) {
					m.funcHandler.RegisterFunction(toolName, tool)
					m.tools = append(m.tools, toolName)
					// m.logger.Info("Registered external MCP tool: [%s] %s", toolName, tool.Function.Description)
				}
			}
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

	// 对xiaozhi客户端进行连接重置而不是完全销毁
	if m.XiaoZhiMCPClient != nil {
		m.XiaoZhiMCPClient.ResetConnection() // 新增方法
	}

	// 对外部MCP客户端进行连接重置
	for name, client := range m.clients {
		if name != "xiaozhi" {
			if resetter, ok := client.(interface{ ResetConnection() error }); ok {
				resetter.ResetConnection()
			}
		}
	}

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
		m.logger.Error(fmt.Sprintf("Error loading MCP config from %s: %v", m.configPath, err))
		return nil
	}

	var config struct {
		MCPServers map[string]interface{} `json:"mcpServers"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		m.logger.Error(fmt.Sprintf("Error parsing MCP config: %v", err))
		return nil
	}

	return config.MCPServers
}

func (m *Manager) HandleXiaoZhiMCPMessage(msgMap map[string]interface{}) error {
	// 处理小智MCP消息
	if m.XiaoZhiMCPClient == nil {
		return fmt.Errorf("XiaoZhiMCPClient is not initialized")
	}
	m.XiaoZhiMCPClient.HandleMCPMessage(msgMap)
	if m.XiaoZhiMCPClient.IsReady() && !m.bRegisteredXiaoZhiMCP {
		// 注册小智MCP工具
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
				m.logger.Error(fmt.Sprintf("注册工具失败: %s, 错误: %v", toolName, err))
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
	m.logger.Info(fmt.Sprintf("Executing tool %s with arguments: %v", toolName, arguments))

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, client := range m.clients {
		if client.HasTool(toolName) {
			return client.CallTool(ctx, toolName, arguments)
		}
	}

	return nil, fmt.Errorf("Tool %s not found in any MCP server", toolName)
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
				m.logger.Info(fmt.Sprintf("MCP client closed: %s", name))
			case <-ctx.Done():
				m.logger.Error(fmt.Sprintf("Timeout closing MCP client %s", name))
			}
		}()

		m.mu.Lock()
		delete(m.clients, name)
		m.mu.Unlock()
	}
}
