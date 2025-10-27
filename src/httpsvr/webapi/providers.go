package webapi

import (
	"fmt"
	"net/http"
	"xiaozhi-server-go/src/configs/database"

	"github.com/gin-gonic/gin"
)

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
		respondError(c, http.StatusNotFound, "No providers found", nil)
		return
	}

	respondSuccess(c, http.StatusOK, providers, "Providers retrieved successfully")
}

// handleSystemProvidersType 获取指定类型Provider
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
		respondError(c, http.StatusBadRequest, "Provider type is required", nil)
		return
	}

	provider, err := database.GetProviderByType(providerType, database.AdminUserID)
	if err != nil {
		respondError(
			c,
			http.StatusNotFound,
			fmt.Sprintf("Provider not found for type: %s", providerType),
			gin.H{"error": err.Error()},
		)
		return
	}

	respondSuccess(
		c,
		http.StatusOK,
		provider,
		fmt.Sprintf("Provider for type %s retrieved successfully", providerType),
	)
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
		respondError(c, http.StatusBadRequest, "Provider type and name are required", nil)
		return
	}

	provider, err := database.GetProviderByName(providerType, name)
	if err != nil {
		respondError(
			c,
			http.StatusNotFound,
			fmt.Sprintf("Provider not found: %s/%s", providerType, name),
			gin.H{"error": err.Error()},
		)
		return
	}

	respondSuccess(
		c,
		http.StatusOK,
		provider,
		fmt.Sprintf("Provider %s/%s retrieved successfully", providerType, name),
	)
}

// handleSystemProvidersCreate 创建Provider
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
		respondError(c, http.StatusNotFound, "用户不存在", nil)
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
		respondError(c, http.StatusBadRequest, "Invalid request data", gin.H{"error": err.Error()})
		return
	}

	// 检查是否已存在相同名称的Provider
	existingProvider, err := database.GetProviderByName(requestData.Type, requestData.Name)
	if err == nil && existingProvider != "" {
		respondError(
			c,
			http.StatusConflict,
			fmt.Sprintf(
				"Provider with name '%s' already exists for type '%s'",
				requestData.Name,
				requestData.Type,
			),
			gin.H{"error": "duplicate_provider_name"},
		)
		return
	}

	s.logger.Info("Creating new provider: type=%s, name=%s", requestData.Type, requestData.Name)

	if err := database.CreateProvider(requestData.Type, requestData.Name, requestData.Data, createUserID); err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to create provider", gin.H{"error": err.Error()})
		return
	}

	respondSuccess(
		c,
		http.StatusCreated,
		gin.H{
			"type": requestData.Type,
			"name": requestData.Name,
		},
		fmt.Sprintf(
			"Provider %s/%s created successfully",
			requestData.Type,
			requestData.Name,
		),
	)
}

// handleSystemProvidersUpdate 更新Provider
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
		respondError(c, http.StatusBadRequest, "Provider type and name are required", nil)
		return
	}

	var requestData struct {
		Data interface{} `json:"data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request data", gin.H{"error": err.Error()})
		return
	}

	s.logger.Info("Updating provider: type=%s, name=%s", providerType, name)

	if err := database.UpdateProvider(providerType, name, requestData.Data, database.AdminUserID); err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to update provider", gin.H{"error": err.Error()})
		return
	}

	respondSuccess(
		c,
		http.StatusOK,
		gin.H{"type": providerType, "name": name},
		fmt.Sprintf("Provider %s/%s updated successfully", providerType, name),
	)
}

// handleSystemProvidersDelete 删除Provider
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
		respondError(c, http.StatusBadRequest, "Provider type and name are required", nil)
		return
	}

	s.logger.Info("Deleting provider: type=%s, name=%s", providerType, name)

	if err := database.DeleteProvider(providerType, name, database.AdminUserID); err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to delete provider", gin.H{"error": err.Error()})
		return
	}

	respondSuccess(
		c,
		http.StatusOK,
		gin.H{"type": providerType, "name": name},
		fmt.Sprintf("Provider %s/%s deleted successfully", providerType, name),
	)
}
