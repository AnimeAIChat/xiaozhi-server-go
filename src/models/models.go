package models

import (
	//"gorm.io/gorm"
	"time"

	"gorm.io/datatypes"
)

// 系统全局配置（只保存一条记录）
type SystemConfig struct {
	ID               uint `gorm:"primaryKey"`
	SelectedASR      string
	SelectedTTS      string
	SelectedLLM      string
	SelectedVLLLM    string
	Prompt           string         `gorm:"type:text"`
	QuickReplyWords  datatypes.JSON // 存储为 JSON 数组
	DeleteAudio      bool
	UsePrivateConfig bool
}

// 智能体结构：智能体属于某个用户，拥有多个设备
type Agent struct {
	ID             uint      `gorm:"primaryKey"           json:"id"`
	Name           string    `gorm:"not null"             json:"name"` // 智能体名称
	LLM            string    `gorm:"default:'ChatGLMLLM'" json:"LLM"`
	Language       string    `gorm:"default:'普通话'"        json:"language"` // 语言，默认为中文
	Voice          string    `gorm:"default:'湾湾小何'"       json:"voice"`    // 语音，默认为湾湾小何
	Prompt         string    `gorm:"type:text"            json:"prompt"`
	ASRSpeed       int       `gorm:"default:2"            json:"asrSpeed"`       // ASR 语音识别速度，1=耐心，2=正常，3=快速
	SpeakSpeed     int       `gorm:"default:2"            json:"speakSpeed"`     // TTS 角色语速，1=慢速，2=正常，3=快速
	Tone           int       `gorm:"default:50"           json:"tone"`           // TTS 角色音调，1-100，低音-高音
	CreatedAt      time.Time `                            json:"createdAt"`      // 创建时间
	UpdatedAt      time.Time `                            json:"updatedAt"`      // 更新时间
	EnabledTools   string    `gorm:"type:text"            json:"enabledTools"`   // 启用的工具列表，字符串格式，如 "tool1,tool2"
	Conversationid string    `                            json:"conversationId"` // 关联的对话AgentDialog的ID
	HeadImg        string    `gorm:"type:varchar(255)"    json:"head_img"`       // 头像URL
	Description    string    `gorm:"type:text"            json:"description"`    // 智能体描述
	CatalogyID     uint      `                            json:"catalogy_id"`    // 分类ID
	Extra          string    `gorm:"type:text"            json:"extra"`          // 额外信息，JSON格式
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

// 用户
type User struct {
	ID       uint   `gorm:"primaryKey"`
	Username string `gorm:"uniqueIndex;not null"`
	Password string // 建议加密
	Role     string // 可选值：admin/user
}

// 模块配置（可选）
type ModuleConfig struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"uniqueIndex;not null"` // 模块名
	Type        string
	ConfigJSON  datatypes.JSON
	Public      bool
	Description string
	Enabled     bool
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
