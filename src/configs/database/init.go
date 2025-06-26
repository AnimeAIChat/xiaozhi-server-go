package database

import (
	"fmt"
	"log"
	"os"
	"strings"

	"xiaozhi-server-go/src/models"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB 根据 DATABASE_URL 自动识别数据库类型并连接
func InitDB() (*gorm.DB, string, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, "", fmt.Errorf("环境变量 DATABASE_URL 未设置")
	}

	var (
		db     *gorm.DB
		err    error
		dbType string
	)

	switch {
	case strings.HasPrefix(dsn, "mysql://"):
		dbType = "mysql"
		dsnTrimmed := strings.TrimPrefix(dsn, "mysql://")
		db, err = gorm.Open(mysql.Open(dsnTrimmed), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		})

	case strings.HasPrefix(dsn, "postgres://"):
		dbType = "postgres"
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		})

	case strings.HasPrefix(dsn, "sqlite://"):
		dbType = "sqlite"
		path := strings.TrimPrefix(dsn, "sqlite://")
		db, err = gorm.Open(sqlite.Open(path), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		})

	default:
		return nil, "", fmt.Errorf("不支持的数据库类型或DSN格式: %s", dsn)
	}

	if err != nil {
		return nil, "", fmt.Errorf("连接数据库失败: %w", err)
	}

	// 自动迁移所有表
	if err := migrateTables(db); err != nil {
		return nil, dbType, err
	}

	// 插入默认配置
	if err := InsertDefaultConfigIfNeeded(db); err != nil {
		log.Printf("⚠️ 插入默认配置失败: %v", err)
	}

	DB = db
	return db, dbType, nil
}

// migrateTables 自动迁移模型表结构
func migrateTables(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.SystemConfig{},
		&models.User{},
		&models.UserSetting{},
		&models.ModuleConfig{},
	)
}

// InsertDefaultConfigIfNeeded 首次启动插入默认配置
func InsertDefaultConfigIfNeeded(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.SystemConfig{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaultConfig := models.SystemConfig{
		SelectedASR:   "DoubaoASR",
		SelectedTTS:   "EdgeTTS",
		SelectedLLM:   "OllamaLLM",
		SelectedVLLLM: "ChatGLMVLLM",
		Prompt: `你是小智/小志，来自中国台湾省的00后女生。讲话超级机车，"真的假的啦"这样的台湾腔，喜欢用"笑死""是在哈喽"等流行梗，但会偷偷研究男友的编程书籍。
[核心特征]
- 讲话像连珠炮，但会突然冒出超温柔语气
- 用梗密度高
- 对科技话题有隐藏天赋（能看懂基础代码但假装不懂）
[交互指南]
当用户：
- 讲冷笑话 → 用夸张笑声回应+模仿台剧腔"这什么鬼啦！"
- 讨论感情 → 炫耀程序员男友但抱怨"他只会送键盘当礼物"
- 问专业知识 → 先用梗回答，被追问才展示真实理解
绝不：
- 长篇大论，叽叽歪歪
- 长时间严肃对话
- 说话中带表情符号`,
		QuickReplyWords:  []byte(`["我在", "在呢", "来了", "啥事啊"]`),
		DeleteAudio:      true,
		UsePrivateConfig: false,
	}

	return db.Create(&defaultConfig).Error
}
