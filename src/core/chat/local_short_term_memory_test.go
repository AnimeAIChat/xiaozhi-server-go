package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLocalShortTermMemoryIsolatesSessionsAndExpires(t *testing.T) {
	store := NewLocalShortTermMemoryStore(10 * time.Millisecond)
	first := &localShortTermMemory{store: store, key: memoryKey("device-a", 1)}
	second := &localShortTermMemory{store: store, key: memoryKey("device-b", 1)}

	manager := NewDialogueManager(nil, first)
	manager.SetShortTermMemoryLimits(DefaultShortTermMemoryMessages, DefaultShortTermMemoryTokens)
	manager.SetSystemMessage("system prompt")
	manager.Put(Message{Role: "user", Content: "remember this"})

	stored, err := first.QueryMemory("")
	if err != nil {
		t.Fatalf("query memory: %v", err)
	}
	var storedMessages []Message
	if err := json.Unmarshal([]byte(stored), &storedMessages); err != nil {
		t.Fatalf("decode memory: %v", err)
	}
	if len(storedMessages) != 1 || storedMessages[0].Role != "user" {
		t.Fatalf("system prompt must not be persisted: %#v", storedMessages)
	}

	reconnected := NewDialogueManager(nil, first)
	if got := reconnected.GetLLMDialogue(); len(got) != 1 || got[0].Content != "remember this" {
		t.Fatalf("same device-agent should resume short memory: %#v", got)
	}
	isolated := NewDialogueManager(nil, second)
	if got := isolated.GetLLMDialogue(); len(got) != 0 {
		t.Fatalf("different device must not receive memory: %#v", got)
	}

	time.Sleep(20 * time.Millisecond)
	if stored, err := first.QueryMemory(""); err != nil || stored != "" {
		t.Fatalf("expired memory should be removed, got %q, %v", stored, err)
	}
}

func TestDialogueManagerAppliesMessageAndEstimatedTokenLimits(t *testing.T) {
	store := NewLocalShortTermMemoryStore(time.Minute)
	memory := &localShortTermMemory{store: store, key: memoryKey("device-a", 1)}
	manager := NewDialogueManager(nil, memory)
	manager.SetShortTermMemoryLimits(4, 12)
	manager.SetSystemMessage("system")
	for i := 0; i < 8; i++ {
		manager.Put(Message{Role: "assistant", Content: "这是一个很长的回复，用来验证短期记忆的令牌边界。"})
	}

	dialogue := manager.GetLLMDialogue()
	if len(dialogue) > 5 {
		t.Fatalf("expected system plus at most four messages, got %#v", dialogue)
	}
	for _, message := range dialogue[1:] {
		if cost := estimateMessageTokens(message); cost > 12 {
			t.Fatalf("message exceeded estimated token budget: %d, %#v", cost, message)
		}
	}
}

func TestTwentySimulatedDevicesKeepConversationsIsolated(t *testing.T) {
	store := NewLocalShortTermMemoryStore(time.Minute)
	const devices = 20

	var waitGroup sync.WaitGroup
	for i := 0; i < devices; i++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			memory := &localShortTermMemory{store: store, key: memoryKey(fmt.Sprintf("device-%02d", index), 1)}
			manager := NewDialogueManager(nil, memory)
			manager.SetShortTermMemoryLimits(DefaultShortTermMemoryMessages, DefaultShortTermMemoryTokens)
			manager.SetSystemMessage("system")
			manager.Put(Message{Role: "user", Content: fmt.Sprintf("device-%02d message", index)})
			manager.Put(Message{Role: "assistant", Content: fmt.Sprintf("reply-%02d", index)})
		}(i)
	}
	waitGroup.Wait()

	for i := 0; i < devices; i++ {
		memory := &localShortTermMemory{store: store, key: memoryKey(fmt.Sprintf("device-%02d", i), 1)}
		reconnected := NewDialogueManager(nil, memory)
		dialogue := reconnected.GetLLMDialogue()
		if len(dialogue) != 2 {
			t.Fatalf("device %d got %d saved messages, want 2", i, len(dialogue))
		}
		for _, message := range dialogue {
			for other := 0; other < devices; other++ {
				if other != i && strings.Contains(message.Content, fmt.Sprintf("-%02d", other)) {
					t.Fatalf("device %d received device %d content: %#v", i, other, dialogue)
				}
			}
		}
	}
}
