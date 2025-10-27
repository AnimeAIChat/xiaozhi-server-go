package webapi

import (
	"net/http"
	"strconv"
	"time"
	"xiaozhi-server-go/src/configs/database"

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
	AgentID  uint   `json:"agentID"`
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
}

// handleDeviceList 设备列表
// @Summary 获取设备列表
// @Description 获取当前Agent的所有设备
// @Tags Device
// @Produce json
// @Success 200 {object} []models.Device "设备列表"
// @Router /user/device/list [get]
func (s *DefaultUserService) handleDeviceList(c *gin.Context) {
	// userID := c.GetUint("user_id")
	agentID := c.Param("id")
	id, err := strconv.Atoi(agentID)
	if err != nil {
		respondError(c, http.StatusBadRequest, "Agent ID 非法", gin.H{"error": "invalid agent id"})
		return
	}
	devices, err := database.ListDevicesByAgent(database.GetDB(), uint(id))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取设备列表失败", gin.H{"error": err.Error()})
		return
	}
	respondSuccess(c, http.StatusOK, devices, "获取设备列表成功")
}

// handleDeviceListByUser 获取当前用户的所有设备
// @Summary 获取当前用户的所有设备
// @Description 获取当前用户的所有设备
// @Tags Device
// @Produce json
// @Success 200 {object} []models.Device "设备列表"
// @Router /user/device/list [get]
func (s *DefaultUserService) handleDeviceListByUser(c *gin.Context) {
	userID := c.GetUint("user_id")
	WithTx(c, func(tx *gorm.DB) error {
		devices, err := database.ListDevicesByUser(tx, userID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "获取设备列表失败", gin.H{"error": err.Error()})
			return err
		}
		respondSuccess(c, http.StatusOK, devices, "获取设备列表成功")
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
	userID := c.GetUint("user_id")
	idStr := c.Param("id")

	device, err := database.FindDeviceByIDAndUser(database.GetDB(), idStr, userID)
	if err != nil {
		respondError(c, http.StatusNotFound, "设备不存在", gin.H{"error": "device not found"})
		return
	}
	if device.UserID == nil || *device.UserID != userID {
		respondError(c, http.StatusForbidden, "无权访问该设备", gin.H{"error": "access denied"})
		return
	}

	respondSuccess(c, http.StatusOK, device, "获取设备详情成功")
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
	userID := c.GetUint("user_id")
	idStr := c.Param("id")
	var req DeviceUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "参数校验失败", gin.H{"error": err.Error()})
		return
	}

	device, err := database.FindDeviceByIDAndUser(database.GetDB(), idStr, userID)
	if err != nil {
		respondError(c, http.StatusNotFound, "设备不存在", gin.H{"error": "device not found"})
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
		_, err = database.GetAgentByID(database.GetDB(), *req.AgentID)
		if err != nil {
			respondError(c, http.StatusNotFound, "智能体不存在", gin.H{"error": "agent not found"})
			return
		}
		device.AgentID = req.AgentID
	}
	if req.Name != nil {
		device.Name = *req.Name
	}
	if err := database.UpdateDevice(database.GetDB(), device); err != nil {
		respondError(c, http.StatusInternalServerError, "更新设备失败", gin.H{"error": err.Error()})
		return
	}
	respondSuccess(c, http.StatusOK, device, "更新设备成功")
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
	userID := c.GetUint("user_id")

	// 取body里的json数据
	var requestData struct {
		DeviceID string `json:"deviceID"`
	}
	if err := c.ShouldBindJSON(&requestData); err != nil {
		respondError(c, http.StatusBadRequest, "请求参数错误", gin.H{"error": "invalid request body"})
		return
	}
	deviceID := requestData.DeviceID
	s.logger.Info("handleDeviceDelete called with id: %s", deviceID)

	_, err := database.FindDeviceByIDAndUser(database.GetDB(), deviceID, userID)
	// 查找设备
	if err != nil {
		respondError(c, http.StatusNotFound, "设备不存在", gin.H{"error": "device not found"})
		return
	}

	err = database.DeleteDevice(database.GetDB(), deviceID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "删除设备失败", gin.H{"error": "failed to delete device"})
		return
	}

	respondSuccess(c, http.StatusOK, nil, "删除设备成功")
}

func (s *DefaultAdminService) handleDeviceDeleteAdmin(c *gin.Context) {
	// 取body里的json数据
	var requestData struct {
		DeviceID string `json:"deviceID"`
	}
	if err := c.ShouldBindJSON(&requestData); err != nil {
		respondError(c, http.StatusBadRequest, "请求参数错误", gin.H{"error": "invalid request body"})
		return
	}
	deviceID := requestData.DeviceID
	s.logger.Info("Admin handleDeviceDelete called with id: %s", deviceID)

	// 查找设备
	_, err := database.FindDeviceByID(database.GetDB(), deviceID)
	if err != nil {
		respondError(c, http.StatusNotFound, "设备不存在", gin.H{"error": "device not found"})
		return
	}
	err = database.DeleteDevice(database.GetDB(), deviceID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "删除设备失败", gin.H{"error": "failed to delete device"})
		return
	}

	respondSuccess(c, http.StatusOK, nil, "删除设备成功")
}
