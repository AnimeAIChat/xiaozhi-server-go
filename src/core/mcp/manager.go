package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	tools                 []string
	XiaoZhiMCPClient      *XiaoZhiMCPClient // XiaoZhiMCPClient用于处理小智MCP相关逻辑
	bRegisteredXiaoZhiMCP bool              // 是否已注册小智MCP工具
	mu                    sync.RWMutex
}

// NewManager 创建一个新的MCP管理器
func NewManager(lg *utils.Logger, fh types.FunctionRegistryInterface, conn Conn) *Manager {

	projectDir := getProjectDir()
	configPath := filepath.Join(projectDir, ".mcp_server_settings.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = ""
	}

	mgr := &Manager{
		logger:                lg,
		funcHandler:           fh,
		conn:                  conn,
		configPath:            configPath,
		clients:               make(map[string]MCPClient),
		tools:                 make([]string, 0),
		bRegisteredXiaoZhiMCP: false,
	}
	// 初始化小智MCP客户端
	mgr.XiaoZhiMCPClient = NewXiaoZhiMCPClient(lg, conn)
	mgr.clients["xiaozhi"] = mgr.XiaoZhiMCPClient

	return mgr
}

// getProjectDir 获取项目根目录
func getProjectDir() string {
	// 实际实现中应该根据实际项目结构确定
	// 这里简单返回当前工作目录
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
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

// InitializeServers 初始化所有MCP服务
func (m *Manager) InitializeServers(ctx context.Context) error {
	config := m.LoadConfig()
	if config == nil {
		return fmt.Errorf("no valid MCP server configuration found")
	}

	for name, srvConfig := range config {
		srvConfigMap, ok := srvConfig.(map[string]interface{})
		if !ok {
			m.logger.Warn(fmt.Sprintf("Invalid configuration format for server %s", name))
			continue
		}

		if _, hasCmd := srvConfigMap["command"]; !hasCmd {
			if _, hasURL := srvConfigMap["url"]; !hasURL {
				m.logger.Warn(fmt.Sprintf("Skipping server %s: neither command nor url specified", name))
				continue
			}
		}

		// 转换配置格式为Config结构
		clientConfig, err := convertConfig(srvConfigMap)
		if err != nil {
			m.logger.Error(fmt.Sprintf("Failed to convert config for server %s: %v", name, err))
			continue
		}

		// 创建客户端
		client, err := NewClient(clientConfig)
		if err != nil {
			m.logger.Error(fmt.Sprintf("Failed to create MCP client for server %s: %v", name, err))
			continue
		}

		// 启动客户端
		if err := client.Start(ctx); err != nil {
			m.logger.Error(fmt.Sprintf("Failed to start MCP client %s: %v", name, err))
			continue
		}

		// 注册客户端
		m.mu.Lock()
		m.clients[name] = client
		m.mu.Unlock()

		m.logger.Info(fmt.Sprintf("Initialized MCP client: %s", name))

		// 获取并注册工具
		clientTools := client.GetAvailableTools()
		m.registerTools(clientTools)
	}

	m.XiaoZhiMCPClient.Start(ctx)

	return nil
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
		m.tools = append(m.tools, toolName)
		if m.funcHandler != nil {
			m.funcHandler.RegisterFunction(toolName, tool)
			log.Printf("Registered tool: [%s] %s", toolName, tool.Function.Description)
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
func (m *Manager) ExecuteTool(ctx context.Context, toolName string, arguments map[string]interface{}) (interface{}, error) {
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
