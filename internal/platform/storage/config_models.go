package storage

import (
	"time"

	"gorm.io/datatypes"
)

// ConfigRecord 完整的配置记录模型，用于数据库存储
type ConfigRecord struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Key         string         `gorm:"uniqueIndex;not null" json:"key"` // 配置键，如 "server", "web", "llm.openai"
	Value       datatypes.JSON `gorm:"not null" json:"value"`           // 配置值，JSON格式
	Description string         `gorm:"type:text" json:"description"`     // 配置描述
	Category    string         `gorm:"index" json:"category"`           // 配置分类，如 "server", "web", "llm", "tts", "asr"
	Version     int            `gorm:"default:1" json:"version"`         // 配置版本号，用于版本控制
	IsActive    bool           `gorm:"default:true" json:"is_active"`    // 是否为活动配置
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// TableName 指定表名
func (ConfigRecord) TableName() string {
	return "config_records"
}

// ConfigSnapshot 配置快照，用于备份和版本控制
type ConfigSnapshot struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"not null" json:"name"`      // 快照名称
	Version   int            `gorm:"not null" json:"version"`   // 快照版本
	Data      datatypes.JSON `gorm:"not null" json:"data"`      // 完整配置数据
	Comment   string         `gorm:"type:text" json:"comment"`  // 快照注释
	CreatedAt time.Time      `json:"created_at"`
}

// TableName 指定表名
func (ConfigSnapshot) TableName() string {
	return "config_snapshots"
}

// ModelSelection 模型选择记录，用于管理用户选择的AI模型提供商
type ModelSelection struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        string    `gorm:"not null;default:'admin';uniqueIndex" json:"user_id"` // 用户ID，默认为admin管理员
	ASRProvider   string    `gorm:"not null" json:"asr_provider"`                        // 选择的ASR提供商
	TTSProvider   string    `gorm:"not null" json:"tts_provider"`                        // 选择的TTS提供商
	LLMProvider   string    `gorm:"not null" json:"llm_provider"`                        // 选择的LLM提供商
	VLLMProvider  string    `gorm:"not null" json:"vllm_provider"`                       // 选择的VLLM提供商
	IsActive      bool      `gorm:"default:true" json:"is_active"`                        // 是否为活动选择
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName 指定表名
func (ModelSelection) TableName() string {
	return "model_selections"
}