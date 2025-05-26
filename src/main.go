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
	"xiaozhi-server-go/src/core"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/ota"

	// 导入所有providers以确保init函数被调用
	_ "xiaozhi-server-go/src/core/providers/asr/doubao"
	_ "xiaozhi-server-go/src/core/providers/llm/ollama"
	_ "xiaozhi-server-go/src/core/providers/llm/openai"
	_ "xiaozhi-server-go/src/core/providers/tts/doubao"
	_ "xiaozhi-server-go/src/core/providers/tts/edge"
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
	logger.Info(fmt.Sprintf("日志系统初始化成功, 配置文件路径: %s", configPath))

	return config, logger, nil
}

func StartWSServer(config *configs.Config, logger *utils.Logger, g *errgroup.Group) (*core.WebSocketServer, error) {
	// 创建 WebSocket 服务
	wsServer, err := core.NewWebSocketServer(config, logger)
	if err != nil {
		return nil, err
	}

	// 启动 WebSocket 服务
	g.Go(func() error {
		if err := wsServer.Start(context.Background()); err != nil {
			logger.Error("WebSocket 服务运行失败", err)
			return err
		}
		return nil
	})

	logger.Info("WebSocket 服务已成功启动")
	return wsServer, nil
}

func StartHttpServer(config *configs.Config, logger *utils.Logger, g *errgroup.Group) (*http.Server, error) {
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
	otaService := ota.NewDefaultOTAService(config.Web.Websocket)
	if err := otaService.Start(context.Background(), router, apiGroup); err != nil {
		logger.Error("OTA 服务启动失败", err)
		return nil, err
	}

	// HTTP Server（支持优雅关机）
	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(config.Web.Port),
		Handler: router,
	}

	g.Go(func() error {
		logger.Info(fmt.Sprintf("Gin 服务已启动，访问地址: http://0.0.0.0:%d", config.Web.Port))
		// ListenAndServe 返回 ErrServerClosed 时表示正常关闭
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP 服务启动失败", err)
			return err
		}
		return nil
	})

	return httpServer, nil
}

// 优雅关机处理
func ShutdownServer(httpServer *http.Server, wsServer *core.WebSocketServer, ctx context.Context, logger *utils.Logger, g *errgroup.Group) {
	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("接收到系统信号，准备关闭服务", sig)
	case <-ctx.Done():
		logger.Info("服务上下文已取消，准备关闭服务")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 先关闭 HTTP 服务
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP 服务优雅关机失败", err)
	} else {
		logger.Info("HTTP 服务已优雅关闭")
	}

	// 再关闭 WebSocket 服务
	if err := wsServer.Stop(); err != nil {
		logger.Error("WebSocket 服务关闭失败", err)
	} else {
		logger.Info("WebSocket 服务已关闭")
	}

	// 等待 errgroup 中其他 goroutine 退出
	if err := g.Wait(); err != nil {
		logger.Error("服务退出时出现错误", err)
		// logger.Close()
		os.Exit(1)
	}
}

func main() {
	// 加载配置和初始化日志系统
	config, logger, err := LoadConfigAndLogger()
	if err != nil {
		fmt.Println("加载配置或初始化日志系统失败:", err)
		os.Exit(1)
	}

	// 用 errgroup 管理两个服务
	g, ctx := errgroup.WithContext(context.Background())

	// 启动 WebSocket 服务
	wsServer, err := StartWSServer(config, logger, g)
	if err != nil {
		logger.Error("启动 WebSocket 服务失败:", err)
		os.Exit(1)
	}

	// 启动 Http 服务
	httpServer, err := StartHttpServer(config, logger, g)
	if err != nil {
		logger.Error("启动 Http 服务失败:", err)
		os.Exit(1)
	}

	// 启动优雅关机处理
	ShutdownServer(httpServer, wsServer, ctx, logger, g)

	logger.Info("服务已成功关闭，程序退出")
}
