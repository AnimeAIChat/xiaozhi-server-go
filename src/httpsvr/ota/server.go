package ota

import (
	"context"

	"xiaozhi-server-go/internal/domain/device/service"
	"xiaozhi-server-go/internal/platform/config"

	"github.com/gin-gonic/gin"
)

type DefaultOTAService struct {
	UpdateURL    string
	config       *config.Config
	deviceService *service.DeviceService
}

// NewDefaultOTAService 构造函数
func NewDefaultOTAService(updateURL string, config *config.Config, deviceService *service.DeviceService) *DefaultOTAService {
	return &DefaultOTAService{
		UpdateURL:    updateURL,
		config:       config,
		deviceService: deviceService,
	}
}

// Start 注册 OTA 相关路由
func (s *DefaultOTAService) Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) error {

	apiGroup.Any("/ota/", s.HandleOTARequest())

	apiGroup.GET("/ota_bin/*filepath", s.HandleFirmwareDownload())

	return nil
}
