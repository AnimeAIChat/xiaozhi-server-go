package task

import (
	"encoding/json"
	"xiaozhi-server-go/src/core/interfaces"
)

// MessageCallback implements TaskCallback for sending messages to clients
type MessageCallback struct {
	conn    interfaces.Conn
	msgType string
	msgCMD  string
}

// NewMessageCallback creates a new MessageCallback instance
func NewMessageCallback(conn interfaces.Conn, msgType string, msgCmd string) *MessageCallback {
	return &MessageCallback{conn: conn, msgType: msgType, msgCMD: msgCmd}
}

func (mc *MessageCallback) OnComplete(result interface{}) {
	msg := map[string]interface{}{
		"type":   mc.msgType,
		"cmd":    mc.msgCMD,
		"result": result,
	}
	data, _ := json.Marshal(msg)
	mc.conn.WriteMessage(1, data)
}

func (mc *MessageCallback) OnError(err error) {
	msg := map[string]interface{}{
		"type":  "task_error",
		"error": err.Error(),
	}
	data, _ := json.Marshal(msg)
	mc.conn.WriteMessage(1, data)
}

// VoiceCallback implements TaskCallback for voice responses
type VoiceCallback struct {
	conn interfaces.ConnectionHandler
}

// NewVoiceCallback creates a new VoiceCallback instance
func NewVoiceCallback(conn interfaces.ConnectionHandler) *VoiceCallback {
	return &VoiceCallback{conn: conn}
}

func (vc *VoiceCallback) OnComplete(result interface{}) {
	// Convert result to text and use speakAndPlay
	if text, ok := result.(string); ok {
		vc.conn.SpeakAndPlay(text, 0)
	}
}

func (vc *VoiceCallback) OnError(err error) {
	// Speak error message
	vc.conn.SpeakAndPlay("任务执行失败: "+err.Error(), 0)
}

// ActionCallback implements TaskCallback for custom actions
type ActionCallback struct {
	successAction func(interface{})
	errorAction   func(error)
}

// NewActionCallback creates a new ActionCallback instance
func NewActionCallback(onSuccess func(interface{}), onError func(error)) *ActionCallback {
	return &ActionCallback{
		successAction: onSuccess,
		errorAction:   onError,
	}
}

func (ac *ActionCallback) OnComplete(result interface{}) {
	if ac.successAction != nil {
		ac.successAction(result)
	}
}

func (ac *ActionCallback) OnError(err error) {
	if ac.errorAction != nil {
		ac.errorAction(err)
	}
}
