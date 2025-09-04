package ota

import (
	"context"

	"github.com/gin-gonic/gin"
)

type DefaultOTAService struct {
	UpdateURL string
}

// NewDefaultOTAService 构造函数
func NewDefaultOTAService(updateURL string) *DefaultOTAService {
	return &DefaultOTAService{UpdateURL: updateURL}
}

// Start 注册 OTA 相关路由
func (s *DefaultOTAService) Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) error {
	apiGroup.OPTIONS("/ota/", handleOtaOptions)
	apiGroup.GET("/ota/", func(c *gin.Context) { handleOtaGet(c, s.UpdateURL) })
	apiGroup.POST("/ota/", func(c *gin.Context) { handleOtaPost(c, s.UpdateURL) })

	engine.GET("/ota_bin/:filename", handleOtaBinDownload)

	return nil
}
