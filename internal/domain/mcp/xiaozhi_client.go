package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xiaozhi-server-go/internal/domain/auth"
	"xiaozhi-server-go/internal/domain/llm"
	"xiaozhi-server-go/internal/platform/config"

	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

type XiaoZhiMCPClient struct {
	conn     *websocket.Conn
	tools    []Tool
	mu       sync.RWMutex
	ctx      context.Context
	logger   Logger
	cfg      *config.Config
	auth     *auth.Manager
	isReady  bool
	readyCh  chan struct{}
}

func NewXiaoZhiMCPClient(logger Logger, cfg *config.Config, auth *auth.Manager) (*XiaoZhiMCPClient, error) {
	c := &XiaoZhiMCPClient{
		tools:   make([]Tool, 0),
		mu:      sync.RWMutex{},
		logger:  logger,
		cfg:     cfg,
		auth:    auth,
		isReady: false,
		readyCh: make(chan struct{}),
	}
	return c, nil
}

// Start starts the XiaoZhi MCP client
func (c *XiaoZhiMCPClient) Start(ctx context.Context) error {
	c.ctx = ctx
	c.logger.Debug("XiaoZhi MCP client started")
	return nil
}

// Stop stops the XiaoZhi MCP client
func (c *XiaoZhiMCPClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.isReady = false
}

// HasTool checks if the XiaoZhi client has the specified tool
func (c *XiaoZhiMCPClient) HasTool(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, tool := range c.tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

// GetAvailableTools gets all available tools for the XiaoZhi client
func (c *XiaoZhiMCPClient) GetAvailableTools() []openai.Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]openai.Tool, 0, len(c.tools))
	for _, tool := range c.tools {
		openaiTool := openai.Tool{
			Type: "function",
			Function: &openai.FunctionDefinition{
				Name:        tool.Name,
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

// CallTool calls the specified tool on the XiaoZhi client
func (c *XiaoZhiMCPClient) CallTool(
	ctx context.Context,
	name string,
	args map[string]interface{},
) (interface{}, error) {
	// Check if tool exists
	if !c.HasTool(name) {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	// Check if connection is ready
	if !c.IsReady() {
		return nil, fmt.Errorf("XiaoZhi MCP client is not ready")
	}

	// Create tool call message
	toolCall := map[string]interface{}{
		"type": "tool_call",
		"tool": map[string]interface{}{
			"name": name,
			"args": args,
		},
		"id": fmt.Sprintf("call_%d", time.Now().UnixNano()),
	}

	// Send tool call message
	if err := c.conn.WriteJSON(toolCall); err != nil {
		return nil, fmt.Errorf("failed to send tool call: %w", err)
	}

	// Wait for response (simplified - in real implementation, you'd need proper message handling)
	// For now, return a placeholder response
	return llm.ActionResponse{
		Action: llm.ActionTypeReqLLM,
		Result: "Tool call sent to XiaoZhi device",
	}, nil
}

// IsReady checks if the XiaoZhi client is ready
func (c *XiaoZhiMCPClient) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isReady && c.conn != nil
}

// ResetConnection resets the XiaoZhi client's connection state
func (c *XiaoZhiMCPClient) ResetConnection() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.isReady = false
	c.readyCh = make(chan struct{})

	return nil
}

// BindConnection binds the XiaoZhi client to a WebSocket connection
func (c *XiaoZhiMCPClient) BindConnection(conn *websocket.Conn) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
	}

	c.conn = conn
	c.isReady = false

	// Send initialize message
	if err := c.SendMCPInitializeMessage(); err != nil {
		return fmt.Errorf("failed to send initialize message: %w", err)
	}

	// Start message handling goroutine
	go c.handleMessages()

	return nil
}

// SendMCPInitializeMessage sends the MCP initialize message
func (c *XiaoZhiMCPClient) SendMCPInitializeMessage() error {
	if c.conn == nil {
		return fmt.Errorf("connection is nil")
	}

	// Generate a simple token for now (in production, this should be properly signed)
	token := fmt.Sprintf("xiaozhi-token-%d", time.Now().Unix())

	initMsg := map[string]interface{}{
		"type": "initialize",
		"protocol_version": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": true,
			},
		},
		"client_info": map[string]interface{}{
			"name":    "xiaozhi-server",
			"version": "1.0.0",
		},
		"auth_token": token,
	}

	if err := c.conn.WriteJSON(initMsg); err != nil {
		return fmt.Errorf("failed to send initialize message: %w", err)
	}

	c.logger.Debug("Sent MCP initialize message")
	return nil
}

// handleMessages handles incoming WebSocket messages
func (c *XiaoZhiMCPClient) handleMessages() {
	defer func() {
		if c.conn != nil {
			c.conn.Close()
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			if c.conn == nil {
				return
			}

			var msg map[string]interface{}
			err := c.conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					c.logger.Error("WebSocket error: %v", err)
				}
				return
			}

			if err := c.HandleMCPMessage(msg); err != nil {
				c.logger.Error("Failed to handle MCP message: %v", err)
			}
		}
	}
}

// HandleMCPMessage handles incoming MCP messages
func (c *XiaoZhiMCPClient) HandleMCPMessage(msg map[string]interface{}) error {
	msgType, ok := msg["type"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid message type")
	}

	switch msgType {
	case "initialize_response":
		return c.handleInitializeResponse(msg)
	case "tools_list":
		return c.handleToolsList(msg)
	case "tool_response":
		return c.handleToolResponse(msg)
	case "error":
		return c.handleError(msg)
	default:
		c.logger.Warn("Unknown message type: %s", msgType)
	}

	return nil
}

func (c *XiaoZhiMCPClient) handleInitializeResponse(msg map[string]interface{}) error {
	c.mu.Lock()
	c.isReady = true
	c.mu.Unlock()

	close(c.readyCh)
	c.logger.Info("XiaoZhi MCP client initialized successfully")
	return nil
}

func (c *XiaoZhiMCPClient) handleToolsList(msg map[string]interface{}) error {
	toolsData, ok := msg["tools"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid tools list format")
	}

	var tools []Tool
	for _, toolData := range toolsData {
		toolMap, ok := toolData.(map[string]interface{})
		if !ok {
			continue
		}

		tool := Tool{
			Name:        toolMap["name"].(string),
			Description: toolMap["description"].(string),
			InputSchema: ToolInputSchema{
				Type: toolMap["inputSchema"].(map[string]interface{})["type"].(string),
			},
		}

		// Parse properties and required fields if present
		if props, ok := toolMap["inputSchema"].(map[string]interface{})["properties"].(map[string]interface{}); ok {
			tool.InputSchema.Properties = props
		}
		if req, ok := toolMap["inputSchema"].(map[string]interface{})["required"].([]interface{}); ok {
			required := make([]string, len(req))
			for i, r := range req {
				required[i] = r.(string)
			}
			tool.InputSchema.Required = required
		}

		tools = append(tools, tool)
	}

	c.mu.Lock()
	c.tools = tools
	c.mu.Unlock()

	c.logger.Info("Updated tools list with %d tools", len(tools))
	return nil
}

func (c *XiaoZhiMCPClient) handleToolResponse(msg map[string]interface{}) error {
	// Handle tool response - this would typically be processed by the connection handler
	// For now, just log it
	c.logger.Debug("Received tool response: %v", msg)
	return nil
}

func (c *XiaoZhiMCPClient) handleError(msg map[string]interface{}) error {
	errorMsg, ok := msg["error"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid error message format")
	}

	code, _ := errorMsg["code"].(float64)
	message, _ := errorMsg["message"].(string)

	c.logger.Error("MCP error received - code: %.0f, message: %s", code, message)
	return nil
}

// WaitForReady waits for the client to be ready
func (c *XiaoZhiMCPClient) WaitForReady(ctx context.Context) error {
	select {
	case <-c.readyCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for XiaoZhi MCP client to be ready")
	}
}