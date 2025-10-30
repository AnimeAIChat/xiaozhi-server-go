package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"xiaozhi-server-go/internal/platform/storage/migrations"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitConfigStore ensures the underlying configuration store is ready.
// Since we no longer use database-backed configuration, this is a no-op.
func InitConfigStore() error {
	return nil
}

// ConfigStore returns the default configuration store implementation.
// Since we no longer use database-backed configuration, this returns nil.
func ConfigStore() interface{} {
	return nil
}

// Global database instance for backward compatibility
var db *gorm.DB

// InitDatabase initializes the SQLite database for authentication and other services.
func InitDatabase() error {
	if db != nil {
		return nil
	}

	// Create data directory if it doesn't exist
	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "xiaozhi.db")

	var err error
	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Auto-migrate tables (fallback for backward compatibility)
	if err := db.AutoMigrate(&AuthClient{}, &DomainEvent{}, &ConfigRecord{}, &ConfigSnapshot{}); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Run migrations
	migrationManager := NewMigrationManager(db)
	migrationManager.AddMigration(&migrations.Migration001Initial{})
	migrationManager.AddMigration(&migrations.Migration002ConfigTables{})

	if err := migrationManager.RunMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// GetDB returns the global database instance.
func GetDB() *gorm.DB {
	if db == nil {
		panic("database not initialized, call InitDatabase() first")
	}
	return db
}

// AuthClient represents the authentication client model for GORM
type AuthClient struct {
	ID        uint           `gorm:"primaryKey"`
	ClientID  string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"client_id"`
	Username  string         `gorm:"not null"                               json:"username"`
	Password  string         `gorm:"not null"                               json:"password"`
	IP        string         `                                              json:"ip"`
	DeviceID  string         `                                              json:"device_id"`
	CreatedAt time.Time      `                                              json:"created_at"`
	ExpiresAt *time.Time     `                                              json:"expires_at,omitempty"`
	Metadata  datatypes.JSON `                                              json:"metadata,omitempty"`
}

// DomainEvent 领域事件存储模型
type DomainEvent struct {
	ID        uint           `gorm:"primaryKey"`
	EventType string         `gorm:"index;not null"` // 事件类型
	SessionID string         `gorm:"index"`          // 会话ID
	UserID    string         `gorm:"index"`          // 用户ID
	Data      datatypes.JSON `gorm:"not null"`       // 事件数据
	CreatedAt time.Time      `gorm:"index"`          // 创建时间
}

// Agent 智能体模型
type Agent struct {
	ID                 uint           `gorm:"primaryKey"`
	Name               string         `gorm:"not null"`
	LLM                string         `gorm:"default:'ChatGLMLLM'"`
	Language           string         `gorm:"default:'普通话'"`
	Voice              string         `gorm:"default:'zh_female_wanwanxiaohe_moon_bigtts'"`
	VoiceName          string         `gorm:"default:'湾湾小何'"`
	Prompt             string         `gorm:"type:text"`
	ASRSpeed           int            `gorm:"default:2"`
	SpeakSpeed         int            `gorm:"default:2"`
	Tone               int            `gorm:"default:50"`
	UserID             uint           `gorm:"not null"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastConversationAt time.Time
	EnabledTools       string         `gorm:"type:text"`
	Conversationid     string
	HeadImg            string         `gorm:"type:varchar(255)"`
	Description        string         `gorm:"type:text"`
	CatalogyID         uint
	Extra              string         `gorm:"type:text"`
}

// AgentDialog 智能体对话模型
type AgentDialog struct {
	ID             uint      `gorm:"primaryKey"`
	Conversationid string
	AgentID        uint      `gorm:"index"`
	UserID         uint      `gorm:"index"`
	Dialog         string    `gorm:"type:text"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Device 设备模型
type Device struct {
	ID               uint           `gorm:"primaryKey"`
	AgentID          *uint          `gorm:"index"`
	UserID           *uint          `gorm:"index"`
	Name             string         `gorm:"not null"`
	DeviceID         string         `gorm:"type:varchar(255);uniqueIndex;not null"`
	ClientID         string         `gorm:"type:varchar(255);uniqueIndex;not null"`
	Version          string
	OTA              bool           `gorm:"default:true"`
	RegisterTime     int64
	LastActiveTime   int64
	RegisterTimeV2   time.Time
	LastActiveTimeV2 time.Time
	Online           bool
	AuthCode         string
	AuthStatus       string
	BoardType        string
	ChipModelName    string
	Channel          int
	SSID             string
	Application      string
	Language         string         `gorm:"default:'zh-CN'"`
	DeviceCode       string
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	Extra            string         `gorm:"type:text"`
	Conversationid   string
	Mode             string
}

// User 用户模型
type User struct {
	ID          uint      `gorm:"primaryKey"`
	Username    string    `gorm:"type:varchar(255);uniqueIndex;not null"`
	Password    string    `json:"-"`
	Nickname    string    `gorm:"type:varchar(255)"`
	HeadImg     string    `gorm:"type:varchar(255)"`
	Role        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Email       string    `gorm:"type:varchar(255);uniqueIndex;"`
	Status      uint      `gorm:"default:1"`
	PhoneNumber string    `gorm:"type:varchar(20);"`
	Extra       string    `gorm:"type:text"`
}

// ServerConfig 服务器配置模型
type ServerConfig struct {
	ID     uint   `gorm:"primaryKey"`
	CfgStr string `gorm:"type:text"`
}

// ServerStatus 服务器状态模型
type ServerStatus struct {
	ID               uint      `gorm:"primaryKey"`
	OnlineDeviceNum  int
	OnlineSessionNum int
	CPUUsage         string
	MemoryUsage      string
	UpdatedAt        time.Time
}
