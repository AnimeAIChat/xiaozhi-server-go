package webapi

import (
	"context"
	"encoding/json"
	"net/http"

	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/platform/errors"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
)

// Service WebAPI服务的HTTP传输层实现
type Service struct {
	logger *utils.Logger
	config *config.Config
}

// NewService 创建新的WebAPI服务实例
func NewService(config *config.Config, logger *utils.Logger) (*Service, error) {
	if config == nil {
		return nil, errors.Wrap(errors.KindConfig, "webapi.new", "config is required", nil)
	}
	if logger == nil {
		return nil, errors.Wrap(errors.KindConfig, "webapi.new", "logger is required", nil)
	}

	service := &Service{
		logger: logger,
		config: config,
	}

	return service, nil
}

// Register 注册WebAPI相关的HTTP路由
func (s *Service) Register(ctx context.Context, router *gin.RouterGroup) error {
	// 基础路由
	router.GET("/cfg", s.handleCfgGet)
	router.POST("/cfg", s.handleCfgPost)
	router.OPTIONS("/cfg", s.handleOptions)

	// 设备相关路由 (暂时返回未实现)
	router.GET("/devices", s.handleDevicesNotImplemented)
	router.POST("/devices", s.handleDevicesNotImplemented)
	router.GET("/devices/:id", s.handleDevicesNotImplemented)
	router.PUT("/devices/:id", s.handleDevicesNotImplemented)
	router.DELETE("/devices/:id", s.handleDevicesNotImplemented)

	// 用户相关路由 (暂时返回未实现)
	router.GET("/users", s.handleUsersNotImplemented)
	router.POST("/users", s.handleUsersNotImplemented)

	// Agent相关路由 (暂时返回未实现)
	router.GET("/agents", s.handleAgentsNotImplemented)
	router.POST("/agents", s.handleAgentsNotImplemented)

	// 提供商相关路由 (暂时返回未实现)
	router.GET("/providers", s.handleProvidersNotImplemented)
	router.POST("/providers", s.handleProvidersNotImplemented)

	// 管理员路由
	s.registerAdminRoutes(router)

	s.logger.InfoTag("HTTP", "WebAPI服务路由注册完成")
	return nil
}

// registerAdminRoutes 注册管理员相关路由
func (s *Service) registerAdminRoutes(router *gin.RouterGroup) {
	adminGroup := router.Group("/admin")
	adminGroup.GET("", s.handleAdminGet)

	// 需要认证的分组
	securedGroup := adminGroup.Group("")
	securedGroup.Use(s.authMiddleware())
	{
		securedGroup.GET("/system", s.handleSystemGet)
		securedGroup.GET("/system/providers/:type", s.handleSystemProvidersType)
	}

	// 需要管理员权限的分组
	adminOnlyGroup := adminGroup.Group("")
	adminOnlyGroup.Use(s.authMiddleware(), s.adminMiddleware())
	{
		adminOnlyGroup.POST("/system", s.handleSystemPost)
		adminOnlyGroup.DELETE("/system/device", s.handleDeviceDeleteAdmin)

		// providers
		adminOnlyGroup.GET("/system/providers", s.handleSystemProvidersGet)
		adminOnlyGroup.GET("/system/providers/:type/:name", s.handleSystemProvidersGetByName)
		adminOnlyGroup.POST("/system/providers/create", s.handleSystemProvidersCreate)
		adminOnlyGroup.PUT("/system/providers/:type/:name", s.handleSystemProvidersUpdate)
		adminOnlyGroup.DELETE("/system/providers/:type/:name", s.handleSystemProvidersDelete)
	}
}

// handleCfgGet 处理配置获取请求
// @Summary 检查配置服务状态
// @Description 检查配置服务的运行状态
// @Tags Config
// @Produce json
// @Success 200 {object} object
// @Router /cfg [get]
func (s *Service) handleCfgGet(c *gin.Context) {
	s.respondSuccess(c, http.StatusOK, nil, "Cfg service is running")
}

// handleCfgPost 处理配置更新请求
func (s *Service) handleCfgPost(c *gin.Context) {
	s.respondSuccess(c, http.StatusOK, nil, "Cfg service is running")
}

// handleOptions 处理OPTIONS请求
func (s *Service) handleOptions(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, AuthorToken")
	c.Status(http.StatusNoContent)
}

// handleAdminGet 处理管理员服务状态检查
func (s *Service) handleAdminGet(c *gin.Context) {
	s.respondSuccess(c, http.StatusOK, nil, "Admin service is running")
}

// SystemConfig 系统配置结构
type SystemConfig struct {
	SelectedASR   string `json:"selectedASR"`
	SelectedTTS   string `json:"selectedTTS"`
	SelectedLLM   string `json:"selectedLLM"`
	SelectedVLLLM string `json:"selectedVLLLM"`
	Prompt        string `json:"prompt"`
}

// handleSystemGet 获取系统配置
// @Summary 获取系统配置
// @Description 获取服务器的系统配置信息，包括选择的提供商和默认提示词
// @Tags Admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} SystemConfig
// @Failure 401 {object} object
// @Router /admin/system [get]
func (s *Service) handleSystemGet(c *gin.Context) {
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

	s.respondSuccess(c, http.StatusOK, data, "System configuration retrieved successfully")
}

// handleSystemPost 更新系统配置
func (s *Service) handleSystemPost(c *gin.Context) {
	var requestData struct {
		Data string `json:"data"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		s.respondError(c, http.StatusBadRequest, "Invalid JSON format")
		return
	}

	if requestData.Data == "" {
		s.respondError(c, http.StatusBadRequest, "Missing 'data' field in request body")
		return
	}

	s.logger.Info("Received system configuration data: %s", requestData.Data)

	var config SystemConfig
	if err := json.Unmarshal([]byte(requestData.Data), &config); err != nil {
		s.respondError(c, http.StatusBadRequest, "Invalid system configuration data")
		return
	}

	s.config.Selected.ASR = config.SelectedASR
	s.config.Selected.TTS = config.SelectedTTS
	s.config.Selected.LLM = config.SelectedLLM
	s.config.Selected.VLLLM = config.SelectedVLLLM
	s.config.System.DefaultPrompt = config.Prompt

	// Database functionality removed - return error for persistence
	s.respondError(c, http.StatusNotImplemented, "Database functionality removed - configuration persistence is not available")
}

// handleSystemProvidersType 获取指定类型的提供商列表
func (s *Service) handleSystemProvidersType(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "Database functionality removed")
}

// 以下是暂时未实现的方法，返回相应的错误信息

func (s *Service) handleDevicesNotImplemented(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "Device management functionality not implemented in new architecture")
}

func (s *Service) handleUsersNotImplemented(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "User management functionality not implemented in new architecture")
}

func (s *Service) handleAgentsNotImplemented(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "Agent management functionality not implemented in new architecture")
}

func (s *Service) handleProvidersNotImplemented(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "Provider management functionality not implemented in new architecture")
}

func (s *Service) handleSystemProvidersGet(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "Provider management functionality not implemented in new architecture")
}

func (s *Service) handleSystemProvidersGetByName(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "Provider management functionality not implemented in new architecture")
}

func (s *Service) handleSystemProvidersCreate(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "Provider management functionality not implemented in new architecture")
}

func (s *Service) handleSystemProvidersUpdate(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "Provider management functionality not implemented in new architecture")
}

func (s *Service) handleSystemProvidersDelete(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "Provider management functionality not implemented in new architecture")
}

func (s *Service) handleDeviceDeleteAdmin(c *gin.Context) {
	s.respondError(c, http.StatusNotImplemented, "Device management functionality not implemented in new architecture")
}

// authMiddleware 认证中间件
func (s *Service) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apikey := c.GetHeader("AuthorToken")
		if apikey != "" {
			// 如果提供了API Token，直接验证
			if apikey != s.config.Server.Token {
				s.logger.Error("无效的API Token %s", apikey)
				s.respondError(c, http.StatusUnauthorized, "无效的API Token")
				c.Abort()
				return
			}
			s.logger.Info("API Token验证通过")
			c.Next()
			return
		}

		token := c.GetHeader("Authorization")
		if token == "" {
			s.logger.Error("未提供认证token")
			s.respondError(c, http.StatusUnauthorized, "未提供认证token")
			c.Abort()
			return
		}
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}

		// TODO: JWT验证逻辑暂时简化
		if token == "" {
			s.logger.Error("无效的token")
			s.respondError(c, http.StatusUnauthorized, "无效的token")
			c.Abort()
			return
		}

		c.Next()
	}
}

// adminMiddleware 管理员权限中间件
func (s *Service) adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 管理员权限检查暂时简化，允许所有已认证请求通过
		c.Next()
	}
}

// respondSuccess 返回成功响应
func (s *Service) respondSuccess(c *gin.Context, statusCode int, data interface{}, message string) {
	c.JSON(statusCode, gin.H{
		"success": true,
		"data":    data,
		"message": message,
		"code":    statusCode,
	})
}

// respondError 返回错误响应
func (s *Service) respondError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{
		"success": false,
		"data":    gin.H{"error": message},
		"message": message,
		"code":    statusCode,
	})
}