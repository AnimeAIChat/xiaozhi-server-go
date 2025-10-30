package websocket

import (
	"context"
	"fmt"
	"net/http"

	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/transport/ws"
	"xiaozhi-server-go/src/core/transport"
	"xiaozhi-server-go/src/core/utils"
)

// WebSocketTransport is a compatibility wrapper exposing the legacy transport interface.
type WebSocketTransport struct {
	config      *config.Config
	logger      *utils.Logger
	server      *ws.Server
	hub         *ws.Hub
	connFactory transport.ConnectionHandlerFactory
}

// NewWebSocketTransport creates a websocket transport backed by the refactored internal package.
func NewWebSocketTransport(cfg *config.Config, logger *utils.Logger) *WebSocketTransport {
	if logger == nil {
		logger = utils.DefaultLogger
	}

	hub := ws.NewHub(logger)
	router := ws.NewRouter(hub, logger, ws.RouterOptions{})
	addr := fmt.Sprintf("%s:%d", cfg.Transport.WebSocket.IP, cfg.Transport.WebSocket.Port)
	server := ws.NewServer(
		ws.ServerConfig{
			Addr: addr,
			Path: "/",
		},
		router,
		hub,
		logger,
	)

	transport := &WebSocketTransport{
		config: cfg,
		logger: logger,
		server: server,
		hub:    hub,
	}

	server.SetHandlerBuilder(func(conn *ws.Connection, req *http.Request) (ws.SessionHandler, error) {
		if transport.connFactory == nil {
			return nil, fmt.Errorf("connection handler factory not configured")
		}
		handler := transport.connFactory.CreateHandler(conn, req)
		if handler == nil {
			return nil, fmt.Errorf("connection handler creation failed")
		}
		return handler, nil
	})

	return transport
}

// Start launches the websocket server.
func (t *WebSocketTransport) Start(ctx context.Context) error {
	return t.server.Start(ctx)
}

// Stop shuts down the websocket server.
func (t *WebSocketTransport) Stop() error {
	return t.server.Stop()
}

// SetConnectionHandler updates the handler factory used for new sessions.
func (t *WebSocketTransport) SetConnectionHandler(handler transport.ConnectionHandlerFactory) {
	t.connFactory = handler
}

// GetActiveConnectionCount reports active websocket connections.
func (t *WebSocketTransport) GetActiveConnectionCount() (int, int) {
	return t.server.Counts()
}

// GetType returns the transport identifier.
func (t *WebSocketTransport) GetType() string {
	return "websocket"
}
