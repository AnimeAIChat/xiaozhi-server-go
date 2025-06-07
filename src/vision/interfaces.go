package vision

import (
	"context"

	"github.com/gin-gonic/gin"
)

// VisionService 定义 Vision 服务接口
type VisionService interface {
	// 将 Vision 的路由注册到 engine 与 apiGroup
	Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) error
}
