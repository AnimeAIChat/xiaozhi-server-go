package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"xiaozhi-server-go/src/configs/database"

	"github.com/gin-gonic/gin"
)

// handleUserProviderTest 验证当前用户可见的已保存 Provider 配置。
// 测试成功必须取得 Provider 的真实响应，不会把字段完整误报为服务可用。
func (s *DefaultUserService) handleUserProviderTest(c *gin.Context) {
	providerType := strings.ToUpper(strings.TrimSpace(c.Param("type")))
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		c.JSON(400, gin.H{"success": false, "message": "Provider 名称不能为空"})
		return
	}

	providers, err := database.GetProviderByTypeInternal(providerType, c.GetUint("user_id"), false)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "message": err.Error()})
		return
	}
	rawConfig, ok := providers[name]
	if !ok {
		c.JSON(404, gin.H{"success": false, "message": "未找到可测试的 Provider"})
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(rawConfig), &config); err != nil {
		c.JSON(400, gin.H{"success": false, "message": "Provider 配置不是有效 JSON"})
		return
	}
	subType, _ := config["type"].(string)
	subType = strings.ToLower(strings.TrimSpace(subType))
	if subType == "" {
		c.JSON(200, gin.H{"success": false, "message": "缺少具体类型（type）"})
		return
	}
	if missing := missingProviderTestFields(providerType, subType, config); len(missing) > 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": fmt.Sprintf("还需填写：%s", strings.Join(missing, "、")),
			"data":    gin.H{"level": "configuration", "missing_fields": missing},
		})
		return
	}

	result, err := s.runLiveProviderTest(c.Request.Context(), providerType, name, subType, config)
	if err != nil {
		c.JSON(200, gin.H{"success": false, "message": err.Error(), "data": gin.H{"level": "live"}})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": result.message,
		"data":    result.data,
	})
}

func stringConfig(config map[string]interface{}, key string) string {
	value, _ := config[key].(string)
	return strings.TrimSpace(value)
}

func missingProviderTestFields(providerType, subType string, config map[string]interface{}) []string {
	required := map[string]map[string][]string{
		"ASR": {
			"doubao": {"appid", "access_token"}, "deepgram": {"addr", "api_key"}, "gosherpa": {"addr"},
			"stepfun": {"api_key", "model", "voice"}, "iflytek": {"appid", "api_key", "api_secret"},
		},
		"TTS": {
			"edge": {"voice"}, "doubao": {"appid", "token", "cluster", "voice"}, "gosherpa": {"cluster"},
			"deepgram": {"cluster", "token", "voice"}, "iflytek": {"appid", "token", "cluster"},
		},
		"LLM": {
			"openai": {"url", "api_key", "model_name"}, "ollama": {"url", "model_name"},
			"doubao": {"url", "api_key", "model_name"}, "coze": {"bot_id", "user_id", "personal_access_token"},
		},
		"VLLLM": {
			"openai": {"url", "api_key", "model_name"}, "ollama": {"url", "model_name"},
		},
	}
	fields, known := required[providerType][subType]
	if !known {
		return []string{"当前类型暂不支持测试"}
	}
	missing := make([]string, 0)
	for _, field := range fields {
		if stringConfig(config, field) == "" {
			missing = append(missing, field)
		}
	}
	return missing
}

// handleSystemProvidersGet 获取所有Provider
// @Summary 获取所有Provider
// @Description 获取系统中所有Provider信息
// @Tags Provider
// @Produce json
// @Success 200 {object} []interface{} "Provider列表"
// @Router /admin/system/providers [get]
func (s *DefaultAdminService) handleSystemProvidersGet(c *gin.Context) {
	providers := database.GetAllProviders(database.AdminUserID)
	if len(providers) == 0 {
		c.JSON(404, gin.H{
			"status":  "error",
			"message": "No providers found",
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "Providers retrieved successfully",
		"data":    providers,
	})
}

// handleSystemProvidersType 获取指定类型Provider【废弃，使用/user/providers/{type}】
// @Summary 获取指定类型Provider
// @Description 根据类型获取Provider信息
// @Tags Provider
// @Produce json
// @Param type path string true "Provider类型"
// @Success 200 {object} interface{} "Provider信息"
// @Router /admin/system/providers/{type} [get]
func (s *DefaultAdminService) handleSystemProvidersType(c *gin.Context) {
	providerType := c.Param("type")
	if providerType == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Provider type is required",
		})
		return
	}

	provider, err := database.GetProviderByType(providerType, database.AdminUserID)
	if err != nil {
		c.JSON(404, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Provider not found for type: %s", providerType),
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": fmt.Sprintf("Provider for type %s retrieved successfully", providerType),
		"data":    provider,
	})
}

// handleSystemProvidersGetByName 获取指定类型和名称的Provider
// @Summary 获取指定类型和名称的Provider
// @Description 根据类型和名称获取Provider信息
// @Tags Provider
// @Produce json
// @Param type path string true "Provider类型"
// @Param name path string true "Provider名称"
// @Success 200 {object} interface{} "Provider信息"
// @Router /admin/system/providers/{type}/{name} [get]
func (s *DefaultAdminService) handleSystemProvidersGetByName(c *gin.Context) {
	providerType := c.Param("type")
	name := c.Param("name")

	if providerType == "" || name == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Provider type and name are required",
		})
		return
	}

	provider, err := database.GetProviderByName(providerType, name)
	if err != nil {
		c.JSON(404, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Provider not found: %s/%s", providerType, name),
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": fmt.Sprintf("Provider %s/%s retrieved successfully", providerType, name),
		"data":    provider,
	})
}

// handleSystemProvidersCreate 创建Provider【废弃，使用/user/providers/create】
// @Summary 创建Provider
// @Description 创建新的Provider
// @Tags Provider
// @Accept json
// @Produce json
// @Param data body object true "Provider创建参数"
// @Success 201 {object} map[string]interface{} "创建结果"
// @Router /user/providers/create [post]
func (s *DefaultAdminService) handleSystemProvidersCreate(c *gin.Context) {
	userID, _ := c.Get("user_id")
	user, err := database.GetUserByID(database.GetDB(), userID.(uint))
	if err != nil || user == nil {
		c.JSON(404, gin.H{
			"status":  "error",
			"message": "用户不存在",
		})
		return
	}
	createUserID := user.ID
	if user.Role == "admin" {
		createUserID = 1
	}

	var requestData struct {
		Type string      `json:"type" binding:"required"`
		Name string      `json:"name" binding:"required"`
		Data interface{} `json:"data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Invalid request data",
			"error":   err.Error(),
		})
		return
	}

	// 检查是否已存在相同名称的Provider
	existingProvider, err := database.GetProviderByName(requestData.Type, requestData.Name)
	if err == nil && existingProvider != "" {
		c.JSON(409, gin.H{
			"status": "error",
			"message": fmt.Sprintf(
				"Provider with name '%s' already exists for type '%s'",
				requestData.Name,
				requestData.Type,
			),
			"error": "duplicate_provider_name",
		})
		return
	}

	s.logger.Info("Creating new provider: type=%s, name=%s", requestData.Type, requestData.Name)

	if err := database.CreateProvider(requestData.Type, requestData.Name, requestData.Data, createUserID); err != nil {
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "Failed to create provider",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(201, gin.H{
		"status": "ok",
		"message": fmt.Sprintf(
			"Provider %s/%s created successfully",
			requestData.Type,
			requestData.Name,
		),
	})
}

// handleSystemProvidersUpdate 更新Provider【废弃，使用/user/providers/{type}/{name}】
// @Summary 更新Provider
// @Description 更新指定类型和名称的Provider
// @Tags Provider
// @Accept json
// @Produce json
// @Param type path string true "Provider类型"
// @Param name path string true "Provider名称"
// @Param data body object true "Provider更新参数"
// @Success 200 {object} map[string]interface{} "更新结果"
// @Router /admin/system/providers/{type}/{name} [put]
func (s *DefaultAdminService) handleSystemProvidersUpdate(c *gin.Context) {
	providerType := c.Param("type")
	name := c.Param("name")

	if providerType == "" || name == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Provider type and name are required",
		})
		return
	}

	var requestData struct {
		Data interface{} `json:"data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Invalid request data",
			"error":   err.Error(),
		})
		return
	}

	s.logger.Info("Updating provider: type=%s, name=%s", providerType, name)

	if err := database.UpdateProvider(providerType, name, requestData.Data, database.AdminUserID); err != nil {
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "Failed to update provider",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": fmt.Sprintf("Provider %s/%s updated successfully", providerType, name),
	})
}

// handleSystemProvidersDelete 删除Provider【废弃，使用/user/providers/{type}/{name}】
// @Summary 删除Provider
// @Description 删除指定类型和名称的Provider
// @Tags Provider
// @Produce json
// @Param type path string true "Provider类型"
// @Param name path string true "Provider名称"
// @Success 200 {object} map[string]interface{} "删除结果"
// @Router /admin/system/providers/{type}/{name} [delete]
func (s *DefaultAdminService) handleSystemProvidersDelete(c *gin.Context) {
	providerType := c.Param("type")
	name := c.Param("name")

	if providerType == "" || name == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Provider type and name are required",
		})
		return
	}

	s.logger.Info("Deleting provider: type=%s, name=%s", providerType, name)

	if err := database.DeleteProvider(providerType, name, database.AdminUserID); err != nil {
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "Failed to delete provider",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": fmt.Sprintf("Provider %s/%s deleted successfully", providerType, name),
	})
}

//=============================================================================== user providers ==============

// handleUserProvidersType 获取指定类型Provider
// @Summary 获取指定类型Provider
// @Description 根据类型获取Provider信息
// @Tags Provider
// @Produce json
// @Param type path string true "Provider类型"
// @Success 200 {object} interface{} "Provider信息"
// @Router /user/providers/{type} [get]
func (s *DefaultUserService) handleUserProvidersType(c *gin.Context) {
	userID, _ := c.Get("user_id")
	providerType := c.Param("type")
	if providerType == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Provider type is required",
		})
		return
	}
	user, err := database.GetUserByID(database.GetDB(), userID.(uint))
	if err != nil || user == nil {
		c.JSON(404, gin.H{
			"status":  "error",
			"message": "用户不存在",
		})
		return
	}

	provider, err := database.GetProviderByTypeInternal(providerType, userID.(uint), (user.Role != "admin"))
	if err != nil {
		c.JSON(404, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Provider not found for type: %s", providerType),
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": fmt.Sprintf("Provider for type %s retrieved successfully", providerType),
		"data":    provider,
	})
}

// handleUserProvidersCreate 用户创建Provider
// @Summary 用户创建Provider
// @Description 用户创建新的Provider，创建的provider为用户私有，其他人不可见
// @Tags Provider
// @Accept json
// @Produce json
// @Param data body object true "Provider创建参数"
// @Success 201 {object} map[string]interface{} "创建结果"
// @Router /user/providers/create [post]
func (s *DefaultUserService) handleUserProvidersCreate(c *gin.Context) {
	userID, _ := c.Get("user_id")
	user, err := database.GetUserByID(database.GetDB(), userID.(uint))
	if err != nil || user == nil {
		c.JSON(404, gin.H{
			"status":  "error",
			"message": "用户不存在",
		})
		return
	}
	createUserID := user.ID
	if user.Role == "admin" {
		createUserID = 1
	}

	var requestData struct {
		Type string      `json:"type" binding:"required"`
		Name string      `json:"name" binding:"required"`
		Data interface{} `json:"data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Invalid request data",
			"error":   err.Error(),
		})
		return
	}

	// 检查是否已存在相同名称的Provider
	existingProvider, err := database.GetProviderByName(requestData.Type, requestData.Name)
	if err == nil && existingProvider != "" {
		c.JSON(409, gin.H{
			"status": "error",
			"message": fmt.Sprintf(
				"Provider with name '%s' already exists for type '%s'",
				requestData.Name,
				requestData.Type,
			),
			"error": "duplicate_provider_name",
		})
		return
	}

	s.logger.Info("Creating new provider: type=%s, name=%s", requestData.Type, requestData.Name)

	if err := database.CreateProvider(requestData.Type, requestData.Name, requestData.Data, createUserID); err != nil {
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "Failed to create provider",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(201, gin.H{
		"status": "ok",
		"message": fmt.Sprintf(
			"Provider %s/%s created successfully",
			requestData.Type,
			requestData.Name,
		),
	})
}

// handleUserProvidersDelete 删除Provider
// @Summary 删除Provider
// @Description 删除指定类型和名称的Provider,仅可删除用户自己创建的
// @Tags Provider
// @Produce json
// @Param type path string true "Provider类型"
// @Param name path string true "Provider名称"
// @Success 200 {object} map[string]interface{} "删除结果"
// @Router /user/providers/{type}/{name} [delete]
func (s *DefaultUserService) handleUserProvidersDelete(c *gin.Context) {
	userID, _ := c.Get("user_id")
	user, err := database.GetUserByID(database.GetDB(), userID.(uint))
	if err != nil || user == nil {
		c.JSON(404, gin.H{
			"status":  "error",
			"message": "用户不存在",
		})
		return
	}

	providerType := c.Param("type")
	name := c.Param("name")

	if providerType == "" || name == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Provider type and name are required",
		})
		return
	}
	deleteUserID := user.ID
	if user.Role == "admin" {
		deleteUserID = database.AdminUserID
	}

	if err := database.DeleteProvider(providerType, name, deleteUserID); err != nil {
		s.logger.Error("Failed to delete provider: %s/%s, error: %v", providerType, name, err)
		// 判断错误是否包含“没有权限”
		if errors.Is(err, database.ErrNoPermission) {
			c.JSON(403, gin.H{
				"status":  "error",
				"message": "没有权限删除该Provider",
			})
			return
		}
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "Failed to delete provider",
			"error":   err.Error(),
		})
		return
	}

	s.logger.Info("Deleting provider: type=%s, name=%s, user_id=%d, userName=%s, role=%s", providerType, name, user.ID, user.Username, user.Role)
	c.JSON(200, gin.H{
		"status":  "ok",
		"message": fmt.Sprintf("Provider %s/%s deleted successfully", providerType, name),
	})
}

// handleUserProvidersUpdate 更新Provider
// @Summary 更新Provider
// @Description 更新指定类型和名称的Provider,仅可更新用户自己创建的
// @Tags Provider
// @Accept json
// @Produce json
// @Param type path string true "Provider类型"
// @Param name path string true "Provider名称"
// @Param data body object true "Provider更新参数"
// @Success 200 {object} map[string]interface{} "更新结果"
// @Router /user/providers/{type}/{name} [put]
func (s *DefaultUserService) handleUserProvidersUpdate(c *gin.Context) {
	userID, _ := c.Get("user_id")
	user, err := database.GetUserByID(database.GetDB(), userID.(uint))
	if err != nil || user == nil {
		c.JSON(404, gin.H{
			"status":  "error",
			"message": "用户不存在",
		})
		return
	}

	providerType := c.Param("type")
	name := c.Param("name")

	if providerType == "" || name == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Provider type and name are required",
		})
		return
	}

	var requestData struct {
		Data interface{} `json:"data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Invalid request data",
			"error":   err.Error(),
		})
		return
	}

	updataUserID := user.ID
	if user.Role == "admin" {
		updataUserID = 1
	}

	s.logger.Info("Updating provider: type=%s, name=%s, user_id=%d", providerType, name, user.ID)

	if err := database.UpdateProvider(providerType, name, requestData.Data, updataUserID); err != nil {
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "Failed to update provider",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "ok",
		"message": fmt.Sprintf("Provider %s/%s updated successfully", providerType, name),
	})
}
