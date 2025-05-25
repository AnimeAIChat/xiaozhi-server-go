package main

import (
	"context"
	"fmt"
	"os"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core"
	"xiaozhi-server-go/src/core/utils"

	// 导入所有providers以确保init函数被调用
	_ "xiaozhi-server-go/src/core/providers/asr/doubao"
	_ "xiaozhi-server-go/src/core/providers/llm/ollama"
	_ "xiaozhi-server-go/src/core/providers/llm/openai"
	_ "xiaozhi-server-go/src/core/providers/tts/doubao"
	_ "xiaozhi-server-go/src/core/providers/tts/edge"
)

func main() {
	// 加载配置,默认使用src/configs/.config.yaml
	config, configPath, err := configs.LoadConfig()
	if err != nil {
		panic(err)
	}

	// 初始化日志系统
	logger, err := utils.NewLogger(config)
	if err != nil {
		panic(err)
	}
	logger.Info(fmt.Sprintf("日志系统初始化成功, 配置文件路径: %s", configPath))

	// 创建 WebSocket 服务
	wsServer, err := core.NewWebSocketServer(config, logger)
	if err != nil {
		logger.Error("创建 WebSocket 服务器失败", err)
		os.Exit(1)
	}

	if err := wsServer.Start(context.Background()); err != nil {
		logger.Error("WebSocket 服务运行失败", err)
	}

	logger.Info("服务已成功关闭，程序退出")
}
