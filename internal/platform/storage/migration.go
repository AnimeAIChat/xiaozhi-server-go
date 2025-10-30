package storage

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"xiaozhi-server-go/internal/platform/errors"
)

// Migration 数据库迁移接口
type Migration interface {
	Version() string
	Description() string
	Up(db *gorm.DB) error
	Down(db *gorm.DB) error
}

// MigrationRecord 迁移记录
type MigrationRecord struct {
	ID        uint      `gorm:"primaryKey"`
	Version   string    `gorm:"uniqueIndex;not null"`
	Name      string    `gorm:"not null"`
	AppliedAt time.Time `gorm:"not null"`
}

// MigrationManager 迁移管理器
type MigrationManager struct {
	db         *gorm.DB
	migrations []Migration
}

// NewMigrationManager 创建迁移管理器
func NewMigrationManager(db *gorm.DB) *MigrationManager {
	return &MigrationManager{
		db:         db,
		migrations: []Migration{},
	}
}

// AddMigration 添加迁移
func (m *MigrationManager) AddMigration(migration Migration) {
	m.migrations = append(m.migrations, migration)
}

// RunMigrations 执行所有待应用的迁移
func (m *MigrationManager) RunMigrations() error {
	// 创建迁移记录表
	if err := m.db.AutoMigrate(&MigrationRecord{}); err != nil {
		return errors.Wrap(errors.KindStorage, "migration.create_table", "failed to create migration table", err)
	}

	// 获取已应用的迁移
	var appliedVersions []string
	if err := m.db.Model(&MigrationRecord{}).Pluck("version", &appliedVersions).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "migration.get_applied", "failed to get applied migrations", err)
	}

	appliedMap := make(map[string]bool)
	for _, version := range appliedVersions {
		appliedMap[version] = true
	}

	// 执行待应用的迁移
	for _, migration := range m.migrations {
		if appliedMap[migration.Version()] {
			continue // 已应用，跳过
		}

		// 开始事务
		tx := m.db.Begin()
		if tx.Error != nil {
			return errors.Wrap(errors.KindStorage, "migration.begin_tx", "failed to begin transaction", tx.Error)
		}

		// 执行迁移
		if err := migration.Up(tx); err != nil {
			tx.Rollback()
			return errors.Wrap(errors.KindStorage, "migration.up", fmt.Sprintf("failed to run migration %s", migration.Version()), err)
		}

		// 记录迁移
		record := &MigrationRecord{
			Version:   migration.Version(),
			Name:      migration.Description(),
			AppliedAt: time.Now(),
		}
		if err := tx.Create(record).Error; err != nil {
			tx.Rollback()
			return errors.Wrap(errors.KindStorage, "migration.record", "failed to record migration", err)
		}

		// 提交事务
		if err := tx.Commit().Error; err != nil {
			return errors.Wrap(errors.KindStorage, "migration.commit", "failed to commit migration", err)
		}
	}

	return nil
}

// RollbackMigration 回滚指定版本的迁移
func (m *MigrationManager) RollbackMigration(version string) error {
	// 查找迁移记录
	var record MigrationRecord
	if err := m.db.Where("version = ?", version).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return errors.New(errors.KindStorage, "migration.not_found", fmt.Sprintf("migration %s not found", version))
		}
		return errors.Wrap(errors.KindStorage, "migration.find_record", "failed to find migration record", err)
	}

	// 查找对应的迁移
	var targetMigration Migration
	for _, migration := range m.migrations {
		if migration.Version() == version {
			targetMigration = migration
			break
		}
	}

	if targetMigration == nil {
		return errors.New(errors.KindStorage, "migration.not_registered", fmt.Sprintf("migration %s not registered", version))
	}

	// 开始事务
	tx := m.db.Begin()
	if tx.Error != nil {
		return errors.Wrap(errors.KindStorage, "migration.rollback_begin_tx", "failed to begin rollback transaction", tx.Error)
	}

	// 执行回滚
	if err := targetMigration.Down(tx); err != nil {
		tx.Rollback()
		return errors.Wrap(errors.KindStorage, "migration.down", fmt.Sprintf("failed to rollback migration %s", version), err)
	}

	// 删除迁移记录
	if err := tx.Delete(&record).Error; err != nil {
		tx.Rollback()
		return errors.Wrap(errors.KindStorage, "migration.delete_record", "failed to delete migration record", err)
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return errors.Wrap(errors.KindStorage, "migration.rollback_commit", "failed to commit rollback", err)
	}

	return nil
}

// GetMigrationHistory 获取迁移历史
func (m *MigrationManager) GetMigrationHistory() ([]MigrationRecord, error) {
	var records []MigrationRecord
	if err := m.db.Order("applied_at DESC").Find(&records).Error; err != nil {
		return nil, errors.Wrap(errors.KindStorage, "migration.history", "failed to get migration history", err)
	}
	return records, nil
}