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

	platformconfig "xiaozhi-server-go/internal/platform/config"
	platformerrors "xiaozhi-server-go/internal/platform/errors"
	platformlogging "xiaozhi-server-go/internal/platform/logging"
	platformobservability "xiaozhi-server-go/internal/platform/observability"
	platformstorage "xiaozhi-server-go/internal/platform/storage"
	httptransport "xiaozhi-server-go/internal/transport/http"
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
	config                *configs.Config
	configPath            string
	logProvider           *platformlogging.Logger
	logger                *utils.Logger
	slogger               *slog.Logger
	observabilityShutdown platformobservability.ShutdownFunc
	authManager           *auth.AuthManager
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
			errors.New("config/logger not initialised"),
		)
	}

	authManager := state.authManager
	if authManager == nil {
		return platformerrors.Wrap(
			platformerrors.KindBootstrap,
			"bootstrap state validation",
			errors.New("auth manager not initialised"),
		)
	}

	logBootstrapGraph(logger, steps)

	if shutdown := state.observabilityShutdown; shutdown != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdown(shutdownCtx); err != nil {
				logger.Warn("[bootstrap] observability shutdown failed: %v", err)
			}
		}()
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

func logBootstrapGraph(logger *utils.Logger, steps []initStep) {
	if logger == nil {
		return
	}
	logger.Info("bootstrap dependency graph:")
	for _, step := range steps {
		if len(step.DependsOn) == 0 {
			logger.Info("  %s (%s)", step.ID, step.Title)
			continue
		}
		logger.Info("  %s (%s) depends on %s", step.ID, step.Title, strings.Join(step.DependsOn, ", "))
	}
	logger.Info("  startServices -> transports/http")
}

func executeInitSteps(ctx context.Context, steps []initStep, state *appState) error {
	if state == nil {
		return platformerrors.Wrap(
			platformerrors.KindBootstrap,
			"execute init steps",
			errors.New("nil bootstrap state"),
		)
	}

	completed := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if _, ok := completed[dep]; !ok {
				return platformerrors.Wrap(
					platformerrors.KindBootstrap,
					step.ID,
					fmt.Errorf("dependency %s not satisfied", dep),
				)
			}
		}
		if step.Execute == nil {
			return platformerrors.Wrap(
				platformerrors.KindBootstrap,
				step.ID,
				errors.New("missing execute function"),
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
			return platformerrors.Wrap(kind, step.ID, err)
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
			ID:        "config:load-runtime",
			Title:     "Load runtime configuration",
			DependsOn: []string{"storage:init-config-store"},
			Kind:      platformerrors.KindConfig,
			Execute:   loadConfigStep,
		},
		{
			ID:        "logging:init-provider",
			Title:     "Initialise logging provider",
			DependsOn: []string{"config:load-runtime"},
			Kind:      platformerrors.KindBootstrap,
			Execute:   initLoggingStep,
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
			DependsOn: []string{"observability:setup-hooks"},
			Kind:      platformerrors.KindBootstrap,
			Execute:   initAuthStep,
		},
	}
}

func initStorageStep(_ context.Context, _ *appState) error {
	if err := platformstorage.InitConfigStore(); err != nil {
		return platformerrors.Wrap(platformerrors.KindStorage, "storage:init-config-store", err)
	}
	return nil
}

func loadConfigStep(_ context.Context, state *appState) error {
	result, err := platformconfig.NewLoader().Load()
	if err != nil {
		return platformerrors.Wrap(platformerrors.KindConfig, "config:load-runtime", err)
	}
	state.config = result.Config
	state.configPath = result.Path
	return nil
}

func initLoggingStep(_ context.Context, state *appState) error {
	if state == nil || state.config == nil {
		return platformerrors.Wrap(
			platformerrors.KindBootstrap,
			"logging:init-provider",
			errors.New("config not loaded"),
		)
	}

	logProvider, err := platformlogging.New(platformlogging.Config{
		Level:    state.config.Log.LogLevel,
		Dir:      state.config.Log.LogDir,
		Filename: state.config.Log.LogFile,
	})
	if err != nil {
		return platformerrors.Wrap(platformerrors.KindBootstrap, "logging:init-provider", err)
	}

	state.logProvider = logProvider
	state.logger = logProvider.Legacy()
	state.slogger = logProvider.Slog()
	utils.DefaultLogger = state.logger

	if state.logger != nil {
		state.logger.Info("[bootstrap] logger ready level=%s config_path=%s", state.config.Log.LogLevel, state.configPath)
	}

	database.SetLogger(state.logger)
	database.InsertDefaultConfigIfNeeded(database.GetDB())

	return nil
}

func setupObservabilityStep(ctx context.Context, state *appState) error {
	if state == nil || state.logger == nil || state.config == nil {
		return platformerrors.Wrap(
			platformerrors.KindBootstrap,
			"observability:setup-hooks",
			errors.New("config/logger not initialised"),
		)
	}

	slogger := state.slogger
	if slogger == nil && state.logger != nil {
		slogger = state.logger.Slog()
	}

	cfg := platformobservability.Config{
		Enabled: strings.EqualFold(state.config.Log.LogLevel, "debug"),
	}

	shutdown, err := platformobservability.Setup(ctx, cfg, slogger)
	if err != nil {
		return platformerrors.Wrap(platformerrors.KindBootstrap, "observability:setup-hooks", err)
	}
	state.observabilityShutdown = shutdown

	return nil
}

func initAuthStep(_ context.Context, state *appState) error {
	if state == nil || state.config == nil || state.logger == nil {
		return platformerrors.Wrap(
			platformerrors.KindBootstrap,
			"auth:init-manager",
			errors.New("missing config/logger"),
		)
	}

	authManager, err := initAuthManager(state.config, state.logger)
	if err != nil {
		return err
	}
	state.authManager = authManager
	return nil
}

func loadConfigAndLogger() (*configs.Config, *utils.Logger, error) {
	state := &appState{}
	graph := InitGraph()
	if len(graph) < 3 {
		return nil, nil, platformerrors.Wrap(
			platformerrors.KindBootstrap,
			"load config and logger",
			errors.New("bootstrap graph missing core steps"),
		)
	}

	if err := executeInitSteps(context.Background(), graph[:3], state); err != nil {
		return nil, nil, err
	}
	return state.config, state.logger, nil
}

func initAuthManager(config *configs.Config, logger *utils.Logger) (*auth.AuthManager, error) {
	storeConfig := &store.StoreConfig{
		Type:     config.Server.Auth.Store.Type,
		ExpiryHr: config.Server.Auth.Store.Expiry,
		Config:   make(map[string]interface{}),
	}

	authManager, err := auth.NewAuthManager(storeConfig, logger)
	if err != nil {
		return nil, platformerrors.Wrap(platformerrors.KindBootstrap, "auth:init-manager", err)
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
		return nil, platformerrors.Wrap(platformerrors.KindBootstrap, "auth:init-manager", err)
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

	otaService := ota.NewDefaultOTAService(config.Web.Websocket)
	if err := otaService.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("OTA 服务启动失败: %v", err)
		return nil, err
	}

	visionService, err := vision.NewDefaultVisionService(config, logger)
	if err != nil {
		logger.Error("Vision 服务初始化失败:%v", err)
		return nil, platformerrors.Wrap(platformerrors.KindVision, "vision:new-service", err)
	}
	if err := visionService.Start(groupCtx, router, apiGroup); err != nil {
		logger.Error("Vision 服务启动失败 %v", err)
		return nil, platformerrors.Wrap(platformerrors.KindVision, "vision:start-service", err)
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
