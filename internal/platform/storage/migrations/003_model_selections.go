package migrations

import (
	"gorm.io/gorm"
)

// Migration003ModelSelections 模型选择表迁移 - 添加模型选择管理表
type Migration003ModelSelections struct{}

func (m *Migration003ModelSelections) Version() string {
	return "003_model_selections"
}

func (m *Migration003ModelSelections) Description() string {
	return "Create model selections table for managing user-selected AI models"
}

func (m *Migration003ModelSelections) Up(db *gorm.DB) error {
	// 创建模型选择表
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS model_selections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id VARCHAR(255) NOT NULL DEFAULT 'admin', -- 用户ID，默认为admin管理员
			asr_provider VARCHAR(255) NOT NULL,           -- 选择的ASR提供商
			tts_provider VARCHAR(255) NOT NULL,           -- 选择的TTS提供商
			llm_provider VARCHAR(255) NOT NULL,           -- 选择的LLM提供商
			vllm_provider VARCHAR(255) NOT NULL,          -- 选择的VLLM提供商
			is_active BOOLEAN DEFAULT TRUE,                -- 是否为活动选择
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(user_id) -- 每个用户只能有一条选择记录
		)
	`).Error; err != nil {
		return err
	}

	// 创建索引
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_model_selections_user_id ON model_selections(user_id)`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_model_selections_active ON model_selections(is_active)`).Error; err != nil {
		return err
	}

	return nil
}

func (m *Migration003ModelSelections) Down(db *gorm.DB) error {
	// 删除表
	if err := db.Exec(`DROP TABLE IF EXISTS model_selections`).Error; err != nil {
		return err
	}

	return nil
}