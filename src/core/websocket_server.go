package core

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/asr"
	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/providers/tts"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/task"

	"github.com/gorilla/websocket"
)

// WebSocketServer WebSocket服务器结构
type WebSocketServer struct {
	config    *configs.Config
	server    *http.Server
	upgrader  Upgrader
	logger    *utils.Logger
	taskMgr   *task.TaskManager
	providers struct {
		asr providers.ASRProvider
		llm providers.LLMProvider
		tts providers.TTSProvider
	}
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

	// 初始化处理模块实例
	if err := ws.initializeProviders(); err != nil {
		return nil, fmt.Errorf("初始化处理模块失败: %v", err)
	}

	return ws, nil
}

// Start 启动WebSocket服务器
func (ws *WebSocketServer) Start(ctx context.Context) error {
	// 检查providers是否已初始化
	if ws.providers.asr == nil || ws.providers.llm == nil || ws.providers.tts == nil {
		ws.logger.Error("必要的服务提供者未初始化")
		return fmt.Errorf("必要的服务提供者未初始化")
	}

	addr := fmt.Sprintf("%s:%d", ws.config.Server.IP, ws.config.Server.Port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.handleWebSocket)

	ws.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	ws.logger.Info(fmt.Sprintf("正在启动WebSocket服务器于 ws://%s...", addr))

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

	// 创建新的连接处理器
	handler := NewConnectionHandler(ws.config, struct {
		asr providers.ASRProvider
		llm providers.LLMProvider
		tts providers.TTSProvider
	}{
		asr: ws.providers.asr,
		llm: ws.providers.llm,
		tts: ws.providers.tts,
	}, ws.logger)

	// Initialize task manager for the handler
	handler.taskMgr = ws.taskMgr
	go handler.Handle(conn)
}

// initializeProviders 初始化所有提供者
func (ws *WebSocketServer) initializeProviders() error {
	ws.logger.Info("开始初始化服务提供者...")
	selectedModule := ws.config.SelectedModule

	// 检查必要的模块配置是否存在
	requiredModules := []string{"ASR", "LLM", "TTS"}
	for _, module := range requiredModules {
		if _, ok := selectedModule[module]; !ok {
			err := fmt.Sprintf("配置文件中缺少必要的模块配置: %s", module)
			ws.logger.Error(err)
			return fmt.Errorf(err)
		}
	}

	// 初始化ASR
	asrType, ok := selectedModule["ASR"]
	if !ok {
		ws.logger.Error("未找到ASR配置")
		return fmt.Errorf("未找到ASR配置")
	}

	if asrType != "" {
		ws.logger.Info(fmt.Sprintf("正在初始化ASR服务(%s)...", asrType))
		if asrCfg, ok := ws.config.ASR[asrType]; ok {
			asrType, _ := asrCfg["type"].(string)
			provider, err := asr.Create(asrType, &asr.Config{
				Type: asrType,
				Data: asrCfg,
			}, ws.config.DeleteAudio)
			if err != nil {
				ws.logger.Error(fmt.Sprintf("初始化ASR失败: %v", err))
				return fmt.Errorf("初始化ASR失败: %v", err)
			}
			ws.providers.asr = provider.(providers.ASRProvider)
			ws.logger.Info("ASR服务初始化成功")
		} else {
			ws.logger.Error(fmt.Sprintf("找不到ASR配置: %s", asrType))
		}
	}

	// 初始化LLM
	llmType, ok := selectedModule["LLM"]
	if !ok {
		ws.logger.Error("未找到LLM配置")
		return fmt.Errorf("未找到LLM配置")
	}

	if llmType != "" {
		ws.logger.Info(fmt.Sprintf("正在初始化LLM服务(%s)...", llmType))
		if llmCfg, ok := ws.config.LLM[llmType]; ok {
			provider, err := llm.Create(llmCfg.Type, &llm.Config{
				Type:        llmCfg.Type,
				ModelName:   llmCfg.ModelName,
				BaseURL:     llmCfg.BaseURL,
				APIKey:      llmCfg.APIKey,
				Temperature: llmCfg.Temperature,
				MaxTokens:   llmCfg.MaxTokens,
				TopP:        llmCfg.TopP,
				Extra:       llmCfg.Extra,
			})
			if err != nil {
				ws.logger.Error(fmt.Sprintf("初始化LLM失败: %v", err))
				return fmt.Errorf("初始化LLM失败: %v", err)
			}
			ws.providers.llm = provider.(providers.LLMProvider)
			ws.logger.Info("LLM服务初始化成功")
		} else {
			ws.logger.Error(fmt.Sprintf("找不到LLM配置: %s", llmType))
		}
	}

	// 初始化TTS
	ttsType, ok := selectedModule["TTS"]
	if !ok {
		ws.logger.Error("未找到TTS配置")
		return fmt.Errorf("未找到TTS配置")
	}

	if ttsType != "" {
		ws.logger.Info(fmt.Sprintf("正在初始化TTS服务(%s)...", ttsType))
		if ttsCfg, ok := ws.config.TTS[ttsType]; ok {
			provider, err := tts.Create(ttsCfg.Type, &tts.Config{
				Type:      ttsCfg.Type,
				Voice:     ttsCfg.Voice,
				Format:    ttsCfg.Format,
				OutputDir: ttsCfg.OutputDir,
				AppID:     ttsCfg.AppID,
				Token:     ttsCfg.Token,
				Cluster:   ttsCfg.Cluster,
			}, ws.config.DeleteAudio)
			if err != nil {
				ws.logger.Error(fmt.Sprintf("初始化TTS失败: %v", err))
				return fmt.Errorf("初始化TTS失败: %v", err)
			}
			ws.providers.tts = provider.(providers.TTSProvider)
			ws.logger.Info("TTS服务初始化成功")
		} else {
			ws.logger.Error(fmt.Sprintf("找不到TTS配置: %s", ttsType))
		}
	}

	// 最终检查所有必需的provider是否都已初始化
	if ws.providers.asr == nil || ws.providers.llm == nil || ws.providers.tts == nil {
		ws.logger.Error("一个或多个必需的服务提供者初始化失败")
		return fmt.Errorf("一个或多个必需的服务提供者初始化失败")
	}

	ws.logger.Info("所有服务提供者初始化成功完成")
	return nil
}
