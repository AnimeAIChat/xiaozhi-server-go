package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"xiaozhi-server-go/internal/domain/auth"
	"xiaozhi-server-go/internal/domain/llm"
	"xiaozhi-server-go/internal/platform/config"

	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

// MCP消息ID常量
const (
	mcpInitializeID = 1 // 初始化消息ID
	mcpToolsListID  = 2 // 工具列表请求ID
	mcpToolCallID   = 3 // 工具调用请求ID

	msgTypeText = 1 // 文本消息类型
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

	// 用于处理工具调用的响应
	callResults     map[int]chan interface{}
	callResultsLock sync.Mutex
	nextID          int
	visionURL       string // 视觉服务URL
	deviceID        string // 设备ID，用于标识设备
	clientID        string // 客户端ID，用于标识客户端
	token           string // 访问令牌
	sessionID       string // 会话ID，用于标识连接
	// 工具名称映射：sanitized name -> original name
	toolNameMap map[string]string
}

func NewXiaoZhiMCPClient(logger Logger, cfg *config.Config, auth *auth.Manager) (*XiaoZhiMCPClient, error) {
	c := &XiaoZhiMCPClient{
		tools:       make([]Tool, 0),
		mu:          sync.RWMutex{},
		logger:      logger,
		cfg:         cfg,
		auth:        auth,
		isReady:     false,
		readyCh:     make(chan struct{}),
		callResults: make(map[int]chan interface{}),
		nextID:      1,
		toolNameMap: make(map[string]string),
	}
	return c, nil
}

// SetID sets the device and client IDs
func (c *XiaoZhiMCPClient) SetID(deviceID string, clientID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deviceID = deviceID
	c.clientID = clientID // 使用clientID作为会话ID
}

// SetToken sets the authentication token
func (c *XiaoZhiMCPClient) SetToken(token string) {
	auth := auth.NewAuthToken(token)
	visionToken, err := auth.GenerateToken(c.deviceID)
	if err != nil {
		c.logger.Error("生成Vision Token失败: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = visionToken
}

// SetVisionURL sets the vision service URL
func (c *XiaoZhiMCPClient) SetVisionURL(visionURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.visionURL = visionURL
}

// SetSessionID sets the session ID
func (c *XiaoZhiMCPClient) SetSessionID(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = sessionID
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

	// 清理资源
	c.isReady = false

	// 取消所有未完成的工具调用
	c.callResultsLock.Lock()
	defer c.callResultsLock.Unlock()

	for id, ch := range c.callResults {
		close(ch)
		delete(c.callResults, id)
	}
}

// HasTool checks if the XiaoZhi client has the specified tool (supports sanitized names)
func (c *XiaoZhiMCPClient) HasTool(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 首先检查原始名称
	for _, tool := range c.tools {
		if tool.Name == name {
			return true
		}
	}

	// 然后检查是否为sanitized名称
	if _, exists := c.toolNameMap[name]; exists {
		return true
	}

	return false
}

// sanitizeToolName sanitizes tool names by replacing dots with underscores
func sanitizeToolName(name string) string {
	return strings.ReplaceAll(name, ".", "_")
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
				Name:        sanitizeToolName(tool.Name),
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
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("调用工具 %s 时发生Panic: %v", name, r)
		}
	}()
	if !c.IsReady() {
		return nil, fmt.Errorf("MCP客户端尚未准备就绪")
	}

	// 获取原始的工具名称
	originalName := name
	if mappedName, exists := c.toolNameMap[name]; exists {
		originalName = mappedName
	} else if !c.HasTool(name) {
		return nil, fmt.Errorf("工具 %s 不存在", name)
	}

	// 获取下一个ID并创建结果通道
	c.callResultsLock.Lock()
	id := c.nextID
	c.nextID++
	resultCh := make(chan interface{}, 1)
	c.callResults[id] = resultCh
	c.callResultsLock.Unlock()

	// 构造工具调用请求
	var arguments interface{} = args

	mcpMessage := map[string]interface{}{
		"type":       "mcp",
		"session_id": c.sessionID, // 使用连接的session_id
		"payload": map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      originalName,
				"arguments": arguments,
			},
		},
	}

	data, err := json.Marshal(mcpMessage)
	if err != nil {
		// 清理资源
		c.callResultsLock.Lock()
		delete(c.callResults, id)
		c.callResultsLock.Unlock()
		return nil, fmt.Errorf("序列化MCP工具调用请求失败: %v", err)
	}

	c.logger.Info("发送客户端mcp工具调用请求: %s，参数: %s", originalName, string(data))
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		// 清理资源
		c.callResultsLock.Lock()
		delete(c.callResults, id)
		c.callResultsLock.Unlock()
		return nil, fmt.Errorf("MCP客户端尚未连接")
	}

	err = conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		// 清理资源
		c.callResultsLock.Lock()
		delete(c.callResults, id)
		c.callResultsLock.Unlock()
		return nil, err
	}

	// 等待响应或超时
	select {
	case result := <-resultCh:
		if err, ok := result.(error); ok {
			return nil, err
		}
		c.logger.Info("客户端mcp工具调用 %s 成功，结果: %v", originalName, result)
		// 将里面的text提取出来
		if resultMap, ok := result.(map[string]interface{}); ok {
			// 先判断isError是否为true
			if isError, ok := resultMap["isError"].(bool); ok && isError {
				if errorMsg, ok := resultMap["error"].(string); ok {
					return nil, fmt.Errorf("工具调用错误: %s", errorMsg)
				}
				return nil, fmt.Errorf("工具调用返回错误，但未提供具体错误信息")
			}
			// 检查content字段是否存在且为非空数组
			if content, ok := resultMap["content"].([]interface{}); ok && len(content) > 0 {
				if textMap, ok := content[0].(map[string]interface{}); ok {
					if text, ok := textMap["text"].(string); ok {
						if strings.Contains(originalName, "self.camera.take_photo") {
							ret := llm.ActionResponse{
								Action: llm.ActionTypeCallHandler,
								Result: llm.ActionResponseCall{
									FuncName: "mcp_handler_take_photo",
									Args:     text,
								},
							}
							return ret, nil
						}
						c.logger.Info("工具调用返回文本: %s", text)
						ret := llm.ActionResponse{
							Action: llm.ActionTypeReqLLM,
							Result: text,
						}
						return ret, nil
					}
				}
			}
		}
		return result, nil
	case <-ctx.Done():
		// 上下文取消或超时
		c.callResultsLock.Lock()
		delete(c.callResults, id)
		c.callResultsLock.Unlock()
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		// 请求超时
		c.callResultsLock.Lock()
		delete(c.callResults, id)
		c.callResultsLock.Unlock()
		return nil, fmt.Errorf("工具调用请求超时")
	}
}

// IsReady checks if the XiaoZhi client is ready
func (c *XiaoZhiMCPClient) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// 不仅要检查ready标志，还要检查连接是否存在
	return c.isReady && c.conn != nil && c.sessionID != ""
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
	c.sessionID = "" // 清除会话ID
	c.tools = make([]Tool, 0)
	c.callResults = make(map[int]chan interface{})
	c.clientID = "" // 清除客户端ID
	c.deviceID = "" // 清除设备ID
	c.token = ""    // 清除访问令牌
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
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	// Generate a simple token for now (in production, this should be properly signed)
	token := c.token
	if token == "" {
		token = fmt.Sprintf("xiaozhi-token-%d", time.Now().Unix())
	}

	initMsg := map[string]interface{}{
		"type":       "mcp",
		"session_id": c.sessionID,
		"payload": map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      mcpInitializeID,
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"roots": map[string]interface{}{
						"listChanged": true,
					},
					"sampling": map[string]interface{}{},
					"vision": map[string]interface{}{
						"url":   c.visionURL,
						"token": token,
					},
				},
				"clientInfo": map[string]interface{}{
					"name":    "XiaozhiClient",
					"version": "1.0.0",
				},
			},
		},
	}

	data, err := json.Marshal(initMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal initialize message: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send initialize message: %w", err)
	}

	c.logger.Debug("Sent MCP initialize message")
	return nil
}

// SendMCPToolsListRequest sends the MCP tools list request
func (c *XiaoZhiMCPClient) SendMCPToolsListRequest() error {
	// 构造MCP工具列表请求
	mcpMessage := map[string]interface{}{
		"type":       "mcp",
		"session_id": c.sessionID,
		"payload": map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      mcpToolsListID, // 使用新的ID
			"method":  "tools/list",
		},
	}

	data, err := json.Marshal(mcpMessage)
	if err != nil {
		return fmt.Errorf("序列化MCP工具列表请求失败: %v", err)
	}

	c.logger.Debug("发送工具列表请求")
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("MCP客户端尚未连接")
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// SendMCPToolsListContinueRequest sends the MCP tools list request with cursor
func (c *XiaoZhiMCPClient) SendMCPToolsListContinueRequest(cursor string) error {
	// 构造MCP工具列表请求
	mcpMessage := map[string]interface{}{
		"type":       "mcp",
		"session_id": c.sessionID,
		"payload": map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      mcpToolsListID, // 使用相同的ID
			"method":  "tools/list",
			"params": map[string]interface{}{
				"cursor": cursor,
			},
		},
	}

	data, err := json.Marshal(mcpMessage)
	if err != nil {
		return fmt.Errorf("序列化MCP工具列表请求失败: %v", err)
	}

	c.logger.Info("发送带 cursor 的工具列表请求: %s", cursor)
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("MCP客户端尚未连接")
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// HandleMCPMessage handles incoming MCP messages
func (c *XiaoZhiMCPClient) HandleMCPMessage(msgMap map[string]interface{}) error {
	// 获取payload
	payload, ok := msgMap["payload"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("MCP消息缺少payload字段")
	}

	// 检查是否有结果字段（response）
	result, hasResult := payload["result"]
	if hasResult {
		// 获取ID，判断是哪个请求的响应
		id, _ := payload["id"].(float64)
		idInt := int(id)

		// 检查是否是工具调用响应
		c.callResultsLock.Lock()
		if resultCh, ok := c.callResults[idInt]; ok {
			resultCh <- result
			delete(c.callResults, idInt)
			c.callResultsLock.Unlock()
			return nil
		}
		c.callResultsLock.Unlock()

		if id == mcpInitializeID { // 如果是初始化响应
			c.logger.Debug("收到MCP初始化响应")

			// 解析服务器信息
			if serverInfo, ok := result.(map[string]interface{})["serverInfo"].(map[string]interface{}); ok {
				name := serverInfo["name"]
				version := serverInfo["version"]
				c.logger.Info("服务器信息 name=%v version=%v", name, version)
			}

			// 初始化完成后，请求工具列表
			return c.SendMCPToolsListRequest()
		} else if id == mcpToolsListID { // 如果是tools/list响应
			c.logger.Debug("收到MCP工具列表响应")

			// 解析工具列表
			toolNames := ""
			if toolsData, ok := result.(map[string]interface{}); ok {
				tools, ok := toolsData["tools"].([]interface{})
				if !ok {
					return fmt.Errorf("工具列表格式错误")
				}

				c.logger.Debug("客户端设备支持的工具数量: %d", len(tools))

				// 解析工具并添加到列表中
				c.mu.Lock()
				for i, tool := range tools {
					toolMap, ok := tool.(map[string]interface{})
					if !ok {
						continue
					}

					// 构造Tool结构体并添加到列表
					name, _ := toolMap["name"].(string)
					desc, _ := toolMap["description"].(string)

					inputSchema := ToolInputSchema{
						Type: "object",
					}

					if schema, ok := toolMap["inputSchema"].(map[string]interface{}); ok {
						if schemaType, ok := schema["type"].(string); ok {
							inputSchema.Type = schemaType
						}

						if properties, ok := schema["properties"].(map[string]interface{}); ok {
							inputSchema.Properties = properties
						}

						if required, ok := schema["required"].([]interface{}); ok {
							inputSchema.Required = make([]string, 0, len(required))
							for _, r := range required {
								if s, ok := r.(string); ok {
									inputSchema.Required = append(inputSchema.Required, s)
								}
							}
						} else {
							inputSchema.Required = make([]string, 0) // 确保是空切片而不是nil
						}
					}

					newTool := Tool{
						Name:        name,
						Description: desc,
						InputSchema: inputSchema,
					}

					c.tools = append(c.tools, newTool)
					// 建立名称映射关系
					sanitizedName := sanitizeToolName(name)
					c.toolNameMap[sanitizedName] = name
					c.logger.Debug("客户端工具 #%d: %v", i+1, name)
					toolNames += fmt.Sprintf("%s ", name)
				}
				c.logger.Info("工具列表: %s", toolNames)

				// 检查是否需要继续获取下一页工具
				if nextCursor, ok := toolsData["nextCursor"].(string); ok && nextCursor != "" {
					// 如果有下一页，发送带cursor的请求
					c.logger.Info("有更多工具，nextCursor: %s", nextCursor)
					c.mu.Unlock()
					return c.SendMCPToolsListContinueRequest(nextCursor)
				} else {
					// 所有工具已获取，设置准备就绪标志
					c.isReady = true
					close(c.readyCh)
				}
				c.mu.Unlock()
			}
		}
	} else if method, hasMethod := payload["method"].(string); hasMethod {
		// 处理客户端发起的请求
		c.logger.Info("收到MCP客户端请求: %s", method)
		// TODO: 实现处理客户端请求的逻辑
	} else if errorData, hasError := payload["error"].(map[string]interface{}); hasError {
		// 处理错误响应
		errorMsg, _ := errorData["message"].(string)
		c.logger.Error("收到MCP错误响应: %v", errorMsg)

		// 检查是否是工具调用响应
		if id, ok := payload["id"].(float64); ok {
			idInt := int(id)

			c.callResultsLock.Lock()
			if resultCh, ok := c.callResults[idInt]; ok {
				resultCh <- fmt.Errorf("MCP错误: %s", errorMsg)
				delete(c.callResults, idInt)
			}
			c.callResultsLock.Unlock()
		}
	}

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

// handleMessages handles incoming WebSocket messages
func (c *XiaoZhiMCPClient) handleMessages() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
		}
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				return
			}

			var msg map[string]interface{}
			err := conn.ReadJSON(&msg)
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