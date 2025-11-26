package stepfun

// BaseEvent 公共事件字段
type BaseEvent struct {
	EventID string `json:"event_id,omitempty"`
	Type    string `json:"type"`
}

// Error 事件
type ErrorDetail struct {
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	EventID string `json:"event_id,omitempty"`
}

type ErrorEvent struct {
	BaseEvent
	Error ErrorDetail `json:"error"`
}

// Session 会话对象
type Session struct {
	ID                      string   `json:"id,omitempty"`
	Object                  string   `json:"object,omitempty"`
	Model                   string   `json:"model,omitempty"`
	Modalities              []string `json:"modalities,omitempty"`
	Instructions            string   `json:"instructions,omitempty"`
	Voice                   string   `json:"voice,omitempty"`
	InputAudioFormat        string   `json:"input_audio_format,omitempty"`
	OutputAudioFormat       string   `json:"output_audio_format,omitempty"`
	MaxResponseOutputTokens string   `json:"max_response_output_tokens,omitempty"`
}

// Session 相关事件
type SessionCreatedEvent struct {
	BaseEvent
	Session Session `json:"session"`
}

type SessionUpdatedEvent struct {
	BaseEvent
	Session Session `json:"session"`
}

// VAD 事件
type SpeechStartedEvent struct {
	BaseEvent
	AudioStartMS int64  `json:"audio_start_ms,omitempty"`
	ItemID       string `json:"item_id,omitempty"`
}

type SpeechStoppedEvent struct {
	BaseEvent
	AudioEndMS int64  `json:"audio_end_ms,omitempty"`
	ItemID     string `json:"item_id,omitempty"`
	ResponseID string `json:"response_id,omitempty"`
}

// 音频内容流式事件
type ResponseAudioDeltaEvent struct {
	BaseEvent
	ResponseID  string `json:"response_id,omitempty"`
	ItemID      string `json:"item_id,omitempty"`
	OutputIndex int    `json:"output_index,omitempty"`
	Delta       string `json:"delta"`
}

type ResponseAudioDoneEvent struct {
	BaseEvent
	ResponseID string `json:"response_id,omitempty"`
	ItemID     string `json:"item_id,omitempty"`
}

// 音频转录流式事件
type ResponseAudioTranscriptDeltaEvent struct {
	BaseEvent
	ResponseID  string `json:"response_id,omitempty"`
	ItemID      string `json:"item_id,omitempty"`
	OutputIndex int    `json:"output_index,omitempty"`
	Delta       string `json:"delta"`
}

type ResponseAudioTranscriptDoneEvent struct {
	BaseEvent
	ResponseID   string `json:"response_id,omitempty"`
	ItemID       string `json:"item_id,omitempty"`
	OutputIndex  int    `json:"output_index,omitempty"`
	ContentIndex int    `json:"content_index,omitempty"`
	Transcript   string `json:"transcript"`
}

// 会话消息结构
type MessageContentPart struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Audio      string `json:"audio,omitempty"`
	Transcript string `json:"transcript,omitempty"`
}

type MessageItem struct {
	ID      string               `json:"id,omitempty"`
	Object  string               `json:"object,omitempty"`
	Type    string               `json:"type"`
	Status  string               `json:"status,omitempty"`
	Role    string               `json:"role,omitempty"`
	Content []MessageContentPart `json:"content,omitempty"`
}

// 会话消息事件
type ConversationItemCreatedEvent struct {
	BaseEvent
	PreviousItemID string      `json:"previous_item_id,omitempty"`
	Item           MessageItem `json:"item"`
}

type ConversationItemDeletedEvent struct {
	BaseEvent
	ItemID string `json:"item_id"`
}

type ConversationItemInputAudioTranscriptionCompletedEvent struct {
	BaseEvent
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	Transcript   string `json:"transcript"`
}

// 输入音频缓冲区事件
type InputAudioBufferCommittedEvent struct {
	BaseEvent
	PreviousItemID string `json:"previous_item_id,omitempty"`
	ItemID         string `json:"item_id"`
}

type InputAudioBufferClearedEvent struct {
	BaseEvent
}

// 推理输出项目事件
type ResponseOutputItemAddedEvent struct {
	BaseEvent
	ResponseID  string      `json:"response_id,omitempty"`
	OutputIndex int         `json:"output_index"`
	Item        MessageItem `json:"item"`
}

type ResponseOutputItemDoneEvent struct {
	BaseEvent
	ResponseID  string      `json:"response_id,omitempty"`
	OutputIndex int         `json:"output_index"`
	Item        MessageItem `json:"item"`
}

type ResponseContentPartAddedEvent struct {
	BaseEvent
	ResponseID   string             `json:"response_id,omitempty"`
	ItemID       string             `json:"item_id,omitempty"`
	OutputIndex  int                `json:"output_index,omitempty"`
	ContentIndex int                `json:"content_index"`
	Part         MessageContentPart `json:"part"`
}

type ResponseContentPartDoneEvent struct {
	BaseEvent
	ResponseID   string             `json:"response_id,omitempty"`
	ItemID       string             `json:"item_id,omitempty"`
	OutputIndex  int                `json:"output_index,omitempty"`
	ContentIndex int                `json:"content_index"`
	Part         MessageContentPart `json:"part"`
}

// Response 对象及事件
type Response struct {
	ID            string        `json:"id"`
	Object        string        `json:"object"`
	Status        string        `json:"status"`
	StatusDetails interface{}   `json:"status_details"`
	Output        []MessageItem `json:"output"`
}

type ResponseCreatedEvent struct {
	BaseEvent
	Response Response `json:"response,omitempty"`
}

type ResponseDoneEvent struct {
	BaseEvent
	Response Response `json:"response"`
}
