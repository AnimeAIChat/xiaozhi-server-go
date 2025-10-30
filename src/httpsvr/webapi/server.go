package webapi

import (
	"context"
	"net/http"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
)

type DefaultCfgService struct {
	logger *utils.Logger
	config *config.Config
}

// NewDefaultCfgService 构造函数
func NewDefaultCfgService(config *config.Config, logger *utils.Logger) (*DefaultCfgService, error) {
	service := &DefaultCfgService{
		logger: logger,
		config: config,
	}

	return service, nil
}

// Start 实现 CfgService 接口，注册所有 Cfg 相关路由
func (s *DefaultCfgService) Start(ctx context.Context, engine *gin.Engine, apiGroup *gin.RouterGroup) error {

	apiGroup.GET("/cfg", s.handleGet)
	apiGroup.POST("/cfg", s.handlePost)
	apiGroup.OPTIONS("/cfg", s.handleOptions)

	s.logger.InfoTag("HTTP", "配置服务路由注册完成")
	return nil
}

func (s *DefaultCfgService) handleGet(c *gin.Context) {
	respondSuccess(c, http.StatusOK, nil, "Cfg service is running")
}

func (s *DefaultCfgService) handlePost(c *gin.Context) {
	respondSuccess(c, http.StatusOK, nil, "Cfg service is running")
}

func (s *DefaultCfgService) handleOptions(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
	c.Status(204) // No Content
}
