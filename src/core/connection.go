package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/chat"
	"xiaozhi-server-go/src/core/function"
	"xiaozhi-server-go/src/core/image"
	"xiaozhi-server-go/src/core/mcp"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/vlllm"
	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/task"

	"github.com/google/uuid"
)

// ConnectionHandler 连接处理器结构
type ConnectionHandler struct {
	// 确保实现 AsrEventListener 接口
	_         providers.AsrEventListener
	config    *configs.Config
	logger    *utils.Logger
	conn      Conn
	closeOnce sync.Once
	taskMgr   *task.TaskManager
	providers struct {
		asr   providers.ASRProvider
		llm   providers.LLMProvider
		tts   providers.TTSProvider
		vlllm *vlllm.Provider // VLLLM提供者，可选
	}

	// 会话相关
	sessionID string
	// 客户端音频相关
	clientAudioFormat        string
	clientAudioSampleRate    int
	clientAudioChannels      int
	clientAudioFrameDuration int

	serverAudioFormat        string // 服务端音频格式
	serverAudioSampleRate    int
	serverAudioChannels      int
	serverAudioFrameDuration int

	clientListenMode string
	isDeviceVerified bool
	closeAfterChat   bool

	// 语音处理相关
	clientVoiceStop bool  // true客户端语音停止, 不再上传语音数据
	serverVoiceStop int32 // 1表示true服务端语音停止, 不再下发语音数据

	opusDecoder *utils.OpusDecoder // Opus解码器

	// 对话相关
	dialogueManager     *chat.DialogueManager
	tts_last_text_index int
	client_asr_text     string // 客户端ASR文本

	// 并发控制
	stopChan         chan struct{}
	clientAudioQueue chan []byte
	clientTextQueue  chan string

	// TTS任务队列
	ttsQueue chan struct {
		text      string
		round     int // 轮次
		textIndex int
	}

	audioMessagesQueue chan struct {
		filepath  string
		text      string
		round     int // 轮次
		textIndex int
	}

	talkRound      int       // 轮次计数
	roundStartTime time.Time // 轮次开始时间
	// functions
	functionRegister *function.FunctionRegistry
	mcpManager       *mcp.Manager
}

// NewConnectionHandler 创建新的连接处理器
func NewConnectionHandler(
	config *configs.Config,
	providerSet *pool.ProviderSet,
	logger *utils.Logger,
) *ConnectionHandler {
	handler := &ConnectionHandler{
		config:           config,
		logger:           logger,
		clientListenMode: "auto",
		stopChan:         make(chan struct{}),
		clientAudioQueue: make(chan []byte, 100),
		clientTextQueue:  make(chan string, 100),
		ttsQueue: make(chan struct {
			text      string
			round     int // 轮次
			textIndex int
		}, 100),
		audioMessagesQueue: make(chan struct {
			filepath  string
			text      string
			round     int // 轮次
			textIndex int
		}, 100),

		tts_last_text_index: -1,

		talkRound: 0,

		serverAudioFormat:        "opus", // 默认使用Opus格式
		serverAudioSampleRate:    24000,
		serverAudioChannels:      1,
		serverAudioFrameDuration: 60,
	}

	handler.sessionID = uuid.New().String() // 生成唯一会话ID

	// 正确设置providers
	if providerSet != nil {
		handler.providers.asr = providerSet.ASR
		handler.providers.llm = providerSet.LLM
		handler.providers.tts = providerSet.TTS
		handler.providers.vlllm = providerSet.VLLLM
		handler.mcpManager = providerSet.MCP
	}

	// 初始化对话管理器
	handler.dialogueManager = chat.NewDialogueManager(handler.logger, nil)
	handler.dialogueManager.SetSystemMessage(config.DefaultPrompt)
	handler.functionRegister = function.NewFunctionRegistry()

	return handler
}

// Handle 处理WebSocket连接
func (h *ConnectionHandler) Handle(conn Conn) {
	defer conn.Close()

	h.conn = conn

	// 发送欢迎消息
	if err := h.sendHelloMessage(); err != nil {
		h.logger.Error(fmt.Sprintf("发送欢迎消息失败: %v", err))
		return
	}

	// 启动消息处理协程
	go h.processClientAudioMessagesCoroutine() // 添加客户端音频消息处理协程
	go h.processClientTextMessagesCoroutine()  // 添加客户端文本消息处理协程
	go h.processTTSQueueCoroutine()            // 添加TTS队列处理协程
	go h.sendAudioMessageCoroutine()           // 添加音频消息发送协程

	// 优化后的MCP管理器处理
	if h.mcpManager == nil {
		h.logger.Info("从资源池未获取到MCP管理器，创建新的MCP管理器")
		h.mcpManager = mcp.NewManager(h.logger, h.functionRegister, conn, h.sessionID)
		// 只有在创建新实例时才需要完整初始化
		if err := h.mcpManager.InitializeServers(context.Background()); err != nil {
			h.logger.Error(fmt.Sprintf("初始化MCP服务器失败: %v", err))
		}
	} else {
		h.logger.Info("使用从资源池获取的MCP管理器，快速绑定连接")
		// 池化的管理器已经预初始化，只需要绑定连接
		if err := h.mcpManager.BindConnection(conn, h.functionRegister, h.sessionID); err != nil {
			h.logger.Error(fmt.Sprintf("绑定MCP管理器连接失败: %v", err))
			return
		}
		// 不需要重新初始化服务器，只需要确保连接相关的服务正常
		h.logger.Info("MCP管理器连接绑定完成，跳过重复初始化")
	}

	// 主消息循环
	for {
		select {
		case <-h.stopChan:
			return
		default:
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				h.logger.Error(fmt.Sprintf("读取消息失败: %v", err))
				return
			}

			if err := h.handleMessage(messageType, message); err != nil {
				h.logger.Error(fmt.Sprintf("处理消息失败: %v", err))
			} else {
				if h.closeAfterChat {
					return
				}
			}
		}
	}
}

// processClientTextMessagesCoroutine 处理文本消息队列
func (h *ConnectionHandler) processClientTextMessagesCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case text := <-h.clientTextQueue:
			if err := h.processClientTextMessage(context.Background(), text); err != nil {
				h.logger.Error(fmt.Sprintf("处理文本数据失败: %v", err))
			}
		}
	}
}

// processClientAudioMessagesCoroutine 处理音频消息队列
func (h *ConnectionHandler) processClientAudioMessagesCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case audioData := <-h.clientAudioQueue:
			if err := h.providers.asr.AddAudio(audioData); err != nil {
				h.logger.Error(fmt.Sprintf("处理音频数据失败: %v", err))
			}
		}
	}
}

func (h *ConnectionHandler) sendAudioMessageCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case task := <-h.audioMessagesQueue:
			h.sendAudioMessage(task.filepath, task.text, task.textIndex, task.round)
		}
	}
}

// OnAsrResult 实现 AsrEventListener 接口
// 返回true则停止语音识别，返回false会继续语音识别
func (h *ConnectionHandler) OnAsrResult(result string) bool {
	//h.logger.Info(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
	if h.clientListenMode == "auto" {
		if result == "" {
			return false
		}
		h.logger.Info(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
		h.handleChatMessage(context.Background(), result)
		return true
	} else if h.clientListenMode == "manual" {
		h.client_asr_text += result
		if result != "" {
			h.logger.Info(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, h.client_asr_text))
		}
		if h.clientVoiceStop {
			h.handleChatMessage(context.Background(), h.client_asr_text)
			return true
		}
		return false
	} else if h.clientListenMode == "realtime" {
		if result == "" {
			return false
		}
		h.stopServerSpeak()
		h.providers.asr.Reset() // 重置ASR状态，准备下一次识别
		h.logger.Info(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
		h.handleChatMessage(context.Background(), result)
		return true
	}
	return false
}

// clientAbortChat 处理中止消息
func (h *ConnectionHandler) clientAbortChat() error {
	h.logger.Info("收到客户端中止消息，停止语音识别")
	h.stopServerSpeak()
	h.sendTTSMessage("stop", "", 0)
	h.clearSpeakStatus()
	return nil
}

func (h *ConnectionHandler) QuitIntent(text string) bool {
	//CMD_exit 读取配置中的退出命令
	exitCommands := h.config.CMDExit
	if exitCommands == nil {
		return false
	}
	cleand_text := utils.RemoveAllPunctuation(text) // 移除标点符号，确保匹配准确
	// 检查是否包含退出命令
	for _, cmd := range exitCommands {
		h.logger.Debug(fmt.Sprintf("检查退出命令: %s,%s", cmd, cleand_text))
		//判断相等
		if cleand_text == cmd {
			h.logger.Info("收到客户端退出意图，准备结束对话")
			h.Close() // 直接关闭连接
			return true
		}
	}
	return false
}

// handleChatMessage 处理聊天消息
func (h *ConnectionHandler) handleChatMessage(ctx context.Context, text string) error {
	if text == "" {
		h.logger.Warn("收到空聊天消息，忽略")
		h.clientAbortChat()
		return fmt.Errorf("聊天消息为空")
	}

	if h.QuitIntent(text) {
		return fmt.Errorf("用户请求退出对话")
	}

	// 增加对话轮次
	h.talkRound++
	h.roundStartTime = time.Now()
	currentRound := h.talkRound
	h.logger.Info(fmt.Sprintf("开始新的对话轮次: %d", currentRound))

	// 判断是否需要验证
	if h.isNeedAuth() {
		if err := h.checkAndBroadcastAuthCode(); err != nil {
			h.logger.Error(fmt.Sprintf("检查认证码失败: %v", err))
			return err
		}
		h.logger.Info("设备未认证，等待管理员认证")
		return nil
	}

	// 智能检测图片URL并自动转换为图片消息
	if imageURL, remainingText, detected := h.detectImageURL(text); detected && h.providers.vlllm != nil {
		h.logger.Info("检测到图片URL，自动转换为图片消息", map[string]interface{}{
			"url":  imageURL,
			"text": remainingText,
		})

		// 构造图片消息数据
		imageData := image.ImageData{
			URL:    imageURL,
			Format: h.extractImageFormat(imageURL),
		}

		// 立即发送STT消息
		sttText := remainingText
		if sttText == "" {
			sttText = "用户发送了一张图片"
		}
		err := h.sendSTTMessage(sttText)
		if err != nil {
			h.logger.Error(fmt.Sprintf("发送STT消息失败: %v", err))
			return fmt.Errorf("发送STT消息失败: %v", err)
		}

		// 发送TTS开始状态
		if err := h.sendTTSMessage("start", "", 0); err != nil {
			h.logger.Error(fmt.Sprintf("发送TTS开始状态失败: %v", err))
			return fmt.Errorf("发送TTS开始状态失败: %v", err)
		}

		// 立即给用户语音反馈，提升用户体验
		immediateResponse := "检测到图片链接，正在下载并识别图片，这就为你分析"
		h.logger.Info("立即播放图片URL识别提示音", map[string]interface{}{
			"response": immediateResponse,
			"url":      imageURL,
		})

		// 重置语音状态，确保能够播放提示音
		atomic.StoreInt32(&h.serverVoiceStop, 0)

		// 立即合成并播放提示音（使用索引0确保优先播放）
		if err := h.SpeakAndPlay(immediateResponse, 0, currentRound); err != nil {
			h.logger.Error(fmt.Sprintf("播放图片URL识别提示音失败: %v", err))
		}

		// 发送思考状态的情绪
		if err := h.sendEmotionMessage("thinking"); err != nil {
			h.logger.Error(fmt.Sprintf("发送思考状态情绪消息失败: %v", err))
			return fmt.Errorf("发送情绪消息失败: %v", err)
		}

		// 添加用户消息到对话历史（包含图片信息的描述）
		userMessage := fmt.Sprintf("%s [用户发送了一张图片: %s]", remainingText, imageURL)
		h.dialogueManager.Put(chat.Message{
			Role:    "user",
			Content: userMessage,
		})

		// 获取对话历史（排除当前图片消息）
		messages := make([]providers.Message, 0)
		for _, msg := range h.dialogueManager.GetLLMDialogue() {
			if msg.Role == "user" && strings.Contains(msg.Content, "[用户发送了一张图片:") {
				continue
			}
			messages = append(messages, providers.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}

		// 使用VLLLM处理图片消息
		return h.genResponseByVLLM(ctx, messages, imageData, remainingText, currentRound)
	}

	// 普通文本消息处理流程
	// 立即发送 stt 消息
	err := h.sendSTTMessage(text)
	if err != nil {
		h.logger.Error(fmt.Sprintf("发送STT消息失败: %v", err))
		return fmt.Errorf("发送STT消息失败: %v", err)
	}

	// 发送tts start状态
	if err := h.sendTTSMessage("start", "", 0); err != nil {
		h.logger.Error(fmt.Sprintf("发送TTS开始状态失败: %v", err))
		return fmt.Errorf("发送TTS开始状态失败: %v", err)
	}

	// 发送思考状态的情绪
	if err := h.sendEmotionMessage("thinking"); err != nil {
		h.logger.Error(fmt.Sprintf("发送思考状态情绪消息失败: %v", err))
		return fmt.Errorf("发送情绪消息失败: %v", err)
	}

	h.logger.Info("收到聊天消息: " + text)

	// 添加用户消息到对话历史
	h.dialogueManager.Put(chat.Message{
		Role:    "user",
		Content: text,
	})

	// 转换消息格式并使用LLM生成回复
	messages := make([]providers.Message, 0)
	for _, msg := range h.dialogueManager.GetLLMDialogue() {
		messages = append(messages, providers.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		})
	}

	return h.genResponseByLLM(ctx, messages, currentRound)
}

func (h *ConnectionHandler) genResponseByLLM(ctx context.Context, messages []providers.Message, round int) error {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error(fmt.Sprintf("genResponseByLLM发生panic: %v", r))
			errorMsg := "抱歉，处理您的请求时发生了错误"
			h.tts_last_text_index = 1 // 重置文本索引
			h.SpeakAndPlay(errorMsg, 1, round)
		}
	}()

	llmStartTime := time.Now()
	h.logger.FormatInfo("开始生成LLM回复, round:%d 打印message", round)
	for _, msg := range messages {
		msg.Print()
	}
	// 使用LLM生成回复
	tools := h.functionRegister.GetAllFunctions()
	responses, err := h.providers.llm.ResponseWithFunctions(ctx, h.sessionID, messages, tools)
	if err != nil {
		return fmt.Errorf("LLM生成回复失败: %v", err)
	}

	// 处理回复
	var responseMessage []string
	processedChars := 0
	textIndex := 0

	atomic.StoreInt32(&h.serverVoiceStop, 0)

	// 处理流式响应
	toolCallFlag := false
	functionName := ""
	functionID := ""
	functionArguments := ""
	contentArguments := ""

	for response := range responses {
		content := response.Content
		toolCall := response.ToolCalls

		if response.Error != "" {
			h.logger.Error(fmt.Sprintf("LLM响应错误: %s", response.Error))
			errorMsg := "抱歉，服务暂时不可用，请稍后再试"
			h.tts_last_text_index = 1 // 重置文本索引
			h.SpeakAndPlay(errorMsg, 1, round)
			return fmt.Errorf("LLM响应错误: %s", response.Error)
		}

		if content != "" {
			// 累加content_arguments
			contentArguments += content
		}

		if !toolCallFlag && strings.HasPrefix(contentArguments, "<tool_call>") {
			toolCallFlag = true
		}

		if len(toolCall) > 0 {
			toolCallFlag = true
			if toolCall[0].ID != "" {
				functionID = toolCall[0].ID
			}
			if toolCall[0].Function.Name != "" {
				functionName = toolCall[0].Function.Name
			}
			if toolCall[0].Function.Arguments != "" {
				functionArguments += toolCall[0].Function.Arguments
			}
		}

		if content != "" {
			if strings.Contains(content, "服务响应异常") {
				h.logger.Error(fmt.Sprintf("检测到LLM服务异常: %s", content))
				errorMsg := "抱歉，服务暂时不可用，请稍后再试"
				h.tts_last_text_index = 1 // 重置文本索引
				h.SpeakAndPlay(errorMsg, 1, round)
				return fmt.Errorf("LLM服务异常")
			}

			if !toolCallFlag {
				responseMessage = append(responseMessage, content)
			}
			// 处理分段
			fullText := utils.JoinStrings(responseMessage)
			if len(fullText) <= processedChars {
				h.logger.Warn(fmt.Sprintf("文本处理异常: fullText长度=%d, processedChars=%d", len(fullText), processedChars))
				continue
			}
			currentText := fullText[processedChars:]

			// 按标点符号分割
			if segment, chars := utils.SplitAtLastPunctuation(currentText); chars > 0 {
				textIndex++
				if textIndex == 1 {
					now := time.Now()
					llmSpentTime := now.Sub(llmStartTime)
					h.logger.Info(fmt.Sprintf("LLM回复耗时 %s 生成第一句话【%s】, round: %d", llmSpentTime, segment, round))
				} else {
					h.logger.Info(fmt.Sprintf("LLM回复分段: %s, index: %d, round:%d", segment, textIndex, round))
				}
				h.tts_last_text_index = textIndex
				err := h.SpeakAndPlay(segment, textIndex, round)
				if err != nil {
					h.logger.Error(fmt.Sprintf("播放LLM回复分段失败: %v", err))
				}
				processedChars += chars
			}
		}
	}

	if toolCallFlag {
		bHasError := false
		if functionID == "" {
			a := utils.Extract_json_from_string(contentArguments)
			if a != nil {
				functionName = a["name"].(string)
				functionArguments = a["arguments"].(string)
				functionID = uuid.New().String()
			} else {
				bHasError = true
			}
			if bHasError {
				h.logger.Error(fmt.Sprintf("函数调用参数解析失败: %v", err))
			}
		}
		if !bHasError {
			// 清空responseMessage
			responseMessage = []string{}
			arguments := make(map[string]interface{})
			if err := json.Unmarshal([]byte(functionArguments), &arguments); err != nil {
				h.logger.Error(fmt.Sprintf("函数调用参数解析失败: %v", err))
			}
			functionCallData := map[string]interface{}{
				"id":        functionID,
				"name":      functionName,
				"arguments": functionArguments,
			}
			h.logger.Info(fmt.Sprintf("函数调用: %v", arguments))
			if h.mcpManager.IsMCPTool(functionName) {
				// 处理MCP函数调用
				result, err := h.mcpManager.ExecuteTool(ctx, functionName, arguments)
				if err != nil {
					h.logger.Error(fmt.Sprintf("MCP函数调用失败: %v", err))
					if result == nil {
						result = "MCP工具调用失败"
					}
				}
				h.logger.Info(fmt.Sprintf("MCP函数调用结果: %v", result))
				actionResult := types.ActionResponse{
					Action: types.ActionTypeReqLLM, // 动作类型
					Result: result,                 // 动作产生的结果
				}
				h.handleFunctionResult(actionResult, functionCallData, textIndex)

			} else {
				// 处理普通函数调用
				//h.functionRegister.CallFunction(functionName, functionCallData)
			}
		}
	}

	// 处理剩余文本
	fullResponse := utils.JoinStrings(responseMessage)
	if len(fullResponse) > processedChars {
		remainingText := fullResponse[processedChars:]
		if remainingText != "" {
			textIndex++
			h.logger.Info(fmt.Sprintf("LLM回复分段[剩余文本]: %s, index: %d, round:%d", remainingText, textIndex, round))
			h.tts_last_text_index = textIndex
			h.SpeakAndPlay(remainingText, textIndex, round)
		}
	} else {
		h.logger.Info(fmt.Sprintf("无剩余文本需要处理: fullResponse长度=%d, processedChars=%d", len(fullResponse), processedChars))
	}

	// 分析回复并发送相应的情绪
	content := utils.JoinStrings(responseMessage)

	// 添加助手回复到对话历史
	if !toolCallFlag {
		h.dialogueManager.Put(chat.Message{
			Role:    "assistant",
			Content: content,
		})
	}

	return nil
}

func (h *ConnectionHandler) handleFunctionResult(result types.ActionResponse, functionCallData map[string]interface{}, textIndex int) {
	switch result.Action {
	case types.ActionTypeError:
		h.logger.Error(fmt.Sprintf("函数调用错误: %v", result.Result))
	case types.ActionTypeNotFound:
		h.logger.Error(fmt.Sprintf("函数未找到: %v", result.Result))
	case types.ActionTypeNone:
		h.logger.Info(fmt.Sprintf("函数调用无操作: %v", result.Result))
	case types.ActionTypeResponse:
		h.logger.Info(fmt.Sprintf("函数调用直接回复: %v", result.Response))
		h.SpeakAndPlay(result.Response.(string), textIndex, h.talkRound)
	case types.ActionTypeReqLLM:
		h.logger.Info(fmt.Sprintf("函数调用后请求LLM: %v", result.Result))
		text, ok := result.Result.(string)
		if ok && len(text) > 0 {
			functionID := functionCallData["id"].(string)
			functionName := functionCallData["name"].(string)
			functionArguments := functionCallData["arguments"].(string)
			h.logger.Info(fmt.Sprintf("函数调用结果: %s", text))
			h.logger.Info(fmt.Sprintf("函数调用参数: %s", functionArguments))
			h.logger.Info(fmt.Sprintf("函数调用名称: %s", functionName))
			h.logger.Info(fmt.Sprintf("函数调用ID: %s", functionID))

			// 添加 assistant 消息，包含 tool_calls
			h.dialogueManager.Put(chat.Message{
				Role: "assistant",
				ToolCalls: []types.ToolCall{{
					ID: functionID,
					Function: types.FunctionCall{
						Arguments: functionArguments,
						Name:      functionName,
					},
					Type:  "function",
					Index: 0,
				}},
			})

			// 添加 tool 消息
			toolCallID := functionID
			if toolCallID == "" {
				toolCallID = uuid.New().String()
			}
			h.dialogueManager.Put(chat.Message{
				Role:       "tool",
				ToolCallID: toolCallID,
				Content:    text,
			})

			messages := make([]providers.Message, 0)
			for _, msg := range h.dialogueManager.GetLLMDialogue() {
				messages = append(messages, providers.Message{
					Role:       msg.Role,
					Content:    msg.Content,
					ToolCalls:  msg.ToolCalls,
					ToolCallID: msg.ToolCallID,
				})
			}
			// 递归调用 chat_with_function_calling 逻辑
			h.genResponseByLLM(context.Background(), messages, h.talkRound)
		} else {
			h.logger.Error(fmt.Sprintf("函数调用结果解析失败: %v", result.Result))
			// 发送错误消息
			errorMessage := fmt.Sprintf("函数调用结果解析失败 %v", result.Result)
			h.SpeakAndPlay(errorMessage, textIndex, h.talkRound)
		}
	}
}

// isNeedAuth 判断是否需要验证
func (h *ConnectionHandler) isNeedAuth() bool {
	if !h.config.Server.Auth.Enabled {
		return false
	}
	return !h.isDeviceVerified
}

// checkAndBroadcastAuthCode 检查并广播认证码
func (h *ConnectionHandler) checkAndBroadcastAuthCode() error {
	// 这里简化了认证逻辑，实际需要根据具体需求实现
	text := "请联系管理员进行设备认证"
	return h.SpeakAndPlay(text, 0, h.talkRound)
}

// processTTSQueueCoroutine 处理TTS队列
func (h *ConnectionHandler) processTTSQueueCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case task := <-h.ttsQueue:
			h.processTTSTask(task.text, task.textIndex, task.round)
		}
	}
}

// 服务端打断说话
func (h *ConnectionHandler) stopServerSpeak() {
	h.logger.Info("服务端停止说话")
	atomic.StoreInt32(&h.serverVoiceStop, 1)
	// 终止tts任务，不再继续将文本加入到tts队列，清空ttsQueue队列
	for {
		select {
		case task := <-h.ttsQueue:
			h.logger.Info(fmt.Sprintf("丢弃一个TTS任务: %s", task.text))
		default:
			// 队列已清空，退出循环
			h.logger.Info("ttsQueue队列已清空，停止处理TTS任务,准备清空音频队列")
			goto clearAudioQueue
		}
	}

clearAudioQueue:
	// 终止audioMessagesQueue发送，清空队列里的音频数据
	for {
		select {
		case task := <-h.audioMessagesQueue:
			h.logger.Info(fmt.Sprintf("丢弃一个音频任务: %s", task.text))
			// 根据配置删除被丢弃的音频文件
			if h.config.DeleteAudio && task.filepath != "" {
				if err := os.Remove(task.filepath); err != nil {
					h.logger.Error(fmt.Sprintf("删除被丢弃的音频文件失败: %v", err))
				} else {
					h.logger.Info(fmt.Sprintf("已删除被丢弃的音频文件: %s", task.filepath))
				}
			}
		default:
			// 队列已清空，退出循环
			h.logger.Info("audioMessagesQueue队列已清空，停止处理音频任务")
			return
		}
	}
}

// processTTSTask 处理单个TTS任务
func (h *ConnectionHandler) processTTSTask(text string, textIndex int, round int) {
	filepath := ""
	defer func() {
		h.audioMessagesQueue <- struct {
			filepath  string
			text      string
			round     int
			textIndex int
		}{filepath, text, round, textIndex}
	}()

	ttsStartTime := time.Now()
	// 过滤表情
	text = utils.RemoveAllEmoji(text)

	if text == "" {
		h.logger.Warn(fmt.Sprintf("收到空文本，无法合成语音, 索引: %d", textIndex))
		return
	}

	// 生成语音文件
	filepath, err := h.providers.tts.ToTTS(text)
	if err != nil {
		h.logger.Error(fmt.Sprintf("TTS转换失败:text(%s) %v", text, err))
		return
	} else {
		h.logger.Info(fmt.Sprintf("TTS转换成功: text(%s), index(%d) %s", text, textIndex, filepath))
	}
	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.logger.Info(fmt.Sprintf("processTTSTask 服务端语音停止, 不再发送音频数据：%s", text))
		// 服务端语音停止时，根据配置删除已生成的音频文件
		if h.config.DeleteAudio && filepath != "" {
			if err := os.Remove(filepath); err != nil {
				h.logger.Error(fmt.Sprintf("删除停止任务的音频文件失败: %v", err))
			} else {
				h.logger.Info(fmt.Sprintf("已删除停止任务的音频文件: %s", filepath))
			}
		}
		return
	}

	if textIndex == 1 {
		now := time.Now()
		ttsSpentTime := now.Sub(ttsStartTime)
		h.logger.Info(fmt.Sprintf("TTS转换耗时: %s, 文本: %s, 索引: %d", ttsSpentTime, text, textIndex))
	}

}

// speakAndPlay 合成并播放语音
func (h *ConnectionHandler) SpeakAndPlay(text string, textIndex int, round int) error {
	defer func() {
		// 将任务加入队列，不阻塞当前流程
		h.ttsQueue <- struct {
			text      string
			round     int
			textIndex int
		}{text, round, textIndex}
	}()

	originText := text // 保存原始文本用于日志
	text = utils.RemoveAllEmoji(text)
	text = utils.RemoveMarkdownSyntax(text) // 移除Markdown语法
	if text == "" {
		h.logger.FormatWarn("SpeakAndPlay 收到空文本，无法合成语音, %d, text:%s.", textIndex, originText)
		return errors.New("收到空文本，无法合成语音")
	}

	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.logger.Info(fmt.Sprintf("speakAndPlay 服务端语音停止, 不再发送音频数据：%s", text))
		text = ""
		return errors.New("服务端语音已停止，无法合成语音")
	}

	if len(text) > 255 {
		h.logger.Warn(fmt.Sprintf("文本过长，超过255字符限制，无法合成语音: %s", text))
		text = ""
		return errors.New("文本过长，超过255字符限制，无法合成语音")
	}

	return nil
}

func (h *ConnectionHandler) clearSpeakStatus() {
	h.logger.Info("清除服务端讲话状态 ")
	h.tts_last_text_index = -1
	h.providers.asr.Reset() // 重置ASR状态
}

func (h *ConnectionHandler) closeOpusDecoder() {
	if h.opusDecoder != nil {
		if err := h.opusDecoder.Close(); err != nil {
			h.logger.Error(fmt.Sprintf("关闭Opus解码器失败: %v", err))
		}
		h.opusDecoder = nil
	}
}

// Close 清理资源
func (h *ConnectionHandler) Close() {
	h.closeOnce.Do(func() {
		close(h.stopChan)

		h.closeOpusDecoder()

		if h.providers.asr != nil {
			if err := h.providers.asr.Reset(); err != nil {
				h.logger.Error(fmt.Sprintf("重置ASR状态失败: %v", err))
			}
		}

		// 清理待处理的音频文件
		if h.config.DeleteAudio {
			// 清理TTS队列中的任务（这些任务还没有生成音频文件，无需删除）
			for {
				select {
				case task := <-h.ttsQueue:
					h.logger.Info(fmt.Sprintf("连接关闭，丢弃TTS任务: %s", task.text))
				default:
					goto clearAudioQueue
				}
			}

		clearAudioQueue:
			// 清理音频消息队列中的文件
			for {
				select {
				case task := <-h.audioMessagesQueue:
					h.logger.Info(fmt.Sprintf("连接关闭，丢弃音频任务: %s", task.text))
					if task.filepath != "" {
						if err := os.Remove(task.filepath); err != nil {
							h.logger.Error(fmt.Sprintf("连接关闭时删除音频文件失败: %v", err))
						} else {
							h.logger.Info(fmt.Sprintf("连接关闭时已删除音频文件: %s", task.filepath))
						}
					}
				default:
					return
				}
			}
		}
	})
}

// detectImageURL 检测文本中的图片URL
func (h *ConnectionHandler) detectImageURL(text string) (imageURL string, remainingText string, detected bool) {
	// 定义图片URL的正则表达式
	imageURLPattern := regexp.MustCompile(`(https?://[^\s]+\.(?:jpg|jpeg|png|gif|webp|bmp))`)

	// 查找图片URL
	matches := imageURLPattern.FindStringSubmatch(text)

	if len(matches) > 0 {
		imageURL = matches[1]
		// 移除URL，保留剩余文本
		remainingText = strings.TrimSpace(strings.Replace(text, imageURL, "", 1))
		if remainingText == "" {
			remainingText = "请描述这张图片"
		}
		h.logger.Info("检测到图片URL", map[string]interface{}{
			"url":            imageURL,
			"remaining_text": remainingText,
		})
		return imageURL, remainingText, true
	}

	return "", text, false
}

// extractImageFormat 从URL中提取图片格式
func (h *ConnectionHandler) extractImageFormat(url string) string {
	// 从URL中提取文件扩展名
	if strings.Contains(url, ".") {
		parts := strings.Split(url, ".")
		if len(parts) > 0 {
			ext := strings.ToLower(parts[len(parts)-1])
			// 处理URL参数的情况
			if strings.Contains(ext, "?") {
				ext = strings.Split(ext, "?")[0]
			}
			// 验证是否是支持的图片格式
			supportedFormats := []string{"jpg", "jpeg", "png", "gif", "webp", "bmp"}
			for _, format := range supportedFormats {
				if ext == format {
					return ext
				}
			}
		}
	}
	// 默认返回jpg
	return "jpg"
}

// genResponseByVLLM 使用VLLLM处理包含图片的消息
func (h *ConnectionHandler) genResponseByVLLM(ctx context.Context, messages []providers.Message, imageData image.ImageData, text string, round int) error {
	h.logger.Info("开始生成VLLLM回复", map[string]interface{}{
		"text":          text,
		"has_url":       imageData.URL != "",
		"has_data":      imageData.Data != "",
		"format":        imageData.Format,
		"message_count": len(messages),
	})

	// 使用VLLLM处理图片和文本
	responses, err := h.providers.vlllm.ResponseWithImage(ctx, h.sessionID, messages, imageData, text)
	if err != nil {
		h.logger.Error(fmt.Sprintf("VLLLM生成回复失败，尝试降级到普通LLM: %v", err))
		// 降级策略：只使用文本部分调用普通LLM
		fallbackText := fmt.Sprintf("用户发送了一张图片并询问：%s（注：当前无法处理图片，只能根据文字回答）", text)
		fallbackMessages := append(messages, providers.Message{
			Role:    "user",
			Content: fallbackText,
		})
		return h.genResponseByLLM(ctx, fallbackMessages, round)
	}

	// 处理VLLLM流式回复
	var responseMessage []string
	processedChars := 0
	textIndex := 0

	atomic.StoreInt32(&h.serverVoiceStop, 0)

	for response := range responses {
		if response == "" {
			continue
		}

		responseMessage = append(responseMessage, response)
		// 处理分段
		fullText := utils.JoinStrings(responseMessage)
		currentText := fullText[processedChars:]

		// 按标点符号分割
		if segment, chars := utils.SplitAtLastPunctuation(currentText); chars > 0 {
			textIndex++
			h.tts_last_text_index = textIndex
			h.SpeakAndPlay(segment, textIndex, round)
			processedChars += chars
		}
	}

	// 处理剩余文本
	remainingText := utils.JoinStrings(responseMessage)[processedChars:]
	if remainingText != "" {
		textIndex++
		h.tts_last_text_index = textIndex
		h.SpeakAndPlay(remainingText, textIndex, round)
	}

	// 获取完整回复内容
	content := utils.JoinStrings(responseMessage)

	// 添加VLLLM回复到对话历史
	h.dialogueManager.Put(chat.Message{
		Role:    "assistant",
		Content: content,
	})

	h.logger.Info("VLLLM回复处理完成", map[string]interface{}{
		"content_length": len(content),
		"text_segments":  textIndex,
	})

	return nil
}
