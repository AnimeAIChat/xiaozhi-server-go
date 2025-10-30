package webapi

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"xiaozhi-server-go/internal/platform/config"
	// "xiaozhi-server-go/src/configs/database" // DISABLED: Database functionality removed
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
)

type DefaultUserService struct {
	logger *utils.Logger
	config *config.Config
}

// NewDefaultUserService 构造函数
func NewDefaultUserService(
	config *config.Config,
	logger *utils.Logger,
) (*DefaultUserService, error) {
	service := &DefaultUserService{
		logger: logger,
		config: config,
	}
	return service, nil
}

// Start 实现用户服务接口，注册所有用户相关路由
func (s *DefaultUserService) Start(
	ctx context.Context,
	engine *gin.Engine,
	apiGroup *gin.RouterGroup,
) error {
	apiGroup.POST("/user/login", s.handleLogin)
	apiGroup.POST("/user/logout", s.handleLogout)

	// 需要认证的用户接口
	authGroup := apiGroup.Group("/user")
	authGroup.Use(AuthMiddleware(s.config))
	{
		authGroup.GET("/profile", s.handleGetProfile)
		authGroup.PUT("/profile", s.handleUpdateProfile)
		authGroup.POST("/change-password", s.handleChangePassword)

		authGroup.GET("/summary", s.handleSystemSummary) // 获取用户汇总信息

		authGroup.POST("/agent/create", s.handleAgentCreate)
		authGroup.GET("/agent/list", s.handleAgentList)
		authGroup.GET("/agent/:id", s.handleAgentGet)
		authGroup.PUT("/agent/:id", s.handleAgentUpdate)
		authGroup.DELETE("/agent/:id", s.handleAgentDelete)

		authGroup.POST("/agent/history_dialog_list/:id", s.handleAgentHistoryDialogList)
		authGroup.GET("/agent/history_dialog/:dialog_id", s.handleAgentGetHistoryDialog)
		authGroup.DELETE("/agent/history_dialog/:dialog_id", s.handleAgentDeleteHistoryDialog)

		authGroup.GET("/device/list/:id", s.handleDeviceList)
		authGroup.GET("/device/list", s.handleDeviceListByUser)
		authGroup.GET("/device/:id", s.handleDeviceGet)
		authGroup.PUT("/device/:id", s.handleDeviceUpdate)
		authGroup.DELETE("/device", s.handleDeviceDelete)

	}

	s.logger.InfoTag("HTTP", "用户服务路由注册完成")
	return nil
}

func (s *DefaultUserService) handleSystemSummary(c *gin.Context) {
	// Database functionality removed - return mock data
	respondSuccess(c, http.StatusOK, gin.H{
		"totle_users":       0,
		"totle_agents":      0,
		"totle_devices":     0,
		"online_users":      0,
		"session_devices":   0,
		"system_memory_use": "0%",
		"system_cpu_use":    "0%",
	}, "获取系统汇总信息成功")
}

// LoginRequest 用户登录请求体
// @Description 用户登录参数
// @Tags User
// @Accept json
// @Produce json
// @Param data body LoginRequest true "登录参数"
// @Success 200 {object} map[string]interface{} "登录成功返回token和用户信息"
// @Router /user/login [post]
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// ChangePasswordRequest 修改密码请求体
// @Description 修改密码参数
// @Tags User
// @Accept json
// @Produce json
// @Param data body ChangePasswordRequest true "修改密码参数"
// @Success 200 {object} map[string]interface{} "修改密码结果"
// @Router /user/change-password [post]
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// handleLogin 用户登录
// @Summary 用户登录
// @Description 用户登录接口
// @Tags User
// @Accept json
// @Produce json
// @Param data body LoginRequest true "登录参数"
// @Success 200 {object} map[string]interface{} "登录成功返回token和用户信息"
// @Router /user/login [post]
func (s *DefaultUserService) handleLogin(c *gin.Context) {
	// Database functionality removed - return error
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "User authentication is not available"})
}

// handleLogout 用户登出
// @Summary 用户登出
// @Description 用户登出接口
// @Tags User
// @Produce json
// @Success 200 {object} map[string]interface{} "登出结果"
// @Router /user/logout [post]
func (s *DefaultUserService) handleLogout(c *gin.Context) {
	// 这里可以实现token黑名单机制
	respondSuccess(c, http.StatusOK, nil, "登出成功")
}

// handleGetProfile 获取用户资料
// @Summary 获取用户资料
// @Description 获取当前登录用户的个人信息
// @Tags User
// @Produce json
// @Success 200 {object} map[string]interface{} "用户资料"
// @Router /user/profile [get]
func (s *DefaultUserService) handleGetProfile(c *gin.Context) {
	// Database functionality removed - return error
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "User profile is not available"})
}

// handleUpdateProfile 更新用户资料
// @Summary 更新用户资料
// @Description 更新当前登录用户的个人信息
// @Tags User
// @Accept json
// @Produce json
// @Param data body object true "用户资料参数"
// @Success 200 {object} map[string]interface{} "更新后的用户资料"
// @Router /user/profile [put]
func (s *DefaultUserService) handleUpdateProfile(c *gin.Context) {
	// Database functionality removed - return error
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Profile update is not available"})
}

// handleChangePassword 修改密码
// @Summary 修改密码
// @Description 修改当前登录用户的密码
// @Tags User
// @Accept json
// @Produce json
// @Param data body ChangePasswordRequest true "修改密码参数"
// @Success 200 {object} map[string]interface{} "修改密码结果"
// @Router /user/change-password [post]
func (s *DefaultUserService) handleChangePassword(c *gin.Context) {
	// Database functionality removed - return error
	respondError(c, http.StatusNotImplemented, "Database functionality removed", gin.H{"error": "Password change is not available"})
}

// 密码哈希
func (s *DefaultUserService) hashPassword(password string) string {
	hash := md5.Sum([]byte(password + "xiaozhi_salt"))
	return hex.EncodeToString(hash[:])
}

// 验证密码
func (s *DefaultUserService) verifyPassword(password, hashedPassword string) bool {
	return s.hashPassword(password) == hashedPassword
}
