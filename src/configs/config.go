package configs

import (
	"os"

	"gopkg.in/yaml.v3"
)

// TokenConfig Token配置
type TokenConfig struct {
	Token string `yaml:"token"`
}

// Config 主配置结构
type Config struct {
	Server struct {
		IP    string `yaml:"ip"`
		Port  int    `yaml:"port"`
		Token string
		Auth  struct {
			Enabled        bool          `yaml:"enabled"`
			AllowedDevices []string      `yaml:"allowed_devices"`
			Tokens         []TokenConfig `yaml:"tokens"`
		} `yaml:"auth"`
	} `yaml:"server"`

	Log struct {
		LogFormat string `yaml:"log_format"`
		LogLevel  string `yaml:"log_level"`
		LogDir    string `yaml:"log_dir"`
		LogFile   string `yaml:"log_file"`
	} `yaml:"log"`

	Web struct {
		Enabled   bool   `yaml:"enabled"`
		Port      int    `yaml:"port"`
		StaticDir string `yaml:"static_dir"`
		Websocket string `yaml:"websocket"`
		VisionURL string `yaml:"vision"`
	} `yaml:"web"`

	DefaultPrompt    string   `yaml:"prompt"`
	Roles            []string `yaml:"roles"` // 角色列表
	DeleteAudio      bool     `yaml:"delete_audio"`
	QuickReply       bool     `yaml:"quick_reply"`
	QuickReplyWords  []string `yaml:"quick_reply_words"`
	UsePrivateConfig bool     `yaml:"use_private_config"`
	LocalMCPFun      []string `yaml:"local_mcp_fun"` // 本地MCP函数映射

	SelectedModule map[string]string `yaml:"selected_module"`

	PoolConfig    PoolConfig    `yaml:"pool_config"`
	McpPoolConfig McpPoolConfig `yaml:"mcp_pool_config"`

	VAD   map[string]VADConfig  `yaml:"VAD"`
	ASR   map[string]ASRConfig  `yaml:"ASR"`
	TTS   map[string]TTSConfig  `yaml:"TTS"`
	LLM   map[string]LLMConfig  `yaml:"LLM"`
	VLLLM map[string]VLLMConfig `yaml:"VLLLM"`

	CMDExit []string `yaml:"CMD_exit"`

	// 连通性检查配置
	ConnectivityCheck ConnectivityCheckConfig `yaml:"connectivity_check"`
}

// VADConfig VAD配置结构
type VADConfig struct {
	Type               string                 `yaml:"type"`
	ModelDir           string                 `yaml:"model_dir"`
	Threshold          float64                `yaml:"threshold"`
	MinSilenceDuration int                    `yaml:"min_silence_duration_ms"`
	Extra              map[string]interface{} `yaml:",inline"`
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

// TTSConfig TTS配置结构
type TTSConfig struct {
	Type            string   `yaml:"type"`
	Voice           string   `yaml:"voice"`
	Format          string   `yaml:"format"`
	OutputDir       string   `yaml:"output_dir"`
	AppID           string   `yaml:"appid"`
	Token           string   `yaml:"token"`
	Cluster         string   `yaml:"cluster"`
	SurportedVoices []string `yaml:"surported_voices"` // 支持的语音列表
}

// LLMConfig LLM配置结构
type LLMConfig struct {
	Type        string                 `yaml:"type"`
	ModelName   string                 `yaml:"model_name"`
	BaseURL     string                 `yaml:"url"`
	APIKey      string                 `yaml:"api_key"`
	Temperature float64                `yaml:"temperature"`
	MaxTokens   int                    `yaml:"max_tokens"`
	TopP        float64                `yaml:"top_p"`
	Extra       map[string]interface{} `yaml:",inline"`
}

// SecurityConfig 图片安全配置结构
type SecurityConfig struct {
	MaxFileSize       int64    `yaml:"max_file_size"`      // 最大文件大小（字节）
	MaxPixels         int64    `yaml:"max_pixels"`         // 最大像素数量
	MaxWidth          int      `yaml:"max_width"`          // 最大宽度
	MaxHeight         int      `yaml:"max_height"`         // 最大高度
	AllowedFormats    []string `yaml:"allowed_formats"`    // 允许的图片格式
	EnableDeepScan    bool     `yaml:"enable_deep_scan"`   // 启用深度安全扫描
	ValidationTimeout string   `yaml:"validation_timeout"` // 验证超时时间
}

// ConnectivityCheckConfig 连通性检查配置结构
type ConnectivityCheckConfig struct {
	Enabled       bool   `yaml:"enabled"`        // 是否启用连通性检查
	Timeout       string `yaml:"timeout"`        // 检查超时时间
	RetryAttempts int    `yaml:"retry_attempts"` // 重试次数
	RetryDelay    string `yaml:"retry_delay"`    // 重试延迟
	TestModes     struct {
		ASRTestAudio  string `yaml:"asr_test_audio"`  // ASR测试音频文件
		LLMTestPrompt string `yaml:"llm_test_prompt"` // LLM测试提示词
		TTSTestText   string `yaml:"tts_test_text"`   // TTS测试文本
	} `yaml:"test_modes"`
}

// VLLMConfig VLLLM配置结构（视觉语言大模型）
type VLLMConfig struct {
	Type        string                 `yaml:"type"`        // API类型，复用LLM的类型
	ModelName   string                 `yaml:"model_name"`  // 模型名称，使用支持视觉的模型
	BaseURL     string                 `yaml:"url"`         // API地址
	APIKey      string                 `yaml:"api_key"`     // API密钥
	Temperature float64                `yaml:"temperature"` // 温度参数
	MaxTokens   int                    `yaml:"max_tokens"`  // 最大令牌数
	TopP        float64                `yaml:"top_p"`       // TopP参数
	Security    SecurityConfig         `yaml:"security"`    // 图片安全配置
	Extra       map[string]interface{} `yaml:",inline"`     // 额外配置
}

// LoadConfig 从文件加载配置
func LoadConfig() (*Config, string, error) {
	path := ".config.yaml"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = "config.yaml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, path, err
	}

	return config, path, nil
}
