package core

import (
	"fmt"
	"strings"

	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/models"
)

func (h *ConnectionHandler) requireOnboardingAgent() bool {
	if database.IsOnboardingAgent(database.GetDB(), h.agentID) {
		return true
	}
	h.LogInfo("拒绝非初始设置助手调用智能体管理工具")
	return false
}

func stringArg(args interface{}, key string) string {
	if values, ok := args.(map[string]interface{}); ok {
		if value, ok := values[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (h *ConnectionHandler) mcp_handler_list_agents(args interface{}) string {
	if !h.requireOnboardingAgent() {
		return "只有初始设置助手可以查看可切换的智能体"
	}
	agents, err := database.ListAgentsByUser(database.GetDB(), database.AdminUserID)
	if err != nil {
		h.logger.Error("获取智能体列表失败: %v", err)
		return "获取智能体列表失败"
	}
	items := make([]string, 0, len(agents))
	for _, agent := range agents {
		if agent.IsOnboarding {
			continue
		}
		description := strings.TrimSpace(agent.Description)
		if description == "" {
			description = "未设置用途说明"
		}
		items = append(items, fmt.Sprintf("ID %d：%s（%s，LLM：%s，音色：%s）", agent.ID, agent.Name, description, agent.LLM, agent.Voice))
	}
	if len(items) == 0 {
		return "还没有其他预设智能体。你可以按用户的明确要求创建一个。"
	}
	return "可切换的预设智能体：" + strings.Join(items, "；")
}

func (h *ConnectionHandler) mcp_handler_create_agent(args interface{}) string {
	if !h.requireOnboardingAgent() {
		return "只有初始设置助手可以创建智能体"
	}
	name := stringArg(args, "name")
	if name == "" {
		return "创建智能体失败：请先确认新智能体的名称"
	}
	llmName := stringArg(args, "llm")
	if llmName == "" {
		llmName = h.config.SelectedModule["LLM"]
	}
	if _, ok := h.config.LLM[llmName]; !ok {
		return fmt.Sprintf("创建智能体失败：未找到已配置的 LLM「%s」", llmName)
	}
	voice := stringArg(args, "voice")
	if voice == "" {
		if ttsName := h.config.SelectedModule["TTS"]; ttsName != "" {
			voice = h.config.TTS[ttsName].Voice
		}
	}
	agent := &models.Agent{
		Name:        name,
		LLM:         llmName,
		Language:    "普通话",
		Voice:       voice,
		Prompt:      stringArg(args, "prompt"),
		Description: stringArg(args, "description"),
		UserID:      database.AdminUserID,
	}
	if err := database.CreateAgent(database.GetDB(), agent); err != nil {
		h.logger.Error("创建智能体失败: %v", err)
		return "创建智能体失败"
	}
	return fmt.Sprintf("已创建智能体「%s」（ID %d），LLM：%s，音色：%s", agent.Name, agent.ID, agent.LLM, agent.Voice)
}

func (h *ConnectionHandler) mcp_handler_update_agent(args interface{}) string {
	if !h.requireOnboardingAgent() {
		return "只有初始设置助手可以修改智能体"
	}
	targetName := stringArg(args, "agent_name")
	if targetName == "" {
		return "修改智能体失败：请先确认要修改的智能体名称"
	}
	agents, err := database.ListAgentsByUser(database.GetDB(), database.AdminUserID)
	if err != nil {
		h.logger.Error("获取智能体列表失败: %v", err)
		return "修改智能体失败：无法获取智能体列表"
	}
	var target *models.Agent
	for index := range agents {
		if agents[index].Name == targetName {
			target = &agents[index]
			break
		}
	}
	if target == nil {
		return fmt.Sprintf("修改智能体失败：没有找到「%s」", targetName)
	}
	if target.IsOnboarding {
		return "初始设置助手由系统管理，不能通过对话修改"
	}

	changes := make([]string, 0, 4)
	if value := stringArg(args, "new_name"); value != "" && value != target.Name {
		target.Name = value
		changes = append(changes, "名称")
	}
	if value := stringArg(args, "llm"); value != "" && value != target.LLM {
		if _, ok := h.config.LLM[value]; !ok {
			return fmt.Sprintf("修改智能体失败：未找到已配置的 LLM「%s」", value)
		}
		target.LLM = value
		changes = append(changes, "LLM")
	}
	if value := stringArg(args, "voice"); value != "" && value != target.Voice {
		target.Voice = value
		changes = append(changes, "音色")
	}
	if value := stringArg(args, "prompt"); value != "" && value != target.Prompt {
		target.Prompt = value
		changes = append(changes, "角色提示词")
	}
	if value := stringArg(args, "description"); value != "" && value != target.Description {
		target.Description = value
		changes = append(changes, "用途说明")
	}
	if len(changes) == 0 {
		return "没有收到需要修改的字段"
	}
	if err := database.UpdateAgent(database.GetDB(), target); err != nil {
		h.logger.Error("更新智能体失败: %v", err)
		return "修改智能体失败"
	}
	return fmt.Sprintf("已修改智能体「%s」的%s", target.Name, strings.Join(changes, "、"))
}
