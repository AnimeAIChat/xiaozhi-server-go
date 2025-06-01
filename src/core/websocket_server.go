package core

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/task"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/providers/vlllm"


	"github.com/gorilla/websocket"
)

// WebSocketServer WebSocket服务器结构
type WebSocketServer struct {
	config      *configs.Config
	server      *http.Server
	upgrader    Upgrader
	logger      *utils.Logger
	taskMgr     *task.TaskManager
	poolManager *pool.PoolManager // 替换providers
	activeConnections sync.Map
}

// Upgrader WebSocket升级器接口
type Upgrader interface {
	Upgrade(w http.ResponseWriter, r *http.Request) (Conn, error)
}

// Conn WebSocket连接接口
type Conn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// NewWebSocketServer 创建新的WebSocket服务器
func NewWebSocketServer(config *configs.Config, logger *utils.Logger) (*WebSocketServer, error) {
	ws := &WebSocketServer{
		config:   config,
		logger:   logger,
		upgrader: NewDefaultUpgrader(),
		taskMgr: func() *task.TaskManager {
			tm := task.NewTaskManager(task.ResourceConfig{
				MaxWorkers:          12,
				MaxTasksPerClient:   20,
				MaxImageTasksPerDay: 50,
				MaxVideoTasksPerDay: 20,
				MaxScheduledTasks:   100,
			})
			tm.Start()
			return tm
		}(),
	}
	// 初始化资源池管理器
	poolManager, err := pool.NewPoolManager(config, logger)
	if err != nil {
		logger.Error(fmt.Sprintf("初始化资源池管理器失败: %v", err))
		return nil, fmt.Errorf("初始化资源池管理器失败: %v", err)
	}
	ws.poolManager = poolManager
	return ws, nil
}

// Start 启动WebSocket服务器
func (ws *WebSocketServer) Start(ctx context.Context) error {
	// 检查资源池是否正常
	if ws.poolManager == nil {
		ws.logger.Error("资源池管理器未初始化")
		return fmt.Errorf("资源池管理器未初始化")
	}

	addr := fmt.Sprintf("%s:%d", ws.config.Server.IP, ws.config.Server.Port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.handleWebSocket)

	ws.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	ws.logger.Info(fmt.Sprintf("启动WebSocket服务器 ws://%s...", addr))

	// 启动服务器关闭监控
	go func() {
		<-ctx.Done()
		ws.logger.Info("收到关闭信号，准备关闭服务器...")
		if err := ws.Stop(); err != nil {
			ws.logger.Error(fmt.Sprintf("服务器关闭时出错: %v", err))
		}
	}()

	// 启动服务器
	if err := ws.server.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			ws.logger.Info("服务器已正常关闭")
			return nil
		}
		ws.logger.Error(fmt.Sprintf("服务器启动失败: %v", err))
		return fmt.Errorf("服务器启动失败: %v", err)
	}

	return nil
}

// defaultUpgrader 默认的WebSocket升级器实现
type defaultUpgrader struct {
	wsUpgrader *websocket.Upgrader
}

// NewDefaultUpgrader 创建默认的WebSocket升级器
func NewDefaultUpgrader() *defaultUpgrader {
	return &defaultUpgrader{
		wsUpgrader: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源的连接
			},
		},
	}
}

// websocketConn 封装gorilla/websocket的连接实现
type websocketConn struct {
	conn *websocket.Conn
}

func (w *websocketConn) ReadMessage() (messageType int, p []byte, err error) {
	return w.conn.ReadMessage()
}

func (w *websocketConn) WriteMessage(messageType int, data []byte) error {
	return w.conn.WriteMessage(messageType, data)
}

func (w *websocketConn) Close() error {
	return w.conn.Close()
}

// Upgrade 实现Upgrader接口
func (u *defaultUpgrader) Upgrade(w http.ResponseWriter, r *http.Request) (Conn, error) {
	conn, err := u.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return &websocketConn{conn: conn}, nil
}

// Stop 停止WebSocket服务器
func (ws *WebSocketServer) Stop() error {
	if ws.server != nil {
		ws.logger.Info("正在关闭WebSocket服务器...")

		// 关闭所有活动连接
		ws.activeConnections.Range(func(key, value interface{}) bool {
			if conn, ok := value.(Conn); ok {
				conn.Close()
			}
			return true
		})

		// 关闭资源池
		if ws.poolManager != nil {
			ws.poolManager.Close()
		}

		// 关闭服务器
		if err := ws.server.Close(); err != nil {
			return fmt.Errorf("服务器关闭失败: %v", err)
		}
	}
	return nil
}

// handleWebSocket 处理WebSocket连接
func (ws *WebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.upgrader.Upgrade(w, r)
	if err != nil {
		ws.logger.Error(fmt.Sprintf("WebSocket升级失败: %v", err))
		return
	}

	clientID := fmt.Sprintf("%p", conn)
	ws.activeConnections.Store(clientID, conn)

	// 从资源池获取提供者集合
	providerSet, err := ws.poolManager.GetProviderSet()
	if err != nil {
		ws.logger.Error(fmt.Sprintf("获取提供者集合失败: %v", err))
		conn.Close()
		return
	}

	// 创建新的连接处理器
	handler := NewConnectionHandler(ws.config, struct {
		asr   providers.ASRProvider
		llm   providers.LLMProvider
		tts   providers.TTSProvider
		vlllm *vlllm.Provider
	}{
		asr:   providerSet.ASR,
		llm:   providerSet.LLM,
		tts:   providerSet.TTS,
		vlllm: providerSet.VLLLM,
	}, ws.logger)

	handler.taskMgr = ws.taskMgr
	go handler.Handle(conn)
}
