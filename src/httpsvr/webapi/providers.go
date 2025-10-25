package webapi

import (
	"errors"
	"fmt"
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
