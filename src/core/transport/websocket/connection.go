package websocket

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketConnection WebSocket连接适配器
type WebSocketConnection struct {
	id         string
	conn       *websocket.Conn
	closed     int32
	lastActive int64
	mu         sync.Mutex
}

// NewWebSocketConnection 创建新的WebSocket连接适配器
func NewWebSocketConnection(id string, conn *websocket.Conn) *WebSocketConnection {
	return &WebSocketConnection{
		id:         id,
		conn:       conn,
		closed:     0,
		lastActive: time.Now().Unix(),
	}
}

// WriteMessage 发送消息
func (c *WebSocketConnection) WriteMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if atomic.LoadInt32(&c.closed) == 1 {
		return fmt.Errorf("连接已关闭")
	}

	atomic.StoreInt64(&c.lastActive, time.Now().Unix())
	return c.conn.WriteMessage(messageType, data)
}

// ReadMessage 读取消息
func (c *WebSocketConnection) ReadMessage(stopChan <-chan struct{}) (int, []byte, error) {
	messageType, data, err := c.conn.ReadMessage()
	if err == nil {
		atomic.StoreInt64(&c.lastActive, time.Now().Unix())
	}
	return messageType, data, err
}

// Close 关闭连接
func (c *WebSocketConnection) Close() error {
	if atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return c.conn.Close()
	}
	return nil
}

// GetID 获取连接ID
func (c *WebSocketConnection) GetID() string {
	return c.id
}

// GetType 获取连接类型
func (c *WebSocketConnection) GetType() string {
	return "websocket"
}

// IsClosed 检查连接是否已关闭
func (c *WebSocketConnection) IsClosed() bool {
	return atomic.LoadInt32(&c.closed) == 1
}

// GetLastActiveTime 获取最后活跃时间
func (c *WebSocketConnection) GetLastActiveTime() time.Time {
	return time.Unix(atomic.LoadInt64(&c.lastActive), 0)
}

// IsStale 检查连接是否过期
func (c *WebSocketConnection) IsStale(timeout time.Duration) bool {
	return time.Since(c.GetLastActiveTime()) > timeout
}
