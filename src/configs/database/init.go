package database

import (
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
)

// InitDB 根据 DATABASE_URL 自动识别数据库类型并连接
func InitDB() (*gorm.DB, string, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, "", fmt.Errorf("环境变量 DATABASE_URL 未设置")
	}

	var db *gorm.DB
	var err error
	var dbType string

	if strings.HasPrefix(dsn, "mysql://") {
		// mysql://user:pass@tcp(host:port)/dbname?params
		dbType = "mysql"
		// 需要转换成gorm的DSN格式，去掉mysql://前缀
		dsnTrimmed := strings.TrimPrefix(dsn, "mysql://")
		db, err = gorm.Open(mysql.Open(dsnTrimmed), &gorm.Config{})
	} else if strings.HasPrefix(dsn, "postgres://") {
		dbType = "postgres"
		dsnTrimmed := dsn
		db, err = gorm.Open(postgres.Open(dsnTrimmed), &gorm.Config{})
	} else if strings.HasPrefix(dsn, "sqlite://") {
		dbType = "sqlite"
		path := strings.TrimPrefix(dsn, "sqlite://")
		db, err = gorm.Open(sqlite.Open(path), &gorm.Config{})
	} else {
		return nil, "", fmt.Errorf("不支持的数据库类型或DSN格式: %s", dsn)
	}

	if err != nil {
		return nil, "", fmt.Errorf("连接数据库失败: %w", err)
	}

	return db, dbType, nil
}
