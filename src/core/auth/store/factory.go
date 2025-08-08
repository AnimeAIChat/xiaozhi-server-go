package store

import (
	"fmt"
)

// CreateAuthStore 根据配置创建认证存储实例
func CreateAuthStore(config *StoreConfig) (AuthStore, error) {
	if config == nil {
		return nil, fmt.Errorf("存储配置不能为空")
	}

	switch config.Type {
	case "database", "":
		// 默认使用数据库存储
		expiryHr := config.ExpiryHr
		if expiryHr <= 0 {
			expiryHr = 24 // 默认24小时
		}
		return NewDatabaseAuthStore(expiryHr), nil

	case "memory":
		// 可选：内存存储
		expiryHr := config.ExpiryHr
		if expiryHr <= 0 {
			expiryHr = 24 // 默认24小时
		}
		return NewMemoryAuthStore(expiryHr), nil

	case "file":
		// TODO: 实现文件存储
		return nil, fmt.Errorf("文件存储暂未实现")

	case "redis":
		// TODO: 实现Redis存储
		return nil, fmt.Errorf("Redis存储暂未实现")

	default:
		return nil, fmt.Errorf("不支持的存储类型: %s", config.Type)
	}
}

// ValidateStoreConfig 验证存储配置
func ValidateStoreConfig(config *StoreConfig) error {
	if config == nil {
		return fmt.Errorf("存储配置不能为空")
	}

	if config.Type == "" {
		config.Type = "database" // 默认使用数据库存储
	}

	supportedTypes := []string{"database", "memory", "file", "redis"}
	isSupported := false
	for _, t := range supportedTypes {
		if config.Type == t {
			isSupported = true
			break
		}
	}

	if !isSupported {
		return fmt.Errorf("不支持的存储类型: %s, 支持的类型: %v", config.Type, supportedTypes)
	}

	if config.ExpiryHr < 0 {
		return fmt.Errorf("过期时间不能为负数")
	}

	return nil
}

// GetDefaultStoreConfig 获取默认存储配置
func GetDefaultStoreConfig() *StoreConfig {
	return &StoreConfig{
		Type:     "database",
		Config:   make(map[string]interface{}),
		ExpiryHr: 24,
	}
}
