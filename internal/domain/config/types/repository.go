package types

import "xiaozhi-server-go/internal/platform/config"

// Repository 定义配置存储库接口
type Repository interface {
	// LoadConfig 加载配置，如果不存在则返回默认配置
	LoadConfig() (*config.Config, error)

	// SaveConfig 保存配置
	SaveConfig(config *config.Config) error

	// InitDefaultConfig 初始化默认配置
	InitDefaultConfig() (*config.Config, error)

	// IsInitialized 检查配置是否已初始化
	IsInitialized() (bool, error)
}