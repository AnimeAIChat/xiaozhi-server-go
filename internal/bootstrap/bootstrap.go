package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	platformconfig "xiaozhi-server-go/internal/platform/config"
	platformlogging "xiaozhi-server-go/internal/platform/logging"
	platformstorage "xiaozhi-server-go/internal/platform/storage"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/auth"
	"xiaozhi-server-go/src/core/auth/store"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/transport"
	"xiaozhi-server-go/src/core/transport/websocket"
	"xiaozhi-server-go/src/core/utils"
	_ "xiaozhi-server-go/src/docs"
	"xiaozhi-server-go/src/httpsvr/ota"
	"xiaozhi-server-go/src/httpsvr/vision"
	"xiaozhi-server-go/src/task"

	cfg "xiaozhi-server-go/src/httpsvr/webapi"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"
	"golang.org/x/sync/errgroup"

	// 导入所有providers以确保init函数被调用
	_ "xiaozhi-server-go/src/core/providers/asr/deepgram"
	_ "xiaozhi-server-go/src/core/providers/asr/doubao"
	_ "xiaozhi-server-go/src/core/providers/asr/gosherpa"
	_ "xiaozhi-server-go/src/core/providers/asr/stepfun"
	_ "xiaozhi-server-go/src/core/providers/llm/coze"
	_ "xiaozhi-server-go/src/core/providers/llm/doubao"
	_ "xiaozhi-server-go/src/core/providers/llm/ollama"
	_ "xiaozhi-server-go/src/core/providers/llm/openai"
	_ "xiaozhi-server-go/src/core/providers/tts/deepgram"
	_ "xiaozhi-server-go/src/core/providers/tts/doubao"
	_ "xiaozhi-server-go/src/core/providers/tts/edge"
	_ "xiaozhi-server-go/src/core/providers/tts/gosherpa"
	_ "xiaozhi-server-go/src/core/providers/vlllm/ollama"
	_ "xiaozhi-server-go/src/core/providers/vlllm/openai"
)

const scalarHTML = `<!DOCTYPE html>
<html lang="zh-CN">
	<head>
		<meta charset="utf-8" />
		<title>小智 API Reference</title>
		<meta name="viewport" content="width=device-width, initial-scale=1" />
	</head>
	<body>
		<script
			id="api-reference"
			data-url="/openapi.json"
			data-layout="modern"
			src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"
		></script>
	</body>
</html>`

// Run 启动整个服务生命周期，负责加载配置、初始化依赖和优雅关停。
func Run(ctx context.Context) error {
	config, logger, err := loadConfigAndLogger()
	if err != nil {
		return fmt.Errorf("load config and logger: %w", err)
	}

	authManager, err := initAuthManager(config, logger)
	if err != nil {
		logger.Error("初始化认证管理器失败: %v", err)
		return fmt.Errorf("init auth manager: %w", err)
	}
	defer func() {
		if authManager != nil {
			if closeErr := authManager.Close(); closeErr != nil {
				logger.Error("认证管理器关闭失败: %v", closeErr)
			}
		}
	}()

	rootCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	signalCtx, stop := signal.NotifyContext(rootCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	group, groupCtx := errgroup.WithContext(rootCtx)

	if err := startServices(config, logger, authManager, group, groupCtx); err != nil {
		cancel()
		return err
	}

	if err := waitForShutdown(signalCtx, cancel, logger, group); err != nil {
		return err
	}

	logger.Info("服务已成功启动")
	logger.Close()
	return nil
}

func loadConfigAndLogger() (*configs.Config, *utils.Logger, error) {
	if err := platformstorage.InitConfigStore(); err != nil {
		fmt.Printf("配置存储初始化失败: %v\n", err)
	}

	result, err := platformconfig.NewLoader().Load()
	if err != nil {
		return nil, nil, err
	}
	config := result.Config
	configPath := result.Path

	logProvider, err := platformlogging.New(platformlogging.Config{
		Level:    config.Log.LogLevel,
		Dir:      config.Log.LogDir,
		Filename: config.Log.LogFile,
	})
	if err != nil {
		return nil, nil, err
	}
	logger := logProvider.Legacy()
	utils.DefaultLogger = logger
	logger.Info("[日志] [初始化成功] [%s] 配置文件路径: %s", config.Log.LogLevel, configPath)

	database.SetLogger(logger)
	database.InsertDefaultConfigIfNeeded(database.GetDB())

	return config, logger, nil
}

func initAuthManager(config *configs.Config, logger *utils.Logger) (*auth.AuthManager, error) {
	storeConfig := &store.StoreConfig{
		Type:     config.Server.Auth.Store.Type,
		ExpiryHr: config.Server.Auth.Store.Expiry,
		Config:   make(map[string]interface{}),
	}

	authManager, err := auth.NewAuthManager(storeConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("初始化认证管理器失败: %v", err)
	}

	return authManager, nil
}

func startTransportServer(
	config *configs.Config,
	logger *utils.Logger,
	_ *auth.AuthManager,
	g *errgroup.Group,
	groupCtx context.Context,
) (*transport.TransportManager, error) {
	poolManager, err := pool.NewPoolManager(config, logger)
	if err != nil {
		logger.Error("初始化资源池管理器失败: %v", err)
		return nil, fmt.Errorf("初始化资源池管理器失败: %v", err)
	}

	taskMgr := task.NewTaskManager(task.ResourceConfig{
		MaxWorkers:        12,
		MaxTasksPerClient: 20,
	})
	taskMgr.Start()

	transportManager := transport.NewTransportManager(config, logger)

	handlerFactory := transport.NewDefaultConnectionHandlerFactory(
		config,
		poolManager,
		taskMgr,
		logger,
	)

	enabledTransports := make([]string, 0)

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

	logger.Info("[传输层] [启用 %v]", enabledTransports)

	g.Go(func() error {
		go func() {
			<-groupCtx.Done()
			logger.Info("收到关闭信号，开始关闭所有传输层...")
			if err := transportManager.StopAll(); err != nil {
				logger.Error("关闭传输层失败: %v", err)
			} else {
				logger.Info("所有传输层已优雅关闭")
			}
		}()

		if err := transportManager.StartAll(groupCtx); err != nil {
			if groupCtx.Err() != nil {
				return nil
			}
			logger.Error("传输层运行失败: %v", err)
			return err
		}
		return nil
	})

	logger.Debug("传输层服务已成功启动")
	return transportManager, nil
}

func startHTTPServer(
	config *configs.Config,
	logger *utils.Logger,
	g *errgroup.Group,
	groupCtx context.Context,
) (*http.Server, error) {
	if config.Log.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()
	router.SetTrustedProxies([]string{"0.0.0.0"})

	corsConfig := cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"X-Requested-With",
			"Cache-Control",
			"X-File-Name",
			"client-id",
			"device-id",
		},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))

	logger.Debug("全局CORS中间件已配置，支持OPTIONS预检请求")

	apiGroup := router.Group("/api")
	router.Use(static.Serve("/", static.LocalFile("./web", true)))

	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api") {
			c.JSON(http.StatusNotFound, cfg.APIResponse{
				Success: false,
				Data:    gin.H{},
				Message: "api Not found",
				Code:    http.StatusNotFound,
			})
			return
		}

		c.File("./web/index.html")
	})

	otaService := ota.NewDefaultOTAService(config.Web.Websocket)
	if err := otaService.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("OTA 服务启动失败: %v", err)
		return nil, err
	}

	visionService, err := vision.NewDefaultVisionService(config, logger)
	if err != nil {
		logger.Error("Vision 服务初始化失败:%v", err)
		return nil, err
	}
	if err := visionService.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("Vision 服务启动失败 %v", err)
		return nil, err
	}

	cfgServer, err := cfg.NewDefaultAdminService(config, logger)
	if err != nil {
		logger.Error("Admin 服务初始化失败:%v", err)
		return nil, err
	}
	if err := cfgServer.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("Admin 服务启动失败 %v", err)
		return nil, err
	}

	userServer, err := cfg.NewDefaultUserService(config, logger)
	if err != nil {
		logger.Error("用户服务初始化失败:%v", err)
		return nil, err
	}
	if err := userServer.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("用户服务启动失败 %v", err)
		return nil, err
	}

	systemConfigService := cfg.NewSystemConfigService(logger, database.GetDB())
	systemConfigService.RegisterRoutes(apiGroup)

	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(config.Web.Port),
		Handler: router,
	}

	router.GET("/openapi.json", func(c *gin.Context) {
		doc, err := swag.ReadDoc()
		if err != nil {
			logger.Error("生成 OpenAPI 文档失败 %v", err)
			c.JSON(http.StatusInternalServerError, cfg.APIResponse{
				Success: false,
				Data:    gin.H{"error": err.Error()},
				Message: "failed to generate openapi spec",
				Code:    http.StatusInternalServerError,
			})
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(doc))
	})

	router.GET("/docs", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(scalarHTML))
	})

	g.Go(func() error {
		logger.Info("[Gin] 访问地址: http://localhost:%d", config.Web.Port)
		logger.Info("[Gin] [OTA] 访问地址: http://localhost:%d/api/ota/", config.Web.Port)
		logger.Info("[API文档] 服务已启动，访问地址: http://localhost:%d/docs", config.Web.Port)

		go func() {
			<-groupCtx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				logger.Error("HTTP服务关闭失败 %v", err)
			} else {
				logger.Info("HTTP服务已优雅关闭")
			}
		}()

		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP 服务启动失败 %v", err)
			return err
		}
		return nil
	})

	return httpServer, nil
}

func waitForShutdown(
	ctx context.Context,
	cancel context.CancelFunc,
	logger *utils.Logger,
	g *errgroup.Group,
) error {
	<-ctx.Done()
	logger.Info("收到系统信号: %v，开始进行资源清理", context.Cause(ctx))

	cancel()

	done := make(chan error, 1)
	go func() {
		done <- g.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			logger.Error("服务关闭过程中出现错误: %v", err)
			return err
		}
		logger.Info("所有服务已成功关闭")
	case <-time.After(15 * time.Second):
		timeoutErr := errors.New("服务关闭超时")
		logger.Error("服务关闭超时，强制退出")
		return timeoutErr
	}
	return nil
}

func startServices(
	config *configs.Config,
	logger *utils.Logger,
	authManager *auth.AuthManager,
	g *errgroup.Group,
	groupCtx context.Context,
) error {
	if _, err := startTransportServer(config, logger, authManager, g, groupCtx); err != nil {
		return fmt.Errorf("启动 Transport 服务失败: %w", err)
	}

	if _, err := startHTTPServer(config, logger, g, groupCtx); err != nil {
		return fmt.Errorf("启动 Http 服务失败: %w", err)
	}

	return nil
}
