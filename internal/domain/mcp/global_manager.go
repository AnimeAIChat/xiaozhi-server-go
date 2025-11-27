package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
	"xiaozhi-server-go/internal/platform/config"
)

// GlobalMCPManager 全局MCP管理器单例
type GlobalMCPManager struct {
	logger Logger
	config *config.Config

	clientsMu sync.RWMutex
	clients  map[string]Client
	tools    []Tool

	once     sync.Once
	initOnce sync.Once
	ready    bool
}

var (
	globalManager *GlobalMCPManager
	managerOnce   sync.Once
)

// GetGlobalMCPManager 获取全局MCP管理器单例
func GetGlobalMCPManager() *GlobalMCPManager {
	managerOnce.Do(func() {
		globalManager = &GlobalMCPManager{
			clients: make(map[string]Client),
			tools:   make([]Tool, 0),
		}
	})
	return globalManager
}

// Initialize 初始化全局MCP管理器（只执行一次）
func (gm *GlobalMCPManager) Initialize(cfg *config.Config, logger Logger) error {
	var initErr error

	gm.initOnce.Do(func() {
		gm.logger = logger
		gm.config = cfg

		logger.Info("开始初始化全局MCP管理器...")

		// 1. 创建本地客户端
		localClient, err := NewLocalClient(logger, cfg)
		if err != nil {
			initErr = fmt.Errorf("创建本地MCP客户端失败: %w", err)
			return
		}

		if err := localClient.Start(context.Background()); err != nil {
			initErr = fmt.Errorf("启动本地MCP客户端失败: %w", err)
			return
		}

		gm.clientsMu.Lock()
		gm.clients["local"] = localClient
		gm.clientsMu.Unlock()

		// 2. 异步初始化外部客户端
		go gm.initializeExternalClients()

		gm.ready = true
		logger.Info("全局MCP管理器初始化完成")
	})

	return initErr
}

// IsReady 检查管理器是否已初始化
func (gm *GlobalMCPManager) IsReady() bool {
	return gm.ready
}

// GetClient 获取指定名称的MCP客户端
func (gm *GlobalMCPManager) GetClient(name string) (Client, bool) {
	gm.clientsMu.RLock()
	defer gm.clientsMu.RUnlock()

	client, exists := gm.clients[name]
	return client, exists
}

// GetAllClients 获取所有MCP客户端
func (gm *GlobalMCPManager) GetAllClients() map[string]Client {
	gm.clientsMu.RLock()
	defer gm.clientsMu.RUnlock()

	// 返回副本以避免并发问题
	clients := make(map[string]Client, len(gm.clients))
	for name, client := range gm.clients {
		clients[name] = client
	}
	return clients
}

// GetAvailableTools 获取所有可用工具
func (gm *GlobalMCPManager) GetAvailableTools() []openai.Tool {
	gm.clientsMu.RLock()
	defer gm.clientsMu.RUnlock()

	tools := make([]openai.Tool, 0)
	for _, client := range gm.clients {
		if client.IsReady() {
			clientTools := client.GetAvailableTools()
			tools = append(tools, clientTools...)
		}
	}
	return tools
}

// ExecuteTool 执行指定的工具
func (gm *GlobalMCPManager) ExecuteTool(ctx context.Context, name string, args map[string]any) (any, error) {
	if !gm.ready {
		return nil, fmt.Errorf("MCP管理器未初始化")
	}

	gm.clientsMu.RLock()
	defer gm.clientsMu.RUnlock()

	for clientName, client := range gm.clients {
		if client.HasTool(name) {
			gm.logger.Debug("执行工具 %s，来自客户端 %s", name, clientName)
			return client.CallTool(ctx, name, args)
		}
	}

	return nil, fmt.Errorf("工具 %s 未找到", name)
}

// Close 关闭所有MCP客户端
func (gm *GlobalMCPManager) Close(ctx context.Context) error {
	gm.clientsMu.Lock()
	defer gm.clientsMu.Unlock()

	for _, client := range gm.clients {
		if client != nil {
			client.Stop()
		}
	}

	gm.clients = make(map[string]Client)
	gm.ready = false
	return nil
}

// initializeExternalClients 异步初始化外部客户端
func (gm *GlobalMCPManager) initializeExternalClients() {
	defer func() {
		if r := recover(); r != nil {
			gm.logger.Error("外部客户端初始化时发生panic: %v", r)
		}
	}()

	gm.logger.Info("开始异步初始化外部MCP客户端...")

	// 加载配置
	configLoader := NewConfigLoader(gm.logger)
	configs, err := configLoader.LoadConfig()
	if err != nil {
		gm.logger.Warn("加载MCP配置失败: %v", err)
		return
	}

	if configs == nil {
		gm.logger.Info("没有找到MCP服务器配置")
		return
	}

	// 并行初始化所有外部客户端
	var wg sync.WaitGroup
	for name, config := range configs {
		wg.Add(1)
		go func(name string, config *Config) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					gm.logger.Error("初始化外部客户端 %s 时发生panic: %v", name, r)
				}
			}()

			if !config.Enabled {
				gm.logger.Debug("外部MCP客户端 %s 已禁用", name)
				return
			}

			gm.logger.Info("初始化外部MCP客户端: %s", name)

			// 创建客户端
			client, err := NewExternalClient(config, gm.logger)
			if err != nil {
				gm.logger.Error("创建外部MCP客户端失败 (服务器: %s): %v", name, err)
				return
			}

			// 启动客户端
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := client.Start(ctx); err != nil {
				gm.logger.Error("启动外部MCP客户端失败 %s: %v", name, err)
				return
			}

			// 注册到管理器
			gm.clientsMu.Lock()
			gm.clients[name] = client
			gm.clientsMu.Unlock()

			gm.logger.Info("外部MCP客户端 %s 初始化成功", name)
		}(name, config)
	}

	wg.Wait()
	gm.logger.Info("所有外部MCP客户端初始化完成")

	// 打印所有可用工具
	gm.printAvailableTools()
}

// printAvailableTools 打印所有可用工具
func (gm *GlobalMCPManager) printAvailableTools() {
	toolsByClient := make(map[string][]string)

	gm.clientsMu.RLock()
	for name, client := range gm.clients {
		if client.IsReady() {
			tools := client.GetAvailableTools()
			for _, tool := range tools {
				toolsByClient[name] = append(toolsByClient[name], tool.Function.Name)
			}
		}
	}
	gm.clientsMu.RUnlock()

	if len(toolsByClient) == 0 {
		gm.logger.Info("没有可用的MCP工具")
		return
	}

	gm.logger.Info("当前所有可用的MCP工具:")
	for clientName, toolNames := range toolsByClient {
		toolsStr := ""
		for i, name := range toolNames {
			if i > 0 {
				toolsStr += "、"
			}
			toolsStr += name
		}
		gm.logger.Info("[%s] %s", clientName, toolsStr)
	}
}