package storage

import (
	"fmt"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
)

// InitConfigStore ensures the underlying configuration store is ready.
func InitConfigStore() error {
	_, _, err := database.InitDB()
	if err != nil {
		return fmt.Errorf("初始化配置存储失败: %w", err)
	}
	return nil
}

// ConfigStore returns the default configuration store implementation.
func ConfigStore() configs.ConfigDBInterface {
	return database.GetServerConfigDB()
}
