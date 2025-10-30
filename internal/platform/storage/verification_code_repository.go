package storage

import (
	"context"

	"gorm.io/gorm"

	"xiaozhi-server-go/internal/domain/device/aggregate"
	"xiaozhi-server-go/internal/domain/device/repository"
	"xiaozhi-server-go/internal/platform/errors"
)

// verificationCodeRepository 验证码仓库实现
type verificationCodeRepository struct {
	db *gorm.DB
}

// NewVerificationCodeRepository 创建验证码仓库实例
func NewVerificationCodeRepository(db *gorm.DB) repository.VerificationCodeRepository {
	return &verificationCodeRepository{
		db: db,
	}
}

// Save 保存验证码
func (r *verificationCodeRepository) Save(ctx context.Context, code *aggregate.VerificationCode) error {
	model := r.toModel(code)
	if err := r.db.WithContext(ctx).Save(model).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "verification_code.save", "failed to save verification code", err)
	}
	return nil
}

// FindByCode 根据验证码查找
func (r *verificationCodeRepository) FindByCode(ctx context.Context, code string, purpose aggregate.VerificationCodePurpose) (*aggregate.VerificationCode, error) {
	var model VerificationCode
	if err := r.db.WithContext(ctx).Where("code = ? AND purpose = ?", code, purpose).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errors.New(errors.KindDomain, "verification_code.find_by_code", "verification code not found")
		}
		return nil, errors.Wrap(errors.KindStorage, "verification_code.find_by_code", "failed to find verification code", err)
	}
	return r.fromModel(&model), nil
}

// FindByDeviceID 根据设备ID查找验证码
func (r *verificationCodeRepository) FindByDeviceID(ctx context.Context, deviceID string, purpose aggregate.VerificationCodePurpose) ([]*aggregate.VerificationCode, error) {
	var models []VerificationCode
	if err := r.db.WithContext(ctx).Where("device_id = ? AND purpose = ?", deviceID, purpose).Find(&models).Error; err != nil {
		return nil, errors.Wrap(errors.KindStorage, "verification_code.find_by_device_id", "failed to find verification codes", err)
	}

	codes := make([]*aggregate.VerificationCode, len(models))
	for i, model := range models {
		codes[i] = r.fromModel(&model)
	}
	return codes, nil
}

// Delete 删除验证码
func (r *verificationCodeRepository) Delete(ctx context.Context, code string, purpose aggregate.VerificationCodePurpose) error {
	if err := r.db.WithContext(ctx).Where("code = ? AND purpose = ?", code, purpose).Delete(&VerificationCode{}).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "verification_code.delete", "failed to delete verification code", err)
	}
	return nil
}

// DeleteExpired 删除过期的验证码
func (r *verificationCodeRepository) DeleteExpired(ctx context.Context) error {
	if err := r.db.WithContext(ctx).Where("expires_at < ? AND is_used = ?", "NOW()", false).Delete(&VerificationCode{}).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "verification_code.delete_expired", "failed to delete expired codes", err)
	}
	return nil
}

// toModel 将领域对象转换为存储模型
func (r *verificationCodeRepository) toModel(code *aggregate.VerificationCode) *VerificationCode {
	model := &VerificationCode{
		ID:        uint(code.ID),
		Code:      code.Code,
		Purpose:   string(code.Purpose),
		ExpiresAt: code.ExpiresAt,
		IsUsed:    code.IsUsed,
		CreatedAt: code.CreatedAt,
		UpdatedAt: code.UpdatedAt,
	}

	if code.UserID != nil {
		model.UserID = code.UserID
	}
	if code.DeviceID != nil {
		model.DeviceID = code.DeviceID
	}
	if code.UsedAt != nil {
		model.UsedAt = code.UsedAt
	}

	return model
}

// fromModel 将存储模型转换为领域对象
func (r *verificationCodeRepository) fromModel(model *VerificationCode) *aggregate.VerificationCode {
	code := &aggregate.VerificationCode{
		ID:        int(model.ID),
		Code:      model.Code,
		Purpose:   aggregate.VerificationCodePurpose(model.Purpose),
		ExpiresAt: model.ExpiresAt,
		IsUsed:    model.IsUsed,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}

	if model.UserID != nil {
		code.UserID = model.UserID
	}
	if model.DeviceID != nil {
		code.DeviceID = model.DeviceID
	}
	if model.UsedAt != nil {
		code.UsedAt = model.UsedAt
	}

	return code
}