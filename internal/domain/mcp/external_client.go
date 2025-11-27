package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xiaozhi-server-go/internal/domain/llm"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sashabaranov/go-openai"
)

// ExternalClient encapsulates external MCP client functionality
type ExternalClient struct {
	client         *mcpclient.Client
	stdioClient    *mcpclient.Client
	config         *Config
	name           string
	tools          []Tool
	ready          bool
	mu             sync.RWMutex
	useStdioClient bool
	logger         Logger
}

// NewExternalClient creates a new external MCP client instance
func NewExternalClient(config *Config, logger Logger) (*ExternalClient, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("MCP client is disabled in config")
	}

	c := &ExternalClient{
		config: config,
		tools:  make([]Tool, 0),
		ready:  false,
		logger: logger,
	}

	// Select appropriate client type based on configuration
	if config.Command != "" {
		// Use command line connection
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

// Start starts the external MCP client and listens for resource updates
func (c *ExternalClient) Start(ctx context.Context) error {
	if c.useStdioClient {
		// Create initialization request
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "zhi-server",
			Version: "1.0.0",
		}

		// Set timeout context
		initCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		// Initialize client
		initResult, err := c.stdioClient.Initialize(initCtx, initRequest)
		if err != nil {
			return fmt.Errorf("failed to initialize stdio MCP client: %w", err)
		}
		c.name = initResult.ServerInfo.Name
		c.logger.InfoTag("MCP", "已初始化服务器: %s %s (命令: %s)",
			initResult.ServerInfo.Name,
			initResult.ServerInfo.Version,
			c.config.Command)

		// Fetch tools list
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

// fetchTools fetches the available tools list
func (c *ExternalClient) fetchTools(ctx context.Context) error {
	if c.useStdioClient {
		// Use protocol to get tools list
		toolsRequest := mcp.ListToolsRequest{}
		tools, err := c.stdioClient.ListTools(ctx, toolsRequest)
		if err != nil {
			return fmt.Errorf("failed to list tools: %w", err)
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		// Clear current tools list
		c.tools = make([]Tool, 0, len(tools.Tools))

		// Add fetched tools
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
		}
		c.logger.InfoTag("MCP", "获取 %s 可用工具: %s", c.name, toolNames)
		return nil
	} else {
		// Original implementation remains unchanged
		// Tools can be obtained through resource types here
		return nil
	}
}

// Stop stops the external MCP client
func (c *ExternalClient) Stop() {
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

// HasTool checks if the specified tool exists
func (c *ExternalClient) HasTool(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// Remove mcp_ prefix if present
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

// GetAvailableTools gets all available tools
func (c *ExternalClient) GetAvailableTools() []openai.Tool {
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

// CallTool calls the specified tool
func (c *ExternalClient) CallTool(
	ctx context.Context,
	name string,
	args map[string]any,
) (interface{}, error) {
	// Remove mcp_ prefix if present
	if len(name) > 4 && name[:4] == "mcp_" {
		name = name[4:]
	}
	// Check if tool exists
	if !c.HasTool(name) {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	if c.useStdioClient {
		c.logger.InfoTag("MCP", "开始调用外部工具: %s, 参数: %+v", name, args)

		// 确保参数类型正确，Python mcp-rag 期望特定格式的参数
		processedArgs := c.processArguments(args)
		c.logger.DebugTag("MCP", "处理后的参数: %+v", processedArgs)

		callRequest := mcp.CallToolRequest{}
		callRequest.Params.Name = name
		callRequest.Params.Arguments = processedArgs

		// 设置较短的超时时间，确保低延迟
		timeoutCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()

		c.logger.DebugTag("MCP", "发送 MCP 调用请求 (超时: 8s)...")
		result, err := c.stdioClient.CallTool(timeoutCtx, callRequest)

		if err != nil {
			c.logger.ErrorTag("MCP", "调用外部工具失败: %s, 错误: %v", name, err)
			return nil, fmt.Errorf("failed to call tool %s: %w", name, err)
		}

		c.logger.InfoTag("MCP", "外部工具调用成功: %s, 内容数量: %d", name, len(result.Content))
		return c.processToolResult(result), nil
	}

	// Original network client does not support direct tool calling
	return nil, fmt.Errorf("tool calling not implemented for network client")
}

// processArguments 确保参数格式与 Python mcp-rag 兼容
func (c *ExternalClient) processArguments(args map[string]any) map[string]any {
	processed := make(map[string]any)

	for key, value := range args {
		switch v := value.(type) {
		case string:
			// 字符串类型保持不变
			processed[key] = v
		case float64:
			// 数字类型需要根据 Python mcp-rag 的期望进行转换
			if key == "limit" || key == "threshold" {
				processed[key] = v
			} else {
				// 其他数字可能需要转换为整数（如果它们是整数）
				if v == float64(int(v)) {
					processed[key] = int(v)
				} else {
					processed[key] = v
				}
			}
		case int:
			processed[key] = v
		case bool:
			processed[key] = v
		default:
			// 其他类型转换为字符串表示
			processed[key] = fmt.Sprintf("%v", v)
		}
	}

	// 确保 mcp-rag 期望的参数都有默认值
	if _, exists := processed["mode"]; !exists {
		processed["mode"] = "raw"
	}
	if _, exists := processed["collection"]; !exists {
		processed["collection"] = "default"
	}
	if _, exists := processed["limit"]; !exists {
		processed["limit"] = 5
	}
	if _, exists := processed["threshold"]; !exists {
		processed["threshold"] = 0.7
	}

	return processed
}

// processToolResult 处理工具调用结果
func (c *ExternalClient) processToolResult(result *mcp.CallToolResult) interface{} {
	// Process return result
	if result == nil || len(result.Content) == 0 {
		return nil
	}

	// Return first content item, or entire content list
	if len(result.Content) == 1 {
		// If text content, return text directly
		if textContent, ok := result.Content[0].(mcp.TextContent); ok {
			return textContent.Text
		}
		ret := llm.ActionResponse{
			Action: llm.ActionTypeReqLLM,
			Result: result.Content[0],
		}
		return ret
	}

	// Handle multiple content items
	processedContent := make([]interface{}, 0, len(result.Content))
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			processedContent = append(processedContent, textContent.Text)
		} else {
			processedContent = append(processedContent, content)
		}
	}
	ret := llm.ActionResponse{
		Action: llm.ActionTypeReqLLM,
		Result: processedContent,
	}
	return ret
}

// IsReady checks if the client is initialized and ready
func (c *ExternalClient) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// ResetConnection resets the connection state
func (c *ExternalClient) ResetConnection() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Keep tool information, only reset connection state
	c.ready = false

	// If there is an active connection, close gracefully
	if c.useStdioClient && c.stdioClient != nil {
		// Do not completely close, just mark as not ready
		// Connection will be re-established on next Start
	}

	return nil
}