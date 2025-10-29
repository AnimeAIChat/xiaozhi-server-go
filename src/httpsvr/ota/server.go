package ota

import (
	"context"

	"xiaozhi-server-go/src/configs"

	"github.com/gin-gonic/gin"
)

type DefaultOTAService struct {
	UpdateURL string
	config    *configs.Config
}

// NewDefaultOTAService 构造函数
func NewDefaultOTAService(updateURL string, config *configs.Config) *DefaultOTAService {
	return &DefaultOTAService{
		UpdateURL: updateURL,
		config:    config,
	}
}

// Start 注册 OTA 相关路由
func (s *DefaultOTAService) Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) error {

	apiGroup.Any("/ota/", s.HandleOTARequest())

	apiGroup.GET("/ota_bin/*filepath", s.HandleFirmwareDownload())

	return nil
}
