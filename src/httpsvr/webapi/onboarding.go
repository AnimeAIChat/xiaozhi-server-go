package webapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type onboardingStatus struct {
	Exists         bool     `json:"exists"`
	Ready          bool     `json:"ready"`
	Issues         []string `json:"issues"`
	AgentID        uint     `json:"agentId,omitempty"`
	LLM            string   `json:"llm,omitempty"`
	Voice          string   `json:"voice,omitempty"`
	PendingDevices int64    `json:"pendingDevices"`
}

// onboardingProviderSetup 从持久化的管理员 Provider 配置中读取已选模块，避免使用进程启动时的旧配置。
func (s *DefaultAdminService) onboardingProviderSetup() (string, string, []string) {
	if s.config == nil || s.config.SelectedModule == nil {
		return "", "", []string{"系统配置尚未初始化"}
	}
	issues := make([]string, 0)
	selectedConfigs := make(map[string]map[string]interface{}, 3)
	for _, providerType := range []string{"ASR", "LLM", "TTS"} {
		name := strings.TrimSpace(s.config.SelectedModule[providerType])
		if name == "" {
			issues = append(issues, providerType+"：尚未选择 Provider")
			continue
		}
		providers, err := database.GetProviderByTypeInternal(providerType, database.AdminUserID, false)
		if err != nil {
			issues = append(issues, providerType+"：读取 Provider 配置失败")
			continue
		}
		raw, ok := providers[name]
		if !ok {
			issues = append(issues, fmt.Sprintf("%s：未找到已选 Provider「%s」", providerType, name))
			continue
		}
		var providerConfig map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &providerConfig); err != nil {
			issues = append(issues, fmt.Sprintf("%s：Provider 配置不是有效 JSON", providerType))
			continue
		}
		subType := strings.ToLower(strings.TrimSpace(stringConfig(providerConfig, "type")))
		if subType == "" {
			issues = append(issues, fmt.Sprintf("%s：Provider 缺少具体类型", providerType))
			continue
		}
		if missing := missingProviderTestFields(providerType, subType, providerConfig); len(missing) > 0 {
			issues = append(issues, fmt.Sprintf("%s「%s」还需填写：%s", providerType, name, strings.Join(missing, "、")))
			continue
		}
		invalidField := ""
		for _, field := range requiredProviderFields(providerType, subType) {
			if isPlaceholder(stringConfig(providerConfig, field)) {
				invalidField = field
				break
			}
		}
		if invalidField != "" {
			issues = append(issues, fmt.Sprintf("%s「%s」的%s仍是示例值", providerType, name, invalidField))
			continue
		}
		selectedConfigs[providerType] = providerConfig
	}
	if len(issues) > 0 {
		return "", "", issues
	}
	voice := stringConfig(selectedConfigs["TTS"], "voice")
	if isPlaceholder(voice) {
		return "", "", []string{"TTS：未填写有效音色"}
	}
	return s.config.SelectedModule["LLM"], voice, nil
}

func requiredProviderFields(providerType, subType string) []string {
	required := map[string]map[string][]string{
		"ASR": {
			"doubao": {"appid", "access_token"}, "deepgram": {"addr", "api_key"}, "gosherpa": {"addr"},
			"stepfun": {"api_key", "model", "voice"}, "iflytek": {"appid", "api_key", "api_secret"},
		},
		"TTS": {
			"edge": {"voice"}, "doubao": {"appid", "token", "cluster", "voice"}, "gosherpa": {"cluster"},
			"deepgram": {"cluster", "token", "voice"}, "iflytek": {"appid", "token", "cluster"},
		},
		"LLM": {
			"openai": {"url", "api_key", "model_name"}, "ollama": {"url", "model_name"},
			"doubao": {"url", "api_key", "model_name"}, "coze": {"bot_id", "user_id", "personal_access_token"},
			"dsh": {"url", "api_key"},
		},
	}
	return required[providerType][subType]
}

func pendingOnboardingDeviceCount() int64 {
	var count int64
	database.GetDB().Model(&models.Device{}).
		Where("agent_id IS NULL AND (user_id IS NULL OR user_id = ?)", database.AdminUserID).
		Count(&count)
	return count
}

func (s *DefaultAdminService) currentOnboardingStatus() onboardingStatus {
	llmName, voice, issues := s.onboardingProviderSetup()
	status := onboardingStatus{
		Ready:          len(issues) == 0,
		Issues:         issues,
		LLM:            llmName,
		Voice:          voice,
		PendingDevices: pendingOnboardingDeviceCount(),
	}
	if agent, err := database.GetOnboardingAgent(database.GetDB()); err == nil {
		status.Exists = true
		status.AgentID = agent.ID
	} else if err != gorm.ErrRecordNotFound {
		status.Issues = append(status.Issues, "读取初始设置助手状态失败")
		status.Ready = false
	}
	return status
}

func (s *DefaultAdminService) handleOnboardingStatus(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok", "data": s.currentOnboardingStatus()})
}

// handleOnboardingActivate 显式创建唯一初始设置助手，并接管此前未绑定的设备。
func (s *DefaultAdminService) handleOnboardingActivate(c *gin.Context) {
	llmName, voice, issues := s.onboardingProviderSetup()
	if len(issues) > 0 {
		c.JSON(400, gin.H{"status": "error", "message": "请先完成 ASR、LLM、TTS 配置", "data": gin.H{"issues": issues}})
		return
	}
	tx := database.GetTxDB()
	agent, created, err := database.ActivateOnboardingAgent(tx, llmName, voice)
	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": "error", "message": "创建初始设置助手失败"})
		return
	}
	bound, err := database.BindUnboundDevicesToOnboardingAgent(tx, agent)
	if err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": "error", "message": "绑定等待中的设备失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"status": "error", "message": "保存初始设置助手失败"})
		return
	}
	message := "初始设置助手已创建"
	if !created {
		message = "初始设置助手已存在，已同步当前 Provider 配置"
	}
	c.JSON(200, gin.H{"status": "ok", "message": message, "data": gin.H{
		"agentId": agent.ID, "created": created, "boundDevices": bound,
	}})
}
