package websocket

import (
	internalws "xiaozhi-server-go/internal/transport/ws"

	"github.com/gorilla/websocket"
)

// WebSocketConnection is kept for backwards compatibility with legacy imports.
type WebSocketConnection = internalws.Connection

// NewWebSocketConnection proxies to the refactored connection constructor.
func NewWebSocketConnection(id string, conn *websocket.Conn) *internalws.Connection {
	return internalws.NewConnection(id, conn)
}
