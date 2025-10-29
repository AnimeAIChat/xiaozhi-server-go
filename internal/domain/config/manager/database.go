package manager

import (
	"fmt"

	"xiaozhi-server-go/internal/domain/config/types"
	"xiaozhi-server-go/src/configs"
)

// DatabaseRepository 基于数据库的配置存储库实现
type DatabaseRepository struct {
	db configs.ConfigDBInterface
}

// NewDatabaseRepository 创建新的数据库配置存储库
func NewDatabaseRepository(db configs.ConfigDBInterface) types.Repository {
	return &DatabaseRepository{
		db: db,
	}
}

// LoadConfig 加载配置
func (r *DatabaseRepository) LoadConfig() (*configs.Config, error) {
	configStr, err := r.db.LoadServerConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config from database: %w", err)
	}

	config := &configs.Config{}
	if configStr != "" {
		err = config.FromString(configStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
		return config, nil
	}

	// 如果数据库为空，返回默认配置
	return r.InitDefaultConfig()
}

// SaveConfig 保存配置
func (r *DatabaseRepository) SaveConfig(config *configs.Config) error {
	configStr := config.ToString()
	return r.db.UpdateServerConfig(configStr)
}

// InitDefaultConfig 初始化默认配置
func (r *DatabaseRepository) InitDefaultConfig() (*configs.Config, error) {
	defaultConfig := configs.NewDefaultInitConfig()
	configStr := defaultConfig.ToString()

	err := r.db.InitServerConfig(configStr)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize default config: %w", err)
	}

	return defaultConfig, nil
}

// IsInitialized 检查配置是否已初始化
func (r *DatabaseRepository) IsInitialized() (bool, error) {
	configStr, err := r.db.LoadServerConfig()
	if err != nil {
		return false, err
	}
	return configStr != "", nil
}