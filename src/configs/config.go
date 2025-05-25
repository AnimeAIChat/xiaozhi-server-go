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
		IP   string `yaml:"ip"`
		Port int    `yaml:"port"`
		Auth struct {
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
	} `yaml:"web"`

	DeleteAudio      bool `yaml:"delete_audio"`
	UsePrivateConfig bool `yaml:"use_private_config"`

	SelectedModule map[string]string `yaml:"selected_module"`

	VAD map[string]VADConfig `yaml:"VAD"`
	ASR map[string]ASRConfig `yaml:"ASR"`
	TTS map[string]TTSConfig `yaml:"TTS"`
	LLM map[string]LLMConfig `yaml:"LLM"`

	CMDExit []string `yaml:"CMD_exit"`
}

// VADConfig VAD配置结构
type VADConfig struct {
	Type               string                 `yaml:"type"`
	ModelDir           string                 `yaml:"model_dir"`
	Threshold          float64                `yaml:"threshold"`
	MinSilenceDuration int                    `yaml:"min_silence_duration_ms"`
	Extra              map[string]interface{} `yaml:",inline"`
}

// ASRConfig ASR配置结构
type ASRConfig map[string]interface{}

// TTSConfig TTS配置结构
type TTSConfig struct {
	Type      string `yaml:"type"`
	Voice     string `yaml:"voice"`
	Format    string `yaml:"format"`
	OutputDir string `yaml:"output_dir"`
	AppID     string `yaml:"appid"`
	Token     string `yaml:"token"`
	Cluster   string `yaml:"cluster"`
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
