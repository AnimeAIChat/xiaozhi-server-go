package webapi

import (
	"net/http"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/internal/domain/config/types"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SystemConfigService 系统配置服务
type SystemConfigService struct {
	logger *utils.Logger
	db     *gorm.DB
	config *config.Config
	repo   types.Repository
}

// NewSystemConfigService 构造函数
func NewSystemConfigService(logger *utils.Logger, db *gorm.DB, config *config.Config, repo types.Repository) *SystemConfigService {
	return &SystemConfigService{
		logger: logger,
		db:     db,
		config: config,
		repo:   repo,
	}
}

// RegisterRoutes 注册路由 - DISABLED: Database functionality removed
func (s *SystemConfigService) RegisterRoutes(apiGroup *gin.RouterGroup) {
	// Database functionality removed - no routes registered
}

type ApplicationConfig struct {
	EnableMCPFilter bool `json:"enableMCPFilter"`
	SaveTtsAudio    bool `json:"saveTtsAudio"`
	SaveUserAudio   bool `json:"saveUserAudio"`
}

// 应用配置相关处理器 - DISABLED: Database functionality removed
func (s *SystemConfigService) handleGetApplicationConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleUpdateApplicationConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

type AuthConfig struct {
	Token  string `json:"token"`
	Expiry int    `json:"expiry"`
}

// 认证配置相关处理器 - DISABLED: Database functionality removed
func (s *SystemConfigService) handleGetAuthConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleUpdateAuthConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

// 传输配置相关处理器 - DISABLED: Database functionality removed
func (s *SystemConfigService) handleGetTransportConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleUpdateTransportConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

// Web配置相关处理器 - DISABLED: Database functionality removed
func (s *SystemConfigService) handleGetWebConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleUpdateWebConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

// 日志配置相关处理器 - DISABLED: Database functionality removed
func (s *SystemConfigService) handleGetLogConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleUpdateLogConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

// 角色配置相关处理器 - DISABLED: Database functionality removed
func (s *SystemConfigService) handleGetRoleConfigs(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleCreateRoleConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleUpdateRoleConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleDeleteRoleConfig(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

// 本地MCP功能相关处理器 - DISABLED: Database functionality removed
func (s *SystemConfigService) handleGetMCPFunctions(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleCreateMCPFunction(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleUpdateMCPFunction(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleDeleteMCPFunction(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

// 退出指令相关处理器 - DISABLED: Database functionality removed
func (s *SystemConfigService) handleGetExitCommands(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleCreateExitCommand(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleUpdateExitCommand(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}

func (s *SystemConfigService) handleDeleteExitCommand(c *gin.Context) {
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Configuration management is not available"})
}
