package configs

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// TokenConfig Token配置
type TokenConfig struct {
	Token string `yaml:"token" json:"token"`
}

// Config 主配置结构
type Config struct {
	Server struct {
		IP    string `yaml:"ip" json:"ip"`
		Port  int    `yaml:"port" json:"port"`
		Token string `json:"token"`
		Auth  struct {
			Enabled bool `yaml:"enabled" json:"enabled"`
			Store   struct {
				Type   string `yaml:"type" json:"type"`     // memory/file/redis
				Expiry int    `yaml:"expiry" json:"expiry"` // 过期时间(小时)
			} `yaml:"store" json:"store"`
			AllowedDevices []string      `yaml:"allowed_devices" json:"allowed_devices"`
			Tokens         []TokenConfig `yaml:"tokens" json:"tokens"`
		} `yaml:"auth" json:"auth"`
	} `yaml:"server" json:"server"`

	// 传输层配置
	Transport struct {
		WebSocket struct {
			Enabled bool   `yaml:"enabled" json:"enabled"`
			IP      string `yaml:"ip" json:"ip"`
			Port    int    `yaml:"port" json:"port"`
		} `yaml:"websocket" json:"websocket"`
	} `yaml:"transport" json:"transport"`

	Log struct {
		LogFormat string `yaml:"log_format" json:"log_format"`
		LogLevel  string `yaml:"log_level" json:"log_level"`
		LogDir    string `yaml:"log_dir" json:"log_dir"`
		LogFile   string `yaml:"log_file" json:"log_file"`
	} `yaml:"log" json:"log"`

	Web struct {
		Enabled      bool   `yaml:"enabled" json:"enabled"`
		Port         int    `yaml:"port" json:"port"`
		StaticDir    string `yaml:"static_dir" json:"static_dir"`
		Websocket    string `yaml:"websocket" json:"websocket"`
		VisionURL    string `yaml:"vision" json:"vision"`
		ActivateText string `yaml:"activate_text" json:"activate_text"` // 发送激活码时携带的文本
	} `yaml:"web" json:"web"`

	DefaultPrompt    string   `yaml:"prompt"             json:"prompt"`
	Roles            []string `yaml:"roles"              json:"roles"` // 角色列表
	DeleteAudio      bool     `yaml:"delete_audio"       json:"delete_audio"`
	QuickReply       bool     `yaml:"quick_reply"        json:"quick_reply"`
	QuickReplyWords  []string `yaml:"quick_reply_words"  json:"quick_reply_words"`
	UsePrivateConfig bool     `yaml:"use_private_config" json:"use_private_config"`
	LocalMCPFun      []string `yaml:"local_mcp_fun"      json:"local_mcp_fun"` // 本地MCP函数映射

	SelectedModule map[string]string `yaml:"selected_module" json:"selected_module"`

	PoolConfig    PoolConfig    `yaml:"pool_config"`
	McpPoolConfig McpPoolConfig `yaml:"mcp_pool_config"`

	ASR   map[string]ASRConfig  `yaml:"ASR"   json:"ASR"`
	TTS   map[string]TTSConfig  `yaml:"TTS"   json:"TTS"`
	LLM   map[string]LLMConfig  `yaml:"LLM"   json:"LLM"`
	VLLLM map[string]VLLMConfig `yaml:"VLLLM" json:"VLLLM"`

	CMDExit []string `yaml:"CMD_exit" json:"CMD_exit"`

	// 连通性检查配置
	ConnectivityCheck ConnectivityCheckConfig `yaml:"connectivity_check" json:"connectivity_check"`
}

type PoolConfig struct {
	PoolMinSize       int `yaml:"pool_min_size"`
	PoolMaxSize       int `yaml:"pool_max_size"`
	PoolRefillSize    int `yaml:"pool_refill_size"`
	PoolCheckInterval int `yaml:"pool_check_interval"`
}
type McpPoolConfig struct {
	PoolMinSize       int `yaml:"pool_min_size"`
	PoolMaxSize       int `yaml:"pool_max_size"`
	PoolRefillSize    int `yaml:"pool_refill_size"`
	PoolCheckInterval int `yaml:"pool_check_interval"`
}

// ASRConfig ASR配置结构
type ASRConfig map[string]interface{}

type VoiceInfo struct {
	Name        string `yaml:"name"         json:"name"`
	DisplayName string `yaml:"display_name" json:"display_name"`
	Sex         string `yaml:"sex"          json:"sex"`
	Description string `yaml:"description"  json:"description"`
	AudioURL    string `yaml:"audio_url"    json:"audio_url"`
}

// TTSConfig TTS配置结构
type TTSConfig struct {
	Type            string      `yaml:"type"             json:"type"`             // TTS类型
	Voice           string      `yaml:"voice"            json:"voice"`            // 语音名称
	Format          string      `yaml:"format"           json:"format"`           // 输出格式
	OutputDir       string      `yaml:"output_dir"       json:"output_dir"`       // 输出目录
	AppID           string      `yaml:"appid"            json:"appid"`            // 应用ID
	Token           string      `yaml:"token"            json:"token"`            // API密钥
	Cluster         string      `yaml:"cluster"          json:"cluster"`          // 集群信息
	SupportedVoices []VoiceInfo `yaml:"supported_voices" json:"supported_voices"` // 支持的语音列表
}

// LLMConfig LLM配置结构
type LLMConfig struct {
	Type        string                 `yaml:"type"        json:"type"`        // LLM类型
	ModelName   string                 `yaml:"model_name"  json:"model_name"`  // 模型名称
	BaseURL     string                 `yaml:"url"         json:"url"`         // API地址
	APIKey      string                 `yaml:"api_key"     json:"api_key"`     // API密钥
	Temperature float64                `yaml:"temperature" json:"temperature"` // 温度参数
	MaxTokens   int                    `yaml:"max_tokens"  json:"max_tokens"`  // 最大令牌数
	TopP        float64                `yaml:"top_p"       json:"top_p"`       // TopP参数
	Extra       map[string]interface{} `yaml:",inline"     json:"extra"`       // 额外配置
}

// SecurityConfig 图片安全配置结构
type SecurityConfig struct {
	MaxFileSize       int64    `yaml:"max_file_size"      json:"max_file_size"`      // 最大文件大小（字节）
	MaxPixels         int64    `yaml:"max_pixels"         json:"max_pixels"`         // 最大像素数量
	MaxWidth          int      `yaml:"max_width"          json:"max_width"`          // 最大宽度
	MaxHeight         int      `yaml:"max_height"         json:"max_height"`         // 最大高度
	AllowedFormats    []string `yaml:"allowed_formats"    json:"allowed_formats"`    // 允许的图片格式
	EnableDeepScan    bool     `yaml:"enable_deep_scan"   json:"enable_deep_scan"`   // 启用深度安全扫描
	ValidationTimeout string   `yaml:"validation_timeout" json:"validation_timeout"` // 验证超时时间
}

// ConnectivityCheckConfig 连通性检查配置结构
type ConnectivityCheckConfig struct {
	Enabled       bool   `yaml:"enabled"        json:"enabled"`        // 是否启用连通性检查
	Timeout       string `yaml:"timeout"        json:"timeout"`        // 检查超时时间
	RetryAttempts int    `yaml:"retry_attempts" json:"retry_attempts"` // 重试次数
	RetryDelay    string `yaml:"retry_delay"    json:"retry_delay"`    // 重试延迟
	TestModes     struct {
		ASRTestAudio  string `yaml:"asr_test_audio" json:"asr_test_audio"`   // ASR测试音频文件
		LLMTestPrompt string `yaml:"llm_test_prompt" json:"llm_test_prompt"` // LLM测试提示词
		TTSTestText   string `yaml:"tts_test_text" json:"tts_test_text"`     // TTS测试文本
	} `yaml:"test_modes"     json:"test_modes"`
}

// VLLMConfig VLLLM配置结构（视觉语言大模型）
type VLLMConfig struct {
	Type        string                 `yaml:"type"        json:"type"`        // API类型，复用LLM的类型
	ModelName   string                 `yaml:"model_name"  json:"model_name"`  // 模型名称，使用支持视觉的模型
	BaseURL     string                 `yaml:"url"         json:"url"`         // API地址
	APIKey      string                 `yaml:"api_key"     json:"api_key"`     // API密钥
	Temperature float64                `yaml:"temperature" json:"temperature"` // 温度参数
	MaxTokens   int                    `yaml:"max_tokens"  json:"max_tokens"`  // 最大令牌数
	TopP        float64                `yaml:"top_p"       json:"top_p"`       // TopP参数
	Security    SecurityConfig         `yaml:"security"    json:"security"`    // 图片安全配置
	Extra       map[string]interface{} `yaml:",inline"     json:"extra"`       // 额外配置
}

var (
	Cfg *Config
)

func (cfg *Config) ToString() string {
	data, _ := yaml.Marshal(cfg)
	return string(data)
}

func (cfg *Config) FromString(data string) error {
	return yaml.Unmarshal([]byte(data), cfg)
}

func (cfg *Config) setDefaults() {
	cfg.Transport.WebSocket.Enabled = true
	cfg.Transport.WebSocket.IP = "0.0.0.0"
	cfg.Transport.WebSocket.Port = 8000

	cfg.Web.Port = 8080

	cfg.Server.Token = "your_token"

	cfg.Log.LogDir = "logs"
	cfg.Log.LogLevel = "INFO"
	cfg.Log.LogFormat = "{time:YYYY-MM-DD HH:mm:ss} - {level} - {message}"
	cfg.Log.LogFile = "server.log"

	cfg.PoolConfig.PoolMinSize = 0
	cfg.PoolConfig.PoolMaxSize = 0
	cfg.PoolConfig.PoolCheckInterval = 30

}

// LoadConfig 加载配置
// 第一次从config.yaml加载，加载后存储到数据库加载
// 如果数据库中已存在配置，则直接加载数据库中的配置
func LoadConfig(dbi ConfigDBInterface) (*Config, string, error) {
	bUseDatabaseCfg := false
	// 尝试从数据库加载配置
	cfgStr, err := dbi.LoadServerConfig()
	if err != nil {
		fmt.Println("加载服务器配置失败:", err)
		return nil, "", err
	}

	config := &Config{}

	path := "database:serverConfig"
	if cfgStr != "" {
		config.FromString(cfgStr)
		Cfg = config
		if bUseDatabaseCfg {
			return Cfg, path, nil
		}
	}

	// 尝试从文件读取
	path = ".config.yaml"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = "config.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		// 读取配置文件失败，使用默认配置
		config.setDefaults()
		data, _ = yaml.Marshal(config)
	} else {
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, path, err
		}
	}

	err = dbi.InitServerConfig(string(data))
	if err != nil {
		fmt.Println("初始化服务器配置到数据库失败:", err)
	}
	Cfg = config
	return config, path, nil
}
