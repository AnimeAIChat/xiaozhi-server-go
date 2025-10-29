package ws

import "errors"

var (
	// ErrHandshakeTimeout indicates the websocket handshake exceeded the configured timeout.
	ErrHandshakeTimeout = errors.New("websocket handshake timed out")
	// ErrSessionShutdown is emitted when the server requests a session shutdown.
	ErrSessionShutdown = errors.New("websocket session shutdown")
)
