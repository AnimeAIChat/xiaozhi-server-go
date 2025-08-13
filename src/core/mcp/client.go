package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"
	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sashabaranov/go-openai"
)

// Config 定义MCP客户端配置
type Config struct {
	Enabled       bool     `yaml:"enabled"`
	ServerAddress string   `yaml:"server_address"`
	ServerPort    int      `yaml:"server_port"`
	Namespace     string   `yaml:"namespace"`
	NodeID        string   `yaml:"node_id"`
	ResourceTypes []string `yaml:"resource_types"`
	Command       string   `yaml:"command,omitempty"` // 命令行连接方式
	Args          []string `yaml:"args,omitempty"`    // 命令行参数
	Env           []string `yaml:"env,omitempty"`     // 环境变量
	URL           string   `yaml:"url,omitempty"`     // SSE连接URL
}

// Client 封装MCP客户端功能
type Client struct {
	client         *mcpclient.Client
	stdioClient    *mcpclient.Client
	config         *Config
	name           string
	tools          []Tool
	ready          bool
	mu             sync.RWMutex
	useStdioClient bool
	logger         *utils.Logger
}

// NewClient 创建一个新的MCP客户端实例
func NewClient(config *Config, logger *utils.Logger) (*Client, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("MCP client is disabled in config")
	}

	c := &Client{
		config: config,
		tools:  make([]Tool, 0),
		ready:  false,
		logger: logger,
	}

	// 根据配置选择适当的客户端类型
	if config.Command != "" {
		// 使用命令行方式连接
		stdioClient, err := mcpclient.NewStdioMCPClient(
			config.Command,
			config.Env,
			config.Args...,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdio MCP client: %w", err)
		}
		c.stdioClient = stdioClient
		c.useStdioClient = true
	} else {
		fmt.Println("Unsupported MCP client type, only stdio client is supported")
	}

	return c, nil
}

// Start 启动MCP客户端并监听资源更新
func (c *Client) Start(ctx context.Context) error {
	if c.useStdioClient {
		// c.logger.Info("Starting MCP stdio client with command: %s", c.config.Command)

		// 创建初始化请求
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "zhi-server",
			Version: "1.0.0",
		}

		// 设置超时上下文
		initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// 初始化客户端
		initResult, err := c.stdioClient.Initialize(initCtx, initRequest)
		if err != nil {
			return fmt.Errorf("failed to initialize stdio MCP client: %w", err)
		}
		c.name = initResult.ServerInfo.Name
		c.logger.Info("Initialized server: %s %s with conmmand: %s",
			initResult.ServerInfo.Name,
			initResult.ServerInfo.Version,
			c.config.Command)

		// 获取工具列表
		err = c.fetchTools(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch tools: %w", err)
		}
	}

	c.mu.Lock()
	c.ready = true
	c.mu.Unlock()

	return nil
}

// fetchTools 获取可用的工具列表
func (c *Client) fetchTools(ctx context.Context) error {
	if c.useStdioClient {
		// 使用协议方式获取工具列表
		toolsRequest := mcp.ListToolsRequest{}
		tools, err := c.stdioClient.ListTools(ctx, toolsRequest)
		if err != nil {
			return fmt.Errorf("failed to list tools: %w", err)
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		// 清空当前工具列表
		c.tools = make([]Tool, 0, len(tools.Tools))

		// 添加获取到的工具
		toolNames := ""
		for _, tool := range tools.Tools {
			required := tool.InputSchema.Required
			if required == nil {
				required = make([]string, 0)
			}
			c.tools = append(c.tools, Tool{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: ToolInputSchema{
					Type:       tool.InputSchema.Type,
					Properties: tool.InputSchema.Properties,
					Required:   required,
				},
			})
			toolNames += fmt.Sprintf("%s, ", tool.Name)
			// log.Printf("Added tool: %s - %s %v; %v; %v", tool.Name, tool.Description, tool.InputSchema, tool.RawInputSchema, tool.Annotations)
		}
		c.logger.Info("Fetching %s available tools %s", c.name, toolNames)
		return nil
	} else {
		// 原有方式的实现保持不变
		// 这里可以通过资源类型获取工具信息
		return nil
	}
}

// Stop 停止MCP客户端
func (c *Client) Stop() {
	if c.useStdioClient {
		if c.stdioClient != nil {
			c.logger.Info("Stopping MCP stdio client")
			c.stdioClient.Close()
		}
	} else {
		if c.client != nil {
			c.logger.Info("Stopping MCP client")
			c.client.Close()
		}
	}

	c.mu.Lock()
	c.ready = false
	c.mu.Unlock()
}

// HasTool 检查是否有指定名称的工具
func (c *Client) HasTool(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// 如果有mcp_前缀，则去掉前缀
	if len(name) > 4 && name[:4] == "mcp_" {
		name = name[4:]
	}

	for _, tool := range c.tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

// GetAvailableTools 获取所有可用工具
func (c *Client) GetAvailableTools() []openai.Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]openai.Tool, 0, len(c.tools))
	for _, tool := range c.tools {
		openaiTool := openai.Tool{
			Type: "function",
			Function: &openai.FunctionDefinition{
				Name:        fmt.Sprintf("mcp_%s", tool.Name),
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

// CallTool 调用指定的工具
func (c *Client) CallTool(
	ctx context.Context,
	name string,
	args map[string]any,
) (interface{}, error) {
	// 如果有mcp_前缀，则去掉前缀
	if len(name) > 4 && name[:4] == "mcp_" {
		name = name[4:]
	}
	// 检查工具是否存在
	if !c.HasTool(name) {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	if c.useStdioClient {
		callRequest := mcp.CallToolRequest{}
		callRequest.Params.Name = name
		callRequest.Params.Arguments = args

		result, err := c.stdioClient.CallTool(ctx, callRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to call tool %s: %w", name, err)
		}

		// 处理返回结果
		if result == nil || len(result.Content) == 0 {
			return nil, nil
		}

		// 返回第一个内容项，或整个内容列表
		if len(result.Content) == 1 {
			// 如果是文本内容，直接返回文本
			if textContent, ok := result.Content[0].(mcp.TextContent); ok {
				return textContent.Text, nil
			}
			ret := types.ActionResponse{
				Action: types.ActionTypeReqLLM,
				Result: result.Content[0],
			}
			return ret, nil
		}

		// 处理多个内容项的情况
		processedContent := make([]interface{}, 0, len(result.Content))
		for _, content := range result.Content {
			if textContent, ok := content.(mcp.TextContent); ok {
				processedContent = append(processedContent, textContent.Text)
			} else {
				processedContent = append(processedContent, content)
			}
		}
		ret := types.ActionResponse{
			Action: types.ActionTypeReqLLM,
			Result: processedContent,
		}
		return ret, nil
	}

	// 原始网络客户端不支持直接调用工具
	return nil, fmt.Errorf("tool calling not implemented for network client")
}

// IsReady 检查客户端是否已初始化完成并准备就绪
func (c *Client) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// ResetConnection 重置连接状态
func (c *Client) ResetConnection() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 保留工具信息，只重置连接状态
	c.ready = false

	// 如果有活跃连接，优雅关闭
	if c.useStdioClient && c.stdioClient != nil {
		// 不完全关闭，只标记为未就绪
		// 在下次Start时会重新建立连接
	}

	return nil
}
