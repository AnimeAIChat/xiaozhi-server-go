package database

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

var (
	AdminUserID uint = 1
)

func InitAdminUser(db *gorm.DB, config *configs.Config) error {
	var count int64
	// 检查是否已经存在管理员用户
	if err := db.Model(&models.User{}).Where("role = ?", "admin").Count(&count).Error; err != nil {
		// 查询失败
		return err
	}
	if count > 0 {
		// 已经存在管理员用户，不需要初始化
		return nil
	}

	password := "123456" // 默认管理员密码
	hash := md5.Sum([]byte(password + "xiaozhi_salt"))
	pwd := hex.EncodeToString(hash[:])
	// 创建管理员用户
	adminUser := &models.User{
		Username:  "admin",
		Email:     "",
		Password:  pwd, // 使用默认的管理员密码
		Role:      "admin",
		Status:    1, // 默认启用状态
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(adminUser).Error; err != nil {
		// 创建失败
		return err
	}
	fmt.Println("管理员用户初始化成功 admin:123456, 请及时修改密码")

	return nil
}

// GetUserByUsername 根据用户名获取用户
func GetUserByUsername(tx *gorm.DB, username string) (*models.User, error) {
	var user models.User
	err := tx.Where("username = ?", username).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByID 根据ID获取用户
func GetUserByID(tx *gorm.DB, userID uint) (*models.User, error) {
	var user models.User
	err := tx.Where("id = ?", userID).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// UpdateUserLastLogin 更新用户最后登录时间
func UpdateUserLastLogin(tx *gorm.DB, userID uint) error {
	err := tx.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"updated_at": time.Now(),
		}).Error
	return err
}

// UpdateUserProfile 更新用户资料
func UpdateUserEmail(tx *gorm.DB, userID uint, email string) error {
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	// 检查邮箱是否已经存在
	var existingUser models.User
	tx.Where("email = ?", email).First(&existingUser)
	if existingUser.ID != 0 && existingUser.ID != userID {
		return fmt.Errorf("邮箱已被其他用户使用")
	}

	if email != "" {
		updates["email"] = email
	}
	err := tx.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(updates).Error
	return err
}

func UpdateUserName(tx *gorm.DB, userID uint, username string) error {
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	// 检查用户名是否已经存在
	var existingUser models.User
	tx.Where("username = ?", username).First(&existingUser)
	if existingUser.ID != 0 && existingUser.ID != userID {
		return fmt.Errorf("用户名已被其他用户使用")
	}

	if username != "" {
		updates["username"] = username
	}
	err := tx.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(updates).Error
	return err
}

func UpdateUserNickname(tx *gorm.DB, userID uint, nickname string) error {
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if nickname != "" {
		updates["nickname"] = nickname
	}
	err := tx.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(updates).Error
	return err
}

func UpdateUserHeadImg(tx *gorm.DB, userID uint, headImg string) error {
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if headImg != "" {
		updates["head_img"] = headImg
	}
	err := tx.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(updates).Error
	return err
}

// UpdateUserPassword 更新用户密码
func UpdateUserPassword(tx *gorm.DB, userID uint, hashedPassword string) error {
	err := tx.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"password":   hashedPassword,
			"updated_at": time.Now(),
		}).Error
	return err
}

// GetSystemSummary
func GetSystemSummary(tx *gorm.DB) (map[string]interface{}, error) {
	// 取ServerStatus
	status, err := GetServerStatus()
	if err != nil {
		return nil, err
	}
	cpuUsage := status.CPUUsage
	memoryUsage := status.MemoryUsage

	// agent总数
	var agentCount int64
	if err := tx.Model(&models.Agent{}).Count(&agentCount).Error; err != nil {
		return nil, err
	}

	summary := map[string]interface{}{
		"total_agents": agentCount,
		"cpu_usage":    cpuUsage,
		"memory_usage": memoryUsage,
	}
	return summary, nil
}
