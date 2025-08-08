package database

import (
	"fmt"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

func InitSystemConfig(db *gorm.DB, config *configs.Config) error {
	var count int64
	if err := db.Model(&models.SystemConfig{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaultConfig := models.SystemConfig{
		ID:               SystemConfigID,
		SelectedASR:      config.SelectedModule["ASR"],
		SelectedTTS:      config.SelectedModule["TTS"],
		SelectedLLM:      config.SelectedModule["LLM"],
		SelectedVLLLM:    config.SelectedModule["VLLLM"],
		Prompt:           config.DefaultPrompt,
		QuickReplyWords:  []byte(`["我在", "在呢", "来了", "啥事啊"]`),
		DeleteAudio:      config.DeleteAudio,
		UsePrivateConfig: config.UsePrivateConfig,
	}

	return db.Create(&defaultConfig).Error
}

func GetSystemConfig(db *gorm.DB) (*models.SystemConfig, error) {
	if db == nil {
		db = DB
	}

	var config models.SystemConfig
	if err := db.First(&config, SystemConfigID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("系统配置未找到")
		}
		return nil, fmt.Errorf("查询系统配置失败: %v", err)
	}
	return &config, nil
}

func UpdateSystemConfig(db *gorm.DB, config *models.SystemConfig) error {
	if db == nil {
		db = DB
	}
	config.ID = SystemConfigID // 确保更新的是唯一的系统配置记录
	if err := db.Save(config).Error; err != nil {
		return fmt.Errorf("更新系统配置失败: %v", err)
	}
	return nil
}
