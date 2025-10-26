package ota

import (
	"context"

	"github.com/gin-gonic/gin"
)

// OTAService 定义 OTA 服务接口
type OTAService interface {
	// 将 OTA 的路由注册到 engine 与 apiGroup
	Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) error
}
