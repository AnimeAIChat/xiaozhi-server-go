package mcp

import (
	"context"
	"strings"
	"time"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/types"
)

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
			res := types.ActionResponse{
				Action: types.ActionTypeCallHandler, // 动作类型
				Result: types.ActionResponseCall{
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
			time := now.Format("2006-01-02 15点04分05秒")
			week := now.Weekday().String()
			str := "当前时间是 " + time + "，今天是" + week + "。"
			res := types.ActionResponse{
				Action: types.ActionTypeReqLLM, // 动作类型
				Result: str,                    // 函数参数
			}
			return res, nil
		})

	return nil
}

func (c *LocalClient) AddToolChangeRole() error {
	roles := c.cfg.Roles
	prompts := map[string]string{}
	roleNames := ""
	if roles == nil {
		c.logger.Warn(
			"AddToolChangeRole: roles settings is nil or empty, Skipping tool registration",
		)
		return nil
	} else {
		for _, role := range roles {
			items := strings.Split(role, "@")
			prompts[items[0]] = items[1]
			roleNames += items[0] + ", "
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
			res := types.ActionResponse{
				Action: types.ActionTypeCallHandler, // 动作类型
				Result: types.ActionResponseCall{
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
	voices := []configs.VoiceInfo{}
	if ttsType, ok := c.cfg.SelectedModule["TTS"]; ok && ttsType != "" {
		voices = c.cfg.TTS[ttsType].SupportedVoices
	}
	voiceDesArr := []string{}
	for _, v := range voices {
		voiceDesArr = append(voiceDesArr, v.Name+"("+v.DisplayName+"-"+v.Sex+")："+v.Description)
	}
	voiceDes := strings.Join(voiceDesArr, ", ")

	InputSchema := ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"voice": map[string]any{
				"type":        "string",
				"description": "新的语音名称，音色描述中的第一部分",
			},
		},
		Required: []string{"voice"},
	}

	c.AddTool("change_voice",
		"当用户想要更换角色语音或音色时调用，当前支持的音色有: "+voiceDes,
		InputSchema,
		func(ctx context.Context, args map[string]any) (interface{}, error) {
			voice := args["voice"].(string)
			res := types.ActionResponse{
				Action: types.ActionTypeCallHandler, // 动作类型
				Result: types.ActionResponseCall{
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
			res := types.ActionResponse{
				Action: types.ActionTypeCallHandler, // 动作类型
				Result: types.ActionResponseCall{
					FuncName: "mcp_handler_play_music", // 函数名
					Args:     song_name,                // 函数参数
				},
			}
			return res, nil
		})

	return nil
}
