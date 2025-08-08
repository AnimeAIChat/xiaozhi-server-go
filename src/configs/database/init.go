package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/models"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"xiaozhi-server-go/src/core/utils"
	xiaozhi_utils "xiaozhi-server-go/src/core/utils"

	gorm_logger "gorm.io/gorm/logger"
)

const (
	SystemConfigID = 1 // 系统配置的唯一ID
	ModuleConfigID = 1 // 模块配置的唯一ID
)

type DBLogger struct {
	logger *xiaozhi_utils.Logger
}

func (l *DBLogger) LogMode(level gorm_logger.LogLevel) gorm_logger.Interface {
	return &DBLogger{
		logger: l.logger,
	}
}

func (l *DBLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	l.logger.Info(msg, data...)
}

func (l *DBLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	l.logger.Warn(msg, data...)
}

func (l *DBLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	l.logger.Error(msg, data...)
}

func (l *DBLogger) Trace(
	ctx context.Context,
	begin time.Time,
	fc func() (sql string, rowsAffected int64),
	err error,
) {
	sql, rows := fc()
	elapsed := time.Since(begin)
	if err != nil {
		if strings.Contains(err.Error(), "record not found") {
			// 忽略记录未找到的错误
			return
		}
		l.logger.Error("SQL Trace Error", map[string]interface{}{
			"sql":     sql,
			"rows":    rows,
			"elapsed": elapsed,
			"err":     err,
		})
	} else {
		l.logger.Debug("SQL Trace", map[string]interface{}{
			"sql":     sql,
			"rows":    rows,
			"elapsed": elapsed,
		})
	}
}

var (
	DB       *gorm.DB
	dbLogger *utils.Logger
)

func GetDB() *gorm.DB {
	if DB == nil {
		panic("数据库未初始化，请先调用 InitDB()")
	}
	return DB
}

// GetTxDB 获取一个新的事务DB（需手动Commit或Rollback）
func GetTxDB() *gorm.DB {
	if DB == nil {
		panic("数据库未初始化，请先调用 InitDB()")
	}
	return DB.Begin()
}

// InitDB 根据 DATABASE_URL 自动识别数据库类型并连接
func InitDB(logger *xiaozhi_utils.Logger, config *configs.Config) (*gorm.DB, string, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, "", fmt.Errorf("环境变量 DATABASE_URL 未设置")
	}

	var (
		db     *gorm.DB
		err    error
		dbType string
		lg     DBLogger
	)
	lg.logger = logger
	dbLogger = logger

	switch {
	case strings.HasPrefix(dsn, "mysql://"):
		dbType = "mysql"
		dsnTrimmed := strings.TrimPrefix(dsn, "mysql://")
		db, err = gorm.Open(mysql.Open(dsnTrimmed), &gorm.Config{
			Logger: &lg,
		})

	case strings.HasPrefix(dsn, "postgres://"):
		dbType = "postgres"
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: &lg,
		})

	case strings.HasPrefix(dsn, "sqlite://"):
		dbType = "sqlite"
		path := strings.TrimPrefix(dsn, "sqlite://")
		db, err = gorm.Open(sqlite.Open(path), &gorm.Config{
			Logger: &lg,
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
	if err := InsertDefaultConfigIfNeeded(db, config); err != nil {
		log.Printf("⚠️ 插入默认配置失败: %v", err)
	}

	DB = db

	// 打印数据库连接成功信息
	switch dbType {
	case "mysql":
		var version string
		db.Raw("SELECT VERSION()").Scan(&version)
		logger.Info("MySQL 数据库连接成功，版本: %s", version)
	case "postgres":
		var version string
		db.Raw("SELECT version()").Scan(&version)
		logger.Info("PostgreSQL 数据库连接成功，版本: %s", version)
	case "sqlite":
		var version string
		db.Raw("SELECT sqlite_version()").Scan(&version)
		logger.Info("SQLite 数据库连接成功，版本: %s", version)
	default:
		logger.Info("数据库连接成功，未识别的数据库类型")
	}
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
func InsertDefaultConfigIfNeeded(db *gorm.DB, config *configs.Config) error {
	if err := InitSystemConfig(db, config); err != nil {
		return fmt.Errorf("初始化系统配置失败: %v", err)
	}
	return nil
}
