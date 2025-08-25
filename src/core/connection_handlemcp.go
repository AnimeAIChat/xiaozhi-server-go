package core

import (
	"context"
	"encoding/json"
	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/vision"
)

func (h *ConnectionHandler) initMCPResultHandlers() {
	// 初始化MCP结果处理器
	// 这里可以添加更多的处理器初始化逻辑
	h.mcpResultHandlers = map[string]func(args interface{}){
		"mcp_handler_exit":         h.mcp_handler_exit,
		"mcp_handler_take_photo":   h.mcp_handler_take_photo,
		"mcp_handler_change_voice": h.mcp_handler_change_voice,
		"mcp_handler_change_role":  h.mcp_handler_change_role,
		"mcp_handler_play_music":   h.mcp_handler_play_music,
	}
}

func (h *ConnectionHandler) handleMCPResultCall(result types.ActionResponse) string {
	errResult := "调用工具失败"
	// 先取result
	if result.Action != types.ActionTypeCallHandler {
		h.logger.Error("handleMCPResultCall: result.Action is not ActionTypeCallHandler, but %d", result.Action)
		return errResult
	}
	if result.Result == nil {
		h.logger.Error("handleMCPResultCall: result.Result is nil")
		return errResult
	}

	// 取出result.Result结构体，包括函数名和参数
	if Caller, ok := result.Result.(types.ActionResponseCall); ok {
		if handler, exists := h.mcpResultHandlers[Caller.FuncName]; exists {
			// 调用对应的处理函数
			handler(Caller.Args)
			return "调用工具成功: " + Caller.FuncName
		} else {
			h.logger.Error("handleMCPResultCall: no handler found for function %s", Caller.FuncName)
		}
	} else {
		h.logger.Error("handleMCPResultCall: result.Result is not a map[string]interface{}")
	}
	return errResult
}

func (h *ConnectionHandler) mcp_handler_play_music(args interface{}) {
	if songName, ok := args.(string); ok {
		h.logger.Info("mcp_handler_play_music: %s", songName)
		if path, name, err := utils.GetMusicFilePathFuzzy(songName); err != nil {
			h.logger.Error("mcp_handler_play_music: Play failed: %v", err)
			h.SystemSpeak("没有找到名为" + songName + "的歌曲")
		} else {
			//h.SystemSpeak("这就为您播放音乐: " + songName)
			h.tts_last_text_index = h.tts_last_text_index + 1

			h.ttsQueue <- struct {
				text      string
				round     int
				textIndex int
				filepath  string
			}{name, h.talkRound, h.tts_last_text_index, path}
		}
	} else {
		h.logger.Error("mcp_handler_play_music: args is not a string")
	}
}

func (h *ConnectionHandler) mcp_handler_change_voice(args interface{}) {
	if voice, ok := args.(string); ok {
		h.logger.Info("mcp_handler_change_voice: %s", voice)
		if err := h.providers.tts.SetVoice(voice); err != nil {
			h.logger.Error("mcp_handler_change_voice: SetVoice failed: %v", err)
			h.SystemSpeak("切换语音失败，没有叫" + voice + "的音色")
		} else {
			h.SystemSpeak("已切换到音色" + voice)
		}
	} else {
		h.logger.Error("mcp_handler_change_voice: args is not a string")
	}
}

func (h *ConnectionHandler) mcp_handler_change_role(args interface{}) {
	if params, ok := args.(map[string]string); ok {
		role := params["role"]
		prompt := params["prompt"]

		h.logger.Info("mcp_handler_change_role: %s", role)
		h.dialogueManager.SetSystemMessage(prompt)
		h.dialogueManager.KeepRecentMessages(5) // 保留最近5条消息
		if getter, ok := h.providers.tts.(configGetter); ok {
			ttsProvider := getter.Config().Type
			if ttsProvider == "edge" {
				if role == "陕西女友" {
					h.providers.tts.SetVoice("zh-CN-shaanxi-XiaoniNeural") // 陕西女友音色
				} else if role == "英语老师" {
					h.providers.tts.SetVoice("zh-CN-XiaoyiNeural") // 英语老师音色
				} else if role == "好奇小男孩" {
					h.providers.tts.SetVoice("zh-CN-YunxiNeural") // 好奇小男孩音色
				}
			}
		}
		h.SystemSpeak("已切换到新角色 " + role)
	} else {
		h.logger.Error("mcp_handler_change_role: args is not a string")
	}
}

func (h *ConnectionHandler) mcp_handler_exit(args interface{}) {
	if text, ok := args.(string); ok {
		h.closeAfterChat = true
		h.SystemSpeak(text)
	} else {
		h.logger.Error("mcp_handler_exit: args is not a string")
	}
}

func (h *ConnectionHandler) mcp_handler_take_photo(args interface{}) {
	// 特殊处理拍照函数，解析为VisionResponse
	resultStr, _ := args.(string)
	var visionResponse vision.VisionResponse
	if err := json.Unmarshal([]byte(resultStr), &visionResponse); err != nil {
		h.logger.Error("解析VisionResponse失败: %v", err)
	}

	if !visionResponse.Success {
		h.logger.Error("拍照失败: %s", visionResponse.Message)
		h.genResponseByLLM(context.Background(), h.dialogueManager.GetLLMDialogue(), h.talkRound)

	}

	h.SystemSpeak(visionResponse.Result)
}
