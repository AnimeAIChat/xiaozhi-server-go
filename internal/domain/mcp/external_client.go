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
		callRequest := mcp.CallToolRequest{}
		callRequest.Params.Name = name
		callRequest.Params.Arguments = args

		result, err := c.stdioClient.CallTool(ctx, callRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to call tool %s: %w", name, err)
		}

		// Process return result
		if result == nil || len(result.Content) == 0 {
			return nil, nil
		}

		// Return first content item, or entire content list
		if len(result.Content) == 1 {
			// If text content, return text directly
			if textContent, ok := result.Content[0].(mcp.TextContent); ok {
				return textContent.Text, nil
			}
			ret := llm.ActionResponse{
				Action: llm.ActionTypeReqLLM,
				Result: result.Content[0],
			}
			return ret, nil
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
		return ret, nil
	}

	// Original network client does not support direct tool calling
	return nil, fmt.Errorf("tool calling not implemented for network client")
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