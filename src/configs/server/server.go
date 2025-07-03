package server

import (
	"context"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
)

// CfgResponse 配置服务的通用响应（用于 Swagger 文档）
type CfgResponse struct {
	Status  string `json:"status" example:"ok"`
	Message string `json:"message" example:"Cfg service is running"`
}

type DefaultCfgService struct {
	logger *utils.Logger
	config *configs.Config
}

// NewDefaultCfgService 构造函数
func NewDefaultCfgService(config *configs.Config, logger *utils.Logger) (*DefaultCfgService, error) {
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

	s.logger.Info("Cfg HTTP服务路由注册完成")
	return nil
}

// handleGet godoc
// @Summary 获取配置服务状态
// @Description 用于前端检测 /cfg 接口是否正常运行
// @Tags Config
// @Produce json
// @Success 200 {object} CfgResponse
// @Router /cfg [get]
func (s *DefaultCfgService) handleGet(c *gin.Context) {
	c.JSON(200, CfgResponse{
		Status:  "ok",
		Message: "Cfg service is running",
	})
}

// handlePost godoc
// @Summary 提交配置或检测接口
// @Description 当前 POST /cfg 接口无实际功能，仅用于协议保留或测试
// @Tags Config
// @Accept json
// @Produce json
// @Success 200 {object} CfgResponse
// @Router /cfg [post]
func (s *DefaultCfgService) handlePost(c *gin.Context) {
	c.JSON(200, CfgResponse{
		Status:  "ok",
		Message: "Cfg service is running",
	})
}

// OPTIONS 请求一般为预检，无需在 Swagger 中记录
func (s *DefaultCfgService) handleOptions(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
	c.Status(204)
}
