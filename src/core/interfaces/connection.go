package interfaces

// Conn WebSocket连接接口
type Conn interface {
	ReadMessage(stopChan <-chan struct{}) (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
}

// ConnectionHandler 连接处理器接口
type ConnectionHandler interface {
	Handle(conn Conn)
	SpeakAndPlay(text string, textIndex int) error
	Close()
}
