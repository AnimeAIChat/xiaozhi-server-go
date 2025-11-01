package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"xiaozhi-server-go/internal/domain/llm"
	"xiaozhi-server-go/internal/domain/llm/aggregate"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/platform/logging"
	"xiaozhi-server-go/src/core/providers/llm/doubao"
)

func main() {
	// 初始化配置和日志
	cfg := config.DefaultConfig()
	logger, err := logging.New(logging.Config{
		Level:    "debug",
		Dir:      "./logs",
		Filename: "test.log",
	})
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}

	// 创建Doubao provider配置
	llmConfig := &llm.Config{
		APIKey:      "test-key", // 使用测试key
		BaseURL:     "https://ark.cn-beijing.volces.com/api/v3",
		MaxTokens:   1000,
		Temperature: 0.7,
		Extra: map[string]interface{}{
			"thinking": "disabled",
		},
	}

	// 创建Doubao provider实例
	provider, err := doubao.NewProvider(llmConfig)
	if err != nil {
		log.Fatalf("Failed to create doubao provider: %v", err)
	}

	// 创建测试请求 - 使用legacy interface
	req := &llm.GenerateRequest{
		SessionID: "test-session",
		Messages: []aggregate.Message{
			{
				ID:        "msg-1",
				Role:      "user",
				Content:   "What time is it?",
				Timestamp: time.Now(),
			},
		},
		Tools: []aggregate.Tool{
			{
				ID:   "tool-1",
				Type: "function",
				Function: aggregate.ToolFunction{
					Name:        "time",
					Description: "Get current time",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{},
					},
				},
			},
		},
		Config: aggregate.Config{
			Provider:    "doubao",
			Model:       "doubao-lite-32k",
			Temperature: 0.7,
			MaxTokens:   1000,
		},
	}

	fmt.Println("Testing Doubao provider with tools...")
	fmt.Printf("Tools in request: %d\n", len(req.Tools))

	// 调用ResponseWithFunctions方法
	response, err := provider.ResponseWithFunctions(context.Background(), req)
	if err != nil {
		log.Fatalf("Error calling ResponseWithFunctions: %v", err)
	}

	fmt.Printf("Response received: %+v\n", response)
}