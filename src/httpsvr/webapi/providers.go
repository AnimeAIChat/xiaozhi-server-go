package webapi

import (
	"errors"
	"fmt"
	"xiaozhi-server-go/src/configs/database"

	"github.com/gin-gonic/gin"
)

// handleUserProvidersType 获取指定类型Provider
// @Summary 获取指定类型Provider
// @Description 根据类型获取Provider信息
// @Tags Provider
// @Produce json
// @Param type path string true "Provider类型"
// @Success 200 {object} interface{} "Provider信息"
// @Router /user/providers/{type} [get]
func (s *DefaultUserService) handleUserProvidersType(c *gin.Context) {

	providerType := c.Param("type")
	if providerType == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Provider type is required",
		})
		return
	}

	provider, err := database.GetProviderByType(providerType)
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

	if err := database.CreateProvider(requestData.Type, requestData.Name, requestData.Data); err != nil {
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

	providerType := c.Param("type")
	name := c.Param("name")

	if providerType == "" || name == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Provider type and name are required",
		})
		return
	}

	if err := database.DeleteProvider(providerType, name); err != nil {
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

	s.logger.Info("Deleting provider: type=%s, name=%s", providerType, name)
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

	if err := database.UpdateProvider(providerType, name, requestData.Data); err != nil {
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
