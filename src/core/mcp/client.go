package mcp

import (
	"context"
	"fmt"
	"strings"
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
	Command       string   `yaml:"command,omitempty"`   // 命令行连接方式
	Args          []string `yaml:"args,omitempty"`      // 命令行参数
	Env           []string `yaml:"env,omitempty"`       // 环境变量
	URL           string   `yaml:"url,omitempty"`       // HTTP端点
	Transport     string   `yaml:"transport,omitempty"` // 支持 stdio/http
}

// Client 封装MCP客户端功能
const (
	transportTypeStdio = "stdio"
	transportTypeHTTP  = "http"
)

// Client 封装MCP客户端功能
type Client struct {
	client        *mcpclient.Client
	config        *Config
	name          string
	tools         []Tool
	ready         bool
	mu            sync.RWMutex
	transportType string
	logger        *utils.Logger
}

// NewClient 创建一个新的MCP客户端实例
func NewClient(config *Config, logger *utils.Logger) (*Client, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("MCP client is disabled in config")
	}

	client := &Client{
		config:        config,
		tools:         make([]Tool, 0),
		ready:         false,
		logger:        logger,
		transportType: config.resolveTransportType(),
	}

	switch client.transportType {
	case transportTypeStdio:
		if config.Command == "" {
			return nil, fmt.Errorf("stdio transport requires command")
		}
		stdioClient, err := mcpclient.NewStdioMCPClient(
			config.Command,
			config.Env,
			config.Args...,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdio MCP client: %w", err)
		}
		client.client = stdioClient
	case transportTypeHTTP:
		if config.URL == "" {
			return nil, fmt.Errorf("http transport requires url")
		}
		httpClient, err := mcpclient.NewStreamableHttpClient(config.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP MCP client: %w", err)
		}
		client.client = httpClient
	default:
		return nil, fmt.Errorf("unsupported transport type: %s", client.transportType)
	}

	return client, nil
}

func (c *Config) resolveTransportType() string {
	transport := strings.TrimSpace(strings.ToLower(c.Transport))
	if transport != "" {
		return transport
	}
	if c.URL != "" {
		return transportTypeHTTP
	}
	return transportTypeStdio
}

// Start 启动MCP客户端并监听资源更新
func (c *Client) Start(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("MCP transport not configured")
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "zhi-server",
		Version: "1.0.0",
	}

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	initResult, err := c.client.Initialize(initCtx, initRequest)
	if err != nil {
		return fmt.Errorf("failed to initialize %s MCP client: %w", c.transportType, err)
	}

	if initResult.ServerInfo.Name != "" || initResult.ServerInfo.Version != "" {
		c.name = initResult.ServerInfo.Name
		c.logger.Debug("[MCP] 已初始化服务器: %s %s，通过 %s 传输",
			initResult.ServerInfo.Name,
			initResult.ServerInfo.Version,
			c.transportType)
	} else {
		c.logger.Debug("[MCP] 已通过 %s 传输初始化MCP服务器", c.transportType)
	}

	if err := c.fetchTools(ctx); err != nil {
		return fmt.Errorf("failed to fetch tools: %w", err)
	}

	c.mu.Lock()
	c.ready = true
	c.mu.Unlock()

	return nil
}

// fetchTools 获取可用的工具列表
func (c *Client) fetchTools(ctx context.Context) error {
	toolsRequest := mcp.ListToolsRequest{}
	tools, err := c.client.ListTools(ctx, toolsRequest)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 清空当前工具列表
	c.tools = make([]Tool, 0, len(tools.Tools))

	var toolNames strings.Builder
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
		if toolNames.Len() > 0 {
			toolNames.WriteString(", ")
		}
		toolNames.WriteString(tool.Name)
	}
	c.logger.Debug("[MCP] 获取 %s 可用工具 %s", c.name, toolNames.String())
	return nil
}

// Stop 停止MCP客户端
func (c *Client) Stop() {
	if c.client != nil {
		c.logger.Info("[MCP] 正在停止 %s MCP客户端", c.transportType)
		if err := c.client.Close(); err != nil {
			c.logger.Error("Failed to close MCP client: %v", err)
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
	if len(name) > 4 && name[:4] == "mcp_" {
		name = name[4:]
	}
	if !c.HasTool(name) {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	callRequest := mcp.CallToolRequest{}
	callRequest.Params.Name = name
	callRequest.Params.Arguments = args

	result, err := c.client.CallTool(ctx, callRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to call tool %s: %w", name, err)
	}

	if result == nil || len(result.Content) == 0 {
		return nil, nil
	}

	if len(result.Content) == 1 {
		if textContent, ok := result.Content[0].(mcp.TextContent); ok {
			return textContent.Text, nil
		}
		return types.ActionResponse{
			Action: types.ActionTypeReqLLM,
			Result: result.Content[0],
		}, nil
	}

	processedContent := make([]interface{}, 0, len(result.Content))
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			processedContent = append(processedContent, textContent.Text)
		} else {
			processedContent = append(processedContent, content)
		}
	}
	return types.ActionResponse{
		Action: types.ActionTypeReqLLM,
		Result: processedContent,
	}, nil
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
	c.ready = false
	return nil
}
