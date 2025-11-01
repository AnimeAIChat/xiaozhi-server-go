package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	domainauth "xiaozhi-server-go/internal/domain/auth"
	"xiaozhi-server-go/internal/domain/config/service"
	domainllm "xiaozhi-server-go/internal/domain/llm"
	domainllminfra "xiaozhi-server-go/internal/domain/llm/infrastructure"
	domainmcp "xiaozhi-server-go/internal/domain/mcp"
	domaintts "xiaozhi-server-go/internal/domain/tts"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/platform/errors"
	"xiaozhi-server-go/internal/platform/logging"
	"xiaozhi-server-go/internal/transport/ws"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/utils"
)

// ConnectionService 处理WebSocket连接相关的业务逻辑
type ConnectionService struct {
	config        *config.Config
	logger        *logging.Logger
	authManager   *domainauth.AuthManager
	llmManager    *domainllm.Manager
	ttsManager    *domaintts.Manager
	mcpManager    *domainmcp.Manager
	configService *service.ConfigService

	// 子服务
	conversationService *ConversationService
	speechService       *SpeechService
	messageQueueService *MessageQueueService
	providerSet         *pool.ProviderSet

	// 连接状态
	conn             ws.Connection
	sessionID        string
	deviceID         string
	clientID         string
	userID           string
	agentID          uint
	isDeviceVerified bool

	// 音频配置
	clientAudioFormat        string
	clientAudioSampleRate    int
	clientAudioChannels      int
	clientAudioFrameDuration int

	serverAudioFormat        string
	serverAudioSampleRate    int
	serverAudioChannels      int
	serverAudioFrameDuration int

	clientListenMode string
	closeAfterChat   bool

	// 语音处理
	opusDecoder *utils.OpusDecoder

	// 工具注册
	functionRegister domainllm.FunctionRegistryInterface

	// MCP结果处理器
	mcpResultHandlers map[string]func(interface{})
}

// ConnectionConfig 连接服务配置
type ConnectionConfig struct {
	Config              *config.Config
	Logger              *logging.Logger
	AuthManager         *domainauth.AuthManager
	LLMManager          *domainllm.Manager
	TTSManager          *domaintts.Manager
	MCPManager          *domainmcp.Manager
	ConfigService       *service.ConfigService
	ConversationService *ConversationService
	SpeechService       *SpeechService
	MessageQueueService *MessageQueueService
	ProviderSet         *pool.ProviderSet
}

// NewConnectionService 创建新的连接服务
func NewConnectionService(config *ConnectionConfig) *ConnectionService {
	service := &ConnectionService{
		config:              config.Config,
		logger:              config.Logger,
		authManager:         config.AuthManager,
		llmManager:          config.LLMManager,
		ttsManager:          config.TTSManager,
		mcpManager:          config.MCPManager,
		configService:       config.ConfigService,
		conversationService: config.ConversationService,
		speechService:       config.SpeechService,
		messageQueueService: config.MessageQueueService,
		providerSet:         config.ProviderSet,

		clientListenMode: "auto",
		closeAfterChat:   false,

		serverAudioFormat:        "opus",
		serverAudioSampleRate:    24000,
		serverAudioChannels:      1,
		serverAudioFrameDuration: 60,

		mcpResultHandlers: make(map[string]func(interface{})),
	}

	// 设置回调函数
	if service.conversationService != nil {
		service.conversationService.SetCallbacks(
			service.onSpeakAndPlay,
			service.onSendMessage,
		)
	}

	if service.speechService != nil {
		service.speechService.SetCallbacks(
			service.onSpeakAndPlay,
			service.onSendMessage,
		)
	}

	return service
}

// Initialize 初始化连接服务
func (s *ConnectionService) Initialize(req *http.Request, conn ws.Connection) error {
	s.conn = conn

	// 解析请求头
	headers := make(map[string]string)
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = values[0]
			if key == "Device-Id" {
				s.deviceID = values[0]
			}
			if key == "Client-Id" {
				s.clientID = values[0]
			}
			if key == "Session-Id" {
				s.sessionID = values[0]
			}
		}
	}

	// 生成会话ID
	if s.sessionID == "" {
		if s.deviceID == "" {
			s.sessionID = uuid.New().String()
		} else {
			s.sessionID = "device-" + strings.Replace(s.deviceID, ":", "_", -1)
		}
	}
	// 同步会话信息到子服务和提供者
	if s.conversationService != nil {
		s.conversationService.sessionID = s.sessionID
		s.conversationService.deviceID = s.deviceID
		s.conversationService.userID = s.userID
	}
	if s.providerSet != nil {
		if llmProvider := s.providerSet.LLM; llmProvider != nil {
			if setter, ok := any(llmProvider).(interface{ SetSessionID(string) }); ok {
				setter.SetSessionID(s.sessionID)
			}
		}
		if ttsProvider := s.providerSet.TTS; ttsProvider != nil {
			if setter, ok := any(ttsProvider).(interface{ SetSessionID(string) }); ok {
				setter.SetSessionID(s.sessionID)
			}
		}
	}

	// 检查设备信息
	if err := s.checkDeviceInfo(); err != nil {
		return fmt.Errorf("检查设备信息失败: %w", err)
	}

	// 初始化工具注册
	s.functionRegister = domainllminfra.NewFunctionRegistry()

	// 初始化MCP结果处理器
	// 设置ASR提供者监听器
	if s.messageQueueService.GetAsrProvider() != nil {
		s.messageQueueService.GetAsrProvider().SetListener(s)
	}

	s.initMCPResultHandlers()

	// 启动消息队列处理
	s.messageQueueService.Start()

	// 绑定MCP连接
	if err := s.bindMCPConnection(); err != nil {
		return fmt.Errorf("绑定MCP连接失败: %w", err)
	}

	s.logger.Legacy().Info(fmt.Sprintf("[连接] 初始化完成 sessionID=%s deviceID=%s userID=%s", s.sessionID, s.deviceID, s.userID))

	return nil
}

// messageLoop 主消息循环
func (s *ConnectionService) messageLoop() error {
	stopChan := make(chan struct{})
	defer close(stopChan)

	for {
		messageType, message, err := s.conn.ReadMessage(stopChan)
		if err != nil {
			if strings.Contains(err.Error(), "connection closed by stop signal") {
				s.logger.Legacy().Info("连接被停止信号关闭，退出消息循环")
			} else {
				s.logger.Legacy().Error(fmt.Sprintf("读取消息失败: %v, 退出消息循环", err))
			}
			return err
		}

		s.logger.Legacy().Debug(fmt.Sprintf("[连接] 收到消息类型: %d, 消息长度: %d", messageType, len(message)))

		if err := s.handleMessage(messageType, message); err != nil {
			s.logger.Legacy().Error(fmt.Sprintf("处理消息失败: %v", err))
		}
	}
}

// handleMessage 处理接收到的消息
func (s *ConnectionService) handleMessage(messageType int, message []byte) error {
	switch messageType {
	case 1: // 文本消息
		s.messageQueueService.EnqueueClientText(string(message))
		return nil
	case 2: // 二进制消息（音频数据）
		return s.handleAudioMessage(message)
	default:
		s.logger.Legacy().Error(fmt.Sprintf("未知的消息类型: %d", messageType))
		return fmt.Errorf("未知的消息类型: %d", messageType)
	}
}

// handleAudioMessage 处理音频消息
func (s *ConnectionService) handleAudioMessage(message []byte) error {
	if s.clientAudioFormat == "pcm" {
		s.messageQueueService.EnqueueClientAudio(message)
	} else if s.clientAudioFormat == "opus" {
		if s.opusDecoder != nil {
			decodedData, err := s.opusDecoder.Decode(message)
			if err != nil {
				s.logger.Legacy().Error(fmt.Sprintf("解码Opus音频失败: %v", err))
				s.messageQueueService.EnqueueClientAudio(message)
			} else {
				s.logger.Legacy().Debug(fmt.Sprintf("Opus解码成功: %d bytes -> %d bytes", len(message), len(decodedData)))
				if len(decodedData) > 0 {
					s.messageQueueService.EnqueueClientAudio(decodedData)
				}
			}
		} else {
			s.messageQueueService.EnqueueClientAudio(message)
		}
	}
	return nil
}

// processClientTextMessage 处理文本消息
func (s *ConnectionService) processClientTextMessage(text string) error {
	// 解析JSON消息
	var msgJSON interface{}
	if err := json.Unmarshal([]byte(text), &msgJSON); err != nil {
		return s.sendMessage(1, []byte(text))
	}

	// 检查是否为整数类型
	if _, ok := msgJSON.(float64); ok {
		return s.sendMessage(1, []byte(text))
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
		return s.handleHelloMessage(msgMap)
	case "abort":
		return s.handleAbortMessage()
	case "listen":
		return s.handleListenMessage(msgMap)
	case "iot":
		return s.handleIotMessage(msgMap)
	case "chat":
		return s.HandleChatMessage(context.Background(), text)
	case "vision":
		return s.handleVisionMessage(msgMap)
	case "image":
		return s.handleImageMessage(context.Background(), msgMap)
	case "mcp":
		return s.mcpManager.HandleXiaoZhiMCPMessage(msgMap)
	default:
		s.logger.Legacy().Warn(fmt.Sprintf("未知消息类型: %s", msgType))
		return fmt.Errorf("未知的消息类型: %s", msgType)
	}
}

// handleHelloMessage 处理欢迎消息
func (s *ConnectionService) handleHelloMessage(msgMap map[string]interface{}) error {
	s.logger.Legacy().Info("[客户端] 收到欢迎消息")

	// 获取客户端编码格式
	if audioParams, ok := msgMap["audio_params"].(map[string]interface{}); ok {
		if format, ok := audioParams["format"].(string); ok {
			s.clientAudioFormat = format
			if format == "pcm" {
				s.serverAudioFormat = "pcm"
			}
		}
		if sampleRate, ok := audioParams["sample_rate"].(float64); ok {
			s.clientAudioSampleRate = int(sampleRate)
		}
		if channels, ok := audioParams["channels"].(float64); ok {
			s.clientAudioChannels = int(channels)
		}
		if frameDuration, ok := audioParams["frame_duration"].(float64); ok {
			s.clientAudioFrameDuration = int(frameDuration)
		}
	}

	s.sendHelloMessage()
	s.closeOpusDecoder()

	// 初始化opus解码器
	opusDecoder, err := utils.NewOpusDecoder(&utils.OpusDecoderConfig{
		SampleRate:  s.clientAudioSampleRate,
		MaxChannels: s.clientAudioChannels,
	})
	if err != nil {
		s.logger.Legacy().Error(fmt.Sprintf("初始化Opus解码器失败: %v", err))
	} else {
		s.opusDecoder = opusDecoder
		s.logger.Legacy().Info("[Opus] [解码器] 初始化成功")
	}

	return nil
}

// handleAbortMessage 处理中止消息
func (s *ConnectionService) handleAbortMessage() error {
	s.logger.Legacy().Info("[客户端] [中止消息] 收到，停止语音识别")
	s.speechService.StopServerVoice()
	s.sendTTSMessage("stop", "", 0)
	s.clearSpeakStatus()
	return nil
}

// handleListenMessage 处理语音相关消息
func (s *ConnectionService) handleListenMessage(msgMap map[string]interface{}) error {
	state, ok := msgMap["state"].(string)
	if !ok {
		return fmt.Errorf("listen消息缺少state参数")
	}

	if mode, ok := msgMap["mode"].(string); ok {
		s.clientListenMode = mode
	}

	switch state {
	case "start":
		// TODO: 处理开始监听
	case "stop":
		// TODO: 处理停止监听
	case "detect":
		// TODO: 处理检测消息
	}

	return nil
}

// handleIotMessage 处理IOT设备消息
func (s *ConnectionService) handleIotMessage(msgMap map[string]interface{}) error {
	// TODO: 实现IOT消息处理
	return nil
}

// handleVisionMessage 处理视觉消息
func (s *ConnectionService) handleVisionMessage(msgMap map[string]interface{}) error {
	// TODO: 实现视觉消息处理
	return nil
}

// handleImageMessage 处理图片消息
func (s *ConnectionService) handleImageMessage(ctx context.Context, msgMap map[string]interface{}) error {
	// TODO: 实现图片消息处理
	return nil
}

// 消息发送方法们

func (s *ConnectionService) sendHelloMessage() error {
	hello := map[string]interface{}{
		"type":       "hello",
		"version":    1,
		"transport":  "websocket",
		"session_id": s.sessionID,
		"audio_params": map[string]interface{}{
			"format":         s.serverAudioFormat,
			"sample_rate":    s.serverAudioSampleRate,
			"channels":       s.serverAudioChannels,
			"frame_duration": s.serverAudioFrameDuration,
		},
	}
	return s.sendJSONMessage(hello)
}

func (s *ConnectionService) sendTTSMessage(state string, text string, textIndex int) error {
	stateMsg := map[string]interface{}{
		"type":        "tts",
		"state":       state,
		"session_id":  s.sessionID,
		"text":        text,
		"index":       textIndex,
		"audio_codec": "opus",
	}
	return s.sendJSONMessage(stateMsg)
}

func (s *ConnectionService) sendSTTMessage(text string) error {
	sttMsg := map[string]interface{}{
		"type":       "stt",
		"text":       text,
		"session_id": s.sessionID,
	}
	return s.sendJSONMessage(sttMsg)
}

func (s *ConnectionService) sendEmotionMessage(emotion string) error {
	data := map[string]interface{}{
		"type":       "llm",
		"text":       utils.GetEmotionEmoji(emotion),
		"emotion":    emotion,
		"session_id": s.sessionID,
	}
	return s.sendJSONMessage(data)
}

func (s *ConnectionService) sendJSONMessage(data map[string]interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}
	return s.sendMessage(1, jsonData)
}

func (s *ConnectionService) sendMessage(messageType int, data []byte) error {
	if s.conn == (ws.Connection{}) {
		return errors.Wrap(errors.KindTransport, "send_message", "连接未初始化", nil)
	}
	return s.conn.WriteMessage(messageType, data)
}

// 回调函数实现

func (s *ConnectionService) onSpeakAndPlay(text string, textIndex int, round int) error {
	if s.speechService == nil {
		return fmt.Errorf("speech service not initialized")
	}

	// ����TTS״̬��Ӧ����������Ե�ASR
	s.speechService.SetTTSLastTextIndex(textIndex)
	s.speechService.PauseASR()

	return s.speechService.SpeakAndPlay(text, textIndex, round)
}

func (s *ConnectionService) onSendAudioMessage(filepath string, text string, textIndex int, round int) {
	logText := utils.SanitizeForLog(text)
	startTime := time.Now()
	fileDeleted := false

	defer func() {
		// 音频发送完成后，根据配置决定是否删除文件
		if !fileDeleted {
			s.deleteAudioFileIfNeeded(filepath, "音频发送完成")
		}

		spentTime := time.Since(startTime).Milliseconds()
		s.logger.Legacy().Debug(fmt.Sprintf("[TTS] [发送任务 %d/%dms/%dms] %s", textIndex, s.speechService.GetTTSLastTextIndex(), spentTime, logText))

		// 检查是否是最后一个文本段
		if s.speechService.GetTTSLastTextIndex() > 0 && textIndex == s.speechService.GetTTSLastTextIndex() {
			s.sendTTSMessage("stop", "", textIndex)
			// 恢复ASR接收
			s.speechService.ResumeASR()
			if s.closeAfterChat {
				s.Close()
			} else {
				s.clearSpeakStatus()
			}
		}
	}()

	if len(filepath) == 0 {
		return
	}

	// 检查轮次
	if round != s.speechService.GetTalkRound() {
		s.logger.Legacy().Info(fmt.Sprintf("sendAudioMessage: 跳过过期轮次的音频: 任务轮次=%d, 当前轮次=%d, 文本=%s",
			round, s.speechService.GetTalkRound(), logText))
		// 即使跳过，也要根据配置删除音频文件
		s.deleteAudioFileIfNeeded(filepath, "跳过过期轮次")
		fileDeleted = true
		return
	}

	if s.speechService.IsServerVoiceStop() {
		s.logger.Legacy().Info(fmt.Sprintf("sendAudioMessage 服务端语音停止, 不再发送音频数据：%s", logText))
		// 服务端语音停止时也要根据配置删除音频文件
		s.deleteAudioFileIfNeeded(filepath, "服务端语音停止")
		return
	}

	var audioData [][]byte
	var duration float64
	var err error

	// 使用TTS提供者的方法将音频转为Opus格式
	if s.serverAudioFormat == "pcm" {
		s.logger.Legacy().Info("服务端音频格式为PCM，直接发送")
		audioData, duration, err = utils.AudioToPCMData(filepath)
		if err != nil {
			s.logger.Legacy().Error(fmt.Sprintf("音频转PCM失败: %v", err))
			return
		}
	} else if s.serverAudioFormat == "opus" {
		audioData, duration, err = utils.AudioToOpusData(filepath)
		if err != nil {
			s.logger.Legacy().Error(fmt.Sprintf("音频转Opus失败: %v", err))
			return
		}
	}

	// 发送TTS状态开始通知
	if err := s.sendTTSMessage("sentence_start", text, textIndex); err != nil {
		s.logger.Legacy().Error(fmt.Sprintf("发送TTS开始状态失败: %v", err))
		return
	}

	if textIndex == 1 {
		now := time.Now()
		spentTime := now.Sub(s.speechService.GetRoundStartTime())
		s.logger.Legacy().Debug("回复首句耗时 %s 第一句话【%s】, round: %d", spentTime, logText, round)
	}

	s.logger.Legacy().Debug("TTS发送(%s): \"%s\" (索引:%d/%d，时长:%f，帧数:%d)", s.serverAudioFormat, logText, textIndex, s.speechService.GetTTSLastTextIndex(), duration, len(audioData))

	// 分时发送音频数据
	if err := s.sendAudioFrames(audioData, text, round); err != nil {
		s.logger.Legacy().Error(fmt.Sprintf("分时发送音频数据失败: %v", err))
		return
	}

	// 发送TTS状态结束通知
	if err := s.sendTTSMessage("sentence_end", text, textIndex); err != nil {
		s.logger.Legacy().Error(fmt.Sprintf("发送TTS结束状态失败: %v", err))
		return
	}
}

func (s *ConnectionService) onSendMessage(messageType int, data []byte) error {
	return s.sendMessage(messageType, data)
}

// 辅助方法们

func (s *ConnectionService) checkDeviceInfo() error {
	// TODO: 实现设备信息检查
	return nil
}

func (s *ConnectionService) bindMCPConnection() error {
	params := map[string]interface{}{
		"session_id": s.sessionID,
		"vision_url": s.config.Web.VisionURL,
		"device_id":  s.deviceID,
		"client_id":  s.clientID,
		"token":      s.config.Server.Token,
	}
	return s.mcpManager.BindConnection(&s.conn, s.functionRegister, params)
}

func (s *ConnectionService) initMCPResultHandlers() {
	// TODO: 初始化MCP结果处理器
}

func (s *ConnectionService) closeOpusDecoder() {
	if s.opusDecoder != nil {
		if err := s.opusDecoder.Close(); err != nil {
			s.logger.Legacy().Error(fmt.Sprintf("关闭Opus解码器失败: %v", err))
		}
		s.opusDecoder = nil
	}
}

func (s *ConnectionService) clearSpeakStatus() {
	s.logger.Legacy().Info("[服务端] [讲话状态] 已清除")
	s.speechService.SetTTSLastTextIndex(-1)
	s.speechService.SetTTSLastAudioIndex(-1)
	s.messageQueueService.ClearQueues()
	time.Sleep(50 * time.Millisecond)
	s.speechService.ResumeASR()
}

// Handle 实现SessionHandler接口
func (s *ConnectionService) Handle() {
	// SessionHandler接口的Handle方法不接收参数
	// 连接已经在Initialize时设置，这里直接处理
	if err := s.messageLoop(); err != nil {
		s.logger.Legacy().Error(fmt.Sprintf("处理连接失败: %v", err))
	}
}

// Close 清理资源
func (s *ConnectionService) Close() {
	s.closeOpusDecoder()
	if s.messageQueueService != nil {
		s.messageQueueService.Stop()
	}
	if s.speechService != nil {
		s.speechService.PauseASR()
	}
	s.releaseProviderSet()
}

func (s *ConnectionService) releaseProviderSet() {
	if s.providerSet == nil {
		return
	}
	if err := s.providerSet.Release(); err != nil {
		s.logger.Legacy().Error(fmt.Sprintf("[连接] 归还资源池失败: %v", err))
	}
	s.providerSet = nil
}

func (s *ConnectionService) sendAudioFrames(audioData [][]byte, text string, round int) error {
	if len(audioData) == 0 {
		return nil
	}

	logText := utils.SanitizeForLog(text)
	startTime := time.Now()
	playPosition := 0 // 播放位置（毫秒）

	// 预缓冲：发送前几帧，提升播放流畅度
	preBufferFrames := 3
	if len(audioData) < preBufferFrames {
		preBufferFrames = len(audioData)
	}
	preBufferTime := time.Duration(s.serverAudioFrameDuration*preBufferFrames) * time.Millisecond // 预缓冲时间（毫秒）

	// 发送预缓冲帧
	for i := 0; i < preBufferFrames; i++ {
		// 检查是否被打断
		if s.speechService.IsServerVoiceStop() || round != s.speechService.GetTalkRound() {
			s.logger.Legacy().Info(fmt.Sprintf("音频发送被中断(预缓冲阶段): 帧=%d/%d, 文本=%s", i+1, preBufferFrames, logText))
			return nil
		}

		if err := s.conn.WriteMessage(2, audioData[i]); err != nil {
			return fmt.Errorf("发送预缓冲音频帧失败: %v", err)
		}
		playPosition += s.serverAudioFrameDuration
	}

	// 发送剩余音频帧
	remainingFrames := audioData[preBufferFrames:]
	for i, chunk := range remainingFrames {
		// 检查是否被打断或轮次变化
		if s.speechService.IsServerVoiceStop() || round != s.speechService.GetTalkRound() {
			s.logger.Legacy().Info(fmt.Sprintf("音频发送被中断: 帧=%d/%d, 文本=%s", i+preBufferFrames+1, len(audioData), logText))
			return nil
		}

		// 检查连接是否关闭
		select {
		case <-s.messageQueueService.GetStopChan():
			return nil
		default:
		}

		// 计算预期发送时间
		expectedTime := startTime.Add(time.Duration(playPosition)*time.Millisecond - preBufferTime)
		currentTime := time.Now()
		delay := expectedTime.Sub(currentTime)

		// 流控延迟处理
		if delay > 0 {
			// 使用简单的可中断睡眠
			ticker := time.NewTicker(10 * time.Millisecond) // 固定10ms检查间隔
			defer ticker.Stop()

			endTime := time.Now().Add(delay)
			for time.Now().Before(endTime) {
				select {
				case <-ticker.C:
					// 检查中断条件
					if s.speechService.IsServerVoiceStop() || round != s.speechService.GetTalkRound() {
						s.logger.Legacy().Info(fmt.Sprintf("音频发送在延迟中被中断: 帧=%d/%d, 文本=%s", i+preBufferFrames+1, len(audioData), logText))
						return nil
					}
				case <-s.messageQueueService.GetStopChan():
					return nil
				}
			}
		}

		// 发送音频帧
		if err := s.conn.WriteMessage(2, chunk); err != nil {
			return fmt.Errorf("发送音频帧失败: %v", err)
		}

		playPosition += s.serverAudioFrameDuration
	}
	time.Sleep(preBufferTime) // 确保预缓冲时间已过
	spentTime := time.Since(startTime).Milliseconds()
	s.logger.Legacy().Info(fmt.Sprintf("[TTS] [音频帧 %d/%dms/%dms] %s", len(audioData), playPosition, spentTime, logText))
	return nil
}

func (s *ConnectionService) deleteAudioFileIfNeeded(filepath string, reason string) {
	if !s.config.Audio.DeleteAudio || filepath == "" {
		return
	}

	// 检查是否是音乐文件，如果是则不删除
	if utils.IsMusicFile(filepath) {
		s.logger.Legacy().Info(fmt.Sprintf(reason+" 跳过删除音乐文件: %s", filepath))
		return
	}

	// 使用os.Stat检查文件是否存在
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		s.logger.Legacy().Debug(fmt.Sprintf(reason+" 文件不存在，无需删除: %s", filepath))
		return
	}

	// 删除非缓存音频文件
	if err := os.Remove(filepath); err != nil {
		// 如果文件不存在，这是正常的（可能已被其他地方删除）
		if os.IsNotExist(err) {
			s.logger.Legacy().Debug(fmt.Sprintf(reason+" 文件已被删除: %s", filepath))
		} else {
			s.logger.Legacy().Error(fmt.Sprintf(reason+" 删除音频文件失败: %v", err))
		}
	} else {
		s.logger.Legacy().Debug(fmt.Sprintf("%s 已删除音频文件: %s", reason, filepath))
	}
}

// GetSessionID 实现SessionHandler接口
func (s *ConnectionService) GetSessionID() string {
	return s.sessionID
}

// HandleChatMessage 处理聊天消息
func (s *ConnectionService) HandleChatMessage(ctx context.Context, text string) error {
	if s.conversationService == nil {
		return fmt.Errorf("conversation service not initialized")
	}

	round := 1
	if s.speechService != nil {
		round = s.speechService.IncrementTalkRound()
		s.speechService.SetRoundStartTime(time.Now())
		s.speechService.SetTTSLastTextIndex(-1)
		s.speechService.SetTTSLastAudioIndex(-1)
	}

	return s.conversationService.HandleChatMessage(ctx, text, round)
}

// ProcessClientTextMessage 处理客户端文本消息
func (s *ConnectionService) ProcessClientTextMessage(ctx context.Context, text string) error {
	return s.processClientTextMessage(text)
}

// OnSendAudioMessage 发送音频消息
func (s *ConnectionService) OnSendAudioMessage(filepath string, text string, textIndex int, round int) {
	s.onSendAudioMessage(filepath, text, textIndex, round)
}

// ProcessTTSTask 处理TTS任务
func (s *ConnectionService) ProcessTTSTask(text string, textIndex int, round int, filepath string) {
	// TODO: 实现TTS任务处理逻辑
}

// OnAsrResult 实现AsrEventListener接口，处理ASR结果
func (s *ConnectionService) OnAsrResult(result string, isFinalResult bool) bool {
	if result == "" {
		return true
	}

	s.logger.Legacy().Info(fmt.Sprintf("[ASR] 收到结果: %s (最终:%v)", utils.SanitizeForLog(result), isFinalResult))

	// 将ASR结果加入队列
	s.messageQueueService.EnqueueASRResult(result)

	return true
}
