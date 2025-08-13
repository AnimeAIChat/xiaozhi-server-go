package models

import "gorm.io/datatypes"

type ASRConfig struct {
	ID   uint           `gorm:"primaryKey" json:"-"`
	Name string         `                  json:"name"`
	Type string         `                  json:"type"` // ASR类型
	Data datatypes.JSON `                  json:"data"` // 其他配置数据
}

type LLMConfig struct {
	ID   uint           `gorm:"primaryKey" json:"-"`
	Name string         `                  json:"name"`
	Type string         `                  json:"type"`
	Data datatypes.JSON `                  json:"data"` // 其他配置数据
}

type TTSConfig struct {
	ID   uint           `gorm:"primaryKey" json:"-"`
	Name string         `                  json:"name"`
	Type string         `                  json:"type"`
	Data datatypes.JSON `                  json:"data"` // 其他配置数据
}

type VLLLMConfig struct {
	ID   uint           `gorm:"primaryKey" json:"-"`
	Name string         `                  json:"name"` // 提供者名称
	Type string         `                  json:"type"`
	Data datatypes.JSON `                  json:"data"` // 其他配置数据
}
