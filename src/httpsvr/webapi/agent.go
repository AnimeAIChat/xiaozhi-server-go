package webapi

import (
	"net/http"
	"strconv"
	"time"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 创建Agent请求体
type AgentCreateRequest struct {
	Prompt     string `json:"prompt"`
	Name       string `json:"name"` // 智能体名称
	LLM        string `json:"LLM"`
	Language   string `json:"language"`   // 语言，默认为中文
	Voice      string `json:"voice"`      // 语音，默认为湾湾小何
	ASRSpeed   int    `json:"asrSpeed"`   // ASR 语音识别速度，1=耐心，2=正常，3=快速
	SpeakSpeed int    `json:"speakSpeed"` // TTS 角色语速，1=慢速，2=正常，3=快速
	Tone       int    `json:"tone"`       // TTS 角色音调，1-100，低音-高音
}

// handleAgentCreate 创建Agent请求体
// @Summary 创建新的智能体
// @Description 创建新的智能体
// @Tags Agent
// @Accept json
// @Produce json
// @Param data body AgentCreateRequest true "Agent创建参数"
// @Success 200 {object} models.Agent "创建成功返回Agent信息"
// @Router /user/agent/create [post]
func (s *DefaultUserService) handleAgentCreate(c *gin.Context) {
	var req AgentCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agent := &models.Agent{
		Prompt:     req.Prompt,
		Name:       req.Name,
		LLM:        req.LLM,
		Language:   req.Language,
		Voice:      req.Voice,
		ASRSpeed:   req.ASRSpeed,
		SpeakSpeed: req.SpeakSpeed,
		Tone:       req.Tone,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	WithTx(c, func(tx *gorm.DB) error {
		if err := database.CreateAgent(tx, agent); err != nil {
			return err
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "data": agent})
		return nil
	})
}

// 构造返回结构体，带 device id 列表
type Agents struct {
	models.Agent
}

// handleAgentList 获取Agent列表
// @Summary 获取当前用户的所有Agent
// @Description 获取当前用户的所有Agent
// @Tags Agent
// @Produce json
// @Success 200 {object} []Agents "Agent列表"
// @Router /user/agent/list [get]
func (s *DefaultUserService) handleAgentList(c *gin.Context) {
	WithTx(c, func(tx *gorm.DB) error {
		agents, err := database.ListAgents(tx)
		if err != nil {
			return err
		}
		var result []Agents
		for _, agent := range agents {
			result = append(result, Agents{
				Agent: agent,
			})
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "data": result})
		return nil
	})
}

// handleAgentGet 获取单个Agent
// @Summary 获取指定ID的Agent
// @Description 根据ID获取Agent详情
// @Tags Agent
// @Produce json
// @Param id path int true "Agent ID"
// @Success 200 {object} models.Agent "Agent信息"
// @Router /user/agent/{id} [get]
func (s *DefaultUserService) handleAgentGet(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	WithTx(c, func(tx *gorm.DB) error {
		agent, err := database.GetAgentByID(tx, uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return err
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "data": agent})
		return nil
	})
}

// handleAgentUpdate 更新Agent
// @Summary 更新指定ID的Agent
// @Description 根据ID更新Agent信息
// @Tags Agent
// @Accept json
// @Produce json
// @Param id path int true "Agent ID"
// @Param data body AgentCreateRequest true "Agent更新参数"
// @Success 200 {object} models.Agent "更新后的Agent信息"
// @Router /user/agent/{id} [put]
func (s *DefaultUserService) handleAgentUpdate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req AgentCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	WithTx(c, func(tx *gorm.DB) error {
		agent, err := database.GetAgentByID(tx, uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return err
		}

		agent.Prompt = req.Prompt
		agent.Name = req.Name
		agent.LLM = req.LLM
		agent.Language = req.Language
		agent.Voice = req.Voice
		agent.ASRSpeed = req.ASRSpeed
		agent.SpeakSpeed = req.SpeakSpeed
		agent.Tone = req.Tone
		agent.UpdatedAt = time.Now() // 更新修改时间
		if agent.CreatedAt.IsZero() {
			agent.CreatedAt = time.Now() // 如果没有设置创建时间，则设置为当前时间
		}

		if err := database.UpdateAgent(tx, agent); err != nil {
			return err
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "data": agent})
		return nil
	})
}

// handleAgentDelete 删除Agent
// @Summary 删除指定ID的Agent
// @Description 根据ID删除Agent
// @Tags Agent
// @Produce json
// @Param id path int true "Agent ID"
// @Success 200 {object} map[string]interface{} "删除结果"
// @Router /user/agent/{id} [delete]
func (s *DefaultUserService) handleAgentDelete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		s.logger.Error("无效的Agent ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	WithTx(c, func(tx *gorm.DB) error {
		if err := database.DeleteAgent(tx, uint(id)); err != nil {
			s.logger.Error("删除Agent失败: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return err
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return nil
	})
}

// handleAgentClearConversation 清空Agent的Conversationid
// @Summary 清空Agent的Conversationid
// @Description 清空指定ID的Agent的Conversationid
// @Tags Agent
// @Produce json
// @Param id path int true "Agent ID"
// @Success 200 {object} map[string]interface{} "清空结果"
// @Router /user/agent/clear_conversation/{id}  [post]
func (s *DefaultUserService) handleAgentClearConversation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		s.logger.Error("无效的Agent ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	// 检查Agent是否属于当前用户
	_, err = database.GetAgentByID(database.GetDB(), uint(id))
	if err != nil {
		s.logger.Error("清空会话id，获取Agent失败: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	WithTx(c, func(tx *gorm.DB) error {
		if err := database.ClearAgentConversation(tx, uint(id)); err != nil {
			s.logger.Error("清空Agent Conversation失败: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return err
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return nil
	})
}

// handleAgentHistoryDialogList 获取Agent对话记录列表
// @Summary 获取Agent对话记录列表
// @Description 获取指定Agent的对话记录列表
// @Tags Agent
// @Produce json
// @Param id path int true "Agent ID"
// @Success 200 {object} []models.AgentDialog "对话记录列表"
// @Router /user/agent/history_dialog_list/{id} [post]
func (s *DefaultUserService) handleAgentHistoryDialogList(c *gin.Context) {
	agentID := c.Param("id")
	id, err := strconv.Atoi(agentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent id"})
		return
	}
	WithTx(c, func(tx *gorm.DB) error {
		dialogs, err := database.GetAgentDialogsWithoutDetailByID(tx, uint(id))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return err
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "data": dialogs})
		return nil
	})
}

// handleAgentGetHistoryDialog 获取Agent的单条对话记录
// @Summary 获取Agent的单条对话记录
// @Description 根据对话ID获取Agent的单条对话记录
// @Tags Agent
// @Produce json
// @Param dialog_id path int true "对话ID"
// @Success 200 {object} models.AgentDialog "对话记录"
// @Router /user/agent/history_dialog/{dialog_id} [get]
func (s *DefaultUserService) handleAgentGetHistoryDialog(c *gin.Context) {
	dialogID := c.Param("dialog_id")
	id, err := strconv.Atoi(dialogID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dialog id"})
		return
	}
	WithTx(c, func(tx *gorm.DB) error {
		dialog, err := database.GetAgentDialogByID(tx, uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "dialog not found"})
			return err
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "data": dialog})
		return nil
	})
}

// handleAgentDeleteHistoryDialog 删除Agent的单条对话记录
// @Summary 删除Agent的单条对话记录
// @Description 根据对话ID删除Agent的单条对话记录
// @Tags Agent
// @Produce json
// @Param dialog_id path int true "对话ID"
// @Success 200 {object} map[string]interface{} "删除结果"
// @Router /user/agent/history_dialog/{dialog_id} [delete]
func (s *DefaultUserService) handleAgentDeleteHistoryDialog(c *gin.Context) {
	dialogID := c.Param("dialog_id")
	id, err := strconv.Atoi(dialogID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dialog id"})
		return
	}
	WithTx(c, func(tx *gorm.DB) error {
		if err := database.DeleteAgentDialogByID(tx, uint(id)); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "dialog not found"})
			return err
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return nil
	})
}
