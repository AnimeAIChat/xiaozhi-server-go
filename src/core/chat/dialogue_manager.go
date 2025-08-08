package chat

import (
	"encoding/json"

	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"
)

type Message = types.Message

// DialogueManager 管理对话上下文和历史
type DialogueManager struct {
	logger   *utils.Logger
	dialogue []Message
	memory   MemoryInterface
}

// NewDialogueManager 创建对话管理器实例
func NewDialogueManager(logger *utils.Logger, memory MemoryInterface) *DialogueManager {
	return &DialogueManager{
		logger:   logger,
		dialogue: make([]Message, 0),
		memory:   memory,
	}
}

func (dm *DialogueManager) SetSystemMessage(systemMessage string) {
	if systemMessage == "" {
		return
	}

	// 如果对话中已经有系统消息，则不再添加
	if len(dm.dialogue) > 0 && dm.dialogue[0].Role == "system" {
		dm.dialogue[0].Content = systemMessage
		return
	}

	// 添加新的系统消息到对话开头
	dm.dialogue = append([]Message{
		{Role: "system", Content: systemMessage},
	}, dm.dialogue...)
}

// 保留最近的几条对话消息
func (dm *DialogueManager) KeepRecentMessages(maxMessages int) {
	if maxMessages <= 0 || len(dm.dialogue) <= maxMessages {
		return
	}
	// 保留system消息和最近的 maxMessages 条消息
	if len(dm.dialogue) > 0 && dm.dialogue[0].Role == "system" {
		// 保留system消息
		dm.dialogue = append(dm.dialogue[:1], dm.dialogue[len(dm.dialogue)-maxMessages:]...)
		return
	}
	// 如果没有system消息，直接保留最近的 maxMessages 条消息
	if len(dm.dialogue) > maxMessages {
		dm.dialogue = dm.dialogue[len(dm.dialogue)-maxMessages:]
	}
}

// GetRecentMessages 获取最近的对话消息
// 如果 maxMessages <= 0，则返回全部对话消息
func (dm *DialogueManager) GetRecentMessages(maxMessages int) []Message {
	if maxMessages <= 0 || len(dm.dialogue) <= maxMessages {
		return dm.dialogue
	}
	// 保留system消息和最近的 maxMessages 条消息
	if len(dm.dialogue) > 0 && dm.dialogue[0].Role == "system" {
		// 保留system消息
		return append([]Message{dm.dialogue[0]}, dm.dialogue[len(dm.dialogue)-maxMessages:]...)
	}
	return dm.dialogue
}

// Put 添加新消息到对话
func (dm *DialogueManager) Put(message Message) {
	dm.dialogue = append(dm.dialogue, message)
}

func (dm *DialogueManager) GetLastTwoMessages() []Message {
	if len(dm.dialogue) < 2 {
		return nil
	}
	return dm.dialogue[len(dm.dialogue)-2:]
}

// GetLLMDialogue 获取完整对话历史
func (dm *DialogueManager) GetLLMDialogue() []Message {
	return dm.dialogue
}

// GetLLMDialogueWithMemory 获取带记忆的对话
func (dm *DialogueManager) GetLLMDialogueWithMemory(memoryStr string) []Message {
	if memoryStr == "" {
		return dm.GetLLMDialogue()
	}

	memoryMsg := Message{
		Role:    "system",
		Content: memoryStr,
	}

	dialogue := make([]Message, 0, len(dm.dialogue)+1)
	dialogue = append(dialogue, memoryMsg)
	dialogue = append(dialogue, dm.dialogue...)

	return dialogue
}

// Clear 清空对话历史
func (dm *DialogueManager) Clear() {
	dm.dialogue = make([]Message, 0)
}

func (dm *DialogueManager) Length() int {
	return len(dm.dialogue)
}

// ToJSON 将对话历史转换为JSON字符串
func (dm *DialogueManager) ToJSON(keepSystemPrompt bool) (string, error) {
	dialogue := dm.dialogue
	if !keepSystemPrompt && len(dialogue) > 0 && dialogue[0].Role == "system" {
		// 如果不保留系统消息，则移除第一条消息
		dialogue = dialogue[1:]
	}
	bytes, err := json.Marshal(dialogue)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// LoadFromJSON 从JSON字符串加载对话历史
func (dm *DialogueManager) LoadFromJSON(jsonStr string) error {
	return json.Unmarshal([]byte(jsonStr), &dm.dialogue)
}
