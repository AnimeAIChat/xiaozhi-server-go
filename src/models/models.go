package models

import (
	//"gorm.io/gorm"
	"time"

	"gorm.io/gorm"
)

// 智能体结构：智能体属于某个用户，拥有多个设备
type Agent struct {
	ID                 uint      `gorm:"primaryKey"           json:"id"`
	Name               string    `gorm:"not null"             json:"name"` // 智能体名称
	LLM                string    `gorm:"default:'ChatGLMLLM'" json:"LLM"`
	Language           string    `gorm:"default:'普通话'"        json:"language"`                            // 语言，默认为中文
	Voice              string    `gorm:"default:'zh_female_wanwanxiaohe_moon_bigtts'"       json:"voice"` // 语音，默认为zh_female_wanwanxiaohe_moon_bigtts
	VoiceName          string    `gorm:"default:'湾湾小何'"       json:"voiceName"`                           // 语音，默认为湾湾小何
	Prompt             string    `gorm:"type:text"            json:"prompt"`
	ASRSpeed           int       `gorm:"default:2"            json:"asrSpeed"`   // ASR 语音识别速度，1=耐心，2=正常，3=快速
	SpeakSpeed         int       `gorm:"default:2"            json:"speakSpeed"` // TTS 角色语速，1=慢速，2=正常，3=快速
	Tone               int       `gorm:"default:50"           json:"tone"`       // TTS 角色音调，1-100，低音-高音
	UserID             uint      `gorm:"not null"             json:"-"`
	CreatedAt          time.Time `                            json:"createdAt"`          // 创建时间
	UpdatedAt          time.Time `                            json:"updatedAt"`          // 更新时间
	LastConversationAt time.Time `                            json:"lastConversationAt"` // 最后对话时间
	Devices            []Device  `gorm:"foreignKey:AgentID"   json:"-"`                  // 关联设备
	EnabledTools       string    `gorm:"type:text"            json:"enabledTools"`       // 启用的工具列表，字符串格式，如 "tool1,tool2"
	Conversationid     string    `                            json:"conversationId"`     // 关联的对话AgentDialog的ID
	HeadImg            string    `gorm:"type:varchar(255)"    json:"head_img"`           // 头像URL
	Description        string    `gorm:"type:text"            json:"description"`        // 智能体描述
	CatalogyID         uint      `                            json:"catalogy_id"`        // 分类ID
	Extra              string    `gorm:"type:text"            json:"extra"`              // 额外信息，JSON格式
}
type AgentDialog struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Conversationid string    `                  json:"conversationId"`
	AgentID        uint      `gorm:"index"      json:"agentID"`          // 外键关联 Agent
	UserID         uint      `gorm:"index"      json:"userID"`           // 外键关联 User
	Dialog         string    `gorm:"type:text"  json:"dialog,omitempty"` // 对话内容
	CreatedAt      time.Time `                  json:"createdAt"`        // 创建时间
	UpdatedAt      time.Time `                  json:"updatedAt"`        // 更新
}

type Device struct {
	ID               uint           `gorm:"primaryKey"                             json:"id"`
	AgentID          *uint          `gorm:"index"                                  json:"agentID"`          // 外键关联 Agent
	UserID           *uint          `gorm:"index"                                  json:"userID"`           // 外键关联 User
	Name             string         `gorm:"not null"                               json:"name"`             // 设备名称
	DeviceID         string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"deviceId"`         // 设备唯一标识,mac地址
	ClientID         string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"clientId"`         // 客户端唯一标识
	Version          string         `                                              json:"version"`          // 设备固件版本号
	OTA              bool           `gorm:"default:true"                           json:"ota"`              // 是否支持OTA升级
	RegisterTime     int64          `                                              json:"-"`                // 注册时间戳
	LastActiveTime   int64          `                                              json:"-"`                // 最后活跃时间戳
	RegisterTimeV2   time.Time      `                                              json:"registerTimeV2"`   // 注册时间
	LastActiveTimeV2 time.Time      `                                              json:"lastActiveTimeV2"` // 最后活跃时间
	Online           bool           `                                              json:"online"`           // 在线状态
	AuthCode         string         `                                              json:"authCode"`         // 认证码
	AuthStatus       string         `                                              json:"authStatus"`       // 认证状态，可选值：pending
	BoardType        string         `                                              json:"boardType"`        // 主板类型，可能为 lichuang-dev/atk-dnesp32s3-box
	ChipModelName    string         `                                              json:"chipModelName"`    // 芯片型号名称，默认为 esp32s3
	Channel          int            `                                              json:"channel"`          // WiFi 频道
	SSID             string         `                                              json:"ssid"`             // WiFi SSID
	Application      string         `                                              json:"application"`      // 应用名称
	Language         string         `gorm:"default:'zh-CN'"                        json:"language"`         // 语言，默认为中文
	DeviceCode       string         `                                              json:"deviceCode"`       // 设备码，简化版的设备唯一标识
	DeletedAt        gorm.DeletedAt `gorm:"index"                                  json:"-"`                // 软删除字段
	Extra            string         `gorm:"type:text"                              json:"extra"`            // 额外信息，JSON格式
	Conversationid   string         `                                              json:"conversationId"`   // 关联的对话AgentDialog的ID
	Mode             string         `                                              json:"mode"`             // 模式:chat/listen/ban
}

// 用户
type User struct {
	ID          uint      `gorm:"primaryKey"                             json:"id"`
	Username    string    `gorm:"type:varchar(255);uniqueIndex;not null" json:"username"`
	Password    string    `                                              json:"-"`        // 密码不下发
	Nickname    string    `gorm:"type:varchar(255)"                      json:"nickname"` // 昵称
	HeadImg     string    `gorm:"type:varchar(255)"                      json:"head_img"` // 头像URL
	Role        string    `                                              json:"role"`     // 可选值：admin/observer/user
	CreatedAt   time.Time `                                              json:"createdAt"`
	UpdatedAt   time.Time `                                              json:"updatedAt"`
	Email       string    `gorm:"type:varchar(255);uniqueIndex;"         json:"email"`  // 用户邮箱
	Status      uint      `gorm:"default:1"                              json:"status"` // 用户状态，1=正常，0=禁用
	PhoneNumber string    `gorm:"type:varchar(20);" json:"phoneNumber"`                 // 手机号码
	Extra       string    `gorm:"type:text"                              json:"extra"`  // 额外信息，JSON格式
}

type ServerConfig struct {
	ID     uint   `gorm:"primaryKey"`
	CfgStr string `gorm:"type:text"` // 服务器的配置内容，从config.yaml转换而来
}

type ServerStatus struct {
	ID               uint      `gorm:"primaryKey"`
	OnlineDeviceNum  int       `json:"onlineDeviceNum"`  // 实时在线设备数量，保持mqtt连接，即使不在对话也属于在线
	OnlineSessionNum int       `json:"onlineSessionNum"` // 正在对话的设备数量，包括mqtt和websocket
	CPUUsage         string    `json:"cpuUsage"`         // CPU使用率
	MemoryUsage      string    `json:"memoryUsage"`      // 内存使用率
	UpdatedAt        time.Time `json:"updatedAt"`
}
