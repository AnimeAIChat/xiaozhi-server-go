package ws

import (
	"context"
	"sync/atomic"
	"time"

	"xiaozhi-server-go/src/core/utils"
)

const defaultCloseTimeout = 5 * time.Second

// SessionHandler adapts legacy connection handlers to the refactored session lifecycle.
type SessionHandler interface {
	Handle()
	Close()
	GetSessionID() string
}

// Session encapsulates the lifecycle of a single websocket connection.
type Session struct {
	id      string
	handler SessionHandler
	conn    *Connection
	logger  *utils.Logger

	ctx    context.Context
	cancel context.CancelCauseFunc

    llmCtx    context.Context
    llmCancel context.CancelFunc

	closed atomic.Bool
}

// NewSession constructs a managed websocket session.

// ResetLLMContext 供ASR流程开始时调用，取消旧LLM回复并生成新context
func (s *Session) ResetLLMContext() {
	if s.llmCancel != nil {
		s.llmCancel()
	}
	s.llmCtx, s.llmCancel = context.WithCancel(context.Background())
}
func NewSession(parent context.Context, handler SessionHandler, conn *Connection, logger *utils.Logger) *Session {
	sessionCtx, cancel := context.WithCancelCause(parent)
	llmCtx, llmCancel := context.WithCancel(context.Background())
	return &Session{
		id:      handler.GetSessionID(),
		handler: handler,
		conn:    conn,
		logger:  logger,
		ctx:     sessionCtx,
		cancel:  cancel,
		llmCtx:  llmCtx,
		llmCancel: llmCancel,
	}
}

// Context returns the session context.
func (s *Session) Context() context.Context {
	return s.ctx
}

// ID exposes the session identifier.
func (s *Session) ID() string {
	return s.id
}

// Run executes the session handler and invokes onDone once exiting.
func (s *Session) Run(onDone func(error)) {
	var runErr error
	defer func() {
		s.Close(runErr)
		if onDone != nil {
			onDone(runErr)
		}
	}()

	s.handler.Handle()
}

// Close attempts to gracefully terminate the session.
func (s *Session) Close(reason error) {
	if reason == nil {
		reason = ErrSessionShutdown
	}

	if !s.closed.CompareAndSwap(false, true) {
		return
	}

	// 关闭LLM回复协程
	if s.llmCancel != nil {
		s.llmCancel()
	}

	if s.cancel != nil {
		s.cancel(reason)
	}

	shutdownCtx, cancel := context.WithTimeoutCause(context.Background(), defaultCloseTimeout, reason)
	defer cancel()

	if s.handler != nil {
		done := make(chan struct{})
		go func() {
			s.handler.Close()
			close(done)
		}()

		select {
		case <-done:
		case <-shutdownCtx.Done():
			if s.logger != nil {
				s.logger.Warn("session %s handler close timed out: %v", s.id, context.Cause(shutdownCtx))
			}
		}
	}

	if s.conn != nil {
		if err := s.conn.Close(); err != nil && s.logger != nil {
			s.logger.Warn("session %s connection close failed: %v", s.id, err)
		}
	}
}
