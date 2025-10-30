package aggregate

import (
	"time"

	"xiaozhi-server-go/internal/platform/errors"
)

// DeviceStatus 设备认证状态
type DeviceStatus string

const (
	DeviceStatusPending  DeviceStatus = "pending"  // 待认证
	DeviceStatusApproved DeviceStatus = "approved" // 已认证
	DeviceStatusRejected DeviceStatus = "rejected" // 已拒绝
)

// Device 设备聚合根
type Device struct {
	ID               int          `json:"id"`
	UserID           *int         `json:"userId"`
	AgentID          *int         `json:"agentId"`
	Name             string       `json:"name"`
	DeviceID         string       `json:"deviceId"`         // 设备唯一标识
	ClientID         string       `json:"clientId"`         // 客户端唯一标识
	Version          string       `json:"version"`          // 设备固件版本
	OTA              bool         `json:"ota"`              // 是否支持OTA
	RegisterTime     time.Time    `json:"registerTime"`     // 注册时间
	LastActiveTime   time.Time    `json:"lastActiveTime"`   // 最后活跃时间
	Online           bool         `json:"online"`           // 在线状态
	AuthCode         string       `json:"authCode"`         // 认证码
	AuthStatus       DeviceStatus `json:"authStatus"`       // 认证状态
	BoardType        string       `json:"boardType"`        // 主板类型
	ChipModelName    string       `json:"chipModelName"`    // 芯片型号
	Channel          int          `json:"channel"`          // WiFi频道
	SSID             string       `json:"ssid"`             // WiFi SSID
	Application      string       `json:"application"`      // 应用信息JSON
	Language         string       `json:"language"`         // 语言
	DeviceCode       string       `json:"deviceCode"`       // 设备码
	LastIP           string       `json:"lastIp"`           // 最后连接IP
	Stats            string       `json:"stats"`            // 统计信息JSON
	TotalTokens      int64        `json:"totalTokens"`      // 总token数
	UsedTokens       int64        `json:"usedTokens"`       // 已使用的token数
	LastSessionEndAt *time.Time   `json:"lastSessionEndAt"` // 上次对话结束时间
	Extra            string       `json:"extra"`            // 额外信息JSON
	ConversationID   string       `json:"conversationId"`   // 对话ID
	Mode             string       `json:"mode"`             // 模式
}

// NewDevice 创建新设备
func NewDevice(deviceID, clientID, name, version string) (*Device, error) {
	if deviceID == "" {
		return nil, errors.New(errors.KindDomain, "device.new", "device ID cannot be empty")
	}

	now := time.Now()
	return &Device{
		DeviceID:       deviceID,
		ClientID:       clientID,
		Name:           name,
		Version:        version,
		RegisterTime:   now,
		LastActiveTime: now,
		AuthStatus:     DeviceStatusPending,
		OTA:            true,
		Language:       "zh-CN",
	}, nil
}

// SetAuthCode 设置认证码
func (d *Device) SetAuthCode(code string) {
	d.AuthCode = code
}

// Approve 批准设备认证
func (d *Device) Approve(userID int) error {
	if d.AuthStatus != DeviceStatusPending {
		return errors.New(errors.KindDomain, "device.approve", "device is not in pending status")
	}

	d.UserID = &userID
	d.AuthStatus = DeviceStatusApproved
	d.AuthCode = "" // 清除认证码
	return nil
}

// UpdateActivity 更新设备活动信息
func (d *Device) UpdateActivity(ip string, appInfo string) {
	d.LastActiveTime = time.Now()
	if ip != "" {
		d.LastIP = ip
	}
	if appInfo != "" {
		d.Application = appInfo
	}
}

// SetLastSessionEnd 设置最后会话结束时间
func (d *Device) SetLastSessionEnd(endTime time.Time) {
	d.LastSessionEndAt = &endTime
}

// IsActivated 检查设备是否已激活
func (d *Device) IsActivated() bool {
	return d.AuthStatus == DeviceStatusApproved
}

// RequiresActivation 检查设备是否需要激活
func (d *Device) RequiresActivation() bool {
	return d.AuthStatus == DeviceStatusPending && d.AuthCode != ""
}