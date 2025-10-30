package migrations

import (
	"gorm.io/gorm"
)

// Migration002ConfigTables 配置表迁移 - 添加配置存储表
type Migration002ConfigTables struct{}

func (m *Migration002ConfigTables) Version() string {
	return "002_config_tables"
}

func (m *Migration002ConfigTables) Description() string {
	return "Create configuration tables for database-backed config storage"
}

func (m *Migration002ConfigTables) Up(db *gorm.DB) error {
	// 创建配置记录表
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS config_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key VARCHAR(255) NOT NULL UNIQUE,
			value JSON NOT NULL,
			description TEXT,
			category VARCHAR(255),
			version INTEGER DEFAULT 1,
			is_active BOOLEAN DEFAULT TRUE,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		return err
	}

	// 创建配置快照表
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS config_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(255) NOT NULL,
			version INTEGER NOT NULL,
			data JSON NOT NULL,
			comment TEXT,
			created_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		return err
	}

	// 创建索引
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_config_records_key ON config_records(key)`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_config_records_category ON config_records(category)`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_config_records_active ON config_records(is_active)`).Error; err != nil {
		return err
	}

	return nil
}

func (m *Migration002ConfigTables) Down(db *gorm.DB) error {
	// 删除表
	if err := db.Exec(`DROP TABLE IF EXISTS config_snapshots`).Error; err != nil {
		return err
	}
	if err := db.Exec(`DROP TABLE IF EXISTS config_records`).Error; err != nil {
		return err
	}

	return nil
}