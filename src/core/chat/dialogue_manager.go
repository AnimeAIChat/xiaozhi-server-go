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

// Put 添加新消息到对话
func (dm *DialogueManager) Put(message Message) {
	dm.dialogue = append(dm.dialogue, message)
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

// ToJSON 将对话历史转换为JSON字符串
func (dm *DialogueManager) ToJSON() (string, error) {
	bytes, err := json.Marshal(dm.dialogue)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// LoadFromJSON 从JSON字符串加载对话历史
func (dm *DialogueManager) LoadFromJSON(jsonStr string) error {
	return json.Unmarshal([]byte(jsonStr), &dm.dialogue)
}
