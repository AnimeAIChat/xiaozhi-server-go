package database

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

const CODE_PURPOSE_ACTIVITY_DEVICE = "activity_device" // 活动设备

// GenerateVerificationCode 生成六位数验证码
// 支持事务的 GenerateVerificationCode
func GenerateVerificationCode(
	tx *gorm.DB,
	purpose string,
	DeviceID string,
	validHours int,
) (*models.VerificationCode, error) {
	// 生成六位数随机验证码
	code, err := generateSixDigitCode()
	if err != nil {
		return nil, fmt.Errorf("生成验证码失败: %w", err)
	}

	// 确保验证码唯一性
	// 设置最大尝试次数，避免死循环
	maxAttempts := 100
	for {
		if maxAttempts--; maxAttempts <= 0 {
			return nil, fmt.Errorf("无法生成唯一验证码，已达到最大尝试次数")
		}
		var existingCode models.VerificationCode
		if err := tx.Where("code = ? AND is_used = false AND expires_at > ?", code, time.Now()).First(&existingCode).Error; err != nil {
			if err.Error() == "record not found" {
				break // 验证码不存在，可以使用
			}
			return nil, fmt.Errorf("检查验证码唯一性失败: %w", err)
		}
		// 验证码已存在，重新生成
		code, err = generateSixDigitCode()
		if err != nil {
			return nil, fmt.Errorf("重新生成验证码失败: %w", err)
		}
	}

	// 创建验证码记录
	verificationCode := &models.VerificationCode{
		Code:      code,
		Purpose:   purpose,
		DeviceID:  DeviceID,
		ExpiresAt: time.Now().Add(time.Duration(validHours) * time.Hour),
		IsUsed:    false,
	}

	if err := tx.Create(verificationCode).Error; err != nil {
		return nil, fmt.Errorf("保存验证码失败: %w", err)
	}

	return verificationCode, nil
}

// GenerateDeviceCode 生成设备唯一标识码
// 此方法随设备数量增加，可能会变得低效
// 目前假设设备数量不会超过十万级别
// 如果设备数量大幅增加，需要使用更高效的唯一标识生成策略
// 支持事务的 GenerateDeviceCode
func GenerateDeviceCode(tx *gorm.DB) (string, error) {
	// 生成六位数随机设备码
	code, err := generateSixDigitCode()
	if err != nil {
		return "", fmt.Errorf("生成设备码失败: %w", err)
	}

	// 确保设备码唯一性
	// 设置最大尝试次数，避免死循环
	maxAttempts := 100
	for {
		if maxAttempts--; maxAttempts <= 0 {
			return "", fmt.Errorf("无法生成唯一设备码，已达到最大尝试次数")
		}
		var existingDevice models.Device
		if err := tx.Unscoped().Where("device_code = ?", code).First(&existingDevice).Error; err != nil {
			if err.Error() == "record not found" {
				break // 设备码不存在，可以使用
			}
			return "", fmt.Errorf("检查设备码唯一性失败: %w", err)
		}
		// 设备码已存在，重新生成
		code, err = generateSixDigitCode()
		if err != nil {
			return "", fmt.Errorf("重新生成设备码失败: %w", err)
		}
	}

	return code, nil
}

// 支持事务的 CheckCode
func CheckCode(tx *gorm.DB, code string, purpose string) (bool, error) {
	var verificationCode models.VerificationCode

	// 查询验证码是否存在且未使用且未过期
	if err := tx.Where("code = ? AND purpose = ?",
		code, purpose).First(&verificationCode).Error; err != nil {
		if err.Error() == "record not found" {
			return false, nil // 验证码不存在或已使用或已过期
		}
		return false, fmt.Errorf("查询验证码失败: %w", err)
	}

	if verificationCode.IsUsed || verificationCode.ExpiresAt.Before(time.Now()) {
		tx.Delete(&verificationCode) // 删除已使用或过期的验证码
		return false, fmt.Errorf("验证码已使用或已过期")
	}

	return true, nil
}

// 支持事务的 DeleteCode
func DeleteCode(tx *gorm.DB, code string, purpose string) error {
	var verificationCode models.VerificationCode

	// 查询验证码是否存在
	if err := tx.Where("code = ? AND purpose = ?", code, purpose).First(&verificationCode).Error; err != nil {
		if err.Error() == "record not found" {
			return fmt.Errorf("验证码不存在")
		}
		return fmt.Errorf("查询验证码失败: %w", err)
	}

	// 删除验证码记录
	if err := tx.Unscoped().Delete(&verificationCode).Error; err != nil {
		return fmt.Errorf("删除验证码失败: %w", err)
	}

	return nil
}

// VerifyCode 验证验证码
func VerifyCode(tx *gorm.DB, code, purpose string) (*models.VerificationCode, error) {
	var verificationCode models.VerificationCode

	if err := tx.Where("code = ? AND purpose = ? AND is_used = false AND expires_at > ?",
		code, purpose, time.Now()).First(&verificationCode).Error; err != nil {
		return nil, fmt.Errorf("验证码无效或已过期")
	}

	return &verificationCode, nil
}

// UseVerificationCode 使用验证码（标记为已使用）
func UseVerificationCode(tx *gorm.DB, code, purpose, deviceID string) error {
	var verificationCode models.VerificationCode

	// 查询验证码是否存在
	if err := tx.Where("code = ? AND purpose = ?  AND device_id = ?", code, purpose, deviceID).First(&verificationCode).Error; err != nil {
		if err.Error() == "record not found" {
			return fmt.Errorf("验证码不存在")
		}
		return fmt.Errorf("查询验证码失败: %w", err)
	}

	// 删除验证码记录
	if err := tx.Unscoped().Delete(&verificationCode).Error; err != nil {
		return fmt.Errorf("删除验证码失败: %w", err)
	}

	return nil
}

// DeleteExpiredCodes 删除过期的验证码
func DeleteExpiredCodes(tx *gorm.DB) error {
	result := tx.Where("expires_at < ?", time.Now()).Delete(&models.VerificationCode{})
	if result.Error != nil {
		return fmt.Errorf("删除过期验证码失败: %w", result.Error)
	}
	return nil
}

// DeleteUsedCodes 删除已使用的验证码
func DeleteUsedCodes(tx *gorm.DB) error {
	result := tx.Where("is_used = true").Delete(&models.VerificationCode{})
	if result.Error != nil {
		return fmt.Errorf("删除已使用验证码失败: %w", result.Error)
	}
	return nil
}

// generateSixDigitCode 生成六位数随机验证码
func generateSixDigitCode() (string, error) {
	// 生成 100000 到 999999 之间的随机数
	min := big.NewInt(100000)
	max := big.NewInt(899999) // 999999 - 100000 = 899999

	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}

	code := n.Add(n, min)
	return fmt.Sprintf("%06d", code.Int64()), nil
}

// GetUserActiveCodes 获取用户的活跃验证码
func GetUserActiveCodes(tx *gorm.DB, userID string) ([]models.VerificationCode, error) {
	var codes []models.VerificationCode
	if err := tx.Where("user_id = ? AND is_used = false AND expires_at > ?",
		userID, time.Now()).Find(&codes).Error; err != nil {
		return nil, fmt.Errorf("获取用户验证码失败: %w", err)
	}
	return codes, nil
}

// GetDeviceActiveCodes 获取设备的活跃验证码
func GetDeviceActiveCodes(tx *gorm.DB, deviceID string) ([]models.VerificationCode, error) {
	var codes []models.VerificationCode
	if err := tx.Where("device_id = ? AND is_used = false AND expires_at > ?",
		deviceID, time.Now()).Find(&codes).Error; err != nil {
		return nil, fmt.Errorf("获取设备验证码失败: %w", err)
	}
	return codes, nil
}