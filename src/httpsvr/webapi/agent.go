package webapi

import (
	"fmt"
	"net/http"
	// "strconv"
	// "time"
	// "xiaozhi-server-go/src/configs/database" // TODO: Remove database dependency
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
	Voice      string `json:"voice"`      // 语音，默认为湾湾小何的音色id
	VoiceName  string `json:"voiceName"`  // 语音名称，默认为湾湾小何
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
	// userID := c.GetUint("user_id")
	var req AgentCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "参数校验失败", gin.H{"error": err.Error()})
		return
	}
	WithTx(c, func(tx *gorm.DB) error {
		// TODO: Database functionality removed - need to implement new storage mechanism
		respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
		return fmt.Errorf("database functionality removed")
		/*
		agent := &models.Agent{
			Prompt:     req.Prompt,
			Name:       req.Name,
			LLM:        req.LLM,
			Language:   req.Language,
			Voice:      req.Voice,
			VoiceName:  req.VoiceName,
			ASRSpeed:   req.ASRSpeed,
			SpeakSpeed: req.SpeakSpeed,
			Tone:       req.Tone,
			UserID:     userID,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := database.CreateAgent(tx, agent); err != nil {
			respondError(c, http.StatusInternalServerError, "创建智能体失败", gin.H{"error": err.Error()})
			return err
		}
		respondSuccess(c, http.StatusOK, agent, "创建智能体成功")
		return nil
		*/
	})
}

// 构造返回结构体，带 device id 列表
type AgentWithDeviceIDs struct {
	models.Agent
	DeviceIDs []uint `json:"deviceIDs"`
}

// handleAgentList 获取Agent列表
// @Summary 获取当前用户的所有Agent
// @Description 获取当前用户的所有Agent及其设备ID列表
// @Tags Agent
// @Produce json
// @Success 200 {object} []AgentWithDeviceIDs "Agent列表"
// @Router /user/agent/list [get]
func (s *DefaultUserService) handleAgentList(c *gin.Context) {
	// userID := c.GetUint("user_id")
	WithTx(c, func(tx *gorm.DB) error {
		// TODO: Database functionality removed - need to implement new storage mechanism
		respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
		return fmt.Errorf("database functionality removed")
		/*
		agents, err := database.ListAgentsByUser(tx, userID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "获取智能体列表失败", gin.H{"error": err.Error()})
			return err
		}
		var result []AgentWithDeviceIDs
		for _, agent := range agents {
			var ids []uint
			for _, d := range agent.Devices {
				ids = append(ids, d.ID)
			}
			result = append(result, AgentWithDeviceIDs{
				Agent:     agent,
				DeviceIDs: ids,
			})
		}
		respondSuccess(c, http.StatusOK, result, "获取智能体列表成功")
		return nil
		*/
	})
}

func (s *DefaultUserService) handleAgentGet(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

func (s *DefaultUserService) handleAgentUpdate(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

func (s *DefaultUserService) handleAgentDelete(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

func (s *DefaultUserService) handleAgentHistoryDialogList(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

func (s *DefaultUserService) handleAgentGetHistoryDialog(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}

func (s *DefaultUserService) handleAgentDeleteHistoryDialog(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "数据库功能已移除", gin.H{"error": "database functionality removed"})
}
