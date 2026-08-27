package webapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/chat"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DeviceBindRequest 设备绑定请求体
// @Description 绑定设备到指定Agent
// @Tags Device
// @Accept json
// @Produce json
// @Param data body DeviceBindRequest true "设备绑定参数"
// @Success 200 {object} models.Device "绑定成功返回设备信息"
// @Router /user/device/bind [post]
type DeviceBindRequest struct {
	AgentID  uint   `json:"agentID" binding:"required"`
	DeviceID string `json:"deviceID"` // 保留兼容旧客户端的设备唯一标识
	MAC      string `json:"mac"`      // 推荐使用设备物理 MAC 地址绑定
	AuthCode string `json:"authCode"`
}

// DeviceUpdateRequest 设备更新请求体
// @Description 更新设备信息
// @Tags Device
// @Accept json
// @Produce json
// @Param data body DeviceUpdateRequest true "设备更新参数"
// @Success 200 {object} models.Device "更新后的设备信息"
// @Router /user/device/{id} [put]
type DeviceUpdateRequest struct {
	Online         *bool      `json:"online,omitempty"`
	AuthStatus     string     `json:"authStatus,omitempty"`
	LastActiveTime *time.Time `json:"lastActiveTime,omitempty"`
	AgentID        *uint      `json:"agent_id,omitempty"`
	Name           *string    `json:"name,omitempty"`
	Disabled       *bool      `json:"disabled,omitempty"`
}

// handleDeviceList 设备列表
// @Summary 获取设备列表
// @Description 获取当前Agent的所有设备
// @Tags Device
// @Produce json
// @Success 200 {object} []models.Device "设备列表"
// @Router /user/device/list [get]
func (s *DefaultUserService) handleDeviceList(c *gin.Context) {
	userID := currentUserID(c)
	agentID := c.Param("id")
	id, err := strconv.Atoi(agentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent id"})
		return
	}
	if _, err := database.GetAgentByIDAndUser(database.GetDB(), uint(id), userID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	devices, err := database.ListDevicesByAgentAndUser(database.GetDB(), uint(id), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "data": devices})
}

func (s *DefaultUserService) handleDeviceBind(c *gin.Context) {
	userID := currentUserID(c)
	var request DeviceBindRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bind request"})
		return
	}
	request.DeviceID = strings.TrimSpace(request.DeviceID)
	request.MAC = strings.TrimSpace(request.MAC)
	if (request.DeviceID == "" && request.MAC == "") || request.AgentID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mac or deviceID and agentID are required"})
		return
	}
	if request.MAC != "" && database.NormalizeMACAddress(request.MAC) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mac address"})
		return
	}

	var boundDevice interface{}
	err := WithTxNoContext(func(tx *gorm.DB) error {
		if _, err := database.GetAgentByIDAndUser(tx, request.AgentID, userID); err != nil {
			return err
		}
		var deviceErr error
		var deviceID = request.DeviceID
		if request.MAC != "" {
			deviceID = request.MAC
		}
		device, err := database.FindDeviceByID(tx, deviceID)
		if request.MAC != "" {
			device, deviceErr = database.FindDeviceByMAC(tx, request.MAC)
		} else {
			deviceErr = err
		}
		if deviceErr != nil {
			return deviceErr
		}
		device.UserID = &userID
		device.AgentID = &request.AgentID
		device.AuthStatus = "bound"
		if err := database.UpdateDevice(tx, device); err != nil {
			return err
		}
		boundDevice = device
		return nil
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device or agent not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "data": boundDevice})
}

// handleDeviceListByUser 获取当前用户的所有设备
// @Summary 获取当前用户的所有设备
// @Description 获取当前用户的所有设备
// @Tags Device
// @Produce json
// @Success 200 {object} []models.Device "设备列表"
// @Router /user/device/list [get]
func (s *DefaultUserService) handleDeviceListByUser(c *gin.Context) {
	userID := currentUserID(c)
	WithTx(c, func(tx *gorm.DB) error {
		devices, err := database.ListDevicesByUser(tx, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return err
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "data": devices})
		return nil
	})
}

// handleDeviceGet 设备详情
// @Summary 获取设备详情
// @Description 获取指定ID的设备信息
// @Tags Device
// @Produce json
// @Param id path int true "设备ID"
// @Success 200 {object} models.Device "设备信息"
// @Router /user/device/{id} [get]
func (s *DefaultUserService) handleDeviceGet(c *gin.Context) {
	userID := currentUserID(c)
	idStr := c.Param("id")

	device, err := database.FindDeviceByIDAndUser(database.GetDB(), idStr, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if device.UserID == nil || *device.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "data": device})
}

// handleDeviceMemoryClear clears the short-lived in-memory conversation context.
func (s *DefaultUserService) handleDeviceMemoryClear(c *gin.Context) {
	userID := currentUserID(c)
	deviceID := strings.TrimSpace(c.Param("id"))
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device id is required"})
		return
	}
	device, err := database.FindDeviceByIDAndUser(database.GetDB(), deviceID, userID)
	if err != nil || device.AgentID == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found or not bound"})
		return
	}
	chat.ClearDeviceAgentShortTermMemory(deviceID, *device.AgentID)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleDeviceUpdate 设备更新
// @Summary 更新设备信息
// @Description 更新指定ID的设备信息
// @Tags Device
// @Accept json
// @Produce json
// @Param id path int true "设备ID"
// @Param data body DeviceUpdateRequest true "设备更新参数"
// @Success 200 {object} models.Device "更新后的设备信息"
// @Router /user/device/{id} [put]
func (s *DefaultUserService) handleDeviceUpdate(c *gin.Context) {
	userID := currentUserID(c)
	idStr := c.Param("id")
	var req DeviceUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	device, err := database.FindDeviceByIDAndUser(database.GetDB(), idStr, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if req.Online != nil {
		device.Online = *req.Online
	}
	if req.AuthStatus != "" {
		device.AuthStatus = req.AuthStatus
	}
	if req.LastActiveTime != nil {
		device.LastActiveTimeV2 = *req.LastActiveTime
	}
	if req.AgentID != nil {
		_, err = database.GetAgentByIDAndUser(database.GetDB(), *req.AgentID, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "agent not found"})
			return
		}
		device.AgentID = req.AgentID
	}
	if req.Name != nil {
		device.Name = *req.Name
	}
	if req.Disabled != nil {
		if *req.Disabled {
			device.Mode = "ban"
		} else {
			device.Mode = ""
		}
	}
	if err := database.UpdateDevice(database.GetDB(), device); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "data": device})
	return
}

// handleDeviceDelete 设备删除
// @Summary 删除设备
// @Description 删除指定设备
// @Tags Device
// @Accept json
// @Produce json
// @Param data body object true "设备删除参数（deviceID）"
// @Success 200 {object} map[string]interface{} "删除结果"
// @Router /user/device [delete]
func (s *DefaultUserService) handleDeviceDelete(c *gin.Context) {
	userID := currentUserID(c)

	// 取body里的json数据
	var requestData struct {
		DeviceID string `json:"deviceID"`
	}
	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	deviceID := requestData.DeviceID
	s.logger.Info("handleDeviceDelete called with id: %s", deviceID)

	device, err := database.FindDeviceByIDAndUser(database.GetDB(), deviceID, userID)
	// 查找设备
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	device.AgentID = nil
	ownerID := database.AdminUserID
	device.UserID = &ownerID
	device.AuthStatus = "pending"
	err = database.UpdateDevice(database.GetDB(), device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *DefaultAdminService) handleDeviceDeleteAdmin(c *gin.Context) {
	// 取body里的json数据
	var requestData struct {
		DeviceID string `json:"deviceID"`
	}
	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	deviceID := requestData.DeviceID
	s.logger.Info("Admin handleDeviceDelete called with id: %s", deviceID)

	// 查找设备
	_, err := database.FindDeviceByID(database.GetDB(), deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	err = database.DeleteDevice(database.GetDB(), deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
