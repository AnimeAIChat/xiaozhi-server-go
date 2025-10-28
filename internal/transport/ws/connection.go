package ws

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"xiaozhi-server-go/internal/domain/mcp"
)

// Connection wraps a gorilla websocket connection and implements the
// src/core.Connection interface used across the legacy stack.
type Connection struct {
	id         string
	socket     *websocket.Conn
	mu         sync.Mutex
	closed     atomic.Bool
	lastActive atomic.Int64
	mcpHolder  atomic.Pointer[mcp.Manager]
}

// NewConnection creates a tracked websocket connection.
func NewConnection(id string, socket *websocket.Conn) *Connection {
	conn := &Connection{
		id:     id,
		socket: socket,
	}
	conn.touch()
	return conn
}

// WriteMessage sends a message to the client.
func (c *Connection) WriteMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed.Load() {
		return fmt.Errorf("connection %s already closed", c.id)
	}

	if err := c.socket.WriteMessage(messageType, data); err != nil {
		return err
	}

	c.touch()
	return nil
}

// ReadMessage receives a message from the client. The stopChan is currently
// ignored to preserve the legacy behaviour.
func (c *Connection) ReadMessage(stopChan <-chan struct{}) (int, []byte, error) {
	messageType, payload, err := c.socket.ReadMessage()
	if err == nil {
		c.touch()
	}
	return messageType, payload, err
}

// Close terminates the underlying websocket connection.
func (c *Connection) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.socket.Close()
}

// GetID returns the session identifier.
func (c *Connection) GetID() string {
	return c.id
}

// GetType returns the connection transport type.
func (c *Connection) GetType() string {
	return "websocket"
}

// IsClosed reports whether the connection has already been closed.
func (c *Connection) IsClosed() bool {
	return c.closed.Load()
}

// GetLastActiveTime exposes when the client last interacted with the server.
func (c *Connection) GetLastActiveTime() time.Time {
	return time.Unix(0, c.lastActive.Load())
}

// IsStale checks whether the connection has been idle for longer than timeout.
func (c *Connection) IsStale(timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	return time.Since(c.GetLastActiveTime()) > timeout
}

// GetMCPManager implements transport.MCPManagerHolder.
func (c *Connection) GetMCPManager() *mcp.Manager {
	return c.mcpHolder.Load()
}

// SetMCPManager implements transport.MCPManagerHolder.
func (c *Connection) SetMCPManager(manager *mcp.Manager) {
	c.mcpHolder.Store(manager)
}

func (c *Connection) touch() {
	c.lastActive.Store(time.Now().UnixNano())
}
