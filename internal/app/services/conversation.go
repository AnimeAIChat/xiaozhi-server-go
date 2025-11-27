package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"xiaozhi-server-go/internal/domain/config/service"
	domainllm "xiaozhi-server-go/internal/domain/llm"
	domainllminter "xiaozhi-server-go/internal/domain/llm/inter"
	domainmcp "xiaozhi-server-go/internal/domain/mcp"
	domaintts "xiaozhi-server-go/internal/domain/tts"
	"xiaozhi-server-go/internal/platform/logging"
	coreproviders "xiaozhi-server-go/internal/core/providers"
	"xiaozhi-server-go/internal/utils"
)

// ConversationService 处理对话相关的业务逻辑
type ConversationService struct {
	llmManager    *domainllm.Manager
	ttsManager    *domaintts.Manager
	mcpManager    *domainmcp.Manager
	configService *service.ConfigService
	logger        *logging.Logger
	llmProvider   coreproviders.LLMProvider

	// 会话状态
	sessionID string
	deviceID  string
	userID    string

	// 对话历史
	conversationHistory []domainllminter.Message

	// 回调函数
	onSpeakAndPlay func(text string, textIndex int, round int) error
	onSendMessage  func(messageType int, data []byte) error
}

// ConversationConfig 对话服务配置
type ConversationConfig struct {
	SessionID     string
	DeviceID      string
	UserID        string
	LLMManager    *domainllm.Manager
	TTSManager    *domaintts.Manager
	MCPManager    *domainmcp.Manager
	ConfigService *service.ConfigService
	Logger        *logging.Logger
	LLMProvider   coreproviders.LLMProvider
}

// NewConversationService 创建新的对话服务
func NewConversationService(config *ConversationConfig) *ConversationService {
	return &ConversationService{
		llmManager:    config.LLMManager,
		ttsManager:    config.TTSManager,
		mcpManager:    config.MCPManager,
		configService: config.ConfigService,
		logger:        config.Logger,
		llmProvider:   config.LLMProvider,
		sessionID:     config.SessionID,
		deviceID:      config.DeviceID,
		userID:        config.UserID,
	}
}

// HandleChatMessage 处理聊天消息
func (s *ConversationService) HandleChatMessage(ctx context.Context, text string, round int) error {
	if text == "" {
		s.logger.Legacy().Warn("收到空聊天消息，忽略")
		return fmt.Errorf("聊天消息为空")
	}

	// 检查退出意图
	if s.QuitIntent(text) {
		return nil
	}
}

// QuitIntent 检查用户意图是否是退出
func (s *ConversationService) QuitIntent(text string) bool {
	// 读取配置中的退出命令
	exitCommands := s.config.System.CMDExit
	if exitCommands == nil {
		return false
	}

	// 移除标点符号，确保匹配准确
	cleanText := utils.RemoveAllPunctuation(text)

	// 检查是否包含退出命令（支持部分匹配）
	for _, cmd := range exitCommands {
		s.logger.Debug("检查退出命令: %s,%s", cmd, cleanText)
		// 判断包含关系
		if strings.Contains(cleanText, cmd) {
			s.logger.Legacy().Info("[客户端] [退出意图] 收到，准备结束对话")
			return true
		}
	}

	// 额外检查一些常见的退出表达方式
	exitPhrases := []string{"退下", "再见", "拜拜", "不聊了", "结束了", "结束吧"}
	for _, phrase := range exitPhrases {
		if strings.Contains(cleanText, phrase) {
			s.logger.Legacy().Info("[客户端] [退出意图] 收到，准备结束对话")
			return true
		}
	}
	return false

	// TODO: 检测是否是唤醒词，实现快速响应
	// if utils.IsWakeUpWord(text) {
	//     s.logger.Legacy().Info(fmt.Sprintf("[唤醒] [检测成功] 文本 '%s' 匹配唤醒词模式", text))
	//     return s.handleWakeUpMessage(ctx, text)
	// } else {
	//     s.logger.Legacy().Info(fmt.Sprintf("[唤醒] [检测失败] 文本 '%s' 不匹配唤醒词模式", text))
	// }

	// TODO: 记录正在处理对话的状态
	s.logger.Legacy().Info("[对话] [开始处理] 文本: %s", text)

	// TODO: 清空音频队列，防止后续音频数据触发新的ASR识别

	// TODO: 增加对话轮次
	// currentRound := s.talkRound + 1
	if round <= 0 {
		round = 1
	}

	s.logger.Legacy().Info("[对话] [轮次 %d] 开始新的对话轮次", round)

	// TODO: 普通文本消息处理流程
	// 立即发送 stt 消息
	// TODO: 发送STT消息

	s.logger.Legacy().Info("[聊天] [消息 %s]", text)

	// TODO: 发送tts start状态
	// TODO: 发送思考状态的情绪

	// 添加用户消息到对话历史
	s.AddUserMessage(text)

	// TODO: 调用LLM生成回复
	return s.genResponseByLLM(ctx, round)
}

// genResponseByLLM 使用LLM生成回复
func (s *ConversationService) genResponseByLLM(ctx context.Context, round int) error {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Legacy().Error("genResponseByLLM发生panic: %v", r)
			_ = "抱歉，处理您的请求时发生了错误" // TODO: 播放错误消息
		}
	}()

	_ = time.Now() // TODO: 记录LLM开始时间

	// TODO: 发布LLM开始事件

	// 准备消息和工具
	messages := s.getConversationHistory()
	tools := s.getAvailableTools()

	s.logger.Legacy().Info("[LLM] 准备调用，消息数量: %d, 工具数量: %d", len(messages), len(tools))
	for i, msg := range messages {
		s.logger.Legacy().Debug("[LLM] 消息 %d: %s - %s", i, msg.Role, msg.Content)
	}

	// 调用LLM生成回复
	s.logger.Legacy().Info("[LLM] 开始调用LLM")

	var (
		responses <-chan domainllminter.ResponseChunk
		err       error
	)

	if s.llmProvider != nil {
		responses, err = s.streamResponseWithProvider(ctx, messages, tools)
	} else {
		responses, err = s.llmManager.Response(ctx, s.sessionID, messages, tools)
	}
	if err != nil {
		s.logger.Legacy().Error("[LLM] 调用失败: %v", err)
		// TODO: 发布LLM错误事件
		return fmt.Errorf("LLM生成回复失败: %v", err)
	}
	s.logger.Legacy().Info("[LLM] 调用成功，开始处理响应")

	// 处理回复
	var responseMessage []string
	processedChars := 0
	textIndex := 0
	fullResponse := ""

	// TODO: 重置语音停止标志
	// atomic.StoreInt32(&s.serverVoiceStop, 0)

	// 处理流式响应
	toolCallFlag := false
	contentArguments := ""

	for response := range responses {
		content := response.Content
		toolCall := response.ToolCalls

		s.logger.Legacy().Debug("[LLM] 收到响应块: content='%s', toolCalls=%d, isDone=%v", content, len(toolCall), response.IsDone)

		if response.Error != nil {
			s.logger.Legacy().Error("LLM响应错误: %s", response.Error.Error())
			_ = "抱歉，服务暂时不可用，请稍后再试" // TODO: 播放错误消息
			return fmt.Errorf("LLM响应错误: %s", response.Error)
		}

		if content != "" {
			// 累加content_arguments
			contentArguments += content
			fullResponse += content
		}

		if !toolCallFlag && strings.HasPrefix(contentArguments, "<tool_call>") {
			toolCallFlag = true
		}

		if len(toolCall) > 0 {
			toolCallFlag = true
			// TODO: 处理工具调用参数
			// functionID = toolCall[0].ID
			// functionName = toolCall[0].Function.Name
			// functionArguments += toolCall[0].Function.Arguments
		}

		if content != "" {
			if strings.Contains(content, "服务响应异常") {
				s.logger.Legacy().Error("检测到LLM服务异常: %s", content)
				_ = "抱歉，LLM服务暂时不可用，请稍后再试" // TODO: 播放错误消息
				return fmt.Errorf("LLM服务异常")
			}

			if toolCallFlag {
				continue
			}

			responseMessage = append(responseMessage, content)
			// 实时处理文本分段
			if err := s.processTextSegment(fullResponse, &processedChars, &textIndex, round); err != nil {
				s.logger.Legacy().Error("处理文本分段失败: %v", err)
			}
		}

		// 检查是否完成
		if response.IsDone {
			break
		}
	}

	// TODO: 处理工具调用
	if toolCallFlag {
		// TODO: 实现工具调用处理逻辑
		s.logger.Legacy().Info("检测到工具调用，但暂未实现处理逻辑")
	}

	// 处理剩余文本
	if len(fullResponse) > processedChars {
		remainingText := fullResponse[processedChars:]
		if remainingText != "" {
			textIndex++
			// TODO: 调用语音播放回调
			if s.onSpeakAndPlay != nil {
				s.onSpeakAndPlay(remainingText, textIndex, round)
			}
		}
	}

	// TODO: 分析回复并发送相应的情绪
	_ = strings.Join(responseMessage, "") // content

	// TODO: 添加助手回复到对话历史

	// TODO: 发布LLM完成事件

	return nil
}

// SetCallbacks 设置回调函数
func (s *ConversationService) SetCallbacks(
	onSpeakAndPlay func(text string, textIndex int, round int) error,
	onSendMessage func(messageType int, data []byte) error,
) {
	s.onSpeakAndPlay = onSpeakAndPlay
	s.onSendMessage = onSendMessage
}

// OnSendAudioMessage 发送音频消息
func (s *ConversationService) OnSendAudioMessage(filepath string, text string, textIndex int, round int) {
	// TODO: 实现音频消息发送逻辑
}

// ProcessTTSTask 处理TTS任务
func (s *ConversationService) ProcessTTSTask(text string, textIndex int, round int, filepath string) {
	// TODO: 实现TTS任务处理逻辑
}

// getConversationHistory 获取对话历史
func (s *ConversationService) getConversationHistory() []domainllminter.Message {
	// 如果没有对话历史，创建一个系统消息
	if len(s.conversationHistory) == 0 {
		return []domainllminter.Message{
			{
				Role:    "system",
				Content: "你是一个智能助手，请用中文回复用户的问题。",
			},
		}
	}
	return s.conversationHistory
}

// getAvailableTools 获取可用的工具列表
func (s *ConversationService) getAvailableTools() []domainllminter.Tool {
	// 从MCP管理器获取完整的工具定义
	openaiTools := s.mcpManager.GetAvailableTools()
	s.logger.Legacy().Info("[MCP] 获取到 %d 个工具", len(openaiTools))
	if len(openaiTools) == 0 {
		return []domainllminter.Tool{}
	}

	// 转换为domain工具格式
	tools := make([]domainllminter.Tool, 0, len(openaiTools))
	for _, tool := range openaiTools {
		s.logger.Legacy().Info("[MCP] 工具: %s - %s", tool.Function.Name, tool.Function.Description)
		tools = append(tools, domainllminter.Tool{
			Type: string(tool.Type),
			Function: domainllminter.ToolFunction{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}

	return tools
}

func (s *ConversationService) streamResponseWithProvider(
	ctx context.Context,
	messages []domainllminter.Message,
	tools []domainllminter.Tool,
) (<-chan domainllminter.ResponseChunk, error) {
	if s.llmProvider == nil {
		return nil, fmt.Errorf("llm provider is not configured")
	}

	coreMessages := make([]coreproviders.Message, len(messages))
	for i, msg := range messages {
		coreMsg := coreproviders.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}

		if len(msg.ToolCalls) > 0 {
			coreMsg.ToolCalls = make([]domainllminter.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				coreMsg.ToolCalls[j] = domainllminter.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: domainllminter.ToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		coreMessages[i] = coreMsg
	}

	coreTools := make([]coreproviders.Tool, len(tools))
	for i, tool := range tools {
		coreTools[i] = coreproviders.Tool{
			Type: tool.Type,
			Function: domainllminter.ToolFunction{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		}
	}

	s.logger.Legacy().Info("[LLM] 调用ResponseWithFunctions，传递 %d 个工具", len(coreTools))
	respChan, err := s.llmProvider.ResponseWithFunctions(ctx, s.sessionID, coreMessages, coreTools)
	if err != nil {
		return nil, err
	}

	outChan := make(chan domainllminter.ResponseChunk, 10)

	go func() {
		defer close(outChan)

		for response := range respChan {
			chunk := domainllminter.ResponseChunk{
				Content: response.Content,
			}

			if response.IsDone {
				chunk.IsDone = true
			}

			if response.Error != nil {
				chunk.Error = response.Error
			}

			if len(response.ToolCalls) > 0 {
				chunk.ToolCalls = make([]domainllminter.ToolCall, len(response.ToolCalls))
				for i, tc := range response.ToolCalls {
					chunk.ToolCalls[i] = domainllminter.ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: domainllminter.ToolCallFunction{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}
			}

			outChan <- chunk

			if chunk.IsDone {
				break
			}
		}
	}()

	return outChan, nil
}

// AddUserMessage 添加用户消息到对话历史
func (s *ConversationService) AddUserMessage(content string) {
	s.conversationHistory = append(s.conversationHistory, domainllminter.Message{
		Role:    "user",
		Content: content,
	})
}

// AddAssistantMessage 添加助手消息到对话历史
func (s *ConversationService) AddAssistantMessage(content string) {
	s.conversationHistory = append(s.conversationHistory, domainllminter.Message{
		Role:    "assistant",
		Content: content,
	})
}

// ClearConversationHistory 清空对话历史
func (s *ConversationService) ClearConversationHistory() {
	s.conversationHistory = []domainllminter.Message{}
}

// processTextSegment 处理文本分段
func (s *ConversationService) processTextSegment(fullResponse string, processedChars *int, textIndex *int, round int) error {
	if len(fullResponse) <= *processedChars {
		return nil
	}

	currentText := fullResponse[*processedChars:]

	// 根据标点和长度进行分段
	segments := s.segmentText(currentText)

	type segmentInfo struct {
		text   string
		rawLen int
	}

	var cleanedSegments []segmentInfo

	for _, segment := range segments {
		rawLen := len(segment)
		if rawLen == 0 {
			continue
		}

		trimmed := strings.TrimSpace(segment)
		if trimmed == "" {
			*processedChars += rawLen
			continue
		}

		cleaned := utils.RemoveAngleBracketContent(trimmed)
		cleaned = utils.RemoveControlCharacters(cleaned)
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			*processedChars += rawLen
			continue
		}

		cleanedSegments = append(cleanedSegments, segmentInfo{
			text:   cleaned,
			rawLen: rawLen,
		})
	}

	for _, seg := range cleanedSegments {
		(*textIndex)++
		if s.onSpeakAndPlay != nil {
			if err := s.onSpeakAndPlay(seg.text, *textIndex, round); err != nil {
				s.logger.Legacy().Error("处理LLM响应分段失败: %v", err)
				return err
			}
		}
		*processedChars += seg.rawLen
	}

	return nil
}

// segmentText 将文本按标点符号和长度进行分段
func (s *ConversationService) segmentText(text string) []string {
	var segments []string
	runes := []rune(text)

	// 定义分段标点符号
	delimiters := []rune{'。', '！', '？', '；', '：', '\n', '.', '!', '?', ';', ':'}

	i := 0
	for i < len(runes) {
		// 寻找下一个分段点
		segmentEnd := i + 50 // 默认最大段长

		// 在50个字符内寻找合适的分割点
		for j := i; j < len(runes) && j < i+50; j++ {
			for _, delimiter := range delimiters {
				if runes[j] == delimiter {
					segmentEnd = j + 1 // 包含标点符号
					break
				}
			}
			if segmentEnd != i+50 {
				break
			}
		}

		// 确保不超过文本长度
		if segmentEnd > len(runes) {
			segmentEnd = len(runes)
		}

		// 添加段落
		if segmentEnd > i {
			segment := string(runes[i:segmentEnd])
			segments = append(segments, segment)
		}

		i = segmentEnd
	}

	return segments
}
