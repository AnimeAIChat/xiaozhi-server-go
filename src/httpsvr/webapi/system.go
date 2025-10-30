package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"xiaozhi-server-go/internal/platform/config"
	// "xiaozhi-server-go/src/configs/database" // TODO: Remove database dependency
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/internal/domain/config/types"

	"github.com/gin-gonic/gin"
)

type DefaultAdminService struct {
	logger *utils.Logger
	config *config.Config
	repo   types.Repository
}

// NewDefaultAdminService 构造函数
func NewDefaultAdminService(
	config *config.Config,
	logger *utils.Logger,
) (*DefaultAdminService, error) {
	service := &DefaultAdminService{
		logger: logger,
		config: config,
		repo:   nil, // 暂时设为nil，后续需要修改调用处传入Repository
	}

	return service, nil
}

// Start 实现 AdminService 接口，注册所有 Admin 相关路由
func (s *DefaultAdminService) Start(
	ctx context.Context,
	engine *gin.Engine,
	apiGroup *gin.RouterGroup,
) error {
	apiGroup.GET("/admin", s.handleGet)

	// 需要登录和管理员权限的分组
	adminGroup := apiGroup.Group("")
	// 查看模型不需要管理员权限
	adminGroup.Use(AuthMiddleware(s.config))
	{
		adminGroup.GET("/admin/system", s.handleSystemGet)
		adminGroup.GET("/admin/system/providers/:type", s.handleSystemProvidersType)
	}
	adminGroup.Use(AuthMiddleware(s.config), AdminMiddleware())
	{
		adminGroup.POST("/admin/system", s.handleSystemPost)

		adminGroup.DELETE("/admin/system/device", s.handleDeviceDeleteAdmin)
		// providers
		adminGroup.GET("/admin/system/providers", s.handleSystemProvidersGet)
		adminGroup.GET("/admin/system/providers/:type/:name", s.handleSystemProvidersGetByName)

		adminGroup.POST("/admin/system/providers/create", s.handleSystemProvidersCreate)
		adminGroup.PUT("/admin/system/providers/:type/:name", s.handleSystemProvidersUpdate)
		adminGroup.DELETE("/admin/system/providers/:type/:name", s.handleSystemProvidersDelete)
	}

	s.logger.InfoTag("HTTP", "管理服务路由注册完成")
	return nil
}

func (s *DefaultAdminService) handleGet(c *gin.Context) {
	respondSuccess(c, http.StatusOK, nil, "Admin service is running")
}

type SystemConfig struct {
	SelectedASR   string `                  json:"selectedASR"`
	SelectedTTS   string `                  json:"selectedTTS"`
	SelectedLLM   string `                  json:"selectedLLM"`
	SelectedVLLLM string `                  json:"selectedVLLLM"`
	Prompt        string ` json:"prompt"`
}

// handleSystemGet 获取系统配置
// @Summary 获取系统配置
// @Description 获取当前系统配置
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{} "系统配置信息"
// @Router /admin/system [get]
func (s *DefaultAdminService) handleSystemGet(c *gin.Context) {
	var config SystemConfig
	config.SelectedASR = s.config.Selected.ASR
	config.SelectedTTS = s.config.Selected.TTS
	config.SelectedLLM = s.config.Selected.LLM
	config.SelectedVLLLM = s.config.Selected.VLLLM
	config.Prompt = s.config.System.DefaultPrompt

	var data map[string]interface{}
	tmp, _ := json.Marshal(config)
	json.Unmarshal(tmp, &data)

	// Database functionality removed - return empty lists
	data["asrList"] = []string{}
	data["llmList"] = []string{}
	data["ttsList"] = []string{}
	data["vllmList"] = []string{}

	// fmt.Println("JSON字符串:", data)
	respondSuccess(c, http.StatusOK, data, "System configuration retrieved successfully")
}

// handleSystemGet 获取系统配置
// @Summary 获取系统配置
// @Description 获取当前系统配置
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{} "系统配置信息"
// @Router /admin/system [get]
func (s *DefaultAdminService) handleSystemPost(c *gin.Context) {
	// 定义请求结构体
	var requestData struct {
		Data string `json:"data"`
	}

	// 绑定JSON数据到结构体
	if err := c.ShouldBindJSON(&requestData); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid JSON format", gin.H{"error": err.Error()})
		return
	}

	// 检查data字段是否为空
	if requestData.Data == "" {
		respondError(c, http.StatusBadRequest, "Missing 'data' field in request body", gin.H{"error": "data field is required"})
		return
	}

	s.logger.Info("Received system configuration data: %s", requestData.Data)

	// 解析data字段中的JSON字符串到SystemConfig结构体
	var config SystemConfig
	if err := json.Unmarshal([]byte(requestData.Data), &config); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid system configuration data", gin.H{"error": err.Error()})
		return
	}

	s.config.Selected.ASR = config.SelectedASR
	s.config.Selected.TTS = config.SelectedTTS
	s.config.Selected.LLM = config.SelectedLLM
	s.config.Selected.VLLLM = config.SelectedVLLLM
	s.config.System.DefaultPrompt = config.Prompt

	// Database functionality removed - return error
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration persistence is not available"})
}

// handleSystemProvidersType 获取指定类型的提供商列表
func (s *DefaultAdminService) handleSystemProvidersType(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}
