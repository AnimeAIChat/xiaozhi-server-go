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
	"strings"
	"syscall"
	"time"

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
	"github.com/joho/godotenv"
	"github.com/swaggo/swag"

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

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
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

func LoadConfigAndLogger() (*configs.Config, *utils.Logger, error) {
	// 加载 .env 文件
	err := godotenv.Load()
	if err != nil {
		fmt.Println("未找到 .env 文件，使用系统环境变量")
	}

	// 初始化数据库连接
	_, _, err = database.InitDB()
	if err != nil {
		fmt.Printf("数据库连接失败: %v\n", err)
	}
	// 加载配置,默认使用.config.yaml
	config, configPath, err := configs.LoadConfig(database.GetServerConfigDB())
	if err != nil {
		return nil, nil, err
	}

	// 初始化日志系统
	logger, err := utils.NewLogger((*utils.LogCfg)(&config.Log))
	if err != nil {
		return nil, nil, err
	}
	utils.DefaultLogger = logger
	logger.Info("[日志] [初始化成功] [%s] 配置文件路径: %s", config.Log.LogLevel, configPath)

	database.SetLogger(logger)
	database.InsertDefaultConfigIfNeeded(database.GetDB())

	return config, logger, nil
}

// initAuthManager 初始化认证管理器
func initAuthManager(config *configs.Config, logger *utils.Logger) (*auth.AuthManager, error) {

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
		logger.Error("初始化资源池管理器失败: %v", err)
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

	logger.Info("[传输层] [启用 %v]", enabledTransports)

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

func StartHttpServer(
	config *configs.Config,
	logger *utils.Logger,
	g *errgroup.Group,
	groupCtx context.Context,
) (*http.Server, error) {
	// 初始化Gin引擎
	if config.Log.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()
	router.SetTrustedProxies([]string{"0.0.0.0"})

	// 配置全局CORS中间件
	corsConfig := cors.Config{
		AllowOrigins: []string{"*"}, // 生产环境应指定具体域名
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
	// 应用全局CORS中间件
	router.Use(cors.New(corsConfig))

	logger.Debug("全局CORS中间件已配置，支持OPTIONS预检请求")

	// API路由全部挂载到/api前缀下
	apiGroup := router.Group("/api")

	// 静态资源服务，前端访问 /web/xxx
	router.Use(static.Serve("/", static.LocalFile("./web", true)))

	// history 路由兜底，只处理 /web 下的 GET 请求
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

	// 启动OTA服务
	otaService := ota.NewDefaultOTAService(config.Web.Websocket)
	if err := otaService.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("OTA 服务启动失败: %v", err)
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

	cfgServer, err := cfg.NewDefaultAdminService(config, logger)
	if err != nil {
		logger.Error("Admin 服务初始化失败 %v", err)
		return nil, err
	}
	if err := cfgServer.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("Admin 服务启动失败 %v", err)
		return nil, err
	}

	userServer, err := cfg.NewDefaultUserService(config, logger)
	if err != nil {
		logger.Error("用户服务初始化失败 %v", err)
		return nil, err
	}
	if err := userServer.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("用户服务启动失败 %v", err)
		return nil, err
	}

	// 启动系统配置服务
	systemConfigService := cfg.NewSystemConfigService(logger, database.GetDB())
	systemConfigService.RegisterRoutes(apiGroup)

	// HTTP Server（支持优雅关机）
	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(config.Web.Port),
		Handler: router,
	}

	// 提供 OpenAPI JSON 文档
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

	// 提供 Scalar UI 作为 API 文档界面
	router.GET("/docs", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(scalarHTML))
	})

	g.Go(func() error {
		logger.Info("[Gin] 访问地址: http://localhost:%d", config.Web.Port)
		logger.Info("[Gin] [OTA] 访问地址: http://localhost:%d/api/ota/", config.Web.Port)
		logger.Info("[API文档] 服务已启动，访问地址: http://localhost:%d/docs", config.Web.Port)

		// 在单独的 goroutine 中监听关闭信号
		go func() {
			<-groupCtx.Done()

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

	// 初始化认证管理器
	authManager, err := initAuthManager(config, logger)
	if err != nil {
		logger.Error("初始化认证管理器失败: %v", err)
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
