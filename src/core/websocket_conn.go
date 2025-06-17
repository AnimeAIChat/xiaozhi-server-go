package core

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var (
	ErrConnectionClosed = errors.New("websocket connection is closed")
)

// websocketConn 封装gorilla/websocket的连接实现
type websocketConn struct {
	conn       *websocket.Conn
	writeMu    sync.Mutex // 写操作互斥锁
	closed     int32      // 原子操作标记连接状态 (0=open, 1=closed)
	lastActive int64      // 最后活跃时间戳（原子操作）
}

func (w *websocketConn) ReadMessage() (messageType int, p []byte, err error) {
	if atomic.LoadInt32(&w.closed) == 1 {
		return 0, nil, ErrConnectionClosed
	}

	// 设置读取超时
	w.conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

	messageType, p, err = w.conn.ReadMessage()
	if err != nil {
		// 如果读取出错，标记连接为已关闭
		atomic.StoreInt32(&w.closed, 1)
		return 0, nil, err
	}

	// 更新最后活跃时间
	atomic.StoreInt64(&w.lastActive, time.Now().Unix())

	return messageType, p, nil
}

func (w *websocketConn) WriteMessage(messageType int, data []byte) error {
	// 检查连接状态
	if atomic.LoadInt32(&w.closed) == 1 {
		return ErrConnectionClosed
	}

	// 使用写锁确保写操作的串行化
	w.writeMu.Lock()
	defer w.writeMu.Unlock()

	// 双重检查，防止在获取锁的过程中连接被关闭
	if atomic.LoadInt32(&w.closed) == 1 {
		return ErrConnectionClosed
	}

	// 设置写入超时
	w.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))

	err := w.conn.WriteMessage(messageType, data)
	if err != nil {
		// 如果写入出错，标记连接为已关闭
		atomic.StoreInt32(&w.closed, 1)
		return err
	}

	// 更新最后活跃时间
	atomic.StoreInt64(&w.lastActive, time.Now().Unix())

	return nil
}

func (w *websocketConn) Close() error {
	// 使用原子操作避免重复关闭
	if !atomic.CompareAndSwapInt32(&w.closed, 0, 1) {
		return nil // 已经关闭过了
	}

	w.writeMu.Lock()
	defer w.writeMu.Unlock()

	// 尝试发送关闭帧（不强制要求成功）
	closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "connection closed")
	w.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	w.conn.WriteMessage(websocket.CloseMessage, closeMsg)

	return w.conn.Close()
}

// IsClosed 检查连接是否已关闭
func (w *websocketConn) IsClosed() bool {
	return atomic.LoadInt32(&w.closed) == 1
}

// GetLastActiveTime 获取最后活跃时间
func (w *websocketConn) GetLastActiveTime() time.Time {
	timestamp := atomic.LoadInt64(&w.lastActive)
	return time.Unix(timestamp, 0)
}

// IsStale 检查连接是否过期（基于最后活跃时间）
func (w *websocketConn) IsStale(timeout time.Duration) bool {
	if w.IsClosed() {
		return true
	}
	return time.Since(w.GetLastActiveTime()) > timeout
}

func (w *websocketConn) GetID() string {
	return w.conn.RemoteAddr().String() // 使用远程地址作为示例ID
}

func (w *websocketConn) GetType() string {
	return "websocket"
}
