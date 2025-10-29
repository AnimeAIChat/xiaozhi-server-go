package ws

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"xiaozhi-server-go/internal/platform/observability"
	"xiaozhi-server-go/src/core/utils"
)

// HandlerBuilder creates a session handler for an upgraded websocket connection.
type HandlerBuilder func(conn *Connection, req *http.Request) (SessionHandler, error)

// Router is responsible for upgrading HTTP connections to websocket sessions.
type Router struct {
	hub    *Hub
	logger *utils.Logger

	upgrader         *websocket.Upgrader
	handshakeTimeout time.Duration
	builder          atomic.Value // HandlerBuilder
}

// RouterOptions configures the websocket router.
type RouterOptions struct {
	HandshakeTimeout time.Duration
	CheckOrigin      func(r *http.Request) bool
}

// NewRouter constructs a websocket router.
func NewRouter(hub *Hub, logger *utils.Logger, opts RouterOptions) *Router {
	upgrader := &websocket.Upgrader{
		CheckOrigin: opts.CheckOrigin,
	}
	if upgrader.CheckOrigin == nil {
		upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	}

	timeout := opts.HandshakeTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &Router{
		hub:              hub,
		logger:           logger,
		upgrader:         upgrader,
		handshakeTimeout: timeout,
	}
}

// SetHandlerBuilder registers the handler builder that will be invoked after a successful upgrade.
func (r *Router) SetHandlerBuilder(builder HandlerBuilder) {
	r.builder.Store(builder)
}

// Handle upgrades the HTTP connection and launches a new websocket session.
func (r *Router) Handle(w http.ResponseWriter, req *http.Request) {
	value := r.builder.Load()
	if value == nil {
		http.Error(w, "websocket handler not ready", http.StatusServiceUnavailable)
		return
	}
	builder := value.(HandlerBuilder)

	ctx := req.Context()
	handshakeCtx, cancel := context.WithTimeoutCause(ctx, r.handshakeTimeout, ErrHandshakeTimeout)
	defer cancel()
	req = req.WithContext(handshakeCtx)

	spanCtx, spanEnd := observability.StartSpan(handshakeCtx, "transport.websocket", "handle")
	var spanErr error
	defer func() {
		spanEnd(spanErr)
	}()

	conn, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		spanErr = err
		observability.RecordMetric(
			spanCtx,
			"websocket.upgrade.error",
			1,
			map[string]string{
				"component": "transport.websocket",
			},
		)
		if r.logger != nil {
			r.logger.ErrorTag("WebSocket", "握手失败: %v", err)
		}
		return
	}

	deviceID, clientID := resolveIdentifiers(req, conn)
	if r.logger != nil {
		r.logger.InfoTag("WebSocket", "建立连接 device=%s client=%s", deviceID, clientID)
	}

	wsConn := NewConnection(clientID, conn)
	observability.RecordMetric(
		spanCtx,
		"websocket.upgrade.success",
		1,
		map[string]string{
			"component": "transport.websocket",
		},
	)

	handler, err := builder(wsConn, req)
	if err != nil || handler == nil {
		spanErr = err
		observability.RecordMetric(
			spanCtx,
			"websocket.connection.error",
			1,
			map[string]string{
				"component": "transport.websocket",
				"reason":    "handler_creation_failed",
			},
		)
		if r.logger != nil {
			r.logger.ErrorTag("WebSocket", "创建连接处理器失败: %v", err)
		}
		_ = wsConn.Close()
		return
	}

	session := NewSession(spanCtx, handler, wsConn, r.logger)
	r.hub.Register(session)

	observability.RecordMetric(
		spanCtx,
		"websocket.connection.opened",
		1,
		map[string]string{
			"component": "transport.websocket",
			"client_id": clientID,
			"device_id": deviceID,
		},
	)

	go session.Run(func(runErr error) {
		r.hub.Unregister(session.ID())
		if runErr != nil && r.logger != nil {
			r.logger.WarnTag("WebSocket", "会话 %s 异常结束: %v", session.ID(), runErr)
		}
		observability.RecordMetric(
			session.Context(),
			"websocket.connection.closed",
			1,
			map[string]string{
				"component": "transport.websocket",
				"client_id": clientID,
				"device_id": deviceID,
			},
		)
	})
}

func resolveIdentifiers(req *http.Request, conn *websocket.Conn) (string, string) {
	deviceID := req.Header.Get("Device-Id")
	clientID := req.Header.Get("Client-Id")

	if deviceID == "" {
		deviceID = req.URL.Query().Get("device-id")
	}
	if clientID == "" {
		clientID = req.URL.Query().Get("client-id")
	}
	if clientID == "" {
		clientID = fmt.Sprintf("%p", conn)
	}
	return deviceID, clientID
}
