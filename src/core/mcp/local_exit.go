package mcp

import (
	"context"
	"xiaozhi-server-go/src/core/types"
)

func (c *LocalClient) AddToolExit() error {

	InputSchema := ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"say_goodbye": map[string]any{
				"type":        "string",
				"description": "用户友好结束对话的告别语",
			},
		},
		Required: []string{"say_goodbye"},
	}

	c.AddTool("exit", "当用户想结束对话或需要退出系统时调用", InputSchema, c.HandlerExit)

	return nil
}

func (c *LocalClient) HandlerExit(ctx context.Context, args map[string]any) (interface{}, error) {
	c.logger.Info("用户请求退出对话，告别语：%s", args["say_goodbye"])
	res := types.ActionResponse{
		Action: types.ActionTypeCallHandler, // 动作类型
		Result: types.ActionResponseCall{
			FuncName: "mcp_handler_exit",  // 函数名
			Args:     args["say_goodbye"], // 函数参数
		},
	}
	return res, nil
}
