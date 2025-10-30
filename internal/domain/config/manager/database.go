package manager

import (
	"encoding/json"
	"fmt"

	"xiaozhi-server-go/internal/domain/config/types"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/platform/errors"
	"xiaozhi-server-go/internal/platform/storage"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DatabaseRepository 基于数据库的配置存储库实现
type DatabaseRepository struct {
	db *gorm.DB
}

// NewDatabaseRepository 创建新的数据库配置存储库
func NewDatabaseRepository(db interface{}) types.Repository {
	if db == nil {
		return &DatabaseRepository{db: storage.GetDB()}
	}
	if gormDB, ok := db.(*gorm.DB); ok {
		return &DatabaseRepository{db: gormDB}
	}
	return &DatabaseRepository{db: storage.GetDB()}
}

// LoadConfig 加载配置
func (r *DatabaseRepository) LoadConfig() (*config.Config, error) {
	// 首先尝试从数据库加载配置
	cfg, err := r.loadConfigFromDB()
	if err != nil {
		// 如果数据库加载失败，返回默认配置
		return config.DefaultConfig(), nil
	}

	if cfg != nil {
		return cfg, nil
	}

	// 如果数据库中没有配置，初始化默认配置
	return r.InitDefaultConfig()
}

// SaveConfig 保存配置
func (r *DatabaseRepository) SaveConfig(cfg *config.Config) error {
	if cfg == nil {
		return errors.Wrap(errors.KindDomain, "config.save", "config cannot be nil", nil)
	}

	// 将配置转换为键值对并保存到数据库
	configMap, err := r.configToMap(cfg)
	if err != nil {
		return errors.Wrap(errors.KindDomain, "config.save", "failed to convert config to map", err)
	}

	// 使用事务确保原子性
	tx := r.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 先将所有现有配置标记为非活跃
	if err := tx.Exec("UPDATE config_records SET is_active = ?", false).Error; err != nil {
		tx.Rollback()
		return errors.Wrap(errors.KindStorage, "config.save", "failed to deactivate existing configs", err)
	}

	// 删除所有非活跃的配置记录，避免唯一约束冲突
	if err := tx.Exec("DELETE FROM config_records WHERE is_active = ?", false).Error; err != nil {
		tx.Rollback()
		return errors.Wrap(errors.KindStorage, "config.save", "failed to delete inactive configs", err)
	}

	// 保存新的配置记录
	for key, value := range configMap {
		category := r.getCategoryFromKey(key)
		description := r.getDescriptionFromKey(key)

		// Convert value to JSON string
		valueJSON, err := json.Marshal(value)
		if err != nil {
			tx.Rollback()
			return errors.Wrap(errors.KindStorage, "config.save", fmt.Sprintf("failed to marshal value for key %s", key), err)
		}

		record := storage.ConfigRecord{
			Key:         key,
			Value:       datatypes.JSON(valueJSON),
			Description: description,
			Category:    category,
			Version:     1,
			IsActive:    true,
		}

		if err := tx.Create(&record).Error; err != nil {
			tx.Rollback()
			return errors.Wrap(errors.KindStorage, "config.save", fmt.Sprintf("failed to save config record for key %s", key), err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return errors.Wrap(errors.KindStorage, "config.save", "failed to commit transaction", err)
	}

	return nil
}

// InitDefaultConfig 初始化默认配置
func (r *DatabaseRepository) InitDefaultConfig() (*config.Config, error) {
	defaultCfg := config.DefaultConfig()
	if err := r.SaveConfig(defaultCfg); err != nil {
		return nil, errors.Wrap(errors.KindDomain, "config.init", "failed to save default config", err)
	}

	// 初始化默认的模型选择
	modelSelectionManager := NewModelSelectionManager(r.db)
	if err := modelSelectionManager.InitDefaultSelection(1); err != nil { // 使用管理员用户ID 1
		return nil, errors.Wrap(errors.KindDomain, "config.init", "failed to init default model selection", err)
	}

	return defaultCfg, nil
}

// IsInitialized 检查配置是否已初始化
func (r *DatabaseRepository) IsInitialized() (bool, error) {
	var count int64
	if err := r.db.Model(&storage.ConfigRecord{}).Where("is_active = ?", true).Count(&count).Error; err != nil {
		return false, errors.Wrap(errors.KindStorage, "config.check_init", "failed to check config initialization", err)
	}
	return count > 0, nil
}

// loadConfigFromDB 从数据库加载配置
func (r *DatabaseRepository) loadConfigFromDB() (*config.Config, error) {
	// 使用原生 SQL 查询来避免 datatypes.JSON 的自动解析问题
	rows, err := r.db.Raw("SELECT key, value FROM config_records WHERE is_active = ?", true).Rows()
	if err != nil {
		return nil, errors.Wrap(errors.KindStorage, "config.load", "failed to query config records", err)
	}
	defer rows.Close()

	configMap := make(map[string]interface{})

	for rows.Next() {
		var key string
		var valueStr string
		if err := rows.Scan(&key, &valueStr); err != nil {
			continue // 跳过无法解析的记录
		}

		var value interface{}
		if err := json.Unmarshal([]byte(valueStr), &value); err != nil {
			continue // 跳过无法解析的记录
		}
		configMap[key] = value
	}

	if len(configMap) == 0 {
		return nil, nil // 没有配置记录
	}

	// 调试：打印展平的配置映射中的 ASR 相关键
	// fmt.Printf("DEBUG: Flattened config map ASR keys:\n")
	// for key, value := range configMap {
	// 	if strings.HasPrefix(key, "ASR.") {
	// 		fmt.Printf("  %s = %v (type: %T)\n", key, value, value)
	// 	}
	// }

	// 将展平的映射重新构建为嵌套结构
	nested := r.unflattenMap(configMap)

	// 调试：打印重建后的嵌套结构
	// if asrSection, ok := nested["ASR"]; ok {
	// 	asrJSON, _ := json.MarshalIndent(asrSection, "", "  ")
	// 	fmt.Printf("DEBUG: Reconstructed nested config ASR section:\n%s\n", string(asrJSON))
	// } else {
	// 	fmt.Printf("DEBUG: ASR section not found in nested config\n")
	// }

	data, err := json.Marshal(nested)
	if err != nil {
		return nil, err
	}

	// fmt.Printf("DEBUG: Final JSON data for unmarshaling:\n%s\n", string(data))

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// fmt.Printf("DEBUG: Unmarshaled config ASR field: %v\n", cfg.ASR)

	return &cfg, nil
}

// configToMap 将配置对象转换为键值对映射
func (r *DatabaseRepository) configToMap(cfg *config.Config) (map[string]interface{}, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal(data, &configMap); err != nil {
		return nil, err
	}

	// 展平嵌套结构为键值对
	flattened := make(map[string]interface{})
	r.flattenMap("", configMap, flattened)
	return flattened, nil
}

// flattenMap 将嵌套映射展平为键值对
func (r *DatabaseRepository) flattenMap(prefix string, src map[string]interface{}, dst map[string]interface{}) {
	for key, value := range src {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		if nested, ok := value.(map[string]interface{}); ok {
			r.flattenMap(fullKey, nested, dst)
		} else {
			dst[fullKey] = value
		}
	}
}

// unflattenMap 将展平的键值对重新构建为嵌套映射
func (r *DatabaseRepository) unflattenMap(src map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range src {
		r.setNestedValue(result, key, value)
	}

	return result
}

// setNestedValue 在嵌套映射中设置值
func (r *DatabaseRepository) setNestedValue(m map[string]interface{}, key string, value interface{}) {
	parts := r.splitKey(key)
	current := m

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
		} else {
			if current[part] == nil {
				current[part] = make(map[string]interface{})
			}
			if nested, ok := current[part].(map[string]interface{}); ok {
				current = nested
			} else {
				newMap := make(map[string]interface{})
				current[part] = newMap
				current = newMap
			}
		}
	}
}

// splitKey 按点分割键
func (r *DatabaseRepository) splitKey(key string) []string {
	// 简单的点分割，实际实现中可能需要处理转义
	var parts []string
	var current string

	for _, char := range key {
		if char == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// getCategoryFromKey 从键获取分类
func (r *DatabaseRepository) getCategoryFromKey(key string) string {
	parts := r.splitKey(key)
	if len(parts) > 0 {
		return parts[0]
	}
	return "general"
}

// getDescriptionFromKey 从键获取描述
func (r *DatabaseRepository) getDescriptionFromKey(key string) string {
	descriptions := map[string]string{
		"server":        "服务器配置",
		"log":           "日志配置",
		"web":           "Web服务配置",
		"transport":     "传输层配置",
		"system":        "系统配置",
		"audio":         "音频配置",
		"pool":          "连接池配置",
		"mcp_pool":      "MCP连接池配置",
		"quick_reply":   "快速回复配置",
		"local_mcp_fun": "本地MCP函数配置",
		"asr":           "语音识别配置",
		"tts":           "语音合成配置",
		"llm":           "大语言模型配置",
		"vllm":          "视觉语言模型配置",
		"mcp":           "模型上下文协议配置",
		"selected":      "选中的服务配置",
	}

	category := r.getCategoryFromKey(key)
	if desc, ok := descriptions[category]; ok {
		return desc
	}
	return fmt.Sprintf("%s 配置", category)
}