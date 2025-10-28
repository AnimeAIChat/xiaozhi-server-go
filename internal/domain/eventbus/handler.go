package eventbus

import (
	"fmt"
	"log"
)

// EventHandler 事件处理器接口
type EventHandler interface {
	Handle(eventType string, data interface{})
}

// DefaultEventHandler 默认事件处理器
type DefaultEventHandler struct{}

// NewDefaultEventHandler 创建默认事件处理器
func NewDefaultEventHandler() *DefaultEventHandler {
	return &DefaultEventHandler{}
}

// Handle 处理事件
func (h *DefaultEventHandler) Handle(eventType string, data interface{}) {
	switch eventType {
	case EventASRResult:
		h.handleASRResult(data.(ASREventData))
	case EventLLMResponse:
		h.handleLLMResponse(data.(LLMEventData))
	case EventTTSSpeak:
		h.handleTTSSpeak(data.(TTSEventData))
	case EventASRError, EventLLMError, EventTTSError:
		h.handleError(data.(SystemEventData))
	default:
		log.Printf("未处理的事件类型: %s", eventType)
	}
}

// handleASRResult 处理ASR结果事件
func (h *DefaultEventHandler) handleASRResult(data ASREventData) {
	fmt.Printf("[事件处理器] ASR结果: 会话=%s, 文本=%s, 最终=%v\n",
		data.SessionID, data.Text, data.IsFinal)
}

// handleLLMResponse 处理LLM响应事件
func (h *DefaultEventHandler) handleLLMResponse(data LLMEventData) {
	fmt.Printf("[事件处理器] LLM响应: 会话=%s, 轮次=%d, 内容=%s, 最终=%v\n",
		data.SessionID, data.Round, data.Content, data.IsFinal)
}

// handleTTSSpeak 处理TTS说话事件
func (h *DefaultEventHandler) handleTTSSpeak(data TTSEventData) {
	fmt.Printf("[事件处理器] TTS说话: 会话=%s, 轮次=%d, 文本索引=%d, 文本=%s\n",
		data.SessionID, data.Round, data.TextIndex, data.Text)
}

// handleError 处理错误事件
func (h *DefaultEventHandler) handleError(data SystemEventData) {
	fmt.Printf("[事件处理器] 系统错误: 级别=%s, 消息=%s\n",
		data.Level, data.Message)
}

// SetupEventHandlers 设置事件处理器
func SetupEventHandlers() {
	handler := NewDefaultEventHandler()

	// 订阅同步事件
	Subscribe(EventASRResult, func(args ...interface{}) {
		if len(args) > 0 {
			handler.Handle(EventASRResult, args[0])
		}
	})

	Subscribe(EventLLMResponse, func(args ...interface{}) {
		if len(args) > 0 {
			handler.Handle(EventLLMResponse, args[0])
		}
	})

	Subscribe(EventTTSSpeak, func(args ...interface{}) {
		if len(args) > 0 {
			handler.Handle(EventTTSSpeak, args[0])
		}
	})

	Subscribe(EventASRError, func(args ...interface{}) {
		if len(args) > 0 {
			handler.Handle(EventASRError, args[0])
		}
	})

	Subscribe(EventLLMError, func(args ...interface{}) {
		if len(args) > 0 {
			handler.Handle(EventLLMError, args[0])
		}
	})

	Subscribe(EventTTSError, func(args ...interface{}) {
		if len(args) > 0 {
			handler.Handle(EventTTSError, args[0])
		}
	})
}