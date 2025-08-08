package mcp

import (
	"context"
	"fmt"
	"sync"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/utils"

	"github.com/sashabaranov/go-openai"
)

type HandlerFunc func(ctx context.Context, args map[string]interface{}) (interface{}, error)

type LocalClient struct {
	tools   []Tool
	mu      sync.RWMutex
	ctx     context.Context
	logger  *utils.Logger
	handler map[string]HandlerFunc
	cfg     *configs.Config
}

func NewLocalClient(logger *utils.Logger, cfg *configs.Config) (*LocalClient, error) {
	c := &LocalClient{
		tools:   make([]Tool, 0),
		handler: make(map[string]HandlerFunc),
		mu:      sync.RWMutex{},
		logger:  logger,
		cfg:     cfg,
	}
	return c, nil
}

func (c *LocalClient) RegisterTools() {
	if c.cfg == nil {
		c.logger.Error("RegisterTools: config is nil")
		return
	}

	if c.cfg.LocalMCPFun == nil {
		c.logger.Warn("RegisterTools: LocalMCPFun is nil")
		return
	}

	funcs := c.cfg.LocalMCPFun
	if len(funcs) == 0 {
		c.logger.Info("RegisterTools: LocalMCPFun is empty")
		return
	}

	for _, funcName := range funcs {
		if funcName == "exit" {
			c.AddToolExit()
			c.logger.Info("RegisterTools: exit tool registered")
		} else if funcName == "time" {
			c.AddToolTime()
			c.logger.Info("RegisterTools: time tool registered")
		} else if funcName == "change_voice" {
			c.AddToolChangeVoice()
			c.logger.Info("RegisterTools: change_voice tool registered")
		} else if funcName == "change_role" {
			c.AddToolChangeRole()
			c.logger.Info("RegisterTools: change_role tool registered")
		} else if funcName == "play_music" {
			c.AddToolPlayMusic()
			c.logger.Info("RegisterTools: play_music tool registered")
		} else {
			c.logger.Warn("RegisterTools: unknown function name %s", funcName)
		}
	}
}

// Start 启动本地MCP客户端
func (c *LocalClient) Start(ctx context.Context) error {
	c.ctx = ctx
	c.RegisterTools()
	c.logger.Info("Local MCP client started")
	return nil
}

// Stop 停止本地MCP客户端
func (c *LocalClient) Stop() {
	// 本地客户端不需要停止任何服务，直接返回
}

// HasTool 检查本地客户端是否有指定名称的工具
func (c *LocalClient) HasTool(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// 如果有local_前缀，则去掉前缀
	if len(name) > 6 && name[:6] == "local_" {
		name = name[6:]
	}
	for _, tool := range c.tools {
		if tool.Name == name {
			return true
		}
	}

	return false
}

// GetAvailableTools 获取本地客户端的所有可用工具
func (c *LocalClient) GetAvailableTools() []openai.Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]openai.Tool, 0, len(c.tools))
	for _, tool := range c.tools {
		openaiTool := openai.Tool{
			Type: "function",
			Function: &openai.FunctionDefinition{
				Name:        fmt.Sprintf("local_%s", tool.Name),
				Description: tool.Description,
				Parameters: map[string]interface{}{
					"type":       tool.InputSchema.Type,
					"properties": tool.InputSchema.Properties,
					"required":   tool.InputSchema.Required,
				},
			},
		}
		result = append(result, openaiTool)
	}
	return result
}

// CallTool 调用本地客户端的指定工具
func (c *LocalClient) CallTool(
	ctx context.Context,
	name string,
	args map[string]interface{},
) (interface{}, error) {
	// 检查工具是否存在
	if !c.HasTool(name) {
		return nil, fmt.Errorf("tool %s not found", name)
	}
	// 如果有local_前缀，则去掉前缀
	if len(name) > 6 && name[:6] == "local_" {
		name = name[6:]
	}
	var handler HandlerFunc
	c.mu.RLock()
	if h, ok := c.handler[name]; ok {
		handler = h
		c.mu.RUnlock()
	} else {
		c.mu.RUnlock()
		return nil, fmt.Errorf("handler for tool %s not found", name)
	}

	return handler(ctx, args)
}

// IsReady 检查本地客户端是否已准备就绪
func (c *LocalClient) IsReady() bool {
	// 本地客户端始终就绪
	return true
}

// ResetConnection 重置本地客户端的连接状态
func (c *LocalClient) ResetConnection() error {
	// 本地客户端没有连接状态，直接返回nil
	return nil
}

func (c *LocalClient) AddTool(
	name string,
	description string,
	input ToolInputSchema,
	handler HandlerFunc,
) error {
	if c.HasTool(name) {
		return fmt.Errorf("tool %s already exists", name)
	}

	tool := Tool{
		Name:        name,
		Description: description,
		InputSchema: input,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.tools = append(c.tools, tool)
	c.handler[name] = handler
	return nil
}
