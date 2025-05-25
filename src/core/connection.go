package core

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/chat"
	"xiaozhi-server-go/src/core/function"
	"xiaozhi-server-go/src/core/mcp"
	"xiaozhi-server-go/src/core/providers"
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
	taskMgr   *task.TaskManager
	providers struct {
		asr providers.ASRProvider
		llm providers.LLMProvider
		tts providers.TTSProvider
	}

	// 会话相关
	sessionID    string
	headers      map[string]string
	clientIP     string
	clientIPInfo map[string]interface{}

	// 客户端音频相关
	clientAudioFormat        string
	clientAudioSampleRate    int
	clientAudioChannels      int
	clientAudioFrameDuration int

	serverAudioFormat        string // 服务端音频格式
	serverAudioSampleRate    int
	serverAudioChannels      int
	serverAudioFrameDuration int

	// 状态标志
	clientAbort      bool
	clientListenMode string
	isDeviceVerified bool
	closeAfterChat   bool

	// 语音处理相关
	clientVoiceStop bool  // true客户端语音停止, 不再上传语音数据
	serverVoiceStop int32 // 1表示true服务端语音停止, 不再下发语音数据

	opusDecoder *utils.OpusDecoder // Opus解码器

	// 对话相关
	dialogueManager      *chat.DialogueManager
	tts_first_text_index int
	tts_last_text_index  int
	client_asr_text      string // 客户端ASR文本

	// 并发控制
	stopChan         chan struct{}
	clientAudioQueue chan []byte
	clientTextQueue  chan string

	// TTS任务队列
	ttsQueue chan struct {
		text      string
		textIndex int
	}

	audioMessagesQueue chan struct {
		filepath  string
		text      string
		textIndex int
	}

	// functions
	functionRegister *function.FunctionRegistry
	mcpManager       *mcp.Manager
}

// NewConnectionHandler 创建新的连接处理器
func NewConnectionHandler(
	config *configs.Config,
	providers struct {
		asr providers.ASRProvider
		llm providers.LLMProvider
		tts providers.TTSProvider
	},
	logger *utils.Logger,
) *ConnectionHandler {
	handler := &ConnectionHandler{
		config:           config,
		providers:        providers,
		logger:           logger,
		clientListenMode: "auto",
		stopChan:         make(chan struct{}),
		clientAudioQueue: make(chan []byte, 100),
		clientTextQueue:  make(chan string, 100),
		ttsQueue: make(chan struct {
			text      string
			textIndex int
		}, 100),
		audioMessagesQueue: make(chan struct {
			filepath  string
			text      string
			textIndex int
		}, 100),

		tts_last_text_index:  -1,
		tts_first_text_index: -1,

		serverAudioFormat:        "opus", // 默认使用Opus格式
		serverAudioSampleRate:    24000,
		serverAudioChannels:      1,
		serverAudioFrameDuration: 60,
	}

	// 初始化对话管理器
	handler.dialogueManager = chat.NewDialogueManager(handler.logger, nil)
	handler.functionRegister = function.NewFunctionRegistry()
	handler.mcpManager = mcp.NewManager(logger, handler.functionRegister)
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

	h.mcpManager.InitializeServers(context.Background())

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
				if h.closeAfterChat {
					return
				}
			}
		}
	}
}

// handleMessage 处理接收到的消息
func (h *ConnectionHandler) handleMessage(messageType int, message []byte) error {
	switch messageType {
	case 1: // 文本消息
		h.clientTextQueue <- string(message)
		return nil
	case 2: // 二进制消息（音频数据）
		if h.clientAudioFormat == "pcm" {
			// 直接将PCM数据放入队列
			h.clientAudioQueue <- message
		} else if h.clientAudioFormat == "opus" {
			// 检查是否初始化了opus解码器
			if h.opusDecoder != nil {
				// 解码opus数据为PCM
				decodedData, err := h.opusDecoder.Decode(message)
				if err != nil {
					h.logger.Error(fmt.Sprintf("解码Opus音频失败: %v", err))
					// 即使解码失败，也尝试将原始数据传递给ASR处理
					h.clientAudioQueue <- message
				} else {
					// 解码成功，将PCM数据放入队列
					h.logger.Debug(fmt.Sprintf("Opus解码成功: %d bytes -> %d bytes", len(message), len(decodedData)))
					if len(decodedData) > 0 {
						h.clientAudioQueue <- decodedData
					}
				}
			} else {
				// 没有解码器，直接传递原始数据
				h.clientAudioQueue <- message
			}
		}
		return nil
	default:
		h.logger.Error(fmt.Sprintf("未知的消息类型: %d", messageType))
		return fmt.Errorf("未知的消息类型: %d", messageType)
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
		h.logger.Info(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
		h.handleChatMessage(context.Background(), result)
		return false
	}
	return false
}

// processClientTextMessage 处理文本数据
func (h *ConnectionHandler) processClientTextMessage(ctx context.Context, text string) error {
	// 解析JSON消息
	var msgJSON interface{}
	if err := json.Unmarshal([]byte(text), &msgJSON); err != nil {
		return h.conn.WriteMessage(1, []byte(text))
	}

	// 检查是否为整数类型
	if _, ok := msgJSON.(float64); ok {
		return h.conn.WriteMessage(1, []byte(text))
	}

	// 解析为map类型处理具体消息
	msgMap, ok := msgJSON.(map[string]interface{})
	if !ok {
		return fmt.Errorf("消息格式错误")
	}

	// 根据消息类型分发处理
	msgType, ok := msgMap["type"].(string)
	if !ok {
		return fmt.Errorf("消息类型错误")
	}

	switch msgType {
	case "hello":
		return h.handleHelloMessage(msgMap)
	case "abort":
		return h.handleAbortMessage()
	case "listen":
		return h.handleListenMessage(msgMap)
	case "iot":
		return h.handleIotMessage(msgMap)
	case "chat":
		return h.handleChatMessage(ctx, text)
	case "vision":
		return h.handleVisionMessage(msgMap)
	default:
		return fmt.Errorf("未知的消息类型: %s", msgType)
	}
}

func (h *ConnectionHandler) handleVisionMessage(msgMap map[string]interface{}) error {
	// 处理视觉消息
	cmd := msgMap["cmd"].(string)
	if cmd == "read_img" {
	}
	return nil
}

// handleHelloMessage 处理欢迎消息
// 客户端会上传语音格式和采样率等信息
func (h *ConnectionHandler) handleHelloMessage(msgMap map[string]interface{}) error {
	h.logger.Info("收到客户端欢迎消息: " + fmt.Sprintf("%v", msgMap))
	// 获取客户端编码格式
	if audioParams, ok := msgMap["audio_params"].(map[string]interface{}); ok {
		if format, ok := audioParams["format"].(string); ok {
			h.logger.Info("客户端音频格式: " + format)
			h.clientAudioFormat = format
			if format == "pcm" {
				// 客户端使用PCM格式，服务端也使用PCM格式
				h.serverAudioFormat = "pcm"
				h.sendHelloMessage()
			}
		}
		if sampleRate, ok := audioParams["sample_rate"].(float64); ok {
			h.logger.Info("客户端采样率: " + fmt.Sprintf("%d", int(sampleRate)))
			h.clientAudioSampleRate = int(sampleRate)
		}
		if channels, ok := audioParams["channels"].(float64); ok {
			h.logger.Info("客户端声道数: " + fmt.Sprintf("%d", int(channels)))
			h.clientAudioChannels = int(channels)
		}
		if frameDuration, ok := audioParams["frame_duration"].(float64); ok {
			h.logger.Info("客户端帧时长: " + fmt.Sprintf("%d", int(frameDuration)))
			h.clientAudioFrameDuration = int(frameDuration)
		}
	}

	h.closeOpusDecoder()
	// 初始化opus解码器
	opusDecoder, err := utils.NewOpusDecoder(&utils.OpusDecoderConfig{
		SampleRate:  h.clientAudioSampleRate, // 客户端使用24kHz采样率
		MaxChannels: h.clientAudioChannels,   // 单声道音频
	})
	if err != nil {
		h.logger.Error(fmt.Sprintf("初始化Opus解码器失败: %v", err))
	} else {
		h.opusDecoder = opusDecoder
		h.logger.Info("Opus解码器初始化成功")
	}

	return nil
}

// handleAbortMessage 处理中止消息
func (h *ConnectionHandler) handleAbortMessage() error {
	h.clientAbort = true
	h.stopServerSpeak()
	return nil
}

// handleListenMessage 处理语音相关消息
func (h *ConnectionHandler) handleListenMessage(msgMap map[string]interface{}) error {
	// 处理mode参数
	if mode, ok := msgMap["mode"].(string); ok {
		h.clientListenMode = mode
		h.logger.Info(fmt.Sprintf("客户端拾音模式：%s", h.clientListenMode))
		h.providers.asr.SetListener(h)
	}

	// 处理state参数
	state, ok := msgMap["state"].(string)
	if !ok {
		return fmt.Errorf("listen消息缺少state参数")
	}

	switch state {
	case "start":
		h.clientVoiceStop = false
		h.client_asr_text = ""
	case "stop":
		h.clientVoiceStop = true
		h.logger.Info("客户端停止语音识别")
		if h.clientListenMode == "manual" {
			h.providers.asr.Finalize()
		}
	case "detect":

		// 处理text参数
		if text, ok := msgMap["text"].(string); ok {
			// TODO: 实现去除标点和长度的函数
			// _, text = removePunctuationAndLength(text)
			return h.handleChatMessage(context.Background(), text)
		}
	}
	return nil
}

// handleIotMessage 处理IOT设备消息
func (h *ConnectionHandler) handleIotMessage(msgMap map[string]interface{}) error {
	if descriptors, ok := msgMap["descriptors"].([]interface{}); ok {
		// 处理设备描述符
		// 这里需要实现具体的IOT设备描述符处理逻辑
		h.logger.Info(fmt.Sprintf("收到IOT设备描述符：%v", descriptors))
	}
	if states, ok := msgMap["states"].([]interface{}); ok {
		// 处理设备状态
		// 这里需要实现具体的IOT设备状态处理逻辑
		h.logger.Info(fmt.Sprintf("收到IOT设备状态：%v", states))
	}
	return nil
}

// sendEmotionMessage 发送情绪消息
func (h *ConnectionHandler) sendEmotionMessage(emotion string) error {
	data := map[string]interface{}{
		"type":       "llm",
		"text":       utils.GetEmotionEmoji(emotion),
		"emotion":    emotion,
		"session_id": h.sessionID,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化情绪消息失败: %v", err)
	}
	return h.conn.WriteMessage(1, jsonData)
}

// handleChatMessage 处理聊天消息
func (h *ConnectionHandler) handleChatMessage(ctx context.Context, text string) error {
	// 判断是否需要验证
	if h.isNeedAuth() {
		if err := h.checkAndBroadcastAuthCode(); err != nil {
			h.logger.Error(fmt.Sprintf("检查认证码失败: %v", err))
			return err
		}
		h.logger.Info("设备未认证，等待管理员认证")
		return nil
	}

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
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return h.genResponseByLLM(ctx, messages)
}

func (h *ConnectionHandler) genResponseByLLM(ctx context.Context, messages []providers.Message) error {
	h.logger.Info("开始生成LLM回复, 打印message")
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
			if !toolCallFlag {
				responseMessage = append(responseMessage, content)
			}

			if h.clientAbort {
				break
			}

			// 处理分段
			fullText := joinStrings(responseMessage)
			currentText := fullText[processedChars:]

			// 按标点符号分割
			if segment, chars := splitAtLastPunctuation(currentText); chars > 0 {
				textIndex++
				h.recode_first_last_text(segment, textIndex)
				h.SpeakAndPlay(segment, textIndex)
				processedChars += chars
			}
		}
	}

	if toolCallFlag {
		bHasError := false
		if functionID == "" {
			a := extract_json_from_string(contentArguments)
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
	remainingText := joinStrings(responseMessage)[processedChars:]
	if remainingText != "" {
		textIndex++
		h.recode_first_last_text(remainingText, textIndex)
		h.SpeakAndPlay(remainingText, textIndex)
	}

	// 分析回复并发送相应的情绪
	content := joinStrings(responseMessage)

	// 添加助手回复到对话历史
	h.dialogueManager.Put(chat.Message{
		Role:    "assistant",
		Content: content,
	})

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
		h.SpeakAndPlay(result.Response.(string), textIndex)
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
			h.genResponseByLLM(context.Background(), messages)
		}
	}
}

// extract_json_from_string 提取字符串中的 JSON 部分
func extract_json_from_string(input string) map[string]interface{} {
	pattern := `(\{.*\})`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(input)
	if len(matches) > 1 {
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(matches[1]), &result); err == nil {
			return result
		}
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

// checkAndBroadcastAuthCode 检查并广播认证码
func (h *ConnectionHandler) checkAndBroadcastAuthCode() error {
	// 这里简化了认证逻辑，实际需要根据具体需求实现
	text := "请联系管理员进行设备认证"
	return h.SpeakAndPlay(text, 0)
}

// processTTSQueueCoroutine 处理TTS队列
func (h *ConnectionHandler) processTTSQueueCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case task := <-h.ttsQueue:
			h.processTTSTask(task.text, task.textIndex)
		}
	}
}

func (h *ConnectionHandler) sendAudioMessageCoroutine() {
	for {
		select {
		case <-h.stopChan:
			return
		case task := <-h.audioMessagesQueue:
			h.sendAudioMessage(task.filepath, task.text, task.textIndex)
		}
	}
}

func (h *ConnectionHandler) sendAudioMessage(filepath string, text string, textIndex int) {
	if len(filepath) == 0 {
		return
	}

	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.logger.Info(fmt.Sprintf("sendAudioMessage 服务端语音停止, 不再发送音频数据：%s", text))
		return
	}

	defer func() {
		if textIndex == h.tts_last_text_index {
			h.sendTTSMessage("stop", "", textIndex)
			h.clearSpeakStatus()
		}
	}()

	var audioData [][]byte
	var duration float64
	var err error

	// 使用TTS提供者的方法将音频转为Opus格式
	if h.serverAudioFormat == "pcm" {
		h.logger.Info("服务端音频格式为PCM，直接发送")
		audioData, duration, err = utils.AudioToPCMData(filepath)
		if err != nil {
			h.logger.Error(fmt.Sprintf("音频转PCM失败: %v", err))
			return
		}
	} else if h.serverAudioFormat == "opus" {
		audioData, duration, err = utils.AudioToOpusData(filepath)
		if err != nil {
			h.logger.Error(fmt.Sprintf("音频转Opus失败: %v", err))
			return
		}
	}

	//fmt.Println("音频时长:", duration)

	// 发送TTS状态开始通知
	if err := h.sendTTSMessage("sentence_start", text, textIndex); err != nil {
		h.logger.Error(fmt.Sprintf("发送TTS开始状态失败: %v", err))
		return
	}

	// 发送音频数据
	for _, chunk := range audioData {
		if err := h.conn.WriteMessage(2, chunk); err != nil {
			h.logger.Error(fmt.Sprintf("发送Opus音频数据失败: %v", err))
			return
		}
	}
	h.logger.Info(fmt.Sprintf("TTS发送(%s): \"%s\" (索引:%d，时长:%f)", h.serverAudioFormat, text, textIndex, duration))
	now := time.Now()
	time.Sleep(time.Duration(duration*1000) * time.Millisecond)
	spent := time.Since(now)
	h.logger.Info(fmt.Sprintf("%s音频数据发送完成, 已休眠: %v", h.serverAudioFormat, spent))
	// 发送TTS状态结束通知
	if err := h.sendTTSMessage("sentence_end", text, textIndex); err != nil {
		h.logger.Error(fmt.Sprintf("发送TTS结束状态失败: %v", err))
		return
	}
}

// 服务端打断说话
func (h *ConnectionHandler) stopServerSpeak() {
	h.logger.Info("stopServerSpeak 服务端停止说话")
	atomic.StoreInt32(&h.serverVoiceStop, 1)
	// 终止tts任务，不再继续将文本加入到tts队列，清空ttsQueue队列
	for {
		select {
		case task := <-h.ttsQueue:
			h.logger.Info(fmt.Sprintf("丢弃一个TTS任务: %s", task.text))
		default:
			// 队列已清空，退出循环
			goto clearAudioQueue
		}
	}

clearAudioQueue:
	// 终止audioMessagesQueue发送，清空队列里的音频数据
	for {
		select {
		case task := <-h.audioMessagesQueue:
			h.logger.Info(fmt.Sprintf("丢弃一个音频任务: %s", task.text))
		default:
			// 队列已清空，退出循环
			return
		}
	}
}

// processTTSTask 处理单个TTS任务
func (h *ConnectionHandler) processTTSTask(text string, textIndex int) {
	if text == "" {
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
		return
	}
	h.audioMessagesQueue <- struct {
		filepath  string
		text      string
		textIndex int
	}{filepath, text, textIndex}
}

// speakAndPlay 合成并播放语音
func (h *ConnectionHandler) SpeakAndPlay(text string, textIndex int) error {
	if text == "" {
		return nil
	}
	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.logger.Info(fmt.Sprintf("speakAndPlay 服务端语音停止, 不再发送音频数据：%s", text))
		return nil
	}
	// 将任务加入队列，不阻塞当前流程
	h.ttsQueue <- struct {
		text      string
		textIndex int
	}{text, textIndex}

	return nil
}

func (h *ConnectionHandler) sendTTSMessage(state string, text string, textIndex int) error {
	// 发送TTS状态结束通知
	stateMsg := map[string]interface{}{
		"type":        "tts",
		"state":       state,
		"session_id":  h.sessionID,
		"text":        text,
		"index":       textIndex,
		"audio_codec": "opus", // 标识使用Opus编码
	}
	data, err := json.Marshal(stateMsg)
	if err != nil {
		return fmt.Errorf("序列化%s状态失败: %v", state, err)
	}
	if err := h.conn.WriteMessage(1, data); err != nil {
		return fmt.Errorf("发送%s状态失败: %v", state, err)
	}
	return nil
}

func (h *ConnectionHandler) sendSTTMessage(text string) error {

	// 立即发送 stt 消息
	sttMsg := map[string]interface{}{
		"type":       "stt",
		"text":       text,
		"session_id": h.sessionID,
	}
	jsonData, err := json.Marshal(sttMsg)
	if err != nil {
		return fmt.Errorf("序列化 STT 消息失败: %v", err)
	}
	if err := h.conn.WriteMessage(1, jsonData); err != nil {
		return fmt.Errorf("发送 STT 消息失败: %v", err)
	}

	return nil
}

func (h *ConnectionHandler) clearSpeakStatus() {
	h.logger.Info("清除服务端讲话状态 ")
	h.tts_last_text_index = -1
	h.tts_first_text_index = -1
	if h.clientListenMode != "realtime" {
		h.providers.asr.Reset() // 重置ASR状态
	}
}

func (h *ConnectionHandler) recode_first_last_text(text string, text_index int) {
	if h.tts_first_text_index == -1 {
		h.logger.Info(fmt.Sprintf("大模型说出第一句话:%s", text))
		h.tts_first_text_index = text_index
	}

	h.tts_last_text_index = text_index
}

// joinStrings 连接字符串切片
func joinStrings(strs []string) string {
	var result string
	for _, s := range strs {
		result += s
	}
	return result
}

// splitAtLastPunctuation 在最后一个标点符号处分割文本
func splitAtLastPunctuation(text string) (string, int) {
	punctuations := []string{"。", "？", "！", "；", "："}
	lastIndex := -1

	for _, punct := range punctuations {
		if idx := strings.LastIndex(text, punct); idx > lastIndex {
			lastIndex = idx
		}
	}

	if lastIndex == -1 {
		return "", 0
	}

	return text[:lastIndex+len("。")], lastIndex + len("。")
}

// sendHelloMessage 发送欢迎消息
func (h *ConnectionHandler) sendHelloMessage() error {
	hello := make(map[string]interface{})
	hello["type"] = "hello"
	hello["version"] = 1
	hello["transport"] = "websocket"
	hello["session_id"] = h.sessionID
	hello["audio_params"] = map[string]interface{}{
		"format":         h.serverAudioFormat,
		"sample_rate":    h.serverAudioSampleRate,
		"channels":       h.serverAudioChannels,
		"frame_duration": h.serverAudioFrameDuration,
	}
	data, err := json.Marshal(hello)
	if err != nil {
		return fmt.Errorf("序列化欢迎消息失败: %v", err)
	}

	return h.conn.WriteMessage(1, data)
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
	close(h.stopChan)
	close(h.clientAudioQueue)
	close(h.clientTextQueue)

	h.closeOpusDecoder()
}
