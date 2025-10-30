package httptransport

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"xiaozhi-server-go/internal/domain/device/aggregate"
	"xiaozhi-server-go/internal/domain/device/service"
	"xiaozhi-server-go/internal/platform/errors"
)

// OTAHandler OTA处理器
type OTAHandler struct {
	deviceService *service.DeviceService
}

// NewOTAHandler 创建OTA处理器
func NewOTAHandler(deviceService *service.DeviceService) *OTAHandler {
	return &OTAHandler{
		deviceService: deviceService,
	}
}

// RegisterRoutes 注册OTA相关路由
func (h *OTAHandler) RegisterRoutes(router *Router) {
	// 设备注册路由
	router.API.POST("/ota/register", h.RegisterDevice)
	// 设备激活路由
	router.API.POST("/ota/activate", h.ActivateDevice)
	// 获取设备信息路由
	router.API.GET("/ota/device/:deviceId", h.GetDevice)
}

// RegisterDeviceRequest 设备注册请求
type RegisterDeviceRequest struct {
	DeviceID  string `json:"deviceId" binding:"required"`
	ClientID  string `json:"clientId" binding:"required"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	IPAddress string `json:"ipAddress"`
	AppInfo   string `json:"appInfo"`
}

// RegisterDeviceResponse 设备注册响应
type RegisterDeviceResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	DeviceID     string `json:"deviceId,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	AuthCode     string `json:"authCode,omitempty"`
	RequiresAuth bool   `json:"requiresAuth"`
}

// RegisterDevice 注册设备
func (h *OTAHandler) RegisterDevice(c *gin.Context) {
	var req RegisterDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, RegisterDeviceResponse{
			Success: false,
			Message: "Invalid request format",
		})
		return
	}

	// 获取客户端IP
	ip := req.IPAddress
	if ip == "" {
		ip = c.ClientIP()
	}

	// 注册设备
	device, _, err := h.deviceService.RegisterDevice(
		c.Request.Context(),
		req.DeviceID,
		req.ClientID,
		req.Name,
		req.Version,
		ip,
		req.AppInfo,
	)

	if err != nil {
		var kind errors.Kind
		if typedErr, ok := err.(*errors.Error); ok {
			kind = typedErr.Kind
		}

		statusCode := http.StatusInternalServerError
		if kind == errors.KindDomain {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, RegisterDeviceResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	// 返回注册结果
	response := RegisterDeviceResponse{
		Success:  true,
		Message:  "Device registered successfully",
		DeviceID: device.DeviceID,
		ClientID: device.ClientID,
	}

	// 如果需要激活码，包含在响应中
	if device.AuthCode != "" {
		response.AuthCode = device.AuthCode
		response.RequiresAuth = true
		response.Message = "Device registered, activation code required"
	} else {
		response.RequiresAuth = false
	}

	c.JSON(http.StatusOK, response)
}

// ActivateDeviceRequest 设备激活请求
type ActivateDeviceRequest struct {
	DeviceID string `json:"deviceId" binding:"required"`
	AuthCode string `json:"authCode" binding:"required"`
}

// ActivateDeviceResponse 设备激活响应
type ActivateDeviceResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	DeviceID string `json:"deviceId,omitempty"`
}

// ActivateDevice 激活设备
func (h *OTAHandler) ActivateDevice(c *gin.Context) {
	var req ActivateDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ActivateDeviceResponse{
			Success: false,
			Message: "Invalid request format",
		})
		return
	}

	// 激活设备
	err := h.deviceService.ActivateDevice(
		c.Request.Context(),
		req.DeviceID,
		req.AuthCode,
	)

	if err != nil {
		var kind errors.Kind
		if typedErr, ok := err.(*errors.Error); ok {
			kind = typedErr.Kind
		}

		statusCode := http.StatusInternalServerError
		if kind == errors.KindDomain {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, ActivateDeviceResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ActivateDeviceResponse{
		Success:  true,
		Message:  "Device activated successfully",
		DeviceID: req.DeviceID,
	})
}

// GetDeviceResponse 获取设备信息响应
type GetDeviceResponse struct {
	Success     bool                `json:"success"`
	Message     string              `json:"message"`
	Device      *DeviceInfo         `json:"device,omitempty"`
}

// DeviceInfo 设备信息
type DeviceInfo struct {
	ID             int                    `json:"id"`
	UserID         *int                   `json:"userId,omitempty"`
	AgentID        *int                   `json:"agentId,omitempty"`
	Name           string                 `json:"name"`
	DeviceID       string                 `json:"deviceId"`
	ClientID       string                 `json:"clientId"`
	Version        string                 `json:"version"`
	RegisterTime   int64                  `json:"registerTime"`
	LastActiveTime int64                  `json:"lastActiveTime"`
	Online         bool                   `json:"online"`
	AuthStatus     aggregate.DeviceStatus `json:"authStatus"`
	BoardType      string                 `json:"boardType"`
	ChipModelName  string                 `json:"chipModelName"`
	Channel        int                    `json:"channel"`
	SSID           string                 `json:"ssid"`
	Application    string                 `json:"application"`
	Language       string                 `json:"language"`
	DeviceCode     string                 `json:"deviceCode"`
	LastIP         string                 `json:"lastIp"`
	Stats          string                 `json:"stats"`
	TotalTokens    int64                  `json:"totalTokens"`
	UsedTokens     int64                  `json:"usedTokens"`
	ConversationID string                 `json:"conversationId"`
	Mode           string                 `json:"mode"`
}

// GetDevice 获取设备信息
func (h *OTAHandler) GetDevice(c *gin.Context) {
	deviceID := c.Param("deviceId")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, GetDeviceResponse{
			Success: false,
			Message: "Device ID is required",
		})
		return
	}

	// 获取设备信息
	device, err := h.deviceService.GetDevice(c.Request.Context(), deviceID)
	if err != nil {
		var kind errors.Kind
		if typedErr, ok := err.(*errors.Error); ok {
			kind = typedErr.Kind
		}

		statusCode := http.StatusInternalServerError
		if kind == errors.KindDomain {
			statusCode = http.StatusNotFound
		}

		c.JSON(statusCode, GetDeviceResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	if device == nil {
		c.JSON(http.StatusNotFound, GetDeviceResponse{
			Success: false,
			Message: "Device not found",
		})
		return
	}

	// 转换为响应格式
	deviceInfo := &DeviceInfo{
		ID:             device.ID,
		UserID:         device.UserID,
		AgentID:        device.AgentID,
		Name:           device.Name,
		DeviceID:       device.DeviceID,
		ClientID:       device.ClientID,
		Version:        device.Version,
		RegisterTime:   device.RegisterTime.Unix(),
		LastActiveTime: device.LastActiveTime.Unix(),
		Online:         device.Online,
		AuthStatus:     device.AuthStatus,
		BoardType:      device.BoardType,
		ChipModelName:  device.ChipModelName,
		Channel:        device.Channel,
		SSID:           device.SSID,
		Application:    device.Application,
		Language:       device.Language,
		DeviceCode:     device.DeviceCode,
		LastIP:         device.LastIP,
		Stats:          device.Stats,
		TotalTokens:    device.TotalTokens,
		UsedTokens:     device.UsedTokens,
		ConversationID: device.ConversationID,
		Mode:           device.Mode,
	}

	c.JSON(http.StatusOK, GetDeviceResponse{
		Success: true,
		Message: "Device found",
		Device:  deviceInfo,
	})
}