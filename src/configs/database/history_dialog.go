package database

import (
	"fmt"
	"time"
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

// 保存 Agent 对话（支持事务）
func SaveAgentDialog(
	tx *gorm.DB,
	AgentID uint,
	userID uint,
	dialogStr string,
	conversationid string,
) error {
	// 检查对话是否已存在
	var existingDialog models.AgentDialog
	err := tx.Where("agent_id = ? AND conversationid = ?", AgentID, conversationid).
		First(&existingDialog).
		Error
	if err == nil {
		// 如果对话已存在，更新对话内容
		existingDialog.Dialog = dialogStr
		existingDialog.UpdatedAt = time.Now()
		return tx.Save(&existingDialog).Error
	}

	agentDialog := &models.AgentDialog{
		AgentID:        AgentID,
		UserID:         userID,
		Dialog:         dialogStr,
		Conversationid: conversationid,
		CreatedAt:      time.Now(),
	}
	return tx.Create(agentDialog).Error
}

func GetAgentDialogByConversationID(
	tx *gorm.DB,
	AgentID uint,
	conversationid string,
) (*models.AgentDialog, error) {
	var agentDialog models.AgentDialog
	err := tx.Where("agent_id = ? AND conversationid = ?", AgentID, conversationid).
		First(&agentDialog).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("对话记录不存在")
		}
		return nil, fmt.Errorf("查询对话记录失败: %v", err)
	}
	return &agentDialog, nil
}

func GetAgentDialogByID(
	tx *gorm.DB,
	id uint,
) (*models.AgentDialog, error) {
	var agentDialog models.AgentDialog
	if err := tx.Where("id = ?", id).First(&agentDialog).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("对话记录不存在")
		}
		return nil, fmt.Errorf("查询对话记录失败: %v", err)
	}
	return &agentDialog, nil
}

func GetAgentDialogsWithoutDetailByID(
	tx *gorm.DB,
	AgentID uint,
) ([]models.AgentDialog, error) {
	var agentDialogs []models.AgentDialog
	err := tx.Where("agent_id = ?", AgentID).
		Order("created_at DESC").
		Omit("Dialog").
		Find(&agentDialogs).
		Error
	if err != nil {
		return nil, fmt.Errorf("查询对话记录失败: %v", err)
	}
	return agentDialogs, nil
}

func DeleteAgentDialogByID(
	tx *gorm.DB,
	id uint,
) error {
	var agentDialog models.AgentDialog
	if err := tx.Where("id = ?", id).First(&agentDialog).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("对话记录不存在")
		}
		return fmt.Errorf("查询对话记录失败: %v", err)
	}
	if err := tx.Delete(&agentDialog).Error; err != nil {
		return fmt.Errorf("删除对话记录失败: %v", err)
	}
	return nil
}

func SaveAgentConversation(tx *gorm.DB, agentID uint, conversationID string) error {
	// 更新 Agent 的 Conversationid 字段
	return tx.Model(&models.Agent{}).
		Where("id = ?", agentID).
		Update("conversationid", conversationID).
		Error
}

func ClearAgentConversation(tx *gorm.DB, agentID uint) error {
	// 清空 Agent 的 Conversationid 字段
	return tx.Model(&models.Agent{}).Where("id = ?", agentID).Update("conversationid", "").Error
}
