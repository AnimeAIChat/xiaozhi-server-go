package database

import (
	"fmt"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

// 创建 Agent（支持事务）
func CreateAgent(tx *gorm.DB, agent *models.Agent) error {
	return tx.Create(agent).Error
}

func CreateDefaultAgent(tx *gorm.DB) (*models.Agent, error) {
	agent := &models.Agent{
		Name:  "默认智能体",
		LLM:   configs.Cfg.SelectedModule["LLM"],
		Voice: "zh_female_wanwanxiaohe_moon_bigtts",
	}
	err := CreateAgent(tx, agent)
	if err != nil {
		return nil, fmt.Errorf("创建默认智能体失败: %v", err)
	}
	return agent, nil
}

// 获取用户所有 Agent（支持事务）
func ListAgents(tx *gorm.DB) ([]models.Agent, error) {
	var agents []models.Agent
	err := tx.Find(&agents).Error
	return agents, err
}

// 获取单个 Agent（支持事务）
func GetAgentByID(tx *gorm.DB, id uint) (*models.Agent, error) {
	var agent models.Agent
	err := tx.Where("id = ?", id).First(&agent).Error
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

// 更新 Agent（支持事务）
func UpdateAgent(tx *gorm.DB, agent *models.Agent) error {
	return tx.Model(agent).Updates(agent).Error
}

// 删除 Agent（支持事务）
func DeleteAgent(tx *gorm.DB, id uint) error {
	// 删除智能体
	result := tx.Where("id = ?", id).Delete(&models.Agent{})
	if result.Error != nil {
		return fmt.Errorf("删除智能体失败: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("智能体不存在或已被删除")
	}
	return nil
}
