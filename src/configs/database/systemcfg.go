package database

import (
	"fmt"
	"strings"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

type ServerConfigDB struct {
	db *gorm.DB
}

var serverConfigDB *ServerConfigDB

func GetServerConfigDB() *ServerConfigDB {
	return serverConfigDB
}

func NewServerConfigDB(db *gorm.DB) *ServerConfigDB {
	serverConfigDB = &ServerConfigDB{db: db}
	return serverConfigDB
}

func (d *ServerConfigDB) GetDB() *gorm.DB {
	return d.db
}

func (d *ServerConfigDB) IsServerConfigExists() (bool, error) {
	var count int64
	if err := d.db.Model(&models.ServerConfig{}).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *ServerConfigDB) InitServerConfig(cfgStr string) error {
	var count int64
	if err := d.db.Model(&models.ServerConfig{}).Count(&count).Error; err != nil {
		if strings.Contains(err.Error(), "no such table") {
			// 创建table
			if err := d.db.AutoMigrate(&models.ServerConfig{}); err != nil {
				return fmt.Errorf("创建服务器配置表失败: %v", err)
			}
		} else {
			return err
		}
	}
	if count > 0 {
		return nil
	}

	defaultConfig := models.ServerConfig{
		ID:     ServerConfigID,
		CfgStr: cfgStr,
	}

	return d.db.Create(&defaultConfig).Error
}

func (d *ServerConfigDB) GetServerConfig() (string, error) {
	var config models.ServerConfig
	if err := d.db.First(&config, ServerConfigID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", fmt.Errorf("查询服务器配置失败: %v", err)
	}
	return config.CfgStr, nil
}

func (d *ServerConfigDB) UpdateServerConfig(cfgStr string) error {
	// 只有一个
	var count int64
	if err := d.db.Model(&models.ServerConfig{}).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("服务器配置未找到")
	}

	return d.db.Model(&models.ServerConfig{}).Where("id = ?", ServerConfigID).Update("cfg_str", cfgStr).Error
}

func (d *ServerConfigDB) LoadServerConfig() (string, error) {
	var config models.ServerConfig

	if err := d.db.AutoMigrate(&models.ServerConfig{}); err != nil {
		return "", fmt.Errorf("创建服务器配置表失败: %v", err)
	}

	if err := d.db.First(&config, ServerConfigID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		if strings.Contains(err.Error(), "no such table") {
			return "", nil
		}
		return "", fmt.Errorf("查询服务器配置失败: %v", err)
	}

	return config.CfgStr, nil
}

// InitSystemConfig 初始化系统配置
func InitSystemConfig(db *gorm.DB, config *configs.Config) error {
	var count int64
	if err := db.Model(&models.SystemConfig{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaultConfig := models.SystemConfig{
		ID:              SystemConfigID,
		SelectedASR:     config.SelectedModule["ASR"],
		SelectedTTS:     config.SelectedModule["TTS"],
		SelectedLLM:     config.SelectedModule["LLM"],
		SelectedVLLLM:   config.SelectedModule["VLLLM"],
		Prompt:          config.DefaultPrompt,
		QuickReplyWords: []byte(`["我在", "在呢", "来了", "啥事啊"]`),
		DeleteAudio:     config.DeleteAudio,
	}

	return db.Create(&defaultConfig).Error
}

// GetSystemConfig 获取系统配置
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

// UpdateSystemConfig 更新系统配置
func UpdateSystemConfig(db *gorm.DB, config *models.SystemConfig) error {
	if db == nil {
		db = DB
	}

	return db.Model(&models.SystemConfig{}).Where("id = ?", SystemConfigID).Updates(config).Error
}
