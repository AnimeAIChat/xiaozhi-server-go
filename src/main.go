// @title 小智服务端 API 文档
// @version 1.0
// @description 小智服务端，包含OTA与Vision等接口
// @host localhost:8080
// @BasePath /api
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	cfg "xiaozhi-server-go/src/configs/server"
	"xiaozhi-server-go/src/core/auth"
	"xiaozhi-server-go/src/core/auth/store"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/transport"
	"xiaozhi-server-go/src/core/transport/websocket"
	"xiaozhi-server-go/src/core/utils"
	_ "xiaozhi-server-go/src/docs"
	"xiaozhi-server-go/src/ota"
	"xiaozhi-server-go/src/task"
	"xiaozhi-server-go/src/vision"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	// 导入所有providers以确保init函数被调用
	_ "xiaozhi-server-go/src/core/providers/asr/deepgram"
	_ "xiaozhi-server-go/src/core/providers/asr/doubao"
	_ "xiaozhi-server-go/src/core/providers/asr/gosherpa"
	_ "xiaozhi-server-go/src/core/providers/llm/coze"
	_ "xiaozhi-server-go/src/core/providers/llm/ollama"
	_ "xiaozhi-server-go/src/core/providers/llm/openai"
	_ "xiaozhi-server-go/src/core/providers/tts/deepgram"
	_ "xiaozhi-server-go/src/core/providers/tts/doubao"
	_ "xiaozhi-server-go/src/core/providers/tts/edge"
	_ "xiaozhi-server-go/src/core/providers/tts/gosherpa"
	_ "xiaozhi-server-go/src/core/providers/vlllm/ollama"
	_ "xiaozhi-server-go/src/core/providers/vlllm/openai"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

func LoadConfigAndLogger() (*configs.Config, *utils.Logger, error) {
	// 加载配置,默认使用.config.yaml
	config, configPath, err := configs.LoadConfig()
	if err != nil {
		return nil, nil, err
	}

	// 初始化日志系统
	logger, err := utils.NewLogger(config)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("日志系统初始化成功, 配置文件路径: %s", configPath)
	utils.DefaultLogger = logger
	return config, logger, nil
}

// initAuthManager 初始化认证管理器
func initAuthManager(config *configs.Config, logger *utils.Logger) (*auth.AuthManager, error) {
	if !config.Server.Auth.Enabled {
		logger.Info("认证功能未启用")
		return nil, nil
	}

	// 创建存储配置
	storeConfig := &store.StoreConfig{
		Type:     config.Server.Auth.Store.Type,
		ExpiryHr: config.Server.Auth.Store.Expiry,
		Config:   make(map[string]interface{}),
	}

	// 创建认证管理器
	authManager, err := auth.NewAuthManager(storeConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("初始化认证管理器失败: %v", err)
	}

	return authManager, nil
}

func StartTransportServer(
	config *configs.Config,
	logger *utils.Logger,
	authManager *auth.AuthManager,
	g *errgroup.Group,
	groupCtx context.Context,
) (*transport.TransportManager, error) {
	// 初始化资源池管理器
	poolManager, err := pool.NewPoolManager(config, logger)
	if err != nil {
		logger.Error(fmt.Sprintf("初始化资源池管理器失败: %v", err))
		return nil, fmt.Errorf("初始化资源池管理器失败: %v", err)
	}

	// 初始化任务管理器
	taskMgr := task.NewTaskManager(task.ResourceConfig{
		MaxWorkers:        12,
		MaxTasksPerClient: 20,
	})
	taskMgr.Start()

	// 创建传输管理器
	transportManager := transport.NewTransportManager(config, logger)

	// 创建连接处理器工厂
	handlerFactory := transport.NewDefaultConnectionHandlerFactory(
		config,
		poolManager,
		taskMgr,
		logger,
	)

	// 根据配置启用不同的传输层
	enabledTransports := make([]string, 0)

	// 检查WebSocket传输层配置
	if config.Transport.WebSocket.Enabled {
		wsTransport := websocket.NewWebSocketTransport(config, logger)
		wsTransport.SetConnectionHandler(handlerFactory)
		transportManager.RegisterTransport("websocket", wsTransport)
		enabledTransports = append(enabledTransports, "WebSocket")
		logger.Debug("WebSocket传输层已注册")
	}

	if len(enabledTransports) == 0 {
		return nil, fmt.Errorf("没有启用任何传输层")
	}

	logger.Info("启用的传输层: %v", enabledTransports)

	// 启动传输层服务
	g.Go(func() error {
		// 监听关闭信号
		go func() {
			<-groupCtx.Done()
			logger.Info("收到关闭信号，开始关闭所有传输层...")
			if err := transportManager.StopAll(); err != nil {
				logger.Error("关闭传输层失败: %v", err)
			} else {
				logger.Info("所有传输层已优雅关闭")
			}
		}()

		// 使用传输管理器启动服务
		if err := transportManager.StartAll(groupCtx); err != nil {
			if groupCtx.Err() != nil {
				return nil // 正常关闭
			}
			logger.Error("传输层运行失败 %v", err)
			return err
		}
		return nil
	})

	logger.Debug("传输层服务已成功启动")
	return transportManager, nil
}

func StartHttpServer(config *configs.Config, logger *utils.Logger, g *errgroup.Group, groupCtx context.Context) (*http.Server, error) {
	// 初始化Gin引擎
	if config.Log.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()
	router.SetTrustedProxies([]string{"0.0.0.0"})

	// API路由全部挂载到/api前缀下
	apiGroup := router.Group("/api")
	// 启动OTA服务
	otaService := ota.NewDefaultOTAService(config.Web.Websocket)
	if err := otaService.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("OTA 服务启动失败", err)
		return nil, err
	}

	// 启动Vision服务
	visionService, err := vision.NewDefaultVisionService(config, logger)
	if err != nil {
		logger.Error("Vision 服务初始化失败 %v", err)
		return nil, err
	}
	if err := visionService.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("Vision 服务启动失败 %v", err)
		return nil, err
	}

	cfgServer, err := cfg.NewDefaultCfgService(config, logger)
	if err != nil {
		logger.Error("配置服务初始化失败 %v", err)
		return nil, err
	}
	if err := cfgServer.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("配置服务启动失败", err)
		return nil, err
	}

	// HTTP Server（支持优雅关机）
	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(config.Web.Port),
		Handler: router,
	}

	// 注册Swagger文档路由
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	g.Go(func() error {
		logger.Info(fmt.Sprintf("Gin 服务已启动，访问地址: http://0.0.0.0:%d", config.Web.Port))

		// 在单独的 goroutine 中监听关闭信号
		go func() {
			<-groupCtx.Done()
			logger.Info("收到关闭信号，开始关闭HTTP服务...")

			// 创建关闭超时上下文
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				logger.Error("HTTP服务关闭失败 %v", err)
			} else {
				logger.Info("HTTP服务已优雅关闭")
			}
		}()

		// ListenAndServe 返回 ErrServerClosed 时表示正常关闭
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP 服务启动失败 %v", err)
			return err
		}
		return nil
	})

	return httpServer, nil
}

func GracefulShutdown(cancel context.CancelFunc, logger *utils.Logger, g *errgroup.Group) {
	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// 等待信号
	sig := <-sigChan
	logger.Info("接收到系统信号: %v，开始优雅关闭服务", sig)

	// 取消上下文，通知所有服务开始关闭
	cancel()

	// 等待所有服务关闭，设置超时保护
	done := make(chan error, 1)
	go func() {
		done <- g.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			logger.Error("服务关闭过程中出现错误 %v", err)
			os.Exit(1)
		}
		logger.Info("所有服务已优雅关闭")
	case <-time.After(15 * time.Second):
		logger.Error("服务关闭超时，强制退出")
		os.Exit(1)
	}
}

func startServices(
	config *configs.Config,
	logger *utils.Logger,
	authManager *auth.AuthManager,
	g *errgroup.Group,
	groupCtx context.Context,
) error {
	// 启动传输层服务
	if _, err := StartTransportServer(config, logger, authManager, g, groupCtx); err != nil {
		return fmt.Errorf("启动传输层服务失败: %w", err)
	}

	// 启动 Http 服务
	if _, err := StartHttpServer(config, logger, g, groupCtx); err != nil {
		return fmt.Errorf("启动 Http 服务失败: %w", err)
	}

	return nil
}

func main() {
	// 加载配置和初始化日志系统
	config, logger, err := LoadConfigAndLogger()
	if err != nil {
		fmt.Println("加载配置或初始化日志系统失败:", err)
		os.Exit(1)
	}

	// 初始化数据库连接
	db, dbType, err := database.InitDB(logger, config)
	_, _ = db, dbType // 避免未使用变量警告
	if err != nil {
		logger.Error("数据库连接失败: %v", err)
		return
	}

	// 初始化认证管理器
	authManager, err := initAuthManager(config, logger)
	if err != nil {
		logger.Error("初始化认证管理器失败:", err)
		os.Exit(1)
	}

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 用 errgroup 管理两个服务
	g, groupCtx := errgroup.WithContext(ctx)

	// 启动所有服务
	if err := startServices(config, logger, authManager, g, groupCtx); err != nil {
		logger.Error("启动服务失败:%v", err)
		cancel()
		os.Exit(1)
	}

	// 启动优雅关机处理
	GracefulShutdown(cancel, logger, g)

	// 关闭认证管理器
	if authManager != nil {
		authManager.Close()
	}

	logger.Info("程序已成功退出")
	logger.Close()
}
