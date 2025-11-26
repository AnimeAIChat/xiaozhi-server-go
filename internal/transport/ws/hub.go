package ws

import (
	"sync"

	"xiaozhi-server-go/internal/utils"
)

// Hub tracks the active websocket sessions for a transport instance.
type Hub struct {
	logger   *utils.Logger
	sessions sync.Map // map[string]*Session
}

// NewHub builds a fresh session hub.
func NewHub(logger *utils.Logger) *Hub {
	return &Hub{
		logger: logger,
	}
}

// Register adds a new session to the hub.
func (h *Hub) Register(session *Session) {
	if session == nil {
		return
	}
	h.sessions.Store(session.ID(), session)
}

// Unregister removes the session from the hub.
func (h *Hub) Unregister(id string) {
	if id == "" {
		return
	}
	h.sessions.Delete(id)
}

// CloseAll terminates all active sessions and waits for their shutdown.
func (h *Hub) CloseAll(reason error) {
	if reason == nil {
		reason = ErrSessionShutdown
	}

	h.sessions.Range(func(key, value any) bool {
		if session, ok := value.(*Session); ok {
			session.Close(reason)
		}
		h.sessions.Delete(key)
		return true
	})
}

// Counts exposes the number of active websocket connections.
func (h *Hub) Counts() (clients int, sessions int) {
	h.sessions.Range(func(key, value any) bool {
		clients++
		return true
	})
	return clients, clients
}
