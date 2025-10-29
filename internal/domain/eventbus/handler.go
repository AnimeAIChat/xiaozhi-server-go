package eventbus

import (
	"fmt"
	"xiaozhi-server-go/src/core/utils"
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
		utils.DefaultLogger.Info(fmt.Sprintf("[事件处理器] 未处理的事件类型: %s", eventType))
	}
}

// handleASRResult 处理ASR结果事件
func (h *DefaultEventHandler) handleASRResult(data ASREventData) {
	// ASR结果现在直接通过listener.OnAsrResult处理，不再通过事件总线
	// 这里保留日志记录用于调试
	utils.DefaultLogger.InfoASR("[事件处理器] ASR结果: 文本=%s, 最终=%v", data.Text, data.IsFinal)
}

// handleLLMResponse 处理LLM响应事件
func (h *DefaultEventHandler) handleLLMResponse(data LLMEventData) {
	if data.IsFinal {
		utils.DefaultLogger.InfoLLM("[事件处理器] [轮次 %d] %s (最终)", data.Round, utils.SanitizeForLog(data.Content))
	} else {
		utils.DefaultLogger.InfoLLM("[事件处理器] [轮次 %d] %s", data.Round, utils.SanitizeForLog(data.Content))
	}
}

// handleTTSSpeak 处理TTS说话事件
func (h *DefaultEventHandler) handleTTSSpeak(data TTSEventData) {
	utils.DefaultLogger.InfoTTS("[事件处理器] [轮次 %d] [文本索引 %d] TTS说话: %s", data.Round, data.TextIndex, data.Text)
}

// handleError 处理错误事件
func (h *DefaultEventHandler) handleError(data SystemEventData) {
	utils.DefaultLogger.Info(fmt.Sprintf("[事件处理器] 系统错误: 级别=%s, 消息=%s", data.Level, data.Message))
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