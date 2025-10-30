package webapi

import (
	"net/http"
	// "xiaozhi-server-go/src/configs/database" // TODO: Remove database dependency

	"github.com/gin-gonic/gin"
)

// handleSystemProvidersGet 获取所有Provider
func (s *DefaultAdminService) handleSystemProvidersGet(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

// handleSystemProvidersGetByType 根据类型获取Provider
func (s *DefaultAdminService) handleSystemProvidersGetByType(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

// handleSystemProvidersGetByName 根据名称获取Provider
func (s *DefaultAdminService) handleSystemProvidersGetByName(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

// handleSystemProvidersCreate 创建Provider
func (s *DefaultAdminService) handleSystemProvidersCreate(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

// handleSystemProvidersUpdate 更新Provider
func (s *DefaultAdminService) handleSystemProvidersUpdate(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

// handleSystemProvidersDelete 删除Provider
func (s *DefaultAdminService) handleSystemProvidersDelete(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}
