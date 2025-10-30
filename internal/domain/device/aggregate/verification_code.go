package aggregate

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"xiaozhi-server-go/internal/platform/errors"
)

// VerificationCodePurpose 验证码用途
type VerificationCodePurpose string

const (
	CodePurposeDeviceActivation VerificationCodePurpose = "activity_device" // 设备激活
)

// VerificationCode 验证码聚合
type VerificationCode struct {
	ID        int                    `json:"id"`
	Code      string                 `json:"code"`
	Purpose   VerificationCodePurpose `json:"purpose"`
	UserID    *string                `json:"userId,omitempty"`
	DeviceID  *string                `json:"deviceId,omitempty"`
	ExpiresAt time.Time              `json:"expiresAt"`
	UsedAt    *time.Time             `json:"usedAt,omitempty"`
	IsUsed    bool                   `json:"isUsed"`
	CreatedAt time.Time              `json:"createdAt"`
	UpdatedAt time.Time              `json:"updatedAt"`
}

// NewVerificationCode 创建新的验证码
func NewVerificationCode(purpose VerificationCodePurpose, deviceID string, validHours int) (*VerificationCode, error) {
	code, err := generateSixDigitCode()
	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "verification_code.new", "failed to generate code", err)
	}

	now := time.Now()
	return &VerificationCode{
		Code:      code,
		Purpose:   purpose,
		DeviceID:  &deviceID,
		ExpiresAt: now.Add(time.Duration(validHours) * time.Hour),
		IsUsed:    false,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// IsExpired 检查验证码是否过期
func (vc *VerificationCode) IsExpired() bool {
	return time.Now().After(vc.ExpiresAt)
}

// IsValid 检查验证码是否有效
func (vc *VerificationCode) IsValid() bool {
	return !vc.IsUsed && !vc.IsExpired()
}

// Use 使用验证码
func (vc *VerificationCode) Use() error {
	if !vc.IsValid() {
		return errors.New(errors.KindDomain, "verification_code.use", "code is not valid")
	}

	now := time.Now()
	vc.UsedAt = &now
	vc.IsUsed = true
	vc.UpdatedAt = now
	return nil
}

// MatchesDevice 检查验证码是否匹配指定设备
func (vc *VerificationCode) MatchesDevice(deviceID string) bool {
	return vc.DeviceID != nil && *vc.DeviceID == deviceID
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