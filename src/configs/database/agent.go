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

func CreateDefaultAgent(tx *gorm.DB, userID uint) (*models.Agent, error) {
	agent := &models.Agent{
		Name:   "默认智能体",
		LLM:    configs.Cfg.SelectedModule["LLM"],
		Voice:  "zh_female_wanwanxiaohe_moon_bigtts",
		UserID: userID,
	}
	err := CreateAgent(tx, agent)
	if err != nil {
		return nil, fmt.Errorf("创建默认智能体失败: %v", err)
	}
	return agent, nil
}

// 获取用户所有 Agent（支持事务）
func ListAgentsByUser(tx *gorm.DB, userID uint) ([]models.Agent, error) {
	var agents []models.Agent
	err := tx.Where("user_id = ?", userID).Preload("Devices").Find(&agents).Error
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

// 支持事务的 GetAgentByIDAndUser
func GetAgentByIDAndUser(tx *gorm.DB, id uint, userID uint) (*models.Agent, error) {
	var agent models.Agent
	err := tx.Where("id = ? AND user_id = ?", id, userID).First(&agent).Error
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
func DeleteAgent(tx *gorm.DB, id uint, userID uint) error {
	// 先把已软删除的设备中指向该 agent 的引用置为空（兼容历史数据，避免外键冲突）
	if err := tx.Unscoped().Model(&models.Device{}).
		Where("agent_id = ? AND deleted_at IS NOT NULL", id).
		Update("agent_id", nil).Error; err != nil {
		return fmt.Errorf("清理已软删除设备的 agent 引用失败: %v", err)
	}

	// 查询是否有设备绑定该 agent（默认会排除软删除记录）
	var deviceCount int64
	err := tx.Model(&models.Device{}).Where("agent_id = ?", id).Count(&deviceCount).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("智能体不存在或已被删除")
		}
		dbLogger.Error("查询 Agent 绑定设备失败: %v", err)
		return fmt.Errorf("操作失败，请联系管理员")
	}
	if deviceCount > 0 {
		return fmt.Errorf("请先解绑智能体绑定的设备")
	}
	// 删除智能体
	result := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Agent{})
	if result.Error != nil {
		return fmt.Errorf("删除智能体失败: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("智能体不存在或已被删除")
	}
	return nil
}
