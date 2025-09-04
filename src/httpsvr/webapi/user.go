package webapi

import (
	"context"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
)

type DefaultUserService struct {
	logger *utils.Logger
	config *configs.Config
}

// NewDefaultUserService 构造函数
func NewDefaultUserService(
	config *configs.Config,
	logger *utils.Logger,
) (*DefaultUserService, error) {
	service := &DefaultUserService{
		logger: logger,
		config: config,
	}
	return service, nil
}

// Start 实现用户服务接口，注册所有用户相关路由
func (s *DefaultUserService) Start(
	ctx context.Context,
	engine *gin.Engine,
	apiGroup *gin.RouterGroup,
) error {

	// 需要认证的用户接口
	userGroup := apiGroup.Group("/user")

	userGroup.GET("/summary", s.handleSystemSummary) // 获取用户汇总信息
	userGroup.GET("/server_config", s.handleServerConfig)

	userGroup.POST("/agent/create", s.handleAgentCreate)
	userGroup.GET("/agent/list", s.handleAgentList)
	userGroup.GET("/agent/:id", s.handleAgentGet)
	userGroup.PUT("/agent/:id", s.handleAgentUpdate)
	userGroup.DELETE("/agent/:id", s.handleAgentDelete)
	userGroup.POST("/agent/clear_conversation/:id", s.handleAgentClearConversation)

	userGroup.POST("/agent/history_dialog_list/:id", s.handleAgentHistoryDialogList)
	userGroup.GET("/agent/history_dialog/:dialog_id", s.handleAgentGetHistoryDialog)
	userGroup.DELETE("/agent/history_dialog/:dialog_id", s.handleAgentDeleteHistoryDialog)

	// providers
	userGroup.GET("/providers/:type", s.handleUserProvidersType)
	userGroup.POST("/providers/create", s.handleUserProvidersCreate)
	userGroup.DELETE("/providers/:type/:name", s.handleUserProvidersDelete)
	userGroup.PUT("/providers/:type/:name", s.handleUserProvidersUpdate)

	s.logger.Info("用户HTTP服务路由注册完成")
	return nil
}

func (s *DefaultUserService) handleServerConfig(c *gin.Context) {
	// 获取服务器配置
	data, _ := database.GetServerConfigDB().GetServerConfig()
	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "获取服务器配置成功",
		"data":    data,
	})
}

func (s *DefaultUserService) handleSystemSummary(c *gin.Context) {

	data, _ := database.GetSystemSummary(database.GetDB())

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "获取系统汇总信息成功",
		"data": gin.H{
			"totle_agents":      data["total_agents"],
			"system_memory_use": data["memory_usage"],
			"system_cpu_use":    data["cpu_usage"],
		},
	})
}
