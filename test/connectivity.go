package main

import (
	"context"
	"fmt"
	"log"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/utils"

	// 导入所有providers以确保init函数被调用
	_ "xiaozhi-server-go/src/core/providers/asr/deepgram"
	_ "xiaozhi-server-go/src/core/providers/asr/doubao"
	_ "xiaozhi-server-go/src/core/providers/llm/coze"
	_ "xiaozhi-server-go/src/core/providers/llm/ollama"
	_ "xiaozhi-server-go/src/core/providers/llm/openai"
	_ "xiaozhi-server-go/src/core/providers/tts/deepgram"
	_ "xiaozhi-server-go/src/core/providers/tts/doubao"
	_ "xiaozhi-server-go/src/core/providers/tts/edge"
	_ "xiaozhi-server-go/src/core/providers/vlllm/ollama"
	_ "xiaozhi-server-go/src/core/providers/vlllm/openai"
)

func main() {
	fmt.Println("=== 连通性检查测试 ===")

	// 加载配置
	config, path, err := configs.LoadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Printf("使用配置文件: %s", path)

	// 创建日志记录器
	logger, err := utils.NewLogger(config)
	if err != nil {
		log.Fatalf("创建日志记录器失败: %v", err)
	}

	// 创建连通性检查配置
	connConfig, err := pool.ConfigFromYAML(&config.ConnectivityCheck)
	if err != nil {
		logger.Warn("解析连通性检查配置失败，使用默认配置: %v", err)
		connConfig = pool.DefaultConnectivityConfig()
	}

	// 打印配置信息
	fmt.Printf("连通性检查配置:\n")
	fmt.Printf("  启用: %v\n", connConfig.Enabled)
	fmt.Printf("  超时时间: %v\n", connConfig.Timeout)
	fmt.Printf("  重试次数: %d\n", connConfig.RetryAttempts)
	fmt.Printf("  重试延迟: %v\n", connConfig.RetryDelay)

	if !connConfig.Enabled {
		fmt.Println("连通性检查已禁用，退出测试")
		return
	}

	// 打印选中的模块
	fmt.Printf("\n选中的模块:\n")
	for moduleType, moduleConfig := range config.SelectedModule {
		fmt.Printf("  %s: %s\n", moduleType, moduleConfig)
	}

	// 创建健康检查器
	healthChecker := pool.NewHealthChecker(config, connConfig, logger)

	ctx := context.Background()

	// 执行基础连通性检查
	fmt.Printf("\n开始执行基础连通性检查...\n")
	err = healthChecker.CheckAllProviders(ctx, pool.BasicCheck)
	if err != nil {
		fmt.Printf("\n❌ 基础连通性检查失败: %v\n", err)
	} else {
		fmt.Printf("\n✅ 基础连通性检查通过！\n")
	}

	// 打印基础检查报告
	fmt.Printf("\n")
	healthChecker.PrintReport()

	// 清空结果准备功能性检查
	healthChecker = pool.NewHealthChecker(config, connConfig, logger)

	// 执行功能性检查
	fmt.Printf("\n开始执行功能性检查...\n")
	err = healthChecker.CheckAllProviders(ctx, pool.FunctionalCheck)
	if err != nil {
		fmt.Printf("\n❌ 功能性检查失败: %v\n", err)
	} else {
		fmt.Printf("\n✅ 功能性检查通过！\n")
	}

	// 打印功能性检查报告
	fmt.Printf("\n")
	healthChecker.PrintReport()

	// 获取详细结果
	results := healthChecker.GetResults()
	fmt.Printf("\n=== 详细检查结果 ===\n")
	for providerType, result := range results {
		fmt.Printf("%s:\n", providerType)
		fmt.Printf("  成功: %v\n", result.Success)
		fmt.Printf("  检查模式: ")
		if result.CheckMode == pool.BasicCheck {
			fmt.Printf("基础连通性检查\n")
		} else {
			fmt.Printf("功能性检查\n")
		}
		fmt.Printf("  耗时: %v\n", result.Duration)
		if result.Error != nil {
			fmt.Printf("  错误: %v\n", result.Error)
		}
		fmt.Printf("  详细信息:\n")
		for key, value := range result.Details {
			fmt.Printf("    %s: %v\n", key, value)
		}
		fmt.Printf("\n")
	}

	fmt.Println("=== 连通性检查测试完成 ===")
}
