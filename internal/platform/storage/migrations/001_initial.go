package migrations

import (
	"gorm.io/gorm"
)

// Migration001Initial 初始迁移 - 创建基础表结构
type Migration001Initial struct{}

func (m *Migration001Initial) Version() string {
	return "001_initial"
}

func (m *Migration001Initial) Description() string {
	return "Create initial database schema with all core tables"
}

func (m *Migration001Initial) Up(db *gorm.DB) error {
	// 注意：这里使用原生的SQL创建表，因为GORM的AutoMigrate可能不适合在迁移中使用
	// 在实际项目中，应该使用具体的SQL语句来精确控制表结构

	// 创建认证客户端表
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS auth_clients (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			client_id VARCHAR(255) NOT NULL UNIQUE,
			username VARCHAR(255) NOT NULL,
			password VARCHAR(255) NOT NULL,
			ip VARCHAR(255),
			device_id VARCHAR(255),
			created_at DATETIME NOT NULL,
			expires_at DATETIME,
			metadata JSON
		)
	`).Error; err != nil {
		return err
	}

	// 创建领域事件表
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS domain_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type VARCHAR(255) NOT NULL,
			session_id VARCHAR(255),
			user_id VARCHAR(255),
			data JSON NOT NULL,
			created_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		return err
	}

	// 创建索引
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_domain_events_event_type ON domain_events(event_type)`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_domain_events_session_id ON domain_events(session_id)`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_domain_events_user_id ON domain_events(user_id)`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_domain_events_created_at ON domain_events(created_at)`).Error; err != nil {
		return err
	}

	// 这里可以继续添加其他表的创建SQL
	// 为了简化，我们先只创建核心表

	return nil
}

func (m *Migration001Initial) Down(db *gorm.DB) error {
	// 删除表
	if err := db.Exec(`DROP TABLE IF EXISTS domain_events`).Error; err != nil {
		return err
	}
	if err := db.Exec(`DROP TABLE IF EXISTS auth_clients`).Error; err != nil {
		return err
	}

	return nil
}