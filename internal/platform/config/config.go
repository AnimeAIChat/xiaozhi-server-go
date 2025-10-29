package config

import (
	"time"
)

type Config struct {
	Server        ServerConfig            `yaml:"server" mapstructure:"server"`
	Log           LogConfig               `yaml:"log" mapstructure:"log"`
	Web           WebConfig               `yaml:"web" mapstructure:"web"`
	Transport     TransportConfig         `yaml:"transport" mapstructure:"transport"`
	System        SystemConfig            `yaml:"system" mapstructure:"system"`
	Audio         AudioConfig             `yaml:"audio" mapstructure:"audio"`
	Pool          PoolConfig              `yaml:"pool_config" mapstructure:"pool_config"`
	McpPool       McpPoolConfig           `yaml:"mcp_pool_config" mapstructure:"mcp_pool_config"`
	QuickReply    QuickReplyConfig        `yaml:"quick_reply" mapstructure:"quick_reply"`
	LocalMCPFun   []LocalMCPFun           `yaml:"local_mcp_fun" mapstructure:"local_mcp_fun"`
	Selected      SelectedConfig          `yaml:"selected_module" mapstructure:"selected_module"`
	ASR           map[string]interface{} `yaml:"ASR" mapstructure:"ASR"`
	TTS           map[string]TTSConfig    `yaml:"TTS" mapstructure:"TTS"`
	LLM           map[string]LLMConfig    `yaml:"LLM" mapstructure:"LLM"`
	VLLLM         map[string]VLLLMConfig  `yaml:"VLLLM" mapstructure:"VLLLM"`
	MCP           MCPConfig               `yaml:"mcp" mapstructure:"mcp"`
}

type ServerConfig struct {
	IP     string        `yaml:"ip" mapstructure:"ip"`
	Port   int           `yaml:"port" mapstructure:"port"`
	Token  string        `yaml:"token" mapstructure:"token"`
	Auth   AuthConfig    `yaml:"auth" mapstructure:"auth"`
}

type AuthConfig struct {
	Enabled bool         `yaml:"enabled" mapstructure:"enabled"`
	Store   StoreConfig  `yaml:"store" mapstructure:"store"`
}

type StoreConfig struct {
	Type           string                 `yaml:"type" mapstructure:"type"`
	Expiry         time.Duration          `yaml:"expiry" mapstructure:"expiry"`
	Cleanup        time.Duration          `yaml:"cleanup" mapstructure:"cleanup"`
	Redis          AuthRedisStore         `yaml:"redis,omitempty" mapstructure:"redis"`
	SQLite         AuthSQLiteStore        `yaml:"sqlite,omitempty" mapstructure:"sqlite"`
	Memory         AuthMemoryStore        `yaml:"memory,omitempty" mapstructure:"memory"`
	CustomMetadata map[string]interface{} `yaml:"metadata,omitempty" mapstructure:"metadata"`
	Labels         map[string]string      `yaml:"labels,omitempty" mapstructure:"labels"`
}

type AuthRedisStore struct {
	Addr     string `yaml:"addr" mapstructure:"addr"`
	Username string `yaml:"username,omitempty" mapstructure:"username"`
	Password string `yaml:"password,omitempty" mapstructure:"password"`
	DB       int    `yaml:"db,omitempty" mapstructure:"db"`
	Prefix   string `yaml:"prefix,omitempty" mapstructure:"prefix"`
}

type AuthSQLiteStore struct {
	DSN string `yaml:"dsn,omitempty" mapstructure:"dsn"`
}

type AuthMemoryStore struct {
	Cleanup time.Duration `yaml:"cleanup" mapstructure:"cleanup"`
}

type LogConfig struct {
	Level    string `yaml:"log_level" mapstructure:"log_level"`
	Dir      string `yaml:"log_dir" mapstructure:"log_dir"`
	File     string `yaml:"log_file" mapstructure:"log_file"`
	Format   string `yaml:"log_format" mapstructure:"log_format"`
}

type WebConfig struct {
	Enabled     bool   `yaml:"enabled" mapstructure:"enabled"`
	Port        int    `yaml:"port" mapstructure:"port"`
	StaticDir   string `yaml:"static_dir" mapstructure:"static_dir"`
	Websocket   string `yaml:"websocket" mapstructure:"websocket"`
	VisionURL   string `yaml:"vision" mapstructure:"vision"`
	ActivateText string `yaml:"activate_text" mapstructure:"activate_text"`
}

type LLMConfig struct {
	Type        string                 `yaml:"type" mapstructure:"type"`
	ModelName   string                 `yaml:"model_name" mapstructure:"model_name"`
	BaseURL     string                 `yaml:"url" mapstructure:"url"`
	APIKey      string                 `yaml:"api_key" mapstructure:"api_key"`
	Temperature float64                `yaml:"temperature" mapstructure:"temperature"`
	MaxTokens   int                    `yaml:"max_tokens" mapstructure:"max_tokens"`
	TopP        float64                `yaml:"top_p" mapstructure:"top_p"`
	Extra       map[string]interface{} `yaml:",inline" mapstructure:",remain"`
}

type TTSConfig struct {
	Type            string      `yaml:"type" mapstructure:"type"`
	Voice           string      `yaml:"voice" mapstructure:"voice"`
	Format          string      `yaml:"format" mapstructure:"format"`
	OutputDir       string      `yaml:"output_dir" mapstructure:"output_dir"`
	AppID           string      `yaml:"appid" mapstructure:"appid"`
	Token           string      `yaml:"token" mapstructure:"token"`
	Cluster         string      `yaml:"cluster" mapstructure:"cluster"`
	SupportedVoices []VoiceInfo `yaml:"supported_voices" mapstructure:"supported_voices"`
}

type VoiceInfo struct {
	Name        string `yaml:"name" mapstructure:"name"`
	DisplayName string `yaml:"display_name" mapstructure:"display_name"`
	Sex         string `yaml:"sex" mapstructure:"sex"`
	Description string `yaml:"description" mapstructure:"description"`
	AudioURL    string `yaml:"audio_url" mapstructure:"audio_url"`
}

type VLLLMConfig struct {
	Type        string                 `yaml:"type" mapstructure:"type"`
	ModelName   string                 `yaml:"model_name" mapstructure:"model_name"`
	BaseURL     string                 `yaml:"url" mapstructure:"url"`
	APIKey      string                 `yaml:"api_key" mapstructure:"api_key"`
	Temperature float64                `yaml:"temperature" mapstructure:"temperature"`
	MaxTokens   int                    `yaml:"max_tokens" mapstructure:"max_tokens"`
	TopP        float64                `yaml:"top_p" mapstructure:"top_p"`
	Security    SecurityConfig         `yaml:"security" mapstructure:"security"`
	Extra       map[string]interface{} `yaml:",inline" mapstructure:",remain"`
}

type SecurityConfig struct {
	MaxFileSize       int64    `yaml:"max_file_size" mapstructure:"max_file_size"`
	MaxPixels         int64    `yaml:"max_pixels" mapstructure:"max_pixels"`
	MaxWidth          int      `yaml:"max_width" mapstructure:"max_width"`
	MaxHeight         int      `yaml:"max_height" mapstructure:"max_height"`
	AllowedFormats    []string `yaml:"allowed_formats" mapstructure:"allowed_formats"`
	EnableDeepScan    bool     `yaml:"enable_deep_scan" mapstructure:"enable_deep_scan"`
	ValidationTimeout string   `yaml:"validation_timeout" mapstructure:"validation_timeout"`
}

type MCPConfig struct {
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
}

type SelectedConfig struct {
	ASR   string `yaml:"ASR" mapstructure:"ASR"`
	TTS   string `yaml:"TTS" mapstructure:"TTS"`
	LLM   string `yaml:"LLM" mapstructure:"LLM"`
	VLLLM string `yaml:"VLLLM" mapstructure:"VLLLM"`
}

// TransportConfig 传输层配置
type TransportConfig struct {
	WebSocket WebSocketConfig `yaml:"websocket" mapstructure:"websocket"`
	MQTTUDP   MQTTUDPConfig   `yaml:"mqtt_udp" mapstructure:"mqtt_udp"`
}

type WebSocketConfig struct {
	Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
	IP      string `yaml:"ip" mapstructure:"ip"`
	Port    int    `yaml:"port" mapstructure:"port"`
}

type MQTTUDPConfig struct {
	Enabled bool         `yaml:"enabled" mapstructure:"enabled"`
	MQTT    MQTTConfig   `yaml:"mqtt" mapstructure:"mqtt"`
	UDP     UDPConfig    `yaml:"udp" mapstructure:"udp"`
}

type MQTTConfig struct {
	IP   string `yaml:"ip" mapstructure:"ip"`
	Port int    `yaml:"port" mapstructure:"port"`
	QoS  int    `yaml:"qos" mapstructure:"qos"`
}

type UDPConfig struct {
	IP                string `yaml:"ip" mapstructure:"ip"`
	ShowPort          int    `yaml:"show_port" mapstructure:"show_port"`
	Port              int    `yaml:"port" mapstructure:"port"`
	SessionTimeout    string `yaml:"session_timeout" mapstructure:"session_timeout"`
	MaxPacketSize     int    `yaml:"max_packet_size" mapstructure:"max_packet_size"`
	EnableReliability bool   `yaml:"enable_reliability" mapstructure:"enable_reliability"`
}

// SystemConfig 系统级配置
type SystemConfig struct {
	DefaultPrompt string   `yaml:"prompt" mapstructure:"prompt"`
	Roles         []Role   `yaml:"roles" mapstructure:"roles"`
	CMDExit       []string `yaml:"CMD_exit" mapstructure:"CMD_exit"`
}

type Role struct {
	Name        string `yaml:"name" mapstructure:"name"`
	Description string `yaml:"description" mapstructure:"description"`
	Enabled     bool   `yaml:"enabled" mapstructure:"enabled"`
}

// AudioConfig 音频配置
type AudioConfig struct {
	DeleteAudio   bool `yaml:"delete_audio" mapstructure:"delete_audio"`
	SaveTTSAudio  bool `yaml:"save_tts_audio" mapstructure:"save_tts_audio"`
	SaveUserAudio bool `yaml:"save_user_audio" mapstructure:"save_user_audio"`
}

// PoolConfig 连接池配置
type PoolConfig struct {
	MinSize       int `yaml:"pool_min_size" mapstructure:"pool_min_size"`
	MaxSize       int `yaml:"pool_max_size" mapstructure:"pool_max_size"`
	RefillSize    int `yaml:"pool_refill_size" mapstructure:"pool_refill_size"`
	CheckInterval int `yaml:"pool_check_interval" mapstructure:"pool_check_interval"`
}

type McpPoolConfig struct {
	MinSize       int `yaml:"pool_min_size" mapstructure:"pool_min_size"`
	MaxSize       int `yaml:"pool_max_size" mapstructure:"pool_max_size"`
	RefillSize    int `yaml:"pool_refill_size" mapstructure:"pool_refill_size"`
	CheckInterval int `yaml:"pool_check_interval" mapstructure:"pool_check_interval"`
}

// QuickReplyConfig 快速回复配置
type QuickReplyConfig struct {
	Enabled bool     `yaml:"enabled" mapstructure:"enabled"`
	Words   []string `yaml:"words" mapstructure:"words"`
}

// LocalMCPFun 本地MCP函数配置
type LocalMCPFun struct {
	Name        string `yaml:"name" mapstructure:"name"`
	Description string `yaml:"description" mapstructure:"description"`
	Enabled     bool   `yaml:"enabled" mapstructure:"enabled"`
}