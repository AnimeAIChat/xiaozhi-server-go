package server

import (
	"context"

	"github.com/gin-gonic/gin"
)

// CfgService 定义 Cfg 服务接口
type CfgService interface {
	Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) error
}
