package webapi

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DefaultUserService struct {
	logger *utils.Logger
	config *configs.Config
}

// NewDefaultUserService 构造函数
func NewDefaultUserService(
	config *configs.Config,
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
	// 获取用户汇总信息
	userID, _ := c.Get("user_id")
	_, err := database.GetUserByID(database.GetDB(), userID.(uint))
	if err != nil {
		respondError(c, http.StatusNotFound, "用户不存在", nil)
		return
	}

	data, _ := database.GetSystemSummary(database.GetDB())

	respondSuccess(c, http.StatusOK, gin.H{
		"totle_users":       data["total_users"],
		"totle_agents":      data["total_agents"],
		"totle_devices":     data["total_devices"],
		"online_users":      data["online_devices"],
		"session_devices":   data["session_devices"],
		"system_memory_use": data["memory_usage"],
		"system_cpu_use":    data["cpu_usage"],
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
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "请求参数错误", gin.H{"error": err.Error()})
		return
	}

	// 获取用户信息
	user, err := database.GetUserByUsername(database.GetDB(), req.Username)
	if err != nil || user == nil {
		respondError(c, http.StatusUnauthorized, "用户名或密码错误", nil)
		return
	}

	// 验证密码
	if !s.verifyPassword(req.Password, user.Password) {
		respondError(c, http.StatusUnauthorized, "用户名或密码错误", nil)
		return
	}

	// 检查用户状态
	if user.Status != 1 {
		respondError(c, http.StatusUnauthorized, "账户已被禁用", nil)
		return
	}

	// 生成JWT token
	token, err := GenerateJWT(user.ID, user.Username)
	if err != nil {
		s.logger.Error("生成JWT失败: %v", err)
		respondError(c, http.StatusInternalServerError, "登录失败", nil)
		return
	}

	// 更新最后登录时间
	database.UpdateUserLastLogin(database.GetDB(), user.ID)

	s.logger.Info("用户登录成功: %s", req.Username)
	role := user.Role
	// 判断 如果时admin，则下发role字段
	data := gin.H{
		"token":    token,
		"user_id":  user.ID,
		"username": user.Username,
		"email":    user.Email,
	}

	// 判断 如果是admin，则下发role字段
	if role != "user" {
		data["role"] = role
	}

	respondSuccess(c, http.StatusOK, data, "登录成功")
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
	userID, _ := c.Get("user_id")

	user, err := database.GetUserByID(database.GetDB(), userID.(uint))
	if err != nil || user == nil {
		respondError(c, http.StatusNotFound, "用户不存在", nil)
		return
	}

	respondSuccess(c, http.StatusOK, gin.H{
		"user_id":    user.ID,
		"username":   user.Username,
		"nickname":   user.Nickname,
		"head_img":   user.HeadImg,
		"email":      user.Email,
		"created_at": user.CreatedAt,
		"updated_at": user.UpdatedAt,
	}, "获取用户资料成功")
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
	userID, _ := c.Get("user_id")

	var updateData struct {
		Nickname string `json:"nickname" binding:"omitempty,min=3,max=20"`
		Email    string `json:"email" binding:"omitempty,email"`
		HeadImg  string `json:"head_img" binding:"omitempty,url"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		respondError(c, http.StatusBadRequest, "请求参数错误", gin.H{"error": err.Error()})
		return
	}
	WithTx(c, func(tx *gorm.DB) error {
		user, err := database.GetUserByID(tx, userID.(uint))
		if err != nil {
			respondError(c, http.StatusNotFound, "用户不存在", nil)
			return err
		}
		if updateData.Nickname != user.Nickname {
			err := database.UpdateUserNickname(tx, userID.(uint), updateData.Nickname)
			if err != nil {
				respondError(c, http.StatusBadRequest, "更新用户昵称失败", gin.H{"error": err.Error()})
				return err
			}
			user.Nickname = updateData.Nickname
		}
		if updateData.HeadImg != "" && updateData.HeadImg != user.HeadImg {
			err := database.UpdateUserHeadImg(tx, userID.(uint), updateData.HeadImg)
			if err != nil {
				respondError(c, http.StatusBadRequest, "更新头像失败", gin.H{"error": err.Error()})
				return err
			}
			user.HeadImg = updateData.HeadImg
		}
		if updateData.Email != user.Email {
			err := database.UpdateUserEmail(tx, userID.(uint), updateData.Email)
			if err != nil {
				respondError(c, http.StatusBadRequest, "更新邮箱失败", gin.H{"error": err.Error()})
				return err
			}
			user.Email = updateData.Email
		}
		respondSuccess(c, http.StatusOK, gin.H{
			"user_id":  user.ID,
			"nickname": user.Nickname,
			"email":    user.Email,
			"head_img": user.HeadImg,
		}, "更新成功")
		return nil
	})
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
	userID, _ := c.Get("user_id")

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "请求参数错误", gin.H{"error": err.Error()})
		return
	}

	// 获取用户信息验证旧密码
	user, err := database.GetUserByID(database.GetDB(), userID.(uint))
	if err != nil || user == nil {
		respondError(c, http.StatusNotFound, "用户不存在", nil)
		return
	}

	// 验证旧密码
	if !s.verifyPassword(req.OldPassword, user.Password) {
		s.logger.Error(
			"用户修改密码失败: 原密码错误: %s, old:%s, req:%s",
			user.Username,
			user.Password,
			req.OldPassword,
		)
		respondError(c, http.StatusBadRequest, "原密码错误", nil)
		return
	}

	// 更新密码
	hashedPassword := s.hashPassword(req.NewPassword)
	if err := database.UpdateUserPassword(database.GetDB(), userID.(uint), hashedPassword); err != nil {
		respondError(c, http.StatusInternalServerError, "密码修改失败", gin.H{"error": err.Error()})
		return
	}

	respondSuccess(c, http.StatusOK, nil, "密码修改成功")
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
