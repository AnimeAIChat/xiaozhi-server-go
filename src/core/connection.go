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

	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
	domainauth "xiaozhi-server-go/internal/domain/auth"
	domainimage "xiaozhi-server-go/internal/domain/image"
	domainllm "xiaozhi-server-go/internal/domain/llm"
	domainllminter "xiaozhi-server-go/internal/domain/llm/inter"
	domainmcp "xiaozhi-server-go/internal/domain/mcp"
	domaintts "xiaozhi-server-go/internal/domain/tts"
	domainttsinter "xiaozhi-server-go/internal/domain/tts/inter"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/platform/storage"
	"xiaozhi-server-go/src/core/chat"
	"xiaozhi-server-go/src/core/function"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/providers/tts"
	"xiaozhi-server-go/src/core/providers/vlllm"
	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/models"
	"xiaozhi-server-go/src/task"
)

type Connection interface {
	domainmcp.Conn
	ReadMessage(stopChan <-chan struct{}) (messageType int, data []byte, err error)
	Close() error
	GetID() string
	GetType() string
	IsClosed() bool
	GetLastActiveTime() time.Time
	IsStale(timeout time.Duration) bool
}

type ttsConfigGetter interface {
	Config() *tts.Config
}

type llmConfigGetter interface {
	Config() *llm.Config
}

// ConnectionHandler 连接处理器结构
type ConnectionHandler struct {
	// 确保实现 AsrEventListener 接口
	_                providers.AsrEventListener
	config           *config.Config
	logger           *utils.Logger
	conn             Connection
	closeOnce        sync.Once
	taskMgr          *task.TaskManager
	authManager      *domainauth.AuthManager // 认证管理器
	safeCallbackFunc func(func(*ConnectionHandler)) func()
	providers        struct {
		asr   providers.ASRProvider
		llm   providers.LLMProvider
		tts   providers.TTSProvider
		vlllm *vlllm.Provider // VLLLM提供者，可选
	}

	// 新架构管理器
	llmManager *domainllm.Manager
	ttsManager *domaintts.Manager

	initialVoice    string // 初始语音名称
	ttsProviderName string // 默认TTS提供者名称
	voiceName       string // 语音名称

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

	// Agent 相关
	agentID      uint          // 设备绑定的AgentID
	userID       string        // 设备绑定的用户ID
	enabledTools []string      // 启用的工具列表
	tools        []openai.Tool // 缓存的工具列表
	// 语音处理相关
	serverVoiceStop int32 // 1表示true服务端语音停止, 不再下发语音数据
	asrPause         int32 // 1 表示暂停将来自客户端的音频发送到 ASR（例如 TTS 播放期间）

	opusDecoder *utils.OpusDecoder // Opus解码器

	// 对话相关
	dialogueManager      *chat.DialogueManager
	tts_last_text_index  int
	tts_last_audio_index int
	client_asr_text      string // 客户端ASR文本

	// 并发控制
	stopChan         chan struct{}
	clientAudioQueue chan []byte
	clientTextQueue  chan string

	// ASR结果队列 - 用于避免重复识别导致的并发处理
	asrResultQueue chan string

	// TTS任务队列
	ttsQueue chan struct {
		text      string
		round     int // 轮次
		textIndex int
		filepath  string
	}

	audioMessagesQueue chan struct {
		filepath  string
		text      string
		round     int // 轮次
		textIndex int
	}

	talkRound      int       // 轮次计数
	roundStartTime time.Time // 轮次开始时间
	lastWakeUpTime time.Time // 上次唤醒处理时间
	// functions
	functionRegister *function.FunctionRegistry
	mcpManager       *domainmcp.Manager

	mcpResultHandlers map[string]func(interface{}) // MCP处理器映射
	ctx               context.Context
}

// NewConnectionHandler 创建新的连接处理器
func NewConnectionHandler(
	config *config.Config,
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
		asrResultQueue:   make(chan string, 10), // ASR结果队列，缓冲大小为10
		ttsQueue: make(chan struct {
			text      string
			round     int // 轮次
			textIndex int
			filepath  string
		}, 100),
		audioMessagesQueue: make(chan struct {
			filepath  string
			text      string
			round     int // 轮次
			textIndex int
		}, 100),

		tts_last_text_index:  -1,
		tts_last_audio_index: -1,

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
		logger.DebugTag("HTTP", "请求头 %s: %s", key, values[0])
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
	handler.checkDeviceInfo()
	agent, prompt := handler.InitWithAgent()
	handler.checkTTSProvider(agent, config) // 检查TTS提供者
	handler.checkLLMProvider(agent, config) // 检查LLM提供者是否匹配

	// 初始化新架构的 LLM 和 TTS Manager
	handler.initManagers(config)

	// 初始化对话管理器
	handler.dialogueManager = chat.NewDialogueManager(handler.logger, nil)
	handler.dialogueManager.SetSystemMessage(prompt)
	handler.functionRegister = function.NewFunctionRegistry()
	handler.initMCPResultHandlers()

	return handler
}

func (h *ConnectionHandler) InitWithAgent() (*models.Agent, string) {
	// Database functionality removed - return default prompt
	prompt := h.config.System.DefaultPrompt
	h.LogInfo("Database functionality removed - using default agent configuration")
	return nil, prompt
}

func (h *ConnectionHandler) checkTTSProvider(agent *models.Agent, config *config.Config) {
	h.ttsProviderName = "default" // 默认TTS提供者名称
	h.voiceName = "default"

	// 获取用户的TTS选择
	_, ttsName, _ := h.getUserModelSelection()

	if getter, ok := h.providers.tts.(ttsConfigGetter); ok {
		// 使用用户选择的TTS提供者
		h.LogInfo(fmt.Sprintf("使用用户选择的TTS提供者: %s", ttsName))

		h.ttsProviderName = getter.Config().Type
		// 从agent配置中获取
		h.voiceName = getter.Config().Voice
		if agent != nil && agent.Voice != "" {
			err, newVoice := h.providers.tts.SetVoice(agent.Voice) // 设置TTS语音
			if err != nil {
				h.LogError(fmt.Sprintf("设置TTS语音为agent配置失败: %v", err))
			} else {
				h.voiceName = newVoice
			}
		}
		h.initialVoice = h.voiceName // 保存初始语音名称
	}
	h.logger.InfoTag("TTS", "使用 TTS 提供者 %s，语音 %s", h.ttsProviderName, h.voiceName)

}

func (h *ConnectionHandler) checkLLMProvider(agent *models.Agent, config *config.Config) {
	if agent == nil {
		return
	}
	agentLLMName := agent.LLM
	// 从agent里获取extra
	apiKey := ""
	baseUrl := ""
	if agent.Extra != "" {
		// 解析Extra字段
		var extra map[string]interface{}
		if err := json.Unmarshal([]byte(agent.Extra), &extra); err == nil {
			if key, ok := extra["api_key"].(string); ok {
				apiKey = key
			}
			if url, ok := extra["base_url"].(string); ok {
				baseUrl = url
			}
		} else {
			h.LogError(fmt.Sprintf("Agent %d 的 Extra 字段格式错误: %v， err:%v", agent.ID, agent.Extra, err))
		}
	}

	// 获取用户的模型选择
	llmName, _, _ := h.getUserModelSelection()

	// 如果用户没有选择，使用agent的LLM
	if llmName == "" {
		llmName = agentLLMName
	}

	// 判断handler.providers.llm 类型是否和用户选择的LLM相同
	if getter, ok := h.providers.llm.(llmConfigGetter); ok {
		currentLLMName := getter.Config().Name
		if currentLLMName != llmName {
			// 根据用户选择的LLM类型设置LLM提供者
			if cfg, ok := config.LLM[llmName]; !ok {
				h.LogError(fmt.Sprintf("用户选择的LLM类型 %s 不存在", llmName))
			} else {
				if apiKey != "" {
					cfg.APIKey = apiKey // 使用Agent的API密钥
				}
				if baseUrl != "" {
					cfg.BaseURL = baseUrl // 使用Agent的BaseURL
				}
				llmCfg := &llm.Config{
					Name:        llmName,
					Type:        cfg.Type,
					ModelName:   cfg.ModelName,
					BaseURL:     cfg.BaseURL,
					APIKey:      cfg.APIKey,
					Temperature: cfg.Temperature,
					MaxTokens:   cfg.MaxTokens,
					TopP:        cfg.TopP,
					Extra:       cfg.Extra,
				}
				newllm, err := llm.Create(cfg.Type, llmCfg)
				if err != nil {
					h.LogError(fmt.Sprintf("创建LLM提供者失败: %v", err))
				} else {
					h.providers.llm = newllm
					h.LogInfo(fmt.Sprintf("已切换到用户选择的LLM提供者: %s", llmName))
				}
			}
		} else {
			if apiKey != "" {
				getter.Config().APIKey = apiKey
			}
			if baseUrl != "" {
				getter.Config().BaseURL = baseUrl
			}
			h.LogInfo(fmt.Sprintf("使用用户选择的LLM类型: %s, BaseURL:%s", llmName, getter.Config().BaseURL))
		}
	}
}

func (h *ConnectionHandler) checkDeviceInfo() {
	h.agentID = 0 // 清空AgentID
	h.userID = "" // 清空用户ID

	if h.deviceID == "" {
		h.LogError("设备ID未设置，无法检查设备绑定状态")
		return
	}

	// 尝试从数据库获取设备信息和用户模型选择
	if err := storage.InitDatabase(); err != nil {
		h.LogError(fmt.Sprintf("初始化数据库失败: %v，使用默认配置", err))
	} else {
		db := storage.GetDB()
		var device storage.Device
		if err := db.Where("device_id = ?", h.deviceID).First(&device).Error; err != nil {
			h.LogWarn(fmt.Sprintf("设备 %s 不存在于数据库中: %v，使用默认配置", h.deviceID, err))
		} else {
			// 获取用户ID
			if device.UserID != nil {
				userIDInt := *device.UserID
				h.userID = fmt.Sprintf("%d", userIDInt)
				h.LogInfo(fmt.Sprintf("设备绑定用户ID: %s", h.userID))

				// 根据用户ID获取用户名
				var user storage.User
				if err := db.Where("id = ?", userIDInt).First(&user).Error; err != nil {
					h.LogWarn(fmt.Sprintf("用户ID %d 不存在于用户表中: %v，使用默认配置", userIDInt, err))
				} else {
					username := user.Username
					h.LogInfo(fmt.Sprintf("设备绑定用户名: %s", username))

					// 获取用户的模型选择
					var modelSelection storage.ModelSelection
					if err := db.Where("user_id = ? AND is_active = ?", username, true).First(&modelSelection).Error; err != nil {
						h.LogWarn(fmt.Sprintf("用户 %s 没有模型选择配置: %v，使用默认配置", username, err))
					} else {
						h.LogInfo(fmt.Sprintf("用户 %s 的模型选择: LLM=%s, TTS=%s, ASR=%s", username, modelSelection.LLMProvider, modelSelection.TTSProvider, modelSelection.ASRProvider))
					}
				}
			} else {
				h.LogWarn(fmt.Sprintf("设备 %s 未绑定用户，使用默认配置", h.deviceID))
			}

			// 获取AgentID（如果存在）
			if device.AgentID != nil {
				h.agentID = uint(*device.AgentID)
			}
		}
	}

	h.LogInfo(fmt.Sprintf("设备绑定状态: AgentID=%d, UserID=%s", h.agentID, h.userID))
}

// getUserModelSelection 获取用户的模型选择，如果没有则返回默认配置
func (h *ConnectionHandler) getUserModelSelection() (llmProvider, ttsProvider, asrProvider string) {
	// 默认使用全局配置
	llmProvider = h.config.Selected.LLM
	ttsProvider = h.config.Selected.TTS
	asrProvider = h.config.Selected.ASR

	// 如果有用户ID，尝试获取用户的模型选择
	if h.userID != "" {
		if err := storage.InitDatabase(); err == nil {
			db := storage.GetDB()

			// 根据用户ID字符串解析为整数，然后获取用户名
			var userIDInt int
			if _, err := fmt.Sscanf(h.userID, "%d", &userIDInt); err == nil {
				var user storage.User
				if err := db.Where("id = ?", userIDInt).First(&user).Error; err == nil {
					username := user.Username

					var modelSelection storage.ModelSelection
					if err := db.Where("user_id = ? AND is_active = ?", username, true).First(&modelSelection).Error; err == nil {
						// 使用用户的模型选择
						llmProvider = modelSelection.LLMProvider
						ttsProvider = modelSelection.TTSProvider
						asrProvider = modelSelection.ASRProvider
						h.LogInfo(fmt.Sprintf("使用用户 %s 的模型选择: LLM=%s, TTS=%s, ASR=%s", username, llmProvider, ttsProvider, asrProvider))
					}
				}
			}
		}
	}

	return llmProvider, ttsProvider, asrProvider
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

func (h *ConnectionHandler) normalizeLogMessage(msg string) string {
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return "[Connection]"
	}
	if strings.HasPrefix(trimmed, "[") {
		return trimmed
	}
	return "[Connection] " + trimmed
}

func (h *ConnectionHandler) LogInfo(msg string) {
	if h.logger != nil {
		h.logger.Info(h.normalizeLogMessage(msg), map[string]interface{}{
			"device": h.deviceID,
		})
	}
}
func (h *ConnectionHandler) LogDebug(msg string) {
	if h.logger != nil {
		h.logger.Debug(h.normalizeLogMessage(msg), map[string]interface{}{
			"device": h.deviceID,
		})
	}
}
func (h *ConnectionHandler) LogError(msg string) {
	if h.logger != nil {
		h.logger.Error(h.normalizeLogMessage(msg), map[string]interface{}{
			"device": h.deviceID,
		})
	}
}

func (h *ConnectionHandler) LogWarn(msg string) {
	if h.logger != nil {
		h.logger.Warn(h.normalizeLogMessage(msg), map[string]interface{}{
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
	go h.processASRResultQueueCoroutine()      // 添加ASR结果队列处理协程
	go h.processTTSQueueCoroutine()            // 添加TTS队列处理协程
	go h.sendAudioMessageCoroutine()           // 添加音频消息发送协程

	h.LogInfo("[协程] 所有消息处理协程已启动")

	// 优化后的MCP管理器处理
	if h.mcpManager == nil {
		h.LogError("没有可用的MCP管理器")
		return

	} else {
		h.LogInfo("[MCP] [管理器] 使用资源池快速绑定连接")
		// 池化的管理器已经预初始化，只需要绑定连接
		params := map[string]interface{}{
			"session_id": h.sessionID,
			"vision_url": h.config.Web.VisionURL,
			"device_id":  h.deviceID,
			"client_id":  h.clientId,
			"token":      h.config.Server.Token,
		}
		if err := h.mcpManager.BindConnection(conn, h.functionRegister, params); err != nil {
			h.LogError(fmt.Sprintf("[MCP] bind connection failed: %v", err))
			return
		}
		// Skip redundant re-initialisation because the pool performed it during acquire.
		h.LogInfo("[MCP] connection attached; reuse existing session bootstrap")
		h.LogInfo("[连接] 开始监听客户端消息...")
		for {
			h.LogDebug("[连接] 等待客户端消息...")
			messageType, message, err := conn.ReadMessage(h.stopChan)
			if err != nil {
				if strings.Contains(err.Error(), "connection closed by stop signal") {
					h.LogInfo("连接被停止信号关闭，退出消息循环")
				} else {
					h.LogError(fmt.Sprintf("读取消息失败: %v, 退出消息循环", err))
				}
				return
			}

			h.LogDebug(fmt.Sprintf("[连接] 收到消息类型: %d, 消息长度: %d", messageType, len(message)))
			if err := h.handleMessage(messageType, message); err != nil {
				h.LogError(fmt.Sprintf("处理消息失败: %v", err))
			}
		}
	}
}

// processASRResultQueueCoroutine 处理ASR结果队列
func (h *ConnectionHandler) processASRResultQueueCoroutine() {
	h.LogInfo("[协程] [ASR队列] ASR结果处理协程启动")
	defer h.LogInfo("[协程] [ASR队列] ASR结果处理协程退出")

	for {
		select {
		case <-h.stopChan:
			h.LogDebug("[协程] [ASR队列] 收到停止信号，退出协程")
			return
		case asrText := <-h.asrResultQueue:
			h.LogInfo(fmt.Sprintf("[协程] [ASR队列] 处理ASR结果: %s", utils.SanitizeForLog(asrText)))

			// 检查是否是重复的唤醒词处理
			if utils.IsWakeUpWord(asrText) {
				now := time.Now()
				if now.Sub(h.lastWakeUpTime) < 3*time.Second {
					h.LogInfo("[协程] [ASR队列] 跳过重复的唤醒词处理")
					continue
				}
			}

			if err := h.handleChatMessage(context.Background(), asrText); err != nil {
				h.LogError(fmt.Sprintf("[协程] [ASR队列] 处理ASR结果失败: %v", err))
			} else {
				h.LogDebug(fmt.Sprintf("[协程] [ASR队列] ASR结果处理完成: %s", utils.SanitizeForLog(asrText)))
			}
		}
	}
}

// processClientTextMessagesCoroutine 处理客户端文本消息队列
func (h *ConnectionHandler) processClientTextMessagesCoroutine() {
	h.LogInfo("[协程] [文本队列] 客户端文本消息处理协程启动")
	defer h.LogInfo("[协程] [文本队列] 客户端文本消息处理协程退出")

	for {
		select {
		case <-h.stopChan:
			h.LogDebug("[协程] [文本队列] 收到停止信号，退出协程")
			return
		case text := <-h.clientTextQueue:
			if err := h.processClientTextMessage(context.Background(), text); err != nil {
				h.LogError(fmt.Sprintf("[协程] [文本队列] 处理文本消息失败: %v", err))
			}
		}
	}
}

// processClientAudioMessagesCoroutine 处理音频消息队列
func (h *ConnectionHandler) processClientAudioMessagesCoroutine() {
	h.LogInfo("[协程] [音频队列] 客户端音频消息处理协程启动")
	defer h.LogInfo("[协程] [音频队列] 客户端音频消息处理协程退出")

	for {
		select {
		case <-h.stopChan:
			h.LogDebug("[协程] [音频队列] 收到停止信号，退出协程")
			return
		case audioData := <-h.clientAudioQueue:
			// 如果已设置为在播放服务端语音时暂停ASR，则跳过发送到ASR
			if atomic.LoadInt32(&h.asrPause) == 1 {
				h.LogDebug("[协程] [音频队列] 当前处于ASR暂停状态，跳过发送客户端音频到ASR")
				continue
			}
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
	h.LogInfo("[协程] [音频发送] 音频消息发送协程启动")
	defer h.LogInfo("[协程] [音频发送] 音频消息发送协程退出")

	for {
		select {
		case <-h.stopChan:
			h.LogDebug("[协程] [音频发送] 收到停止信号，退出协程")
			return
		case task := <-h.audioMessagesQueue:
			h.sendAudioMessage(task.filepath, task.text, task.textIndex, task.round)
		}
	}
}

// OnAsrResult 实现 AsrEventListener 接口
// 返回true则停止语音识别，返回false会继续语音识别
func (h *ConnectionHandler) OnAsrResult(result string, isFinalResult bool) bool {
	//h.LogInfo(fmt.Sprintf("[%s] ASR识别结果: %s", h.clientListenMode, result))
	if h.providers.asr.GetSilenceCount() >= 2 {
		h.LogInfo("[ASR] [静音检测] 连续两次，结束对话")
		h.closeAfterChat = true // 如果连续两次静音，则结束对话
		result = "[SILENCE_TIMEOUT] 长时间未检测到用户说话，请礼貌的结束对话"
	}
	if h.clientListenMode == "auto" {
		if result == "" {
			return false
		}
		h.LogInfo(fmt.Sprintf("[ASR] [识别结果 %s/%s]", h.clientListenMode, utils.SanitizeForLog(result)))
		// 将ASR结果放入队列，避免并发处理
		select {
		case h.asrResultQueue <- result:
			h.LogDebug(fmt.Sprintf("[ASR] [队列] 已将结果放入队列: %s", utils.SanitizeForLog(result)))
		default:
			h.LogWarn(fmt.Sprintf("[ASR] [队列] 队列已满，丢弃结果: %s", utils.SanitizeForLog(result)))
		}
		return true
	} else if h.clientListenMode == "manual" {
		h.client_asr_text += result
		if isFinalResult {
			// 将ASR结果放入队列，避免并发处理
			select {
			case h.asrResultQueue <- h.client_asr_text:
				h.LogDebug(fmt.Sprintf("[ASR] [队列] 已将手动结果放入队列: %s", utils.SanitizeForLog(h.client_asr_text)))
			default:
				h.LogWarn(fmt.Sprintf("[ASR] [队列] 队列已满，丢弃手动结果: %s", utils.SanitizeForLog(h.client_asr_text)))
			}
			return true
		}
		return false
	} else if h.clientListenMode == "realtime" {
		if result == "" {
			return false
		}
		h.stopServerSpeak()
		h.providers.asr.Reset() // 重置ASR状态，准备下一次识别
		h.LogInfo(fmt.Sprintf("[ASR] [识别结果 %s/%s]", h.clientListenMode, utils.SanitizeForLog(result)))
		// 将ASR结果放入队列，避免并发处理
		select {
		case h.asrResultQueue <- result:
			h.LogDebug(fmt.Sprintf("[ASR] [队列] 已将实时结果放入队列: %s", utils.SanitizeForLog(result)))
		default:
			h.LogWarn(fmt.Sprintf("[ASR] [队列] 队列已满，丢弃实时结果: %s", utils.SanitizeForLog(result)))
		}
		return true
	}
	return false
}

// clientAbortChat 处理中止消息
func (h *ConnectionHandler) clientAbortChat() error {
	h.LogInfo("[客户端] [中止消息] 收到，停止语音识别")
	h.stopServerSpeak()
	h.sendTTSMessage("stop", "", 0)
	h.clearSpeakStatus()
	return nil
}

func (h *ConnectionHandler) QuitIntent(text string) bool {
	//CMD_exit 读取配置中的退出命令
	exitCommands := h.config.System.CMDExit
	if exitCommands == nil {
		return false
	}
	cleand_text := utils.RemoveAllPunctuation(text) // 移除标点符号，确保匹配准确
	// 检查是否包含退出命令
	for _, cmd := range exitCommands {
		h.logger.Debug("检查退出命令: %s,%s", cmd, cleand_text)
		//判断相等
		if cleand_text == cmd {
			h.LogInfo("[客户端] [退出意图] 收到，准备结束对话")
			h.Close() // 直接关闭连接
			return true
		}
	}
	return false
}

// handleChatMessage 处理聊天消息
func (h *ConnectionHandler) handleChatMessage(ctx context.Context, text string) error {
	if text == "" {
		h.LogWarn("收到空聊天消息，忽略")
		h.clientAbortChat()
		return fmt.Errorf("聊天消息为空")
	}

	if h.QuitIntent(text) {
		return nil
	}

	// 检测是否是唤醒词，实现快速响应
	if utils.IsWakeUpWord(text) {
		h.LogInfo(fmt.Sprintf("[唤醒] [检测成功] 文本 '%s' 匹配唤醒词模式", text))
		return h.handleWakeUpMessage(ctx, text)
	} else {
		h.LogInfo(fmt.Sprintf("[唤醒] [检测失败] 文本 '%s' 不匹配唤醒词模式", text))
	}

	// 记录正在处理对话的状态
	h.LogInfo(fmt.Sprintf("[对话] [开始处理] 文本: %s", utils.SanitizeForLog(text)))

	// 清空音频队列，防止后续音频数据触发新的ASR识别
	queueCleared := false
	for {
		select {
		case <-h.clientAudioQueue:
			if !queueCleared {
				h.LogDebug("[对话] [队列清理] 清空音频队列")
				queueCleared = true
			}
		default:
			goto clearedAudioQueue
		}
	}
clearedAudioQueue:

	// 增加对话轮次
	h.talkRound++
	h.roundStartTime = time.Now()
	currentRound := h.talkRound
	h.LogInfo(fmt.Sprintf("[对话] [轮次 %d] 开始新的对话轮次", currentRound))

	// 普通文本消息处理流程
	// 立即发送 stt 消息
	err := h.sendSTTMessage(text)
	if err != nil {
		h.LogError(fmt.Sprintf("发送STT消息失败: %v", err))
		return fmt.Errorf("发送STT消息失败: %v", err)
	}

	h.LogInfo(fmt.Sprintf("[聊天] [消息 %s]", utils.SanitizeForLog(text)))

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
		// 对话处理完成，记录日志
		h.LogInfo(fmt.Sprintf("[对话] [轮次 %d] 处理完成", round))
	}()

	llmStartTime := time.Now()
	//h.logger.Info("开始生成LLM回复, round:%d ", round)
	for _, msg := range messages {
		_ = msg
		//msg.Print()
	}

	// 发布LLM开始事件
	if publisher := llm.GetEventPublisher(h.providers.llm); publisher != nil {
		publisher.SetSessionID(h.sessionID)
		publisher.PublishLLMResponse("", false, round, nil, 0, "") // 开始事件
	}

	// 使用LLM生成回复
	tools := h.functionRegister.GetAllFunctions()

	// 转换消息格式
	interMessages := make([]domainllminter.Message, len(messages))
	for i, msg := range messages {
		interMsg := domainllminter.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		
		// 转换ToolCalls
		if len(msg.ToolCalls) > 0 {
			interMsg.ToolCalls = make([]domainllminter.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				interMsg.ToolCalls[j] = domainllminter.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: domainllminter.ToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
		
		interMessages[i] = interMsg
	}

	// 转换工具格式
	interTools := make([]domainllminter.Tool, len(tools))
	for i, tool := range tools {
		interTools[i] = domainllminter.Tool{
			Type: string(tool.Type),
			Function: domainllminter.ToolFunction{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		}
	}

	responses, err := h.llmManager.Response(ctx, h.sessionID, interMessages, interTools)
	if err != nil {
		// 发布LLM错误事件
		if publisher := llm.GetEventPublisher(h.providers.llm); publisher != nil {
			publisher.PublishLLMError(err, round)
		}
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

		if response.Error != nil {
			h.LogError(fmt.Sprintf("LLM响应错误: %s", response.Error.Error()))
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
				errorMsg := "抱歉，LLM服务暂时不可用，请稍后再试"
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
				h.logger.Warn("文本处理异常: fullText长度=%d, processedChars=%d", len(fullText), processedChars)
				continue
			}
			currentText := fullText[processedChars:]

			// 按标点符号分割
			if segment, charsCnt := utils.SplitAtLastPunctuation(currentText); charsCnt > 0 {
				textIndex++
				segment = strings.TrimSpace(segment)
				h.tts_last_text_index = textIndex
				err := h.SpeakAndPlay(segment, textIndex, round)
				if err != nil {
					h.LogError(fmt.Sprintf("播放LLM回复分段失败: %v", err))
				}
				processedChars += charsCnt

				// 发布LLM响应事件
				if publisher := llm.GetEventPublisher(h.providers.llm); publisher != nil {
					spentTime := ""
					if textIndex == 1 {
						now := time.Now()
						llmSpentTime := now.Sub(llmStartTime)
						spentTime = llmSpentTime.String()
					}
					publisher.PublishLLMResponse(segment, false, round, nil, textIndex, spentTime)
				}
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

	// 发布LLM完成事件
	if publisher := llm.GetEventPublisher(h.providers.llm); publisher != nil {
		publisher.PublishLLMResponse(content, true, round, nil, 0, "")
	}

	return nil
}

// handleWakeUpMessage 处理唤醒消息，实现快速响应
func (h *ConnectionHandler) handleWakeUpMessage(ctx context.Context, text string) error {
	h.LogInfo(fmt.Sprintf("[唤醒] [快速响应] 检测到唤醒词: %s", utils.SanitizeForLog(text)))

	// 记录唤醒处理时间
	h.lastWakeUpTime = time.Now()

	// 停止任何正在进行的音频播放
	h.stopServerSpeak()

	// 重置语音停止标志，允许新的唤醒响应播放
	atomic.StoreInt32(&h.serverVoiceStop, 0)

	// 增加对话轮次
	h.talkRound++
	h.roundStartTime = time.Now()
	currentRound := h.talkRound
	h.LogInfo(fmt.Sprintf("[对话] [轮次 %d] 唤醒响应", currentRound))

	// 立即发送STT消息
	err := h.sendSTTMessage(text)
	if err != nil {
		h.LogError(fmt.Sprintf("发送STT消息失败: %v", err))
		return fmt.Errorf("发送STT消息失败: %v", err)
	}

	// 发送TTS开始状态
	if err := h.sendTTSMessage("start", "", 0); err != nil {
		h.LogError(fmt.Sprintf("发送TTS开始状态失败: %v", err))
		return fmt.Errorf("发送TTS开始状态失败: %v", err)
	}

	// 发送开心情绪（唤醒响应应该比较友好）
	if err := h.sendEmotionMessage("happy"); err != nil {
		h.LogError(fmt.Sprintf("发送情绪消息失败: %v", err))
		return fmt.Errorf("发送情绪消息失败: %v", err)
	}

	// 添加用户消息到对话历史
	h.dialogueManager.Put(chat.Message{
		Role:    "user",
		Content: text,
	})

	// 直接回复简单的唤醒响应，不需要调用LLM
	var wakeUpResponses []string
	if h.config.QuickReply.Enabled && len(h.config.QuickReply.Words) > 0 {
		wakeUpResponses = h.config.QuickReply.Words
	} else {
		// 默认回复词
		wakeUpResponses = []string{"在呢", "您好", "我在听", "请讲"}
	}
	responseText := utils.RandomSelectFromArray(wakeUpResponses)

	// 添加助手回复到对话历史
	h.dialogueManager.Put(chat.Message{
		Role:    "assistant",
		Content: responseText,
	})

	// 直接播放响应
	h.tts_last_text_index = 1
	err = h.SpeakAndPlay(responseText, 1, currentRound)
	if err != nil {
		h.LogError(fmt.Sprintf("播放唤醒响应失败: %v", err))
		return fmt.Errorf("播放唤醒响应失败: %v", err)
	}

	h.LogInfo(fmt.Sprintf("[唤醒] [响应完成] %s", responseText))
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
		h.LogWarn("[SystemSpeak] 收到空文本，无法合成语音")
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
	return !h.isDeviceVerified
}

// processTTSQueueCoroutine 处理TTS队列
func (h *ConnectionHandler) processTTSQueueCoroutine() {
	h.LogInfo("[协程] [TTS队列] TTS队列处理协程启动")
	defer h.LogInfo("[协程] [TTS队列] TTS队列处理协程退出")

	for {
		select {
		case <-h.stopChan:
			h.LogDebug("[协程] [TTS队列] 收到停止信号，退出协程")
			return
		case task := <-h.ttsQueue:
			h.processTTSTask(task.text, task.textIndex, task.round, task.filepath)
		}
	}
}

// 服务端打断说话
func (h *ConnectionHandler) stopServerSpeak() {
	h.LogInfo("[服务端] [语音] 停止说话")
	atomic.StoreInt32(&h.serverVoiceStop, 1)
	h.cleanTTSAndAudioQueue(false)
}

func (h *ConnectionHandler) deleteAudioFileIfNeeded(filepath string, reason string) {
	if !h.config.Audio.DeleteAudio || filepath == "" {
		return
	}

	// 检查是否是音乐文件，如果是则不删除
	if utils.IsMusicFile(filepath) {
		h.LogInfo(fmt.Sprintf(reason+" 跳过删除音乐文件: %s", filepath))
		return
	}

	// 检查文件是否存在，避免重复删除
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		h.LogDebug(fmt.Sprintf(reason+" 文件不存在，无需删除: %s", filepath))
		return
	}

	// 删除非缓存音频文件
	if err := os.Remove(filepath); err != nil {
		// 如果文件不存在，这是正常的（可能已被其他地方删除）
		if os.IsNotExist(err) {
			h.LogDebug(fmt.Sprintf(reason+" 文件已被删除: %s", filepath))
		} else {
			h.LogError(fmt.Sprintf(reason+" 删除音频文件失败: %v", err))
		}
	} else {
		h.LogDebug(fmt.Sprintf("%s 已删除音频文件: %s", reason, filepath))
	}
}

// processTTSTask 处理单个TTS任务
func (h *ConnectionHandler) processTTSTask(text string, textIndex int, round int, filepath string) {
	hasAudio := false
	defer func() {
		if hasAudio {
			h.tts_last_audio_index = textIndex
			h.audioMessagesQueue <- struct {
				filepath  string
				text      string
				round     int
				textIndex int
			}{filepath, text, round, textIndex}
		} else {
			h.logger.DebugTag("TTS", "跳过音频任务，样本索引=%d，暂无可播放内容", textIndex)
		}
	}()

	if filepath != "" {
		hasAudio = true
		return
	}

	ttsStartTime := time.Now()
	// 过滤表情
	cleanText := utils.RemoveAllEmoji(text)
	// 移除括号及括号内的内容（如：（语速起飞）、（突然用气声）等）
	cleanText = utils.RemoveParentheses(cleanText)

	if cleanText == "" {
		h.logger.DebugTag("TTS", "收到空文本，索引=%d", textIndex)
		return
	}

	text = cleanText
	logText := utils.SanitizeForLog(text)

	// 生成语音文件
	generatedFile, err := h.ttsManager.ToTTS(text)
	if err != nil {
		h.LogError(fmt.Sprintf("TTS转换失败:text(%s) %v", logText, err))
		return
	}

	filepath = generatedFile
	hasAudio = true
	h.logger.DebugTag("TTS", "转换成功 text=%s index=%d 文件=%s", logText, textIndex, filepath)

	// 发布TTS完成事件
	// TODO: 新架构的事件发布需要重新实现
	// if publisher := tts.GetEventPublisher(h.providers.tts); publisher != nil {
	//     publisher.PublishTTSCompleted(text, textIndex, round, filepath)
	// }

	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.LogInfo(fmt.Sprintf("processTTSTask 服务端语音停止, 不再发送音频数据：%s", logText))
		// 服务端语音停止时，根据配置删除已生成的音频文件
		h.deleteAudioFileIfNeeded(filepath, "服务端语音停止时")
		hasAudio = false
		filepath = ""
		return
	}

	if textIndex == 1 {
		now := time.Now()
		ttsSpentTime := now.Sub(ttsStartTime)
		h.logger.Debug("TTS转换耗时: %s, 文本: %s, 索引: %d", ttsSpentTime, logText, textIndex)
	}

}

// speakAndPlay 合成并播放语音
func (h *ConnectionHandler) SpeakAndPlay(text string, textIndex int, round int) error {
	// 发布TTS说话事件
	// TODO: 新架构的事件发布需要重新实现
	// if publisher := tts.GetEventPublisher(h.providers.tts); publisher != nil {
	//     publisher.SetSessionID(h.sessionID)
	//     publisher.PublishTTSSpeak(text, textIndex, round)
	// }

	// 暂停将客户端音频发送到ASR（避免TTS播放期间触发ASR导致服务端sequence冲突）
	atomic.StoreInt32(&h.asrPause, 1)

	defer func() {
		// 在函数返回时不立即恢复；实际恢复在音频发送完成的地方处理（sendAudioMessage defer）
		// 将任务加入队列，不阻塞当前流程
		h.ttsQueue <- struct {
			text      string
			round     int
			textIndex int
			filepath  string
		}{
			text:      text,
			round:     round,
			textIndex: textIndex,
			filepath:  "",
		}
	}()

	originText := text // 保存原始文本用于日志
	text = utils.RemoveAllEmoji(text)
	text = utils.RemoveMarkdownSyntax(text) // 移除Markdown语法
	if text == "" {
		// 如果清理后的文本为空，可能是纯表情符号或纯Markdown，跳过语音合成
		h.logger.Debug("SpeakAndPlay 跳过空文本分段，原始文本: %s, 索引: %d", utils.SanitizeForLog(originText), textIndex)
		return nil // 不报错，直接跳过
	}

	if atomic.LoadInt32(&h.serverVoiceStop) == 1 { // 服务端语音停止
		h.LogInfo(fmt.Sprintf("speakAndPlay 服务端语音停止, 不再发送音频数据：%s", utils.SanitizeForLog(text)))
		text = ""
		return errors.New("服务端语音已停止，无法合成语音")
	}

	if len(text) > 255 {
		h.logger.Warn("文本过长，超过255字符限制，截断合成语音: %s", utils.SanitizeForLog(text))
		text = text[:255] // 截断文本
	}

	return nil
}

func (h *ConnectionHandler) clearSpeakStatus() {
	h.LogInfo("[服务端] [讲话状态] 已清除")
	h.tts_last_text_index = -1
	h.tts_last_audio_index = -1
	h.providers.asr.Reset() // 重置ASR状态
	// 恢复ASR接收，避免打断后无法重新启动ASR
	atomic.StoreInt32(&h.asrPause, 0)
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
			h.LogInfo(fmt.Sprintf(msgPrefix+"丢弃一个TTS任务: %s", utils.SanitizeForLog(task.text)))
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
			h.LogInfo(fmt.Sprintf(msgPrefix+"丢弃一个音频任务: %s", utils.SanitizeForLog(task.text)))
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
			h.providers.tts.SetVoice(h.initialVoice) // 恢复初始语音
		}
		if h.providers.asr != nil {
			h.providers.asr.ResetSilenceCount() // 重置静音计数
			if err := h.providers.asr.Reset(); err != nil {
				h.LogError(fmt.Sprintf("重置ASR状态失败: %v", err))
			}
			if err := h.providers.asr.CloseConnection(); err != nil {
				h.LogError(fmt.Sprintf("断开ASR状态失败: %v", err))
			}
		}
		h.cleanTTSAndAudioQueue(true)
		// 确保解除ASR暂停标志，避免遗留状态
		atomic.StoreInt32(&h.asrPause, 0)
	})
}

// genResponseByVLLM 使用VLLLM处理包含图片的消息
func (h *ConnectionHandler) genResponseByVLLM(ctx context.Context, messages []providers.Message, imageData domainimage.ImageData, text string, round int) error {
	h.logger.InfoTag("VLLLM", "开始生成回复 %v", map[string]interface{}{
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

// initManagers 初始化新架构的 LLM 和 TTS Manager
func (h *ConnectionHandler) initManagers(config *config.Config) {
	// 获取用户的模型选择
	llmName, ttsName, asrName := h.getUserModelSelection()

	// 初始化 LLM Manager
	if llmName != "" {
		if llmCfg, ok := config.LLM[llmName]; ok {
			llmConfig := domainllminter.LLMConfig{
				Provider:    llmCfg.Type,
				Model:       llmCfg.ModelName,
				APIKey:      llmCfg.APIKey,
				BaseURL:     llmCfg.BaseURL,
				Temperature: float32(llmCfg.Temperature),
				MaxTokens:   llmCfg.MaxTokens,
				Timeout:     60, // 默认超时时间
			}
			h.llmManager = domainllm.NewManager(llmConfig)
			if h.userID != "" {
				h.LogInfo(fmt.Sprintf("使用用户 %s 的LLM提供者: %s (%s)", h.userID, llmName, llmCfg.Type))
			} else {
				h.LogInfo(fmt.Sprintf("使用默认LLM提供者: %s (%s)", llmName, llmCfg.Type))
			}
		} else {
			h.LogError(fmt.Sprintf("LLM 配置不存在: %s", llmName))
		}
	}

	// 初始化 TTS Manager
	if ttsName != "" {
		if ttsCfg, ok := config.TTS[ttsName]; ok {
			ttsConfig := domainttsinter.TTSConfig{
				Provider:        ttsCfg.Type,
				Voice:           ttsCfg.Voice,
				Speed:           1.0, // 默认语速
				Pitch:           1.0, // 默认音调
				Volume:          1.0, // 默认音量
				SampleRate:      24000, // 默认采样率
				Format:          ttsCfg.Format,
				Language:        "zh-CN", // 默认语言
			}
			h.ttsManager = domaintts.NewManager(ttsConfig, config)
			if h.userID != "" {
				h.LogInfo(fmt.Sprintf("使用用户 %s 的TTS提供者: %s (%s)", h.userID, ttsName, ttsCfg.Type))
			} else {
				h.LogInfo(fmt.Sprintf("使用默认TTS提供者: %s (%s)", ttsName, ttsCfg.Type))
			}
		} else {
			h.LogError(fmt.Sprintf("TTS 配置不存在: %s", ttsName))
		}
	}

	// 记录用户的ASR选择（ASR目前不支持动态切换）
	if asrName != "" {
		if h.userID != "" {
			h.LogInfo(fmt.Sprintf("用户 %s 选择的ASR提供者: %s (当前不支持动态切换)", h.userID, asrName))
		} else {
			h.LogInfo(fmt.Sprintf("默认ASR提供者: %s (当前不支持动态切换)", asrName))
		}
	}
}
