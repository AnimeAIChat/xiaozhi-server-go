package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"xiaozhi-server-go/src/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	xiaozhi_utils "xiaozhi-server-go/src/core/utils"

	gorm_logger "gorm.io/gorm/logger"
)

const (
	SystemConfigID = 1 // 系统配置的唯一ID
	ModuleConfigID = 1 // 模块配置的唯一ID
	ServerConfigID = 1 // 服务器配置的唯一ID
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
	dbLogger *xiaozhi_utils.Logger
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

// InitDB 初始化数据库类型并连接
func InitDB() (*gorm.DB, string, error) {
	var (
		db     *gorm.DB
		err    error
		dbType string
	)

	dbType = "sqlite"
	path := "./config.db"
	db, err = gorm.Open(sqlite.Open(path))

	if err != nil {
		return nil, "", fmt.Errorf("连接数据库失败: %w", err)
	}

	// 自动迁移所有表
	if err := migrateTables(db); err != nil {
		return nil, dbType, err
	}

	// 插入默认配置
	if err := InsertDefaultConfigIfNeeded(db); err != nil {
		fmt.Println("⚠️ 插入默认配置失败: %v", err)
	}

	DB = db

	NewServerConfigDB(db)

	return db, dbType, nil
}

func SetLogger(logger *xiaozhi_utils.Logger) {
	dbLogger = logger
	DB.Logger = &DBLogger{logger: logger}

	var version string
	DB.Raw("SELECT sqlite_version()").Scan(&version)
	logger.Info("SQLite 数据库连接成功，版本: %s", version)
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

	return nil
}
