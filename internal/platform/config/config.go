package config

import (
	"time"
)

type Config struct {
	Server        ServerConfig
	Log           LogConfig
	Web           WebConfig
	Transport     TransportConfig
	System        SystemConfig
	Audio         AudioConfig
	Pool          PoolConfig
	McpPool       McpPoolConfig
	QuickReply    QuickReplyConfig
	LocalMCPFun   []LocalMCPFun
	Selected      SelectedConfig
	ASR           map[string]interface{}
	TTS           map[string]TTSConfig
	LLM           map[string]LLMConfig
	VLLLM         map[string]VLLLMConfig
	MCP           MCPConfig
}

type ServerConfig struct {
	IP     string
	Port   int
	Token  string
	Auth   AuthConfig
	Device DeviceRegistrationConfig
}

type AuthConfig struct {
	Enabled bool
	Store   StoreConfig
}

type StoreConfig struct {
	Type           string
	Expiry         time.Duration
	Cleanup        time.Duration
	Redis          AuthRedisStore
	SQLite         AuthSQLiteStore
	Memory         AuthMemoryStore
	CustomMetadata map[string]interface{}
	Labels         map[string]string
}

type AuthRedisStore struct {
	Addr     string
	Username string
	Password string
	DB       int
	Prefix   string
}

type AuthSQLiteStore struct {
	DSN string
}

type AuthMemoryStore struct {
	Cleanup time.Duration
}

type DeviceRegistrationConfig struct {
	RequireActivationCode bool // 是否需要激活码，默认false
	DefaultAdminUserID    uint // 默认管理员用户ID，用于不需要激活码的情况
}

type LogConfig struct {
	Level    string
	Dir      string
	File     string
	Format   string
}

type WebConfig struct {
	Enabled     bool
	Port        int
	StaticDir   string
	Websocket   string
	VisionURL   string
	ActivateText string
}

type LLMConfig struct {
	Type        string
	ModelName   string
	BaseURL     string
	APIKey      string
	Temperature float64
	MaxTokens   int
	TopP        float64
	Extra       map[string]interface{}
}

type TTSConfig struct {
	Type            string
	Voice           string
	Format          string
	OutputDir       string
	AppID           string
	Token           string
	Cluster         string
	SupportedVoices []VoiceInfo
}

type VoiceInfo struct {
	Name        string
	DisplayName string
	Sex         string
	Description string
	AudioURL    string
}

type VLLLMConfig struct {
	Type        string
	ModelName   string
	BaseURL     string
	APIKey      string
	Temperature float64
	MaxTokens   int
	TopP        float64
	Security    SecurityConfig
	Extra       map[string]interface{}
}

type SecurityConfig struct {
	MaxFileSize       int64
	MaxPixels         int64
	MaxWidth          int
	MaxHeight         int
	AllowedFormats    []string
	EnableDeepScan    bool
	ValidationTimeout string
}

type MCPConfig struct {
	Enabled bool
}

type SelectedConfig struct {
	ASR   string
	TTS   string
	LLM   string
	VLLLM string
}

// TransportConfig 传输层配置
type TransportConfig struct {
	WebSocket WebSocketConfig
	MQTTUDP   MQTTUDPConfig
}

type WebSocketConfig struct {
	Enabled bool
	IP      string
	Port    int
}

type MQTTUDPConfig struct {
	Enabled bool
	MQTT    MQTTConfig
	UDP     UDPConfig
}

type MQTTConfig struct {
	IP   string
	Port int
	QoS  int
}

type UDPConfig struct {
	IP                string
	ShowPort          int
	Port              int
	SessionTimeout    string
	MaxPacketSize     int
	EnableReliability bool
}

// SystemConfig 系统级配置
type SystemConfig struct {
	DefaultPrompt string
	Roles         []Role
	CMDExit       []string
	MusicDir      string
}

type Role struct {
	Name        string
	Description string
	Enabled     bool
}

// AudioConfig 音频配置
type AudioConfig struct {
	DeleteAudio   bool
	SaveTTSAudio  bool
	SaveUserAudio bool
}

// PoolConfig 连接池配置
type PoolConfig struct {
	MinSize       int
	MaxSize       int
	RefillSize    int
	CheckInterval int
}

type McpPoolConfig struct {
	MinSize       int
	MaxSize       int
	RefillSize    int
	CheckInterval int
}

// QuickReplyConfig 快速回复配置
type QuickReplyConfig struct {
	Enabled bool
	Words   []string
}

// LocalMCPFun 本地MCP函数配置
type LocalMCPFun struct {
	Name        string
	Description string
	Enabled     bool
}