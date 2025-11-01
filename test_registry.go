package main

import (
	"fmt"
	"log"

	"github.com/sashabaranov/go-openai"
	"xiaozhi-server-go/internal/domain/llm/infrastructure"
)

func main() {
	// 创建函数注册器
	registry := infrastructure.NewFunctionRegistry()

	// 注册一些测试工具
	tool1 := openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "time",
			Description: "Get current time",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{},
			},
		},
	}

	tool2 := openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "exit",
			Description: "Exit conversation",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	}

	// 注册工具
	err := registry.RegisterFunction("time", tool1)
	if err != nil {
		log.Fatalf("Failed to register time tool: %v", err)
	}

	err = registry.RegisterFunction("exit", tool2)
	if err != nil {
		log.Fatalf("Failed to register exit tool: %v", err)
	}

	// 获取所有工具
	tools := registry.GetAllFunctions()
	fmt.Printf("Retrieved %d tools from registry\n", len(tools))

	// 模拟connection.go中的转换逻辑
	interTools := make([]interface{}, 0, len(tools))
	for _, toolInterface := range tools {
		tool, ok := toolInterface.(openai.Tool)
		if !ok {
			fmt.Printf("工具类型转换失败: %T\n", toolInterface)
			continue
		}
		interTools = append(interTools, tool)
		fmt.Printf("Converted tool: %s - %s\n", tool.Function.Name, tool.Function.Description)
	}

	fmt.Printf("Successfully converted %d tools\n", len(interTools))
}