package manager

import (
	"xiaozhi-server-go/internal/domain/config/types"
	"xiaozhi-server-go/internal/platform/config"
)

// DatabaseRepository 基于数据库的配置存储库实现
// Note: Since we removed database-backed configuration, this is now a simple passthrough
type DatabaseRepository struct{}

// NewDatabaseRepository 创建新的数据库配置存储库
func NewDatabaseRepository(db interface{}) types.Repository {
	return &DatabaseRepository{}
}

// LoadConfig 加载配置
func (r *DatabaseRepository) LoadConfig() (*config.Config, error) {
	// Return default config since we no longer use database storage
	return config.DefaultConfig(), nil
}

// SaveConfig 保存配置
func (r *DatabaseRepository) SaveConfig(config *config.Config) error {
	// No-op since we don't save to database anymore
	return nil
}

// InitDefaultConfig 初始化默认配置
func (r *DatabaseRepository) InitDefaultConfig() (*config.Config, error) {
	return config.DefaultConfig(), nil
}

// IsInitialized 检查配置是否已初始化
func (r *DatabaseRepository) IsInitialized() (bool, error) {
	return true, nil
}