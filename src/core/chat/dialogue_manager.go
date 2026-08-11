package chat

import (
	"encoding/json"
	"sync"

	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"
)

type Message = types.Message

// DialogueManager 管理对话上下文和历史
type DialogueManager struct {
	logger   *utils.Logger
	dialogue []Message
	memory   MemoryInterface
	mu       sync.RWMutex

	maxMessages        int
	maxEstimatedTokens int
}

// NewDialogueManager 创建对话管理器实例
func NewDialogueManager(logger *utils.Logger, memory MemoryInterface) *DialogueManager {
	dm := &DialogueManager{
		logger:   logger,
		dialogue: make([]Message, 0),
		memory:   memory,
	}
	if memory == nil {
		return dm
	}

	saved, err := memory.QueryMemory("")
	if err == nil && saved != "" {
		if err := json.Unmarshal([]byte(saved), &dm.dialogue); err != nil && logger != nil {
			logger.Warn("加载短期记忆失败，将使用空会话: %v", err)
		}
	}
	return dm
}

func (dm *DialogueManager) SetSystemMessage(systemMessage string) {
	if systemMessage == "" {
		return
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

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

func (dm *DialogueManager) RemoveSecondMessageForToolType() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.removeSecondMessageForToolTypeLocked()
}

func (dm *DialogueManager) removeSecondMessageForToolTypeLocked() {
	// 如果第二条的类型是"role": "tool",则移除这条
	if len(dm.dialogue) < 2 || dm.dialogue[1].Role != "tool" {
		return
	}
	dm.dialogue = append(dm.dialogue[:1], dm.dialogue[2:]...)
}

// 保留最近的几条对话消息
func (dm *DialogueManager) KeepRecentMessages(maxMessages int) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.keepRecentMessagesLocked(maxMessages)
	dm.saveMemoryLocked()
}

func (dm *DialogueManager) keepRecentMessagesLocked(maxMessages int) {
	if maxMessages <= 0 || len(dm.dialogue) <= maxMessages {
		return
	}
	// 保留system消息和最近的 maxMessages 条消息
	if len(dm.dialogue) > 0 && dm.dialogue[0].Role == "system" {
		// 保留system消息
		dm.dialogue = append(dm.dialogue[:1], dm.dialogue[len(dm.dialogue)-maxMessages:]...)
		dm.removeSecondMessageForToolTypeLocked()
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
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if maxMessages <= 0 || len(dm.dialogue) <= maxMessages {
		return cloneMessages(dm.dialogue)
	}
	// 保留system消息和最近的 maxMessages 条消息
	if len(dm.dialogue) > 0 && dm.dialogue[0].Role == "system" {
		// 保留system消息
		return cloneMessages(append([]Message{dm.dialogue[0]}, dm.dialogue[len(dm.dialogue)-maxMessages:]...))
	}
	return cloneMessages(dm.dialogue)
}

// Put 添加新消息到对话
func (dm *DialogueManager) Put(message Message) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	// 如果最近一条是user消息且当前也是user消息，则插入一个空的assistant消息
	if len(dm.dialogue) > 0 && dm.dialogue[len(dm.dialogue)-1].Role == "user" && message.Role == "user" {
		dm.dialogue = append(dm.dialogue, Message{Role: "assistant", Content: "..."})
	}
	dm.dialogue = append(dm.dialogue, message)
	dm.applyLimitsLocked()
	dm.saveMemoryLocked()
}

func (dm *DialogueManager) GetLastTwoMessages() []Message {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	if len(dm.dialogue) < 2 {
		return nil
	}
	return cloneMessages(dm.dialogue[len(dm.dialogue)-2:])
}

// GetLLMDialogue 获取完整对话历史
func (dm *DialogueManager) GetLLMDialogue() []Message {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return cloneMessages(dm.dialogue)
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

	dm.mu.RLock()
	defer dm.mu.RUnlock()
	dialogue := make([]Message, 0, len(dm.dialogue)+1)
	dialogue = append(dialogue, memoryMsg)
	dialogue = append(dialogue, cloneMessages(dm.dialogue)...)

	return dialogue
}

// Clear 清空对话历史
func (dm *DialogueManager) Clear() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.dialogue = make([]Message, 0)
	if dm.memory != nil {
		_ = dm.memory.ClearMemory()
	}
}

func (dm *DialogueManager) Length() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return len(dm.dialogue)
}

// ToJSON 将对话历史转换为JSON字符串
func (dm *DialogueManager) ToJSON(keepSystemPrompt bool) (string, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
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
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if err := json.Unmarshal([]byte(jsonStr), &dm.dialogue); err != nil {
		return err
	}
	dm.applyLimitsLocked()
	dm.saveMemoryLocked()
	return nil
}

// SetShortTermMemoryLimits applies predictable bounds to the non-system history.
// Token usage is estimated locally so this remains provider-independent.
func (dm *DialogueManager) SetShortTermMemoryLimits(maxMessages, maxEstimatedTokens int) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.maxMessages = maxMessages
	dm.maxEstimatedTokens = maxEstimatedTokens
	dm.applyLimitsLocked()
	dm.saveMemoryLocked()
}

// EndSession marks the memory entry for expiry after its configured grace period.
func (dm *DialogueManager) EndSession() {
	if memory, ok := dm.memory.(interface{ EndSession() }); ok {
		memory.EndSession()
	}
}

func (dm *DialogueManager) applyLimitsLocked() {
	if dm.maxMessages > 0 {
		dm.keepRecentMessagesLocked(dm.maxMessages)
	}
	if dm.maxEstimatedTokens <= 0 || len(dm.dialogue) == 0 {
		return
	}

	start := 0
	var system *Message
	if dm.dialogue[0].Role == "system" {
		copy := dm.dialogue[0]
		system = &copy
		start = 1
	}
	kept := make([]Message, 0, len(dm.dialogue)-start)
	used := 0
	for i := len(dm.dialogue) - 1; i >= start; i-- {
		message := dm.dialogue[i]
		cost := estimateMessageTokens(message)
		if len(kept) == 0 && cost > dm.maxEstimatedTokens {
			message = truncateMessageToTokenBudget(message, dm.maxEstimatedTokens)
			cost = estimateMessageTokens(message)
		}
		if len(kept) > 0 && used+cost > dm.maxEstimatedTokens {
			break
		}
		kept = append(kept, message)
		used += cost
	}
	for left, right := 0, len(kept)-1; left < right; left, right = left+1, right-1 {
		kept[left], kept[right] = kept[right], kept[left]
	}
	if system != nil {
		dm.dialogue = append([]Message{*system}, kept...)
	} else {
		dm.dialogue = kept
	}
	dm.removeSecondMessageForToolTypeLocked()
}

func (dm *DialogueManager) saveMemoryLocked() {
	if dm.memory == nil {
		return
	}
	dialogue := dm.dialogue
	if len(dialogue) > 0 && dialogue[0].Role == "system" {
		dialogue = dialogue[1:]
	}
	if err := dm.memory.SaveMemory(cloneMessages(dialogue)); err != nil && dm.logger != nil {
		dm.logger.Warn("保存短期记忆失败: %v", err)
	}
}

func cloneMessages(messages []Message) []Message {
	cloned := make([]Message, len(messages))
	copy(cloned, messages)
	for i := range cloned {
		cloned[i].ToolCalls = append([]types.ToolCall(nil), messages[i].ToolCalls...)
	}
	return cloned
}

func truncateMessageToTokenBudget(message Message, budget int) Message {
	if budget <= 0 || estimateMessageTokens(message) <= budget {
		return message
	}
	runes := []rune(message.Content)
	base := message
	base.Content = ""
	if estimateMessageTokens(base) > budget {
		return base
	}
	left, right, best := 0, len(runes), len(runes)
	for left <= right {
		middle := (left + right) / 2
		candidate := base
		candidate.Content = string(runes[middle:])
		if estimateMessageTokens(candidate) <= budget {
			best = middle
			right = middle - 1
		} else {
			left = middle + 1
		}
	}
	base.Content = string(runes[best:])
	return base
}
