package database

import (
	"fmt"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

const OnboardingAgentName = "初始设置助手"

const onboardingPrompt = `你是初始设置助手。在管理员完成本地模型配置并创建你之后，新设备会自动绑定到你，帮助个人用户完成初始设置。
先简短欢迎并说明：可以直接使用你，也可以让我列出并切换到已有的智能体。
当用户明确要求查看、创建或修改智能体时，使用相应的本地工具；创建或修改前，若关键信息不清楚，先询问确认。
你可以修改其他智能体的名称、提示词、LLM 和音色；不要删除智能体，也不要切换到你自己。回答简短、清晰、适合语音播放。`

// 创建 Agent（支持事务）
func CreateAgent(tx *gorm.DB, agent *models.Agent) error {
	if agent == nil {
		return fmt.Errorf("智能体不能为空")
	}
	agent.UserID = AdminUserID
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

// GetOnboardingAgent 只查询已经由管理员显式创建的初始设置助手，不会隐式创建。
func GetOnboardingAgent(tx *gorm.DB) (*models.Agent, error) {
	var agent models.Agent
	if err := tx.Where("user_id = ? AND is_onboarding = ?", AdminUserID, true).First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

// ActivateOnboardingAgent 创建唯一的初始设置助手；若已存在，仅同步其 LLM 与音色，不会创建重复记录。
func ActivateOnboardingAgent(tx *gorm.DB, llmName, voice string) (*models.Agent, bool, error) {
	agent, err := GetOnboardingAgent(tx)
	if err == nil {
		agent.LLM = llmName
		agent.Voice = voice
		if err := UpdateAgent(tx, agent); err != nil {
			return nil, false, fmt.Errorf("同步初始设置助手配置失败: %w", err)
		}
		return agent, false, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, false, fmt.Errorf("查询初始设置助手失败: %w", err)
	}

	agent = &models.Agent{
		Name:         OnboardingAgentName,
		LLM:          llmName,
		Language:     "普通话",
		Voice:        voice,
		Prompt:       onboardingPrompt,
		UserID:       AdminUserID,
		EnabledTools: "switch_agent,list_agents,create_agent,update_agent",
		Description:  "帮助新设备完成初始设置，并管理其他智能体。",
		IsOnboarding: true,
	}
	if err := CreateAgent(tx, agent); err != nil {
		return nil, false, fmt.Errorf("创建初始设置助手失败: %w", err)
	}
	return agent, true, nil
}

// BindUnboundDevicesToOnboardingAgent 只接管尚未归属其他用户的未绑定设备。
func BindUnboundDevicesToOnboardingAgent(tx *gorm.DB, agent *models.Agent) (int64, error) {
	if agent == nil || !agent.IsOnboarding {
		return 0, fmt.Errorf("初始设置助手无效")
	}
	result := tx.Model(&models.Device{}).
		Where("agent_id IS NULL AND (user_id IS NULL OR user_id = ?)", AdminUserID).
		Updates(map[string]interface{}{
			"agent_id":    agent.ID,
			"user_id":     agent.UserID,
			"auth_status": "onboarding",
		})
	if result.Error != nil {
		return 0, fmt.Errorf("绑定等待中的设备失败: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func IsOnboardingAgent(tx *gorm.DB, id uint) bool {
	if id == 0 {
		return false
	}
	var count int64
	return tx.Model(&models.Agent{}).Where("id = ? AND user_id = ? AND is_onboarding = ?", id, AdminUserID, true).Count(&count).Error == nil && count == 1
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
	if IsOnboardingAgent(tx, id) {
		return fmt.Errorf("初始设置助手是系统智能体，不能删除")
	}
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
