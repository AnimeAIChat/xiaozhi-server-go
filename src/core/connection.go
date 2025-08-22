package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/auth"
	"xiaozhi-server-go/src/core/chat"
	"xiaozhi-server-go/src/core/function"
	"xiaozhi-server-go/src/core/image"
	"xiaozhi-server-go/src/core/mcp"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/tts"
	"xiaozhi-server-go/src/core/providers/vlllm"
	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/task"

	"github.com/google/uuid"
)

// Connection 统一连接接口
type Connection interface {
	// 发送消息
	WriteMessage(messageType int, data []byte) error
	// 读取消息
	ReadMessage(stopChan <-chan struct{}) (messageType int, data []byte, err error)
	// 关闭连接
	Close() error
	// 获取连接ID
	GetID() string
	// 获取连接类型
	GetType() string
	// 检查连接状态
	IsClosed() bool
	// 获取最后活跃时间
	GetLastActiveTime() time.Time
	// 检查是否过期
	IsStale(timeout time.Duration) bool
}

type configGetter interface {
	Config() *tts.Config
}

// ConnectionHandler 连接处理器结构
type ConnectionHandler struct {
	// 确保实现 AsrEventListener 接口
	_                providers.AsrEventListener
	config           *configs.Config
	logger           *utils.Logger
	conn             Connection
	closeOnce        sync.Once
	taskMgr          *task.TaskManager
	authManager      *auth.AuthManager // 认证管理器
	safeCallbackFunc func(func(*ConnectionHandler)) func()
	providers        struct {
		asr   providers.ASRProvider
		llm   providers.LLMProvider
		tts   providers.TTSProvider
		vlllm *vlllm.Provider // VLLLM提供者，可选
	}

	initailVoice string // 初始语音名称

	// 会话相关
	sessionID     string            // 设备与服务端会话ID
	deviceID      string            // 设备ID
	clientId      string            // 客户端ID
	headers       map[string]string // HTTP头部信息
	transportType string            // 传输类型

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
	quickReplyCache     *utils.QuickReplyCache

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

	mcpResultHandlers map[string]func(interface{}) // MCP处理器映射
	ctx               context.Context
}

// NewConnectionHandler 创建新的连接处理器
func NewConnectionHandler(
	config *configs.Config,
	providerSet *pool.ProviderSet,
	logger *utils.Logger,
	req *http.Request,
	ctx context.Context,
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

		ctx: ctx,

		headers: make(map[string]string),
	}

	for key, values := range req.Header {
		if len(values) > 0 {
			handler.headers[key] = values[0] // 取第一个值
		}
		if key == "Device-Id" {
			handler.deviceID = values[0] // 设备ID
		}
		if key == "Client-Id" {
			handler.clientId = values[0] // 客户端ID
		}
		if key == "Session-Id" {
			handler.sessionID = values[0] // 会话ID
		}
		if key == "Transport-Type" {
			handler.transportType = values[0] // 传输类型
		}
		logger.Info("HTTP头部信息: %s: %s", key, values[0])
	}

	if handler.sessionID == "" {
		if handler.deviceID == "" {
			handler.sessionID = uuid.New().String() // 如果没有设备ID，则生成新的会话ID
		} else {
			handler.sessionID = "device-" + strings.Replace(handler.deviceID, ":", "_", -1)
		}
	}

	// 正确设置providers
	if providerSet != nil {
		handler.providers.asr = providerSet.ASR
		handler.providers.llm = providerSet.LLM
		handler.providers.tts = providerSet.TTS
		handler.providers.vlllm = providerSet.VLLLM
		handler.mcpManager = providerSet.MCP
	}

	ttsProvider := "default" // 默认TTS提供者名称
	voiceName := "default"
	if getter, ok := handler.providers.tts.(configGetter); ok {
		ttsProvider = getter.Config().Type
		voiceName = getter.Config().Voice
		handler.initailVoice = voiceName // 保存初始语音名称
	}
	logger.Info("使用TTS提供者: %s, 语音名称: %s", ttsProvider, voiceName)
	handler.quickReplyCache = utils.NewQuickReplyCache(ttsProvider, voiceName)

	// 初始化对话管理器
	handler.dialogueManager = chat.NewDialogueManager(handler.logger, nil)
	handler.dialogueManager.SetSystemMessage(config.DefaultPrompt)
	handler.functionRegister = function.NewFunctionRegistry()
	handler.initMCPResultHandlers()

	return handler
}

func (h *ConnectionHandler) SetTaskCallback(callback func(func(*ConnectionHandler)) func()) {
	h.safeCallbackFunc = callback
}

func (h *ConnectionHandler) SubmitTask(taskType string, params map[string]interface{}) {
	_task, id := task.NewTask(h.ctx, "", params)
	h.LogInfo(fmt.Sprintf("提交任务: %s, ID: %s, 参数: %v", _task.Type, id, params))
	// 创建安全回调用于任务完成时调用
	var taskCallback func(result interface{})
	if h.safeCallbackFunc != nil {
		taskCallback = func(result interface{}) {
			fmt.Print("任务完成回调: ")
			safeCallback := h.safeCallbackFunc(func(handler *ConnectionHandler) {
				// 处理任务完成逻辑
				handler.handleTaskComplete(_task, id, result)
			})
			// 执行安全回调
			if safeCallback != nil {
				safeCallback()
			}
		}
	}
	cb := task.NewCallBack(taskCallback)
	_task.Callback = cb
	h.taskMgr.SubmitTask(h.sessionID, _task)
}

func (h *ConnectionHandler) handleTaskComplete(task *task.Task, id string, result interface{}) {
	h.LogInfo(fmt.Sprintf("任务 %s 完成，ID: %s, %v", task.Type, id, result))
}

func (h *ConnectionHandler) LogInfo(msg string) {
	if h.logger != nil {
		h.logger.Info(msg, map[string]interface{}{
			"device": h.deviceID,
		})
	}
}
func (h *ConnectionHandler) LogError(msg string) {
	if h.logger != nil {
		h.logger.Error(msg, map[string]interface{}{
			"device": h.deviceID,
		})
	}
}

// Handle 处理WebSocket连接
func (h *ConnectionHandler) Handle(conn Connection) {
	defer conn.Close()

	h.conn = conn

	// 启动消息处理协程
	go h.processClientAudioMessagesCoroutine() // 添加客户端音频消息处理协程
	go h.processClientTextMessagesCoroutine()  // 添加客户端文本消息处理协程
	go h.processTTSQueueCoroutine()            // 添加TTS队列处理协程
	go h.sendAudioMessageCoroutine()           // 添加音频消息发送协程

	// 优化后的MCP管理器处理
	if h.mcpManager == nil {
		h.LogError("没有可用的MCP管理器")
		return

	} else {
		h.LogInfo("使用从资源池获取的MCP管理器，快速绑定连接")
		// 池化的管理器已经预初始化，只需要绑定连接
		params := map[string]interface{}{
			"session_id": h.sessionID,
			"vision_url": h.config.Web.VisionURL,
			"device_id":  h.deviceID,
			"client_id":  h.clientId,
			"token":      h.config.Server.Token,
		}
		if err := h.mcpManager.BindConnection(conn, h.functionRegister, params); err != nil {
			h.LogError(fmt.Sprintf("绑定MCP管理器连接失败: %v", err))
			return
		}
		// 不需要重新初始化服务器，只需要确保连接相关的服务正常
		h.LogInfo("MCP管理器连接绑定完成，跳过重复初始化")
	}

	// 主消息循环
	for {
		select {
		case <-h.stopChan:
			return
		default:
			messageType, message, err := conn.ReadMessage(h.stopChan)
			if err != nil {
				h.LogError(fmt.Sprintf("读取消息失败: %v, 退出主消息循环", err))
				return
			}

			if err := h.handleMessage(messageType, message); err != nil {
				h.LogError(fmt.Sprintf("处理消息失败: %v", err))
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
				h.LogError(fmt.Sprintf("处理文本数据失败: %v", err))
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
			if h.closeAfterChat {
				continue
			}
			if err := h.providers.asr.AddAudio(audioData); err != nil {
				h.LogError(fmt.Sprintf("处理音频数据失败: %v", err))
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
	//h.LogInfo(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
	if h.providers.asr.GetSilenceCount() >= 2 {
		h.LogInfo("检测到连续两次静音，结束对话")
		h.closeAfterChat = true // 如果连续两次静音，则结束对话
		result = "长时间未检测到用户说话，请礼貌的结束对话"
	}
	if h.clientListenMode == "auto" {
		if result == "" {
			return false
		}
		h.LogInfo(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
		h.handleChatMessage(context.Background(), result)
		return true
	} else if h.clientListenMode == "manual" {
		h.client_asr_text += result
		if result != "" {
			h.LogInfo(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, h.client_asr_text))
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
		h.LogInfo(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
		h.handleChatMessage(context.Background(), result)
		return true
	}
	return false
}

// clientAbortChat 处理中止消息
func (h *ConnectionHandler) clientAbortChat() error {
	h.LogInfo("收到客户端中止消息，停止语音识别")
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
			h.LogInfo("收到客户端退出意图，准备结束对话")
			h.Close() // 直接关闭连接
			return true
		}
	}
	return false
}

func (h *ConnectionHandler) quickReplyWakeUpWords(text string) bool {
	// 检查是否包含唤醒词
	if !h.config.QuickReply || h.talkRound != 1 {
		return false
	}
	if !utils.IsWakeUpWord(text) {
		return false
	}

	repalyWords := h.config.QuickReplyWords
	reply_text := utils.RandomSelectFromArray(repalyWords)
	h.tts_last_text_index = 1 // 重置文本索引
	h.SpeakAndPlay(reply_text, 1, h.talkRound)

	return true
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
	h.LogInfo(fmt.Sprintf("开始新的对话轮次: %d", currentRound))

	// 普通文本消息处理流程
	// 立即发送 stt 消息
	err := h.sendSTTMessage(text)
	if err != nil {
		h.LogError(fmt.Sprintf("发送STT消息失败: %v", err))
		return fmt.Errorf("发送STT消息失败: %v", err)
	}

	// 发送tts start状态
	if err := h.sendTTSMessage("start", "", 0); err != nil {
		h.LogError(fmt.Sprintf("发送TTS开始状态失败: %v", err))
		return fmt.Errorf("发送TTS开始状态失败: %v", err)
	}

	// 发送思考状态的情绪
	if err := h.sendEmotionMessage("thinking"); err != nil {
		h.LogError(fmt.Sprintf("发送思考状态情绪消息失败: %v", err))
		return fmt.Errorf("发送情绪消息失败: %v", err)
	}

	h.LogInfo("收到聊天消息: " + text)

	if h.quickReplyWakeUpWords(text) {
		return nil
	}

	// 添加用户消息到对话历史
	h.dialogueManager.Put(chat.Message{
		Role:    "user",
		Content: text,
	})

	return h.genResponseByLLM(ctx, h.dialogueManager.GetLLMDialogue(), currentRound)
}

func (h *ConnectionHandler) genResponseByLLM(ctx context.Context, messages []providers.Message, round int) error {
	defer func() {
		if r := recover(); r != nil {
			h.LogError(fmt.Sprintf("genResponseByLLM发生panic: %v", r))
			errorMsg := "抱歉，处理您的请求时发生了错误"
			h.tts_last_text_index = 1 // 重置文本索引
			h.SpeakAndPlay(errorMsg, 1, round)
		}
	}()

	llmStartTime := time.Now()
	//h.logger.Info("开始生成LLM回复, round:%d ", round)
	for _, msg := range messages {
		_ = msg
		//msg.Print()
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
			h.LogError(fmt.Sprintf("LLM响应错误: %s", response.Error))
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
				h.LogError(fmt.Sprintf("检测到LLM服务异常: %s", content))
				errorMsg := "抱歉，服务暂时不可用，请稍后再试"
				h.tts_last_text_index = 1 // 重置文本索引
				h.SpeakAndPlay(errorMsg, 1, round)
				return fmt.Errorf("LLM服务异常")
			}

			if toolCallFlag {
				continue
			}

			responseMessage = append(responseMessage, content)
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
					h.LogInfo(fmt.Sprintf("LLM回复耗时 %s 生成第一句话【%s】, round: %d", llmSpentTime, segment, round))
				} else {
					h.LogInfo(fmt.Sprintf("LLM回复分段: %s, index: %d, round:%d", segment, textIndex, round))
				}
				h.tts_last_text_index = textIndex
				err := h.SpeakAndPlay(segment, textIndex, round)
				if err != nil {
					h.LogError(fmt.Sprintf("播放LLM回复分段失败: %v", err))
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
				argumentsJson, err := json.Marshal(a["arguments"])
				if err != nil {
					h.LogError(fmt.Sprintf("函数调用参数解析失败: %v", err))
				}
				functionArguments = string(argumentsJson)
				functionID = uuid.New().String()
			} else {
				bHasError = true
			}
			if bHasError {
				h.LogError(fmt.Sprintf("函数调用参数解析失败: %v", err))
			}
		}
		if !bHasError {
			// 清空responseMessage
			responseMessage = []string{}
			arguments := make(map[string]interface{})
			if err := json.Unmarshal([]byte(functionArguments), &arguments); err != nil {
				h.LogError(fmt.Sprintf("函数调用参数解析失败: %v", err))
			}
			functionCallData := map[string]interface{}{
				"id":        functionID,
				"name":      functionName,
				"arguments": functionArguments,
			}
			h.LogInfo(fmt.Sprintf("函数调用: %v", arguments))
			if h.mcpManager.IsMCPTool(functionName) {
				// 处理MCP函数调用
				result, err := h.mcpManager.ExecuteTool(ctx, functionName, arguments)
				if err != nil {
					h.LogError(fmt.Sprintf("MCP函数调用失败: %v", err))
					if result == nil {
						result = "MCP工具调用失败"
					}
				}
				// 判断result 是否是types.ActionResponse类型
				if actionResult, ok := result.(types.ActionResponse); ok {
					h.handleFunctionResult(actionResult, functionCallData, textIndex)
				} else {
					h.LogInfo(fmt.Sprintf("MCP函数调用结果: %v", result))
					actionResult := types.ActionResponse{
						Action: types.ActionTypeReqLLM, // 动作类型
						Result: result,                 // 动作产生的结果
					}
					h.handleFunctionResult(actionResult, functionCallData, textIndex)
				}

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
			h.LogInfo(fmt.Sprintf("LLM回复分段[剩余文本]: %s, index: %d, round:%d", remainingText, textIndex, round))
			h.tts_last_text_index = textIndex
			h.SpeakAndPlay(remainingText, textIndex, round)
		}
	} else {
		h.logger.Debug("无剩余文本需要处理: fullResponse长度=%d, processedChars=%d", len(fullResponse), processedChars)
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

func (h *ConnectionHandler) addToolCallMessage(toolResultText string, functionCallData map[string]interface{}) {

	functionID := functionCallData["id"].(string)
	functionName := functionCallData["name"].(string)
	functionArguments := functionCallData["arguments"].(string)
	h.LogInfo(fmt.Sprintf("函数调用结果: %s", toolResultText))
	h.LogInfo(fmt.Sprintf("函数调用参数: %s", functionArguments))
	h.LogInfo(fmt.Sprintf("函数调用名称: %s", functionName))
	h.LogInfo(fmt.Sprintf("函数调用ID: %s", functionID))

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
		Content:    toolResultText,
	})
}

func (h *ConnectionHandler) handleFunctionResult(result types.ActionResponse, functionCallData map[string]interface{}, textIndex int) {
	switch result.Action {
	case types.ActionTypeError:
		h.LogError(fmt.Sprintf("函数调用错误: %v", result.Result))
	case types.ActionTypeNotFound:
		h.LogError(fmt.Sprintf("函数未找到: %v", result.Result))
	case types.ActionTypeNone:
		h.LogInfo(fmt.Sprintf("函数调用无操作: %v", result.Result))
	case types.ActionTypeResponse:
		h.LogInfo(fmt.Sprintf("函数调用直接回复: %v", result.Response))
		h.SystemSpeak(result.Response.(string))
	case types.ActionTypeCallHandler:
		resultStr := h.handleMCPResultCall(result)
		h.addToolCallMessage(resultStr, functionCallData)
	case types.ActionTypeReqLLM:
		h.LogInfo(fmt.Sprintf("函数调用后请求LLM: %v", result.Result))
		text, ok := result.Result.(string)
		if ok && len(text) > 0 {
			h.addToolCallMessage(text, functionCallData)
			h.genResponseByLLM(context.Background(), h.dialogueManager.GetLLMDialogue(), h.talkRound)

		} else {
			h.LogError(fmt.Sprintf("函数调用结果解析失败: %v", result.Result))
			// 发送错误消息
			errorMessage := fmt.Sprintf("函数调用结果解析失败 %v", result.Result)
			h.SystemSpeak(errorMessage)
		}
	}
}

func (h *ConnectionHandler) SystemSpeak(text string) error {
	if text == "" {
		h.logger.Warn("SystemSpeak 收到空文本，无法合成语音")
		return errors.New("收到空文本，无法合成语音")
	}
	texts := utils.SplitByPunctuation(text)
	index := 0
	for _, item := range texts {
		index++
		h.tts_last_text_index = index // 重置文本索引
		h.SpeakAndPlay(item, index, h.talkRound)
	}
	return nil
}

// isNeedAuth 判断是否需要验证
func (h *ConnectionHandler) isNeedAuth() bool {
	if !h.config.Server.Auth.Enabled {
		return false
	}
	return !h.isDeviceVerified
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
	h.LogInfo("服务端停止说话")
	atomic.StoreInt32(&h.serverVoiceStop, 1)
	h.cleanTTSAndAudioQueue(false)
}

func (h *ConnectionHandler) deleteAudioFileIfNeeded(filepath string, reason string) {
	if !h.config.DeleteAudio || filepath == "" {
		return
	}

	// 检查是否为快速回复缓存文件，如果是则不删除
	if h.quickReplyCache != nil && h.quickReplyCache.IsCachedFile(filepath) {
		h.LogInfo(fmt.Sprintf(reason+" 跳过删除缓存音频文件: %s", filepath))
		return
	}

	// 检查是否是音乐文件，如果是则不删除
	if utils.IsMusicFile(filepath) {
		h.LogInfo(fmt.Sprintf(reason+" 跳过删除音乐文件: %s", filepath))
		return
	}

	// 删除非缓存音频文件
	if err := os.Remove(filepath); err != nil {
		h.LogError(fmt.Sprintf(reason+" 删除音频文件失败: %v", err))
	} else {
		h.logger.Debug(fmt.Sprintf(reason+" 已删除音频文件: %s", filepath))
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

	if utils.IsQuickReplyHit(text, h.config.QuickReplyWords) {
		// 尝试从缓存查找音频文件
		if cachedFile := h.quickReplyCache.FindCachedAudio(text); cachedFile != "" {
			h.LogInfo(fmt.Sprintf("使用缓存的快速回复音频: %s", cachedFile))
			filepath = cachedFile
			return
		}
	}
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
		h.LogError(fmt.Sprintf("TTS转换失败:text(%s) %v", text, err))
		return
	} else {
		h.logger.Debug(fmt.Sprintf("TTS转换成功: text(%s), index(%d) %s", text, textIndex, filepath))
		// 如果是快速回复词，保存到缓存
		if utils.IsQuickReplyHit(text, h.config.QuickReplyWords) {
			if err := h.quickReplyCache.SaveCachedAudio(text, filepath); err != nil {
				h.LogError(fmt.Sprintf("保存快速回复音频失败: %v", err))
			} else {
				h.LogInfo(fmt.Sprintf("成功缓存快速回复音频: %s", text))
			}
		}
	}
	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.LogInfo(fmt.Sprintf("processTTSTask 服务端语音停止, 不再发送音频数据：%s", text))
		// 服务端语音停止时，根据配置删除已生成的音频文件
		h.deleteAudioFileIfNeeded(filepath, "服务端语音停止时")
		return
	}

	if textIndex == 1 {
		now := time.Now()
		ttsSpentTime := now.Sub(ttsStartTime)
		h.logger.Debug(fmt.Sprintf("TTS转换耗时: %s, 文本: %s, 索引: %d", ttsSpentTime, text, textIndex))
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
		h.logger.Warn("SpeakAndPlay 收到空文本，无法合成语音, %d, text:%s.", textIndex, originText)
		return errors.New("收到空文本，无法合成语音")
	}

	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.LogInfo(fmt.Sprintf("speakAndPlay 服务端语音停止, 不再发送音频数据：%s", text))
		text = ""
		return errors.New("服务端语音已停止，无法合成语音")
	}

	if len(text) > 255 {
		h.logger.Warn(fmt.Sprintf("文本过长，超过255字符限制，截断合成语音: %s", text))
		text = text[:255] // 截断文本
	}

	return nil
}

func (h *ConnectionHandler) clearSpeakStatus() {
	h.LogInfo("清除服务端讲话状态 ")
	h.tts_last_text_index = -1
	h.providers.asr.Reset() // 重置ASR状态
}

func (h *ConnectionHandler) closeOpusDecoder() {
	if h.opusDecoder != nil {
		if err := h.opusDecoder.Close(); err != nil {
			h.LogError(fmt.Sprintf("关闭Opus解码器失败: %v", err))
		}
		h.opusDecoder = nil
	}
}

func (h *ConnectionHandler) cleanTTSAndAudioQueue(bClose bool) error {
	msgPrefix := ""
	if bClose {
		msgPrefix = "关闭连接，"
	}
	// 终止tts任务，不再继续将文本加入到tts队列，清空ttsQueue队列
	for {
		select {
		case task := <-h.ttsQueue:
			h.LogInfo(fmt.Sprintf(msgPrefix+"丢弃一个TTS任务: %s", task.text))
		default:
			// 队列已清空，退出循环
			h.LogInfo(msgPrefix + "ttsQueue队列已清空，停止处理TTS任务,准备清空音频队列")
			goto clearAudioQueue
		}
	}

clearAudioQueue:
	// 终止audioMessagesQueue发送，清空队列里的音频数据
	for {
		select {
		case task := <-h.audioMessagesQueue:
			h.LogInfo(fmt.Sprintf(msgPrefix+"丢弃一个音频任务: %s", task.text))
			// 根据配置删除被丢弃的音频文件
			h.deleteAudioFileIfNeeded(task.filepath, msgPrefix+"丢弃音频任务时")
		default:
			// 队列已清空，退出循环
			h.LogInfo(msgPrefix + "audioMessagesQueue队列已清空，停止处理音频任务")
			return nil
		}
	}
}

// Close 清理资源
func (h *ConnectionHandler) Close() {
	h.closeOnce.Do(func() {
		close(h.stopChan)

		h.closeOpusDecoder()
		if h.providers.tts != nil {
			h.providers.tts.SetVoice(h.initailVoice) // 恢复初始语音
		}
		if h.providers.asr != nil {
			if err := h.providers.asr.Reset(); err != nil {
				h.LogError(fmt.Sprintf("重置ASR状态失败: %v", err))
			}
		}
		h.cleanTTSAndAudioQueue(true)
	})
}

// genResponseByVLLM 使用VLLLM处理包含图片的消息
func (h *ConnectionHandler) genResponseByVLLM(ctx context.Context, messages []providers.Message, imageData image.ImageData, text string, round int) error {
	h.logger.Info("开始生成VLLLM回复 %v", map[string]interface{}{
		"text":          text,
		"has_url":       imageData.URL != "",
		"has_data":      imageData.Data != "",
		"format":        imageData.Format,
		"message_count": len(messages),
	})

	// 使用VLLLM处理图片和文本
	responses, err := h.providers.vlllm.ResponseWithImage(ctx, h.sessionID, messages, imageData, text)
	if err != nil {
		h.LogError(fmt.Sprintf("VLLLM生成回复失败，尝试降级到普通LLM: %v", err))
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

	h.LogInfo(fmt.Sprintf("VLLLM回复处理完成 …%v", map[string]interface{}{
		"content_length": len(content),
		"text_segments":  textIndex,
	}))

	return nil
}
