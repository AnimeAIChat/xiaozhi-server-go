package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/httpsvr/vision"
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
		"mcp_handler_switch_agent": h.mcp_handler_switch_agent,
	}
}

// mcp_handler_switch_agent 处理切换智能体的请求，参数可以是 {"agent_id": <number>} 或 {"agent_id": "123"} 或 {"agent_name": "名字"}
func (h *ConnectionHandler) mcp_handler_switch_agent(args interface{}) {
	var newAgentID uint = 0
	var agentName string

	switch v := args.(type) {
	case map[string]interface{}:
		if idv, ok := v["agent_id"]; ok {
			switch idt := idv.(type) {
			case float64:
				newAgentID = uint(idt)
			case int:
				newAgentID = uint(idt)
			case string:
				if n, err := strconv.Atoi(idt); err == nil {
					newAgentID = uint(n)
				}
			}
		}
		if namev, ok := v["agent_name"]; ok {
			if s, ok2 := namev.(string); ok2 {
				agentName = s
			}
		}
	case string:
		// 如果直接传入字符串，尝试解析为数字ID，否则作为名字
		if n, err := strconv.Atoi(v); err == nil {
			newAgentID = uint(n)
		} else {
			agentName = v
		}
	case float64:
		newAgentID = uint(v)
	case int:
		newAgentID = uint(v)
	default:
		h.logger.Error("mcp_handler_switch_agent: unsupported arg type %T", v)
		return
	}

	if newAgentID != 0 && newAgentID == h.agentID {
		h.logger.Info("mcp_handler_switch_agent: already using agent %d", newAgentID)
		h.SystemSpeak("您已经在使用该智能体")
		return
	}

	agents, err := database.ListAgentsByUser(database.GetDB(), database.AdminUserID)
	// 查找agent
	if err != nil {
		h.logger.Error("mcp_handler_switch_agent: ListAgentsByUser failed: %v", err)
		h.SystemSpeak("切换智能体失败：无法获取智能体列表")
		return
	}
	device, err := database.FindDeviceByID(database.GetDB(), h.deviceID) // 确保设备存在
	if err != nil || device == nil {
		h.logger.Error("mcp_handler_switch_agent: FindDeviceByID failed: %v", err)
		h.SystemSpeak("切换智能体失败：无法获取设备信息")
		return
	}

	for _, ag := range agents {
		if ag.ID == newAgentID || (agentName != "" && ag.Name == agentName) {
			// 找到对应的agent
			h.logger.Info("mcp_handler_switch_agent: found agent %d, name %s", ag.ID, ag.Name)
			h.agentID = ag.ID
			device.AgentID = &ag.ID
			database.UpdateDevice(database.GetDB(), device) // 更新设备的agent_id
			agent, prompt := h.InitWithAgent()
			// 更新对话系统提示并保留最近上下文
			h.dialogueManager.SetSystemMessage(prompt)
			h.dialogueManager.KeepRecentMessages(5)
			// 重新检查并切换提供者
			h.checkTTSProvider(agent, h.config)
			h.checkLLMProvider(agent, h.config)

			if agent != nil && agent.Name != "" {
				h.SystemSpeak("已切换到 " + agent.Name)
			} else {
				h.SystemSpeak("已切换到新的智能体")
			}
			return
		}
	}
	h.SystemSpeak("没有找到对应的智能体")
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
			h.sendAudioMessage(path, name, h.tts_last_text_index, h.talkRound)
		}
	} else {
		h.logger.Error("mcp_handler_play_music: args is not a string")
	}
}

func (h *ConnectionHandler) mcp_handler_change_voice(args interface{}) {
	if voice, ok := args.(string); ok {
		h.logger.Info("mcp_handler_change_voice: %s", voice)
		if err, voiceName := h.providers.tts.SetVoice(voice); err != nil {
			h.logger.Error("mcp_handler_change_voice: SetVoice failed: %v", err)
			h.SystemSpeak("切换语音失败，没有叫" + voice + "的音色")
		} else {
			h.LogInfo(fmt.Sprintf("mcp_handler_change_voice: SetVoice success: %s", voiceName))
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
		if getter, ok := h.providers.tts.(ttsConfigGetter); ok {
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
	// 特殊处理拍照函数，解析新的 Vision API 响应结构
	resultStr, _ := args.(string)
	type visionAPIResponse struct {
		Success bool                      `json:"success"`
		Message string                    `json:"message"`
		Code    int                       `json:"code"`
		Data    vision.VisionAnalysisData `json:"data"`
	}

	var resp visionAPIResponse
	if err := json.Unmarshal([]byte(resultStr), &resp); err != nil {
		h.logger.Error("解析 Vision API 响应失败: %v", err)
		return
	}

	if !resp.Success {
		errMsg := resp.Data.Error
		if errMsg == "" && resp.Message != "" {
			errMsg = resp.Message
		}
		h.logger.Error("拍照失败: %s", errMsg)
		h.genResponseByLLM(context.Background(), h.dialogueManager.GetLLMDialogue(), h.talkRound)
		return
	}

	h.SystemSpeak(resp.Data.Result)
}
