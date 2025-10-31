package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	domainauth "xiaozhi-server-go/internal/domain/auth"
	authstore "xiaozhi-server-go/internal/domain/auth/store"
	"xiaozhi-server-go/internal/domain/config/manager"
	"xiaozhi-server-go/internal/domain/config/types"
	"xiaozhi-server-go/internal/domain/device/service"
	"xiaozhi-server-go/internal/domain/eventbus"
	platformerrors "xiaozhi-server-go/internal/platform/errors"
	platformlogging "xiaozhi-server-go/internal/platform/logging"
	platformobservability "xiaozhi-server-go/internal/platform/observability"
	platformstorage "xiaozhi-server-go/internal/platform/storage"
	platformconfig "xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/transport/http"
	"xiaozhi-server-go/src/core/mcp"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/transport"
	"xiaozhi-server-go/src/core/transport/websocket"
	"xiaozhi-server-go/src/core/utils"
	_ "xiaozhi-server-go/internal/platform/docs"
	"xiaozhi-server-go/src/httpsvr/ota"
	"xiaozhi-server-go/src/httpsvr/vision"
	"xiaozhi-server-go/src/task"

	cfg "xiaozhi-server-go/src/httpsvr/webapi"

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

type stepFn func(context.Context, *appState) error

type initStep struct {
	ID        string
	Title     string
	DependsOn []string
	Kind      platformerrors.Kind
	Execute   stepFn
}

type appState struct {
	config                *platformconfig.Config
	configPath            string
	configRepo            types.Repository
	logProvider           *platformlogging.Logger
	logger                *utils.Logger
	slogger               *slog.Logger
	observabilityShutdown platformobservability.ShutdownFunc
	authManager           *domainauth.AuthManager
	mcpManager            *mcp.Manager
}

// Run 启动整个服务生命周期，负责加载配置、初始化依赖和优雅关停。
func Run(ctx context.Context) error {
	state := &appState{}

	steps := InitGraph()
	if err := executeInitSteps(ctx, steps, state); err != nil {
		return err
	}

	config := state.config
	logger := state.logger
	if config == nil || logger == nil {
		return platformerrors.Wrap(
			platformerrors.KindBootstrap,
			"bootstrap state validation",
			"config/logger not initialised",
			errors.New("config/logger not initialised"),
		)
	}

	authManager := state.authManager
	if authManager == nil {
		return platformerrors.Wrap(
			platformerrors.KindBootstrap,
			"bootstrap state validation",
			"auth manager not initialised",
			errors.New("auth manager not initialised"),
		)
	}

	mcpManager := state.mcpManager
	if mcpManager == nil {
		return platformerrors.New(
			platformerrors.KindBootstrap,
			"bootstrap state validation",
			"mcp manager not initialised",
		)
	}

	logBootstrapGraph(logger, steps)

	if shutdown := state.observabilityShutdown; shutdown != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdown(shutdownCtx); err != nil {
				logger.WarnTag("引导", "可观测性未正常关闭: %v", err)
			}
		}()
	}

	defer func() {
		if authManager != nil {
			if closeErr := authManager.Close(); closeErr != nil {
				logger.ErrorTag("认证", "认证管理器未正常关闭: %v", closeErr)
			}
		}
	}()

	rootCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	signalCtx, stop := signal.NotifyContext(rootCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	group, groupCtx := errgroup.WithContext(rootCtx)

	if err := startServices(state.config, logger, authManager, mcpManager, state.configRepo, group, groupCtx); err != nil {
		cancel()
		return err
	}

	if err := waitForShutdown(signalCtx, cancel, logger, group); err != nil {
		return err
	}

	logger.InfoTag("引导", "服务已成功启动")
	logger.Close()
	return nil
}

func logBootstrapGraph(logger *utils.Logger, steps []initStep) {
	if logger == nil {
		return
	}
	logger.InfoTag("引导", "初始化依赖关系概览")
	for _, step := range steps {
		if len(step.DependsOn) == 0 {
			logger.InfoTag("引导", "根步骤 %s（%s）", step.ID, step.Title)
			continue
		}
		logger.InfoTag("引导", "%s（%s） 依赖 %s", step.ID, step.Title, strings.Join(step.DependsOn, ", "))
	}
	logger.InfoTag("引导", "startServices 依赖 transports/http")
}

func executeInitSteps(ctx context.Context, steps []initStep, state *appState) error {
	if state == nil {
		return platformerrors.New(
			platformerrors.KindBootstrap,
			"execute init steps",
			"nil bootstrap state",
		)
	}

	completed := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if _, ok := completed[dep]; !ok {
				return platformerrors.New(
					platformerrors.KindBootstrap,
					step.ID,
					fmt.Sprintf("dependency %s not satisfied", dep),
				)
			}
		}
		if step.Execute == nil {
			return platformerrors.New(
				platformerrors.KindBootstrap,
				step.ID,
				"missing execute function",
			)
		}
		if err := step.Execute(ctx, state); err != nil {
			var typed *platformerrors.Error
			if errors.As(err, &typed) {
				return err
			}

			kind := step.Kind
			if kind == "" {
				kind = platformerrors.KindBootstrap
			}
			return platformerrors.Wrap(kind, step.ID, "bootstrap step failed", err)
		}
		completed[step.ID] = struct{}{}
	}
	return nil
}

func InitGraph() []initStep {
	return []initStep{
		{
			ID:      "storage:init-config-store",
			Title:   "Initialise configuration store",
			Kind:    platformerrors.KindStorage,
			Execute: initStorageStep,
		},
		{
			ID:      "storage:init-database",
			Title:   "Initialise database",
			Kind:    platformerrors.KindStorage,
			Execute: initDatabaseStep,
		},
		{
			ID:        "config:load-default",
			Title:     "Load configuration from database",
			DependsOn: []string{"storage:init-config-store", "storage:init-database"},
			Kind:      platformerrors.KindConfig,
			Execute:   loadDefaultConfigStep,
		},
		{
			ID:        "logging:init-provider",
			Title:     "Initialise logging provider",
			DependsOn: []string{"config:load-default"},
			Kind:      platformerrors.KindBootstrap,
			Execute:   initLoggingStep,
		},
		{
			ID:        "mcp:init-manager",
			Title:     "Initialise MCP manager",
			DependsOn: []string{"logging:init-provider"},
			Kind:      platformerrors.KindBootstrap,
			Execute:   initMCPManagerStep,
		},
		{
			ID:        "observability:setup-hooks",
			Title:     "Setup observability hooks",
			DependsOn: []string{"logging:init-provider"},
			Kind:      platformerrors.KindBootstrap,
			Execute:   setupObservabilityStep,
		},
		{
			ID:        "auth:init-manager",
			Title:     "Initialise auth manager",
			DependsOn: []string{"observability:setup-hooks", "storage:init-database"},
			Kind:      platformerrors.KindBootstrap,
			Execute:   initAuthStep,
		},
	}
}

func initStorageStep(_ context.Context, _ *appState) error {
	if err := platformstorage.InitConfigStore(); err != nil {
		return platformerrors.Wrap(platformerrors.KindStorage, "storage:init-config-store", "failed to initialize config store", err)
	}
	return nil
}

func initDatabaseStep(_ context.Context, _ *appState) error {
	if err := platformstorage.InitDatabase(); err != nil {
		return platformerrors.Wrap(platformerrors.KindStorage, "storage:init-database", "failed to initialize database", err)
	}
	return nil
}

func loadDefaultConfigStep(_ context.Context, state *appState) error {
	// Create database-backed config repository
	configRepo := manager.NewDatabaseRepository(platformstorage.GetDB())
	state.configRepo = configRepo

	// Load configuration from database, fallback to defaults if not found
	config, err := configRepo.LoadConfig()
	if err != nil {
		return platformerrors.Wrap(platformerrors.KindConfig, "config:load-default", "failed to load config from database", err)
	}

	state.config = config
	state.configPath = "database:config"
	return nil
}

func initLoggingStep(_ context.Context, state *appState) error {
	if state == nil || state.config == nil {
		return platformerrors.New(
			platformerrors.KindBootstrap,
			"logging:init-provider",
			"config not loaded",
		)
	}

	logProvider, err := platformlogging.New(platformlogging.Config{
		Level:    state.config.Log.Level,
		Dir:      state.config.Log.Dir,
		Filename: state.config.Log.File,
	})
	if err != nil {
		return platformerrors.Wrap(platformerrors.KindBootstrap, "logging:init-provider", "failed to initialize logging provider", err)
	}

	state.logProvider = logProvider
	state.logger = logProvider.Legacy()
	state.slogger = logProvider.Slog()
	utils.DefaultLogger = state.logger

	if state.logger != nil {
		state.logger.InfoTag(
			"引导",
			"日志模块就绪 [%s] %s",
			state.config.Log.Level,
			state.configPath,
		)
	}

	// 设置事件处理器
	eventbus.SetupEventHandlers()

	return nil
}

func setupObservabilityStep(ctx context.Context, state *appState) error {
	if state == nil || state.logger == nil || state.config == nil {
		return platformerrors.New(
			platformerrors.KindBootstrap,
			"observability:setup-hooks",
			"config/logger not initialised",
		)
	}

	slogger := state.slogger
	if slogger == nil && state.logger != nil {
		slogger = state.logger.Slog()
	}

	cfg := platformobservability.Config{
		Enabled: strings.EqualFold(state.config.Log.Level, "debug"),
	}

	shutdown, err := platformobservability.Setup(ctx, cfg, slogger)
	if err != nil {
		return platformerrors.Wrap(platformerrors.KindBootstrap, "observability:setup-hooks", "failed to setup observability hooks", err)
	}
	state.observabilityShutdown = shutdown

	return nil
}

func initAuthStep(_ context.Context, state *appState) error {
	if state == nil || state.config == nil || state.logger == nil {
		return platformerrors.New(
			platformerrors.KindBootstrap,
			"auth:init-manager",
			"missing config/logger",
		)
	}

	authManager, err := initAuthManager(state.config, state.logger)
	if err != nil {
		return err
	}
	state.authManager = authManager
	return nil
}

func initMCPManagerStep(_ context.Context, state *appState) error {
	if state == nil || state.config == nil || state.logger == nil {
		return platformerrors.New(
			platformerrors.KindBootstrap,
			"mcp:init-manager",
			"missing config/logger",
		)
	}

	// Use new config format for MCP manager
	mcpManager := mcp.NewManagerForPool(state.logger, state.config)
	state.mcpManager = mcpManager
	return nil
}

func initAuthManager(config *platformconfig.Config, logger *utils.Logger) (*domainauth.AuthManager, error) {
	storeType := strings.ToLower(strings.TrimSpace(config.Server.Auth.Store.Type))
	storeCfg := authstore.Config{
		Driver: storeType,
		TTL:    config.Server.Auth.Store.Expiry,
	}

	if storeCfg.Driver == "" || storeCfg.Driver == "database" || storeCfg.Driver == "sqlite" {
		storeCfg.Driver = authstore.DriverSQLite
	}

	cleanupInterval := config.Server.Auth.Store.Cleanup
	if cleanupInterval <= 0 {
		cleanupInterval = 10 * time.Minute // default cleanup interval
	}

	switch storeCfg.Driver {
	case authstore.DriverMemory:
		if config.Server.Auth.Store.Memory.Cleanup > 0 {
			cleanupInterval = config.Server.Auth.Store.Memory.Cleanup
		}
		storeCfg.Memory = &authstore.MemoryConfig{
			GCInterval: cleanupInterval,
		}
	case authstore.DriverSQLite:
		storeCfg.SQLite = &authstore.SQLiteConfig{
			DSN: config.Server.Auth.Store.SQLite.DSN,
		}
	case authstore.DriverRedis:
		storeCfg.Redis = &authstore.RedisConfig{
			Addr:     config.Server.Auth.Store.Redis.Addr,
			Username: config.Server.Auth.Store.Redis.Username,
			Password: config.Server.Auth.Store.Redis.Password,
			DB:       config.Server.Auth.Store.Redis.DB,
			Prefix:   config.Server.Auth.Store.Redis.Prefix,
		}
		if storeCfg.Redis.Addr == "" {
			return nil, platformerrors.Wrap(
				platformerrors.KindBootstrap,
				"auth:init-manager",
				"redis store addr is required",
				errors.New("redis store addr is required"),
			)
		}
	default:
		logger.WarnTag("认证", "不支持的存储类型 %s，已自动回退至内存模式", storeType)
		storeCfg.Driver = authstore.DriverMemory
		storeCfg.Memory = &authstore.MemoryConfig{GCInterval: cleanupInterval}
	}

	storeDeps := authstore.Dependencies{
		SQLiteDB: platformstorage.GetDB(),
	}
	authStore, err := authstore.New(storeCfg, storeDeps)
	if err != nil {
		return nil, platformerrors.Wrap(platformerrors.KindBootstrap, "auth:init-manager", "failed to create auth store", err)
	}

	crypto := domainauth.NewMemoryCryptoManager(logger, storeCfg.TTL)
	opts := domainauth.Options{
		Store:           authStore,
		Logger:          logger,
		Crypto:          crypto,
		SessionTTL:      storeCfg.TTL,
		CleanupInterval: cleanupInterval,
	}

	authManager, err := domainauth.NewManager(opts)
	if err != nil {
		return nil, platformerrors.Wrap(platformerrors.KindBootstrap, "auth:init-manager", "failed to create auth manager", err)
	}

	return authManager, nil
}

func parseDurationOrWarn(logger *utils.Logger, value string, field string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		logger.WarnTag("配置", "无法解析 %s，原始值 %s（%v）", field, value, err)
		return 0
	}
	if duration <= 0 {
		logger.WarnTag("配置", "%s 必须为正数，当前值为 %s", field, value)
		return 0
	}
	return duration
}

func startTransportServer(
	config *platformconfig.Config,
	logger *utils.Logger,
	authManager *domainauth.AuthManager,
	mcpManager *mcp.Manager,
	g *errgroup.Group,
	groupCtx context.Context,
) (*transport.TransportManager, error) {
	var poolManager *pool.PoolManager
	var poolErr error

	// 异步初始化资源池管理器
	poolInitDone := make(chan struct{})
	go func() {
		defer close(poolInitDone)
		poolManager, poolErr = pool.NewPoolManagerWithMCP(config, logger, mcpManager)
		if poolErr != nil {
			logger.ErrorTag("引导", "初始化资源池管理器失败: %v", poolErr)
			return
		}

		// 预热资源池以减少首次请求延迟
		if err := poolManager.Warmup(context.Background()); err != nil {
			logger.WarnTag("引导", "资源池预热失败: %v", err)
		}
	}()

	taskMgr := task.NewTaskManager(task.ResourceConfig{
		MaxWorkers:        8,
		MaxTasksPerClient: 20,
	})
	taskMgr.Start()

	transportManager := transport.NewTransportManager(config, logger)

	handlerFactory := transport.NewDefaultConnectionHandlerFactory(
		config,
		nil, // poolManager will be set later
		taskMgr,
		logger,
	)

	enabledTransports := make([]string, 0)

	if config.Transport.WebSocket.Enabled {
		wsTransport := websocket.NewWebSocketTransport(config, logger)
		wsTransport.SetConnectionHandler(handlerFactory)
		transportManager.RegisterTransport("websocket", wsTransport)
		enabledTransports = append(enabledTransports, "WebSocket")
		logger.DebugTag("传输", "WebSocket 驱动已注册")
	}

	if len(enabledTransports) == 0 {
		return nil, fmt.Errorf("未启用任何传输驱动")
	}

	logger.InfoTag("传输", "已启用的传输驱动: %v", enabledTransports)

	g.Go(func() error {
		go func() {
			<-groupCtx.Done()
			logger.InfoTag("传输", "收到关闭信号，正在依次关闭传输驱动")
			if err := transportManager.StopAll(); err != nil {
				logger.ErrorTag("传输", "关闭传输驱动失败: %v", err)
			} else {
				logger.InfoTag("传输", "所有传输驱动已优雅关闭")
			}
		}()

		// 等待池管理器初始化完成
		<-poolInitDone
		if poolErr != nil {
			return poolErr
		}

		// 更新处理器工厂的池管理器
		handlerFactory.SetPoolManager(poolManager)

		if err := transportManager.StartAll(groupCtx); err != nil {
			if groupCtx.Err() != nil {
				return nil
			}
			logger.ErrorTag("传输", "传输驱动运行失败: %v", err)
			return err
		}
		return nil
	})

	logger.DebugTag("传输", "传输服务已成功启动")
	return transportManager, nil
}

func startHTTPServer(
	config *platformconfig.Config,
	logger *utils.Logger,
	configRepo types.Repository,
	g *errgroup.Group,
	groupCtx context.Context,
) (*http.Server, error) {
	httpRouter, err := httptransport.Build(httptransport.Options{
		Config: config,
		Logger: logger,
	})
	if err != nil {
		return nil, err
	}
	router := httpRouter.Engine
	apiGroup := httpRouter.API

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

	// 初始化设备服务
	db := platformstorage.GetDB()
	deviceRepo := platformstorage.NewDeviceRepository(db)
	verificationRepo := platformstorage.NewVerificationCodeRepository(db)

	deviceService := service.NewDeviceService(
		deviceRepo,
		verificationRepo,
		config.Server.Device.RequireActivationCode,
		int(config.Server.Device.DefaultAdminUserID),
	)

	otaHandler := httptransport.NewOTAHandler(deviceService)
	otaHandler.RegisterRoutes(httpRouter)

	otaService := ota.NewDefaultOTAService(config.Web.Websocket, config, deviceService)
	if err := otaService.Start(groupCtx, router, apiGroup); err != nil {
		logger.ErrorTag("OTA", "OTA 服务启动失败: %v", err)
		return nil, err
	}

	visionService, err := vision.NewDefaultVisionService(config, logger)
	if err != nil {
		logger.ErrorTag("视觉", "Vision 服务初始化失败: %v", err)
		return nil, platformerrors.Wrap(platformerrors.KindVision, "vision:new-service", "failed to create vision service", err)
	}
	if err := visionService.Start(groupCtx, router, apiGroup); err != nil {
		logger.ErrorTag("视觉", "Vision 服务启动失败: %v", err)
		return nil, platformerrors.Wrap(platformerrors.KindVision, "vision:start-service", "failed to start vision service", err)
	}

	cfgServer, err := cfg.NewDefaultAdminService(config, logger)
	if err != nil {
		logger.ErrorTag("管理后台", "Admin 服务初始化失败: %v", err)
		return nil, err
	}
	if err := cfgServer.Start(groupCtx, router, apiGroup); err != nil {
		logger.ErrorTag("管理后台", "Admin 服务启动失败: %v", err)
		return nil, err
	}

	userServer, err := cfg.NewDefaultUserService(config, logger)
	if err != nil {
		logger.ErrorTag("用户服务", "用户服务初始化失败: %v", err)
		return nil, err
	}
	if err := userServer.Start(groupCtx, router, apiGroup); err != nil {
		logger.ErrorTag("用户服务", "用户服务启动失败: %v", err)
		return nil, err
	}

	// Note: System config service removed as we no longer use database-backed configuration

	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(config.Web.Port),
		Handler: router,
	}

	router.GET("/openapi.json", func(c *gin.Context) {
		doc, err := swag.ReadDoc()
		if err != nil {
			logger.ErrorTag("HTTP", "生成 OpenAPI 文档失败: %v", err)
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
		logger.InfoTag("HTTP", "Gin 服务已启动，访问地址 http://localhost:%d", config.Web.Port)
		logger.InfoTag("HTTP", "OTA 服务入口: http://localhost:%d/api/ota/", config.Web.Port)
		logger.InfoTag("HTTP", "在线文档入口: http://localhost:%d/docs", config.Web.Port)

		go func() {
			<-groupCtx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				logger.ErrorTag("HTTP", "HTTP 服务关闭失败: %v", err)
			} else {
				logger.InfoTag("HTTP", "HTTP 服务已优雅关闭")
			}
		}()

		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.ErrorTag("HTTP", "HTTP 服务启动失败: %v", err)
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
	logger.InfoTag("引导", "收到系统信号 %v，正在进行资源清理", context.Cause(ctx))

	cancel()

	done := make(chan error, 1)
	go func() {
		done <- g.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			logger.ErrorTag("引导", "服务关闭过程中出现错误: %v", err)
			return err
		}
		logger.InfoTag("引导", "所有服务已成功关闭")
	case <-time.After(15 * time.Second):
		timeoutErr := errors.New("服务关闭超时")
		logger.ErrorTag("引导", "服务关闭超时，已强制退出")
		return timeoutErr
	}
	return nil
}

func startServices(
	config *platformconfig.Config,
	logger *utils.Logger,
	authManager *domainauth.AuthManager,
	mcpManager *mcp.Manager,
	configRepo types.Repository,
	g *errgroup.Group,
	groupCtx context.Context,
) error {
	if _, err := startTransportServer(config, logger, authManager, mcpManager, g, groupCtx); err != nil {
		return fmt.Errorf("启动 Transport 服务失败: %w", err)
	}

	if _, err := startHTTPServer(config, logger, configRepo, g, groupCtx); err != nil {
		return fmt.Errorf("启动 Http 服务失败: %w", err)
	}

	return nil
}

// loadConfigAndLogger 加载配置和日志记录器（用于测试）
func loadConfigAndLogger() (*platformconfig.Config, *utils.Logger, error) {
	state := &appState{}

	// 执行必要的初始化步骤
	steps := []initStep{
		{
			ID:      "storage:init-config-store",
			Title:   "Initialise configuration store",
			Kind:    platformerrors.KindStorage,
			Execute: initStorageStep,
		},
		{
			ID:      "storage:init-database",
			Title:   "Initialise database",
			Kind:    platformerrors.KindStorage,
			Execute: initDatabaseStep,
		},
		{
			ID:        "config:load-default",
			Title:     "Load configuration from database",
			DependsOn: []string{"storage:init-config-store", "storage:init-database"},
			Kind:      platformerrors.KindConfig,
			Execute:   loadDefaultConfigStep,
		},
		{
			ID:        "logging:init-provider",
			Title:     "Initialise logging provider",
			DependsOn: []string{"config:load-default"},
			Kind:      platformerrors.KindBootstrap,
			Execute:   initLoggingStep,
		},
	}

	if err := executeInitSteps(context.Background(), steps, state); err != nil {
		return nil, nil, err
	}

	return state.config, state.logger, nil
}
