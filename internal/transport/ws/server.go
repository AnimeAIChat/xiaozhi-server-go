package ws

import (
	"context"
	"net/http"
	"time"

	"xiaozhi-server-go/internal/utils"
)

// ServerConfig stores the settings required to expose the websocket transport.
type ServerConfig struct {
	Addr             string
	Path             string
	HandshakeTimeout time.Duration
}

// Server coordinates the websocket router, hub and lifecycle management.
type Server struct {
	cfg     ServerConfig
	hub     *Hub
	router  *Router
	logger  *utils.Logger
	httpSrv *http.Server
}

// NewServer builds a websocket transport server.
func NewServer(cfg ServerConfig, router *Router, hub *Hub, logger *utils.Logger) *Server {
	if cfg.Path == "" {
		cfg.Path = "/"
	}

	return &Server{
		cfg:    cfg,
		router: router,
		hub:    hub,
		logger: logger,
	}
}

// SetHandlerBuilder wires the handler construction callback.
func (s *Server) SetHandlerBuilder(builder HandlerBuilder) {
	s.router.SetHandlerBuilder(builder)
}

// Start boots the HTTP server and listens for websocket upgrades.
func (s *Server) Start(ctx context.Context) error {
	if s.httpSrv != nil {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.Path, s.router.Handle)

	s.httpSrv = &http.Server{
		Addr:    s.cfg.Addr,
		Handler: mux,
	}

	if ctx != nil {
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeoutCause(context.Background(), defaultCloseTimeout, context.Cause(ctx))
			defer cancel()
			_ = s.httpSrv.Shutdown(shutdownCtx)
		}()
	}

	if s.logger != nil {
		s.logger.InfoTag("WebSocket", "监听地址 %s%s", s.cfg.Addr, s.cfg.Path)
	}

	err := s.httpSrv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully stops the websocket server and active sessions.
func (s *Server) Stop() error {
	if s.httpSrv == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeoutCause(context.Background(), defaultCloseTimeout, ErrSessionShutdown)
	defer cancel()

	if err := s.httpSrv.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
		return err
	}

	s.hub.CloseAll(ErrSessionShutdown)
	s.httpSrv = nil
	return nil
}

// Counts exposes active client and session counts.
func (s *Server) Counts() (int, int) {
	return s.hub.Counts()
}
