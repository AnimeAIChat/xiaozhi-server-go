package websocket

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/transport"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gorilla/websocket"
)

// WebSocketTransport WebSocket传输层实现
type WebSocketTransport struct {
	config            *configs.Config
	server            *http.Server
	logger            *utils.Logger
	connHandler       transport.ConnectionHandlerFactory
	activeConnections sync.Map
	upgrader          *websocket.Upgrader
}

// NewWebSocketTransport 创建新的WebSocket传输层
func NewWebSocketTransport(config *configs.Config, logger *utils.Logger) *WebSocketTransport {
	return &WebSocketTransport{
		config: config,
		logger: logger,
		upgrader: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源的连接
			},
		},
	}
}

// Start 启动WebSocket传输层
func (t *WebSocketTransport) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", t.config.Server.IP, t.config.Server.Port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", t.handleWebSocket)

	t.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	t.logger.Info("启动WebSocket传输层 ws://%s", addr)

	// 监听关闭信号
	go func() {
		<-ctx.Done()
		t.Stop()
	}()

	if err := t.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("WebSocket传输层启动失败: %v", err)
	}

	return nil
}

// Stop 停止WebSocket传输层
func (t *WebSocketTransport) Stop() error {
	if t.server != nil {
		t.logger.Info("WebSocket传输层...")

		// 关闭所有活动连接
		t.activeConnections.Range(func(key, value interface{}) bool {
			if handler, ok := value.(transport.ConnectionHandler); ok {
				handler.Close()
			}
			t.activeConnections.Delete(key)
			return true
		})

		return t.server.Close()
	}
	return nil
}

// SetConnectionHandler 设置连接处理器工厂
func (t *WebSocketTransport) SetConnectionHandler(handler transport.ConnectionHandlerFactory) {
	t.connHandler = handler
}

// GetActiveConnectionCount 获取活跃连接数
func (t *WebSocketTransport) GetActiveConnectionCount() int {
	count := 0
	t.activeConnections.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// GetType 获取传输类型
func (t *WebSocketTransport) GetType() string {
	return "websocket"
}

// handleWebSocket 处理WebSocket连接
func (t *WebSocketTransport) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := t.upgrader.Upgrade(w, r, nil)
	if err != nil {
		t.logger.Error("WebSocket升级失败: %v", err)
		return
	}

	clientID := fmt.Sprintf("%p", conn)
	t.logger.Info("收到WebSocket连接请求: %s", r.Header.Get("Device-Id"))
	wsConn := NewWebSocketConnection(clientID, conn)

	if t.connHandler == nil {
		t.logger.Error("连接处理器工厂未设置")
		conn.Close()
		return
	}

	handler := t.connHandler.CreateHandler(wsConn, r)
	if handler == nil {
		t.logger.Error("创建连接处理器失败")
		conn.Close()
		return
	}

	t.activeConnections.Store(clientID, handler)
	t.logger.Info("WebSocket客户端 %s 连接已建立，资源已分配", clientID)

	// 启动连接处理，并在结束时清理资源
	go func() {
		defer func() {
			// 连接结束时清理
			t.activeConnections.Delete(clientID)
			handler.Close()
		}()

		handler.Handle()
	}()
}
