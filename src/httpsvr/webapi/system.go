package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/internal/domain/config/types"

	"github.com/gin-gonic/gin"
)

type DefaultAdminService struct {
	logger *utils.Logger
	config *configs.Config
	repo   types.Repository
}

// NewDefaultAdminService 构造函数
func NewDefaultAdminService(
	config *configs.Config,
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
	config.SelectedASR = s.config.SelectedModule["ASR"]
	config.SelectedTTS = s.config.SelectedModule["TTS"]
	config.SelectedLLM = s.config.SelectedModule["LLM"]
	config.SelectedVLLLM = s.config.SelectedModule["VLLLM"]
	config.Prompt = s.config.DefaultPrompt

	var data map[string]interface{}
	tmp, _ := json.Marshal(config)
	json.Unmarshal(tmp, &data)

	asrList, ttsList, llmList, vllmList := database.GetProviderNameList(database.AdminUserID)
	data["asrList"] = asrList
	data["llmList"] = llmList
	data["ttsList"] = ttsList
	data["vllmList"] = vllmList

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

	s.config.SelectedModule["ASR"] = config.SelectedASR
	s.config.SelectedModule["TTS"] = config.SelectedTTS
	s.config.SelectedModule["LLM"] = config.SelectedLLM
	s.config.SelectedModule["VLLM"] = config.SelectedVLLLM
	s.config.DefaultPrompt = config.Prompt

	if s.repo != nil {
		if err := s.repo.SaveConfig(s.config); err != nil {
			s.logger.Error("保存系统配置失败: %v", err)
			respondError(c, http.StatusInternalServerError, "保存系统配置失败", gin.H{"error": err.Error()})
			return
		}
	} else {
		// 回退到旧方法
		s.config.SaveToDB(database.GetServerConfigDB())
	}
	respondSuccess(c, http.StatusOK, nil, "System configuration saved successfully")
}
