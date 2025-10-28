package configs

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 主配置结构
type Config struct {
	Server struct {
		IP    string `yaml:"ip" json:"ip"`
		Port  int    `yaml:"port" json:"port"`
		Token string `json:"token"`
		Auth  struct {
			Store struct {
				Type           string            `yaml:"type" json:"type"`                   // memory/sqlite/redis
				Expiry         int               `yaml:"expiry" json:"expiry"`               // 过期时间(小时)
				Cleanup        string            `yaml:"cleanup,omitempty" json:"cleanup"`   // 数据清理周期 (duration string)
				Redis          AuthRedisStore    `yaml:"redis,omitempty" json:"redis"`       // Redis 配置
				Sqlite         AuthSQLiteStore   `yaml:"sqlite,omitempty" json:"sqlite"`     // SQLite 配置
				Memory         AuthMemoryStore   `yaml:"memory,omitempty" json:"memory"`     // 内存配置
				CustomMetadata map[string]any    `yaml:"metadata,omitempty" json:"metadata"` // 额外元数据
				Labels         map[string]string `yaml:"labels,omitempty" json:"labels"`     // 默认标签
			} `yaml:"store" json:"store"`
		} `yaml:"auth" json:"auth"`
	} `yaml:"server" json:"server"`

	// 传输层配置
	Transport struct {
		WebSocket struct {
			Enabled bool   `yaml:"enabled" json:"enabled"`
			IP      string `yaml:"ip" json:"ip"`
			Port    int    `yaml:"port" json:"port"`
		} `yaml:"websocket" json:"websocket"`

		MQTTUDP struct {
			Enabled bool `yaml:"enabled" json:"enabled"`
			MQTT    struct {
				IP   string `yaml:"ip" json:"ip"`
				Port int    `yaml:"port" json:"port"`
				QoS  int    `yaml:"qos" json:"qos"`
			} `yaml:"mqtt" json:"mqtt"`
			UDP struct {
				IP                string `yaml:"ip" json:"ip"`
				ShowPort          int    `yaml:"show_port" json:"show_port"` // 显示端口
				Port              int    `yaml:"port" json:"port"`
				SessionTimeout    string `yaml:"session_timeout" json:"session_timeout"`
				MaxPacketSize     int    `yaml:"max_packet_size" json:"max_packet_size"`
				EnableReliability bool   `yaml:"enable_reliability" json:"enable_reliability"`
			} `yaml:"udp" json:"udp"`
		} `yaml:"mqtt_udp" json:"mqtt_udp"`
	} `yaml:"transport" json:"transport"`

	Log struct {
		LogLevel string `yaml:"log_level" json:"log_level"`
		LogDir   string `yaml:"log_dir" json:"log_dir"`
		LogFile  string `yaml:"log_file" json:"log_file"`
	} `yaml:"log" json:"log"`

	Web struct {
		Port         int    `yaml:"port" json:"port"`
		StaticDir    string `yaml:"static_dir" json:"static_dir"`
		Websocket    string `yaml:"websocket" json:"websocket"`
		VisionURL    string `yaml:"vision" json:"vision"`
		ActivateText string `yaml:"activate_text" json:"activate_text"` // 发送激活码时携带的文本
	} `yaml:"web" json:"web"`

	DefaultPrompt string        `yaml:"prompt"             json:"prompt"`
	Roles         []Role        `yaml:"roles"              json:"roles"` // 角色列表
	DeleteAudio   bool          `yaml:"delete_audio"       json:"delete_audio"`
	LocalMCPFun   []LocalMCPFun `yaml:"local_mcp_fun"      json:"local_mcp_fun"` // 本地MCP函数映射
	SaveTTSAudio  bool          `yaml:"save_tts_audio"  json:"save_tts_audio"`   // 是否保存TTS音频文件
	SaveUserAudio bool          `yaml:"save_user_audio" json:"save_user_audio"`  // 是否保存用户音频文件

	SelectedModule map[string]string `yaml:"selected_module" json:"selected_module"`

	PoolConfig    PoolConfig    `yaml:"pool_config"`
	McpPoolConfig McpPoolConfig `yaml:"mcp_pool_config"`

	ASR   map[string]ASRConfig  `yaml:"ASR"   json:"ASR"`
	TTS   map[string]TTSConfig  `yaml:"TTS"   json:"TTS"`
	LLM   map[string]LLMConfig  `yaml:"LLM"   json:"LLM"`
	VLLLM map[string]VLLMConfig `yaml:"VLLLM" json:"VLLLM"`

	CMDExit []string `yaml:"CMD_exit" json:"CMD_exit"`
}

type LocalMCPFun struct {
	Name        string `yaml:"name"         json:"name"`        // 函数名称
	Description string `yaml:"description"  json:"description"` // 函数描述
	Enabled     bool   `yaml:"enabled"      json:"enabled"`     // 是否启用
}

type AuthRedisStore struct {
	Addr     string `yaml:"addr" json:"addr"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
	DB       int    `yaml:"db,omitempty" json:"db,omitempty"`
	Prefix   string `yaml:"prefix,omitempty" json:"prefix,omitempty"`
}

type AuthSQLiteStore struct {
	DSN string `yaml:"dsn,omitempty" json:"dsn,omitempty"`
}

type AuthMemoryStore struct {
	Cleanup string `yaml:"cleanup,omitempty" json:"cleanup,omitempty"`
}

type Role struct {
	Name        string `yaml:"name"         json:"name"`        // 角色名称
	Description string `yaml:"description"  json:"description"` // 角色描述
	Enabled     bool   `yaml:"enabled"      json:"enabled"`     // 是否启用
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
	Name        string `yaml:"name"         json:"name"`         // 语音名称，对应tts的音色字符串，如 zh_female_wanwanxiaohe_moon_bigtts
	Language    string `yaml:"language"     json:"language"`     // 语言，标记语种，用于前端选择
	DisplayName string `yaml:"display_name" json:"display_name"` // 显示名称，前端显示用，如湾湾小何
	Sex         string `yaml:"sex"          json:"sex"`          // 性别，男/女
	Description string `yaml:"description"  json:"description"`  // 音色的描述信息
	AudioURL    string `yaml:"audio_url"    json:"audio_url"`    // 音频URL，用于试听
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

func (cfg *Config) SaveToDB(dbi ConfigDBInterface) error {
	data := cfg.ToString()
	return dbi.UpdateServerConfig(data)
}

// LoadConfig 加载配置
// 完全从数据库加载配置，如果数据库为空则使用默认配置并初始化数据库
func LoadConfig(dbi ConfigDBInterface) (*Config, string, error) {
	bUseDatabaseCfg := true
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

func CheckAndModifyConfig(cfg *Config) *Config {
	// 检查Cfg.LocalMCPFun全部小写并去除空格
	if cfg.LocalMCPFun == nil {
		cfg.LocalMCPFun = []LocalMCPFun{}
	}
	fmt.Printf("检查配置: LocalMCPFun cnt %d\n", len(cfg.LocalMCPFun))
	if len(cfg.LocalMCPFun) < 10 {
		for i := 0; i < len(cfg.LocalMCPFun); i++ {
			cfg.LocalMCPFun[i].Name = strings.ToLower(strings.TrimSpace(cfg.LocalMCPFun[i].Name))
			cfg.LocalMCPFun[i].Description = strings.ToLower(strings.TrimSpace(cfg.LocalMCPFun[i].Description))
		}
	}
	// 检查默认配置的ASR,LLM,TTS和VLLLM是否存在
	if cfg.SelectedModule == nil {
		cfg.SelectedModule = map[string]string{}
	}
	if cfg.LLM == nil {
		cfg.LLM = map[string]LLMConfig{}
	}
	if cfg.VLLLM == nil {
		cfg.VLLLM = map[string]VLLMConfig{}
	}
	if cfg.ASR == nil {
		cfg.ASR = map[string]ASRConfig{}
	}
	if cfg.TTS == nil {
		cfg.TTS = map[string]TTSConfig{}
	}
	fmt.Printf("检查配置: LLM:%d, VLLLM:%d, ASR:%d, TTS:%d\n", len(cfg.LLM), len(cfg.VLLLM), len(cfg.ASR), len(cfg.TTS))
	fmt.Println("检查配置: SelectedModule", cfg.SelectedModule)
	// 如果SelectedModule没有选择或者选择的不存在，则选择第一个
	llmName, ok := cfg.SelectedModule["LLM"]
	_, exists := cfg.LLM[llmName]
	if !ok || llmName == "" || !exists {
		// 选择LLM中有的作为默认
		for name := range cfg.LLM {
			cfg.SelectedModule["LLM"] = name
			fmt.Println("未设置默认LLM或设置的LLM不存在，已设置为", name)
			break
		}
	}

	vlllmName, ok := cfg.SelectedModule["VLLLM"]
	_, exists = cfg.VLLLM[vlllmName]
	if !ok || vlllmName == "" || !exists {
		// 选择VLLLM中有的作为默认
		for name := range cfg.VLLLM {
			cfg.SelectedModule["VLLLM"] = name
			fmt.Println("未设置默认VLLLM或设置的VLLLM不存在，已设置为", name)
			break
		}
	}

	asrName, ok := cfg.SelectedModule["ASR"]
	_, exists = cfg.ASR[asrName]
	// ASRConfig 是 map[string]interface{}，只判断 key 是否存在和 name 非空
	if !ok || asrName == "" || !exists {
		// 选择ASR中有的作为默认
		for name := range cfg.ASR {
			cfg.SelectedModule["ASR"] = name
			fmt.Println("未设置默认ASR或设置的ASR不存在，已设置为", name)
			break
		}
	}

	ttsName, ok := cfg.SelectedModule["TTS"]
	_, exists = cfg.TTS[ttsName]
	if !ok || ttsName == "" || !exists {
		// 选择TTS中有的作为默认
		for name := range cfg.TTS {
			cfg.SelectedModule["TTS"] = name
			fmt.Println("未设置默认TTS或设置的TTS不存在，已设置为", name)
			break
		}
	}

	return cfg
}
