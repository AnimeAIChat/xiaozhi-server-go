package websocket

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"xiaozhi-server-go/internal/platform/observability"
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
	addr := fmt.Sprintf("%s:%d", t.config.Transport.WebSocket.IP, t.config.Transport.WebSocket.Port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", t.handleWebSocket)

	t.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	t.logger.Info("[WebSocket] ws://%s", addr)

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
func (t *WebSocketTransport) GetActiveConnectionCount() (int, int) {
	count := 0
	t.activeConnections.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count, count
}

// GetType 获取传输类型
func (t *WebSocketTransport) GetType() string {
	return "websocket"
}

// handleWebSocket 处理WebSocket连接
func (t *WebSocketTransport) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	ctx, spanEnd := observability.StartSpan(r.Context(), "transport.websocket", "handle")
	var spanErr error
	defer func() {
		spanEnd(spanErr)
	}()

	r = r.WithContext(ctx)

	conn, err := t.upgrader.Upgrade(w, r, nil)
	if err != nil {
		spanErr = err
		observability.RecordMetric(ctx, "websocket.upgrade.error", 1, map[string]string{
			"component": "transport.websocket",
		})
		t.logger.Error("WebSocket升级失败: %v", err)
		return
	}
	observability.RecordMetric(ctx, "websocket.upgrade.success", 1, map[string]string{
		"component": "transport.websocket",
	})

	deviceID := r.Header.Get("Device-Id")
	clientID := r.Header.Get("Client-Id")
	if deviceID == "" {
		// 尝试从url里获取
		deviceID = r.URL.Query().Get("device-id")
		r.Header.Set("Device-Id", deviceID)
		t.logger.Info("尝试从URL获取Device-Id: %v", r.URL)
	}
	if clientID == "" {
		// 尝试从url里获取
		clientID = r.URL.Query().Get("client-id")
		r.Header.Set("Client-Id", clientID)
	}
	if clientID == "" {
		clientID = fmt.Sprintf("%p", conn)
	}
	t.logger.Info("[WebSocket] [请求连接 %s/%s]", deviceID, clientID)
	wsConn := NewWebSocketConnection(clientID, conn)

	if t.connHandler == nil {
		spanErr = fmt.Errorf("connection handler not configured")
		observability.RecordMetric(ctx, "websocket.connection.error", 1, map[string]string{
			"component": "transport.websocket",
			"reason":    "handler_not_configured",
		})
		t.logger.Error("连接处理器尚未配置")
		conn.Close()
		return
	}

	handler := t.connHandler.CreateHandler(wsConn, r)
	if handler == nil {
		spanErr = fmt.Errorf("connection handler creation failed")
		observability.RecordMetric(ctx, "websocket.connection.error", 1, map[string]string{
			"component": "transport.websocket",
			"reason":    "handler_creation_failed",
		})
		t.logger.Error("创建连接处理器失败")
		conn.Close()
		return
	}

	t.activeConnections.Store(clientID, handler)
	t.logger.Info("[WebSocket] [连接建立 %s] 资源已就绪", clientID)
	observability.RecordMetric(ctx, "websocket.connection.opened", 1, map[string]string{
		"component": "transport.websocket",
		"client_id": clientID,
		"device_id": deviceID,
	})

	// 连接处理器在退出时回收资源
	go func(baseCtx context.Context, id, device string, h transport.ConnectionHandler) {
		sessionCtx, sessionEnd := observability.StartSpan(baseCtx, "transport.websocket", "session")
		defer sessionEnd(nil)

		defer func() {
			// 连接结束时清理
			t.activeConnections.Delete(id)
			h.Close()
			observability.RecordMetric(sessionCtx, "websocket.connection.closed", 1, map[string]string{
				"component": "transport.websocket",
				"client_id": id,
				"device_id": device,
			})
		}()

		h.Handle()
	}(ctx, clientID, deviceID, handler)
}
