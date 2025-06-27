package models

import (
	//"gorm.io/gorm"
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

// 用户
type User struct {
	ID       uint   `gorm:"primaryKey"`
	Username string `gorm:"uniqueIndex;not null"`
	Password string // 建议加密
	Role     string // 可选值：admin/user
	Setting  UserSetting
}

// 用户设置
type UserSetting struct {
	ID              uint `gorm:"primaryKey"`
	UserID          uint `gorm:"uniqueIndex"` // 一对一
	SelectedASR     string
	SelectedTTS     string
	SelectedLLM     string
	SelectedVLLLM   string
	PromptOverride  string `gorm:"type:text"`
	QuickReplyWords datatypes.JSON
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
