package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xiaozhi-server-go/internal/domain/llm"
	"xiaozhi-server-go/internal/platform/config"

	"github.com/sashabaranov/go-openai"
)

type HandlerFunc func(ctx context.Context, args map[string]interface{}) (interface{}, error)

type LocalClient struct {
	tools   []Tool
	mu      sync.RWMutex
	ctx     context.Context
	logger  Logger
	handler map[string]HandlerFunc
	cfg     *config.Config
}

func NewLocalClient(logger Logger, cfg *config.Config) (*LocalClient, error) {
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
		c.logger.Debug("RegisterTools: LocalMCPFun is empty")
		return
	}

	for _, localFunc := range funcs {
		if localFunc.Name == "exit" && localFunc.Enabled {
			c.AddToolExit()
			c.logger.Info("RegisterTools: exit tool registered")
		} else if localFunc.Name == "time" && localFunc.Enabled {
			c.AddToolTime()
			c.logger.Info("RegisterTools: time tool registered")
		} else if localFunc.Name == "change_voice" && localFunc.Enabled {
			c.AddToolChangeVoice()
			c.logger.Info("RegisterTools: change_voice tool registered")
		} else if localFunc.Name == "change_role" && localFunc.Enabled {
			c.AddToolChangeRole()
			c.logger.Info("RegisterTools: change_role tool registered")
		} else if localFunc.Name == "play_music" && localFunc.Enabled {
			c.AddToolPlayMusic()
			c.logger.Info("RegisterTools: play_music tool registered")
		} else if localFunc.Name == "switch_agent" && localFunc.Enabled {
			c.AddToolSwitchAgent()
			c.logger.Info("RegisterTools: switch_agent tool registered")
		} else {
			if localFunc.Enabled {
				c.logger.Warn("RegisterTools: unknown function name %s", localFunc.Name)
			}
		}
	}
}

// Start starts the local MCP client
func (c *LocalClient) Start(ctx context.Context) error {
	c.ctx = ctx
	c.RegisterTools()
	c.logger.Debug("Local MCP client started")
	return nil
}

// Stop stops the local MCP client
func (c *LocalClient) Stop() {
	// Local client doesn't need to stop any services, just return
}

// HasTool checks if the local client has the specified tool
func (c *LocalClient) HasTool(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// Remove local_ prefix if present
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

// GetAvailableTools gets all available tools for the local client
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

// CallTool calls the specified tool on the local client
func (c *LocalClient) CallTool(
	ctx context.Context,
	name string,
	args map[string]interface{},
) (interface{}, error) {
	// Check if tool exists
	if !c.HasTool(name) {
		return nil, fmt.Errorf("tool %s not found", name)
	}
	// Remove local_ prefix if present
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

// IsReady checks if the local client is ready
func (c *LocalClient) IsReady() bool {
	// Local client is always ready
	return true
}

// ResetConnection resets the local client's connection state
func (c *LocalClient) ResetConnection() error {
	// Local client has no connection state, just return nil
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

func (c *LocalClient) AddToolExit() error {
	InputSchema := ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"say_goodbye": map[string]any{
				"type":        "string",
				"description": "用户友好结束对话的告别语",
			},
		},
		Required: []string{"say_goodbye"},
	}

	c.AddTool("exit",
		"当用户想结束对话或需要退出系统时调用",
		InputSchema,
		func(ctx context.Context, args map[string]any) (interface{}, error) {
			c.logger.Info("用户请求退出对话，告别语：%s", args["say_goodbye"])
			res := llm.ActionResponse{
				Action: llm.ActionTypeCallHandler, // 动作类型
				Result: llm.ActionResponseCall{
					FuncName: "mcp_handler_exit",  // 函数名
					Args:     args["say_goodbye"], // 函数参数
				},
			}
			return res, nil
		})

	return nil
}

func (c *LocalClient) AddToolTime() error {
	InputSchema := ToolInputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}

	c.AddTool("get_time",
		"获取今天日期或者当前时间信息时调用",
		InputSchema,
		func(ctx context.Context, args map[string]any) (interface{}, error) {
			now := time.Now()
			timeStr := now.Format("2006-01-02 15点04分05秒")
			week := now.Weekday().String()
			str := "当前时间是 " + timeStr + "，今天是" + week + "。"
			res := llm.ActionResponse{
				Action: llm.ActionTypeReqLLM, // 动作类型
				Result: str,                    // 函数参数
			}
			return res, nil
		})

	return nil
}

func (c *LocalClient) AddToolChangeRole() error {
	roles := c.cfg.System.Roles
	prompts := map[string]string{}
	roleNames := ""
	if len(roles) == 0 {
		c.logger.Warn(
			"AddToolChangeRole: roles settings is nil or empty, Skipping tool registration",
		)
		return nil
	} else {
		for _, role := range roles {

			prompts[role.Name] = role.Description
			roleNames += role.Name + ", "
		}
	}

	InputSchema := ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"role": map[string]any{
				"type":        "string",
				"description": "新的角色名称",
			},
		},
		Required: []string{"role"},
	}

	c.AddTool("change_role",
		"当用户想切换角色/模型性格/助手名字时调用,可选的角色有：["+roleNames+"]",
		InputSchema,
		func(ctx context.Context, args map[string]any) (interface{}, error) {
			role := args["role"].(string)
			res := llm.ActionResponse{
				Action: llm.ActionTypeCallHandler, // 动作类型
				Result: llm.ActionResponseCall{
					FuncName: "mcp_handler_change_role", // 函数名
					Args: map[string]string{
						"role":   role, // 函数参数
						"prompt": prompts[role],
					},
				},
			}
			return res, nil
		})

	return nil
}

func (c *LocalClient) AddToolChangeVoice() error {
	// 由于移除了数据库配置，暂时简化音色切换功能
	voiceDes := "默认音色"

	InputSchema := ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"voice": map[string]any{
				"type":        "string",
				"description": "新的语音名称",
			},
		},
		Required: []string{"voice"},
	}

	c.AddTool("change_voice",
		"当用户想要更换角色语音或音色时调用，当前支持的音色有: "+voiceDes,
		InputSchema,
		func(ctx context.Context, args map[string]any) (interface{}, error) {
			voice := args["voice"].(string)
			res := llm.ActionResponse{
				Action: llm.ActionTypeCallHandler, // 动作类型
				Result: llm.ActionResponseCall{
					FuncName: "mcp_handler_change_voice", // 函数名
					Args:     voice,                      // 函数参数
				},
			}
			return res, nil
		})

	return nil
}

func (c *LocalClient) AddToolPlayMusic() error {
	InputSchema := ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"song_name": map[string]any{
				"type":        "string",
				"description": "歌曲名称，如果用户没有指定具体歌名则为'random', 明确指定的时返回音乐的名字 示例: ```用户:播放两只老虎\n参数：两只老虎``` ```用户:播放音乐 \n参数：random ```",
			},
		},
		Required: []string{"song_name"},
	}

	c.AddTool("play_music",
		"当用户想要播放音乐/听歌/唱歌时调用",
		InputSchema,
		func(ctx context.Context, args map[string]any) (interface{}, error) {
			song_name := args["song_name"].(string)
			res := llm.ActionResponse{
				Action: llm.ActionTypeCallHandler, // 动作类型
				Result: llm.ActionResponseCall{
					FuncName: "mcp_handler_play_music", // 函数名
					Args:     song_name,                // 函数参数
				},
			}
			return res, nil
		})

	return nil
}

func (c *LocalClient) AddToolSwitchAgent() error {
	InputSchema := ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"agent_id": map[string]any{
				"type":        "number",
				"description": "要切换到的智能体ID",
			},
			"agent_name": map[string]any{
				"type":        "string",
				"description": "智能体名称",
			},
		},
		Required: []string{},
	}

	c.AddTool("switch_agent",
		"当用户想切换智能体时调用，必须提供agent_id（数字）或agent_name（字符串）其中之一",
		InputSchema,
		func(ctx context.Context, args map[string]any) (interface{}, error) {
			// 验证至少提供了一个参数
			if _, hasID := args["agent_id"]; !hasID {
				if _, hasName := args["agent_name"]; !hasName {
					c.logger.Warn("switch_agent: 必须提供 agent_id 或 agent_name 其中之一")
					return llm.ActionResponse{
						Action: llm.ActionTypeReqLLM,
						Result: "切换智能体需要提供智能体ID或名称",
					}, nil
				}
			}

			// 将完整的参数传给连接处理器的 handler，由 handler 解析并执行切换
			res := llm.ActionResponse{
				Action: llm.ActionTypeCallHandler,
				Result: llm.ActionResponseCall{
					FuncName: "mcp_handler_switch_agent",
					Args:     args,
				},
			}
			return res, nil
		})

	return nil
}