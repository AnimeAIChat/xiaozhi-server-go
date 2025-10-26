package stepfun

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"xiaozhi-server-go/src/core/providers/asr"
	"xiaozhi-server-go/src/core/utils"

	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
)

// Ensure Provider implements asr.Provider interface
var _ asr.Provider = (*Provider)(nil)

// Provider 阶跃ASR提供者实现
type Provider struct {
	*asr.BaseProvider

	// 配置
	apiKey string
	model  string
	voice  string
	wsURL  string
	logger *utils.Logger
	prompt string

	// 流式识别相关字段
	conn        *websocket.Conn
	isStreaming bool
	result      string
	err         error
	connMutex   sync.Mutex
}

func NewProvider(config *asr.Config, deleteFile bool, logger *utils.Logger) (*Provider, error) {
	base := asr.NewBaseProvider(config, deleteFile)

	// 从config.Data中获取配置
	apiKey, ok := config.Data["api_key"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少api_key配置")
	}

	model, ok := config.Data["model"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少model配置")
	}
	voice, ok := config.Data["voice"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少voice配置")
	}

	prompt, ok := config.Data["prompt"].(string)
	if !ok {
		prompt = "你是由阶跃星辰提供的AI聊天助手，你擅长中文、英文及多语种对话。"
	}

	provider := &Provider{
		BaseProvider: base,
		apiKey:       apiKey,
		model:        model,
		voice:        voice,
		wsURL:        fmt.Sprintf("wss://api.stepfun.com/v1/realtime?model=%s", model),
		logger:       logger,
		prompt:       prompt,
	}

	provider.InitAudioProcessing()

	return provider, nil
}

// Ensure Provider implements asr.Provider interface
var _ asr.Provider = (*Provider)(nil)

// AddAudio 添加音频数据到缓冲区
func (p *Provider) AddAudio(data []byte) error {
	return p.AddAudioWithContext(context.Background(), data)
}

// AddAudioWithContext 带上下文添加音频数据
func (p *Provider) AddAudioWithContext(ctx context.Context, data []byte) error {
	// 检查并启动流式识别
	p.connMutex.Lock()
	isStreaming := p.isStreaming
	p.connMutex.Unlock()

	if !isStreaming {
		if err := p.StartStreaming(ctx); err != nil {
			return err
		}
	}

	// 发送音频数据（Base64编码）
	if len(data) > 0 {
		if err := p.sendAppendAudio(data); err != nil {
			return err
		}
	}

	return nil
}

// Transcribe 直接识别整段音频（简化：流式通道发送并等待回调结果）
func (p *Provider) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	if p.isStreaming {
		return "", fmt.Errorf("正在进行流式识别, 请先调用Reset")
	}
	if err := p.StartStreaming(ctx); err != nil {
		return "", err
	}
	defer p.Cleanup()

	if err := p.AddAudioWithContext(ctx, audioData); err != nil {
		return "", err
	}
	// 这里不阻塞等待最终结果，由监听器回调返回
	return p.result, nil
}

// StartStreaming 建立与Step Realtime的WebSocket连接并发送session.update
func (p *Provider) StartStreaming(ctx context.Context) error {
	p.logger.Info("----开始Step流式识别----")
	p.ResetStartListenTime()

	if p.isStreaming {
		p.logger.Debug("Step流式识别已启动，跳过初始化")
		return nil
	}

	// 加锁保护连接初始化
	p.connMutex.Lock()
	// 确保旧连接关闭
	if p.conn != nil {
		p.logger.Debug("Step流式识别关闭旧连接")
		p.closeConnection()
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))

	conn, resp, err := dialer.DialContext(ctx, p.wsURL, headers)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return fmt.Errorf("WebSocket连接失败(状态码:%d): %v", status, err)
	}
	p.logger.Debug("Step流式识别建立WebSocket连接成功")
	p.conn = conn
	p.connMutex.Unlock()

	// 发送 session.update
	sessionPayload := map[string]interface{}{
		"event_id": fmt.Sprintf("event_%d", time.Now().UnixNano()),
		"type":     "session.update",
		"session": map[string]interface{}{
			"modalities":          []string{"text", "audio"},
			"instructions":        p.prompt,
			"voice":               p.voice,
			"input_audio_format":  "pcm16",
			"output_audio_format": "pcm16",
			"turn_detection": map[string]interface{}{
				"type":                       "server_vad",
				"energy_awakeness_threshold": 100, // 放大激活阈值
			},
		},
	}
	if err := p.sendJSON(sessionPayload); err != nil {
		p.connMutex.Lock()
		_ = p.conn.Close()
		p.conn = nil
		p.connMutex.Unlock()
		return fmt.Errorf("发送session.update失败: %v", err)
	}

	// 标记流式识别并启动读取协程
	p.isStreaming = true
	go p.readLoop()
	return nil
}

func (p *Provider) sendAppendAudio(data []byte) error {
	// 将PCM16字节编码为Base64
	encoded := base64.StdEncoding.EncodeToString(data)
	payload := map[string]interface{}{
		"event_id": fmt.Sprintf("event_%d", time.Now().UnixNano()),
		"type":     "input_audio_buffer.append",
		"audio":    encoded,
	}

	return p.sendJSON(payload)
}

func (p *Provider) sendJSON(v interface{}) error {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	if p.conn == nil {
		return fmt.Errorf("WebSocket连接不存在")
	}
	bytes, err := sonic.Marshal(v)
	if err != nil {
		return err
	}
	return p.conn.WriteMessage(websocket.TextMessage, bytes)
}

func (p *Provider) readLoop() {
	p.logger.Info("Step流式识别协程已启动")
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("Step流式识别协程panic: %v", r)
		}
		p.connMutex.Lock()
		p.isStreaming = false
		if p.conn != nil {
			p.closeConnection()
		}
		p.connMutex.Unlock()
		p.logger.Info("Step流式识别协程已结束")
	}()

	var baseEvent BaseEvent
	for {
		if !p.isStreaming || p.conn == nil {
			return
		}
		conn := p.conn

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			p.setErrorAndStop(err)
			return
		}
		if msgType != websocket.TextMessage {
			// 忽略非文本帧
			continue
		}

		if err := sonic.Unmarshal(data, &baseEvent); err != nil {
			p.setErrorAndStop(fmt.Errorf("解析服务端事件失败: %v", err))
			return
		}

		p.logger.Debug("Step流式识别事件类型: %s", baseEvent.Type)
		switch baseEvent.Type {
		case "error":
			e := ErrorEvent{}
			if err := sonic.Unmarshal(data, &e); err != nil {
				p.logger.Error("解析服务端事件失败: %v", err)
				return
			}
			p.setErrorAndStop(fmt.Errorf("服务端错误: %v", e.Error.Message))
			return
		case "session.created":
			e := SessionCreatedEvent{}
			if err := sonic.Unmarshal(data, &e); err != nil {
				p.logger.Error("解析服务端事件失败: %v", err)
				return
			}
			p.logger.Info("type: %s, sessionID: %s", e.Type, e.Session.ID)
		case "session.updated", "input_audio_buffer.speech_started", "input_audio_buffer.speech_stopped", "input_audio_buffer.committed", "input_audio_buffer.cleared":
			// 无需特殊处理
			continue
		case "conversation.item.input_audio_transcription.completed":
			e := ConversationItemInputAudioTranscriptionCompletedEvent{}
			if err := sonic.Unmarshal(data, &e); err != nil {
				p.logger.Error("解析服务端事件失败: %v", err)
				return
			}
			// 读取转写结果
			text := e.Transcript
			p.logger.Debug("[DEBUG] Step识别结果: %s", text)
			p.connMutex.Lock()
			p.result = text
			p.connMutex.Unlock()

			if listener := p.BaseProvider.GetListener(); listener != nil {
				if text == "" && p.SilenceTime() > 30*time.Second {
					p.BaseProvider.SilenceCount += 1
					text = "你没有听清我说话"
				} else if text != "" {
					p.BaseProvider.SilenceCount = 0
				}
				if finished := listener.OnAsrResult(text); finished {
					return
				}
			}
		default:
			// 其他事件忽略
		}
	}
}

func (p *Provider) setErrorAndStop(err error) {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()
	p.err = err
	p.isStreaming = false
	msg := err.Error()
	if strings.Contains(msg, "use of closed network connection") {
		p.logger.Debug("Step setErrorAndStop: %v", err)
	} else {
		p.logger.Error("Step setErrorAndStop: %v", err)
	}
	if p.conn != nil {
		p.closeConnection()
	}
}

func (p *Provider) closeConnection() {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("关闭连接时发生错误: %v", r)
		}
	}()
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

// Reset 重置ASR状态
func (p *Provider) Reset() error {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	p.isStreaming = false
	p.closeConnection()
	p.result = ""
	p.err = nil

	// 重置音频处理
	p.InitAudioProcessing()
	p.logger.Info("Step ASR状态已重置")
	return nil
}

// Initialize 初始化
func (p *Provider) Initialize() error { return nil }

// Cleanup 清理资源
func (p *Provider) Cleanup() error {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()
	p.closeConnection()
	p.logger.Info("Step ASR资源已清理")
	return nil
}

func init() {
	// 注册阶跃ASR提供者
	asr.Register("stepfun", func(config *asr.Config, deleteFile bool, logger *utils.Logger) (asr.Provider, error) {
		return NewProvider(config, deleteFile, logger)
	})
}
