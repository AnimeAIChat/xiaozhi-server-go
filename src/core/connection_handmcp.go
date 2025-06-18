package core

import (
	"context"
	"encoding/json"
	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/vision"
)

func (h *ConnectionHandler) initMCPResultHandlers() {
	// 初始化MCP结果处理器
	// 这里可以添加更多的处理器初始化逻辑
	h.mcpResultHandlers = map[string]func(args interface{}){
		"mcp_handler_exit":       h.mcp_handler_exit,
		"mcp_handler_take_photo": h.mcp_handler_take_photo,
	}
}

func (h *ConnectionHandler) handleMCPResultCall(result types.ActionResponse) {
	// 先取result
	if result.Action != types.ActionTypeCallHandler {
		h.logger.Error("handleMCPResultCall: result.Action is not ActionTypeCallHandler, but %d", result.Action)
		return
	}
	if result.Result == nil {
		h.logger.Error("handleMCPResultCall: result.Result is nil")
		return
	}

	// 取出result.Result结构体，包括函数名和参数
	if Caller, ok := result.Result.(types.ActionResponseCall); ok {
		if handler, exists := h.mcpResultHandlers[Caller.FuncName]; exists {
			// 调用对应的处理函数
			handler(Caller.Args)
		} else {
			h.logger.Error("handleMCPResultCall: no handler found for function %s", Caller.FuncName)
		}
	} else {
		h.logger.Error("handleMCPResultCall: result.Result is not a map[string]interface{}")
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
