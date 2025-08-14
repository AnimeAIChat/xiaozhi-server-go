package database

import (
	"fmt"
	"strings"

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
