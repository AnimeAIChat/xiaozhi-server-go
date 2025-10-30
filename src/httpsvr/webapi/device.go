package webapi

import (
	"net/http"
	// "strconv"
	// "time"
	// "xiaozhi-server-go/src/configs/database" // TODO: Remove database dependency
	models "xiaozhi-server-go/src/models"

	"github.com/gin-gonic/gin"
)

type DeviceDoc = models.Device

// DeviceBindRequest 设备绑定请求体
type DeviceBindRequest struct {
	DeviceID string `json:"device_id" binding:"required"`
	Name     string `json:"name" binding:"required"`
}

// DeviceUpdateRequest 设备更新请求体
type DeviceUpdateRequest struct {
	Name    *string `json:"name,omitempty"`
	AgentID *uint   `json:"agent_id,omitempty"`
}

// handleDeviceList 获取指定Agent的所有设备
func (s *DefaultUserService) handleDeviceList(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

// handleDeviceListByUser 获取当前用户的所有设备
func (s *DefaultUserService) handleDeviceListByUser(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

// handleDeviceGet 获取单个设备
func (s *DefaultUserService) handleDeviceGet(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

// handleDeviceUpdate 更新设备
func (s *DefaultUserService) handleDeviceUpdate(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

// handleDeviceDelete 删除设备
func (s *DefaultUserService) handleDeviceDelete(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

// handleDeviceDeleteAdmin 管理员删除设备
func (s *DefaultAdminService) handleDeviceDeleteAdmin(c *gin.Context) {
respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}
