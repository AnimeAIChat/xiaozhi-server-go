package chat

import (
	"encoding/json"
	"strconv"
	"sync"
	"time"
	"unicode"
)

const (
	// DefaultShortTermMemoryMessages keeps approximately six user-assistant turns.
	DefaultShortTermMemoryMessages = 12
	// DefaultShortTermMemoryTokens is a provider-independent local estimate.
	DefaultShortTermMemoryTokens = 3000
	DefaultShortTermMemoryTTL    = 30 * time.Minute
)

type localShortTermMemoryEntry struct {
	dialogue  []Message
	expiresAt time.Time
}

// LocalShortTermMemoryStore is intentionally in-memory only. It gives personal
// deployments short-lived continuity without persisting conversation text to DB.
type LocalShortTermMemoryStore struct {
	mu       sync.Mutex
	ttl      time.Duration
	sessions map[string]localShortTermMemoryEntry
}

func NewLocalShortTermMemoryStore(ttl time.Duration) *LocalShortTermMemoryStore {
	if ttl <= 0 {
		ttl = DefaultShortTermMemoryTTL
	}
	return &LocalShortTermMemoryStore{ttl: ttl, sessions: make(map[string]localShortTermMemoryEntry)}
}

var defaultLocalShortTermMemoryStore = NewLocalShortTermMemoryStore(DefaultShortTermMemoryTTL)

// NewDeviceAgentShortTermMemory creates one isolated short-lived memory bucket.
func NewDeviceAgentShortTermMemory(deviceID string, agentID uint) MemoryInterface {
	if deviceID == "" || agentID == 0 {
		return nil
	}
	return &localShortTermMemory{store: defaultLocalShortTermMemoryStore, key: memoryKey(deviceID, agentID)}
}

// ClearDeviceAgentShortTermMemory removes the in-memory history for one device-agent pair.
func ClearDeviceAgentShortTermMemory(deviceID string, agentID uint) {
	if deviceID == "" || agentID == 0 {
		return
	}
	defaultLocalShortTermMemoryStore.clear(memoryKey(deviceID, agentID))
}

type localShortTermMemory struct {
	store *LocalShortTermMemoryStore
	key   string
}

func (m *localShortTermMemory) QueryMemory(_ string) (string, error) {
	dialogue := m.store.load(m.key)
	if len(dialogue) == 0 {
		return "", nil
	}
	data, err := json.Marshal(dialogue)
	return string(data), err
}

func (m *localShortTermMemory) SaveMemory(dialogue []Message) error {
	m.store.save(m.key, dialogue)
	return nil
}

func (m *localShortTermMemory) ClearMemory() error {
	m.store.clear(m.key)
	return nil
}

func (m *localShortTermMemory) EndSession() {
	m.store.expireAfterGracePeriod(m.key)
}

func (s *LocalShortTermMemoryStore) load(key string) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpiredLocked(time.Now())
	entry, ok := s.sessions[key]
	if !ok {
		return nil
	}
	return cloneMessages(entry.dialogue)
}

func (s *LocalShortTermMemoryStore) save(key string, dialogue []Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanExpiredLocked(time.Now())
	s.sessions[key] = localShortTermMemoryEntry{dialogue: cloneMessages(dialogue), expiresAt: time.Now().Add(s.ttl)}
}

func (s *LocalShortTermMemoryStore) clear(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key)
}

func (s *LocalShortTermMemoryStore) expireAfterGracePeriod(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessions[key]
	if ok {
		entry.expiresAt = time.Now().Add(s.ttl)
		s.sessions[key] = entry
	}
}

func (s *LocalShortTermMemoryStore) cleanExpiredLocked(now time.Time) {
	for key, entry := range s.sessions {
		if !entry.expiresAt.After(now) {
			delete(s.sessions, key)
		}
	}
}

func memoryKey(deviceID string, agentID uint) string {
	return deviceID + "\x00" + strconv.FormatUint(uint64(agentID), 10)
}

func estimateMessageTokens(message Message) int {
	return estimateTokens(message.Role+message.Content+message.ToolCallID) + len(message.ToolCalls)*8
}

// estimateTokens is deliberately conservative: CJK characters count as one
// token and groups of four ASCII word characters count as one token.
func estimateTokens(text string) int {
	tokens, asciiWordLength := 0, 0
	flushASCII := func() {
		if asciiWordLength > 0 {
			tokens += (asciiWordLength + 3) / 4
			asciiWordLength = 0
		}
	}
	for _, r := range text {
		if r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			asciiWordLength++
			continue
		}
		flushASCII()
		if !unicode.IsSpace(r) {
			tokens++
		}
	}
	flushASCII()
	return tokens
}
