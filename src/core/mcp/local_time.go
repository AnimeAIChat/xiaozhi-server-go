package mcp

import (
	"context"
	"time"
	"xiaozhi-server-go/src/core/types"
)

func (c *LocalClient) AddToolTime() error {

	InputSchema := ToolInputSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}

	c.AddTool("get_time", "获取今天日期或者当前时间信息时调用", InputSchema, c.HandlerTime)

	return nil
}

func (c *LocalClient) HandlerTime(ctx context.Context, args map[string]any) (interface{}, error) {
	now := time.Now()
	time := now.Format("2006-01-02 15:04:05")
	week := now.Weekday().String()
	str := "当前时间是 " + time + "，今天是" + week + "。"
	res := types.ActionResponse{
		Action: types.ActionTypeReqLLM, // 动作类型
		Result: str,                    // 函数参数
	}
	return res, nil
}
