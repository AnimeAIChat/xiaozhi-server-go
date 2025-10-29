package types

import "xiaozhi-server-go/src/configs"

// Repository 定义配置存储库接口
type Repository interface {
	// LoadConfig 加载配置，如果不存在则返回默认配置
	LoadConfig() (*configs.Config, error)

	// SaveConfig 保存配置
	SaveConfig(config *configs.Config) error

	// InitDefaultConfig 初始化默认配置
	InitDefaultConfig() (*configs.Config, error)

	// IsInitialized 检查配置是否已初始化
	IsInitialized() (bool, error)
}