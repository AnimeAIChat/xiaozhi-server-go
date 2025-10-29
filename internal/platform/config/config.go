package config

import (
	"time"
)

type Config struct {
	Server   ServerConfig   `yaml:"server" mapstructure:"server"`
	Log      LogConfig      `yaml:"log" mapstructure:"log"`
	Web      WebConfig      `yaml:"web" mapstructure:"web"`
	LLM      LLMConfig      `yaml:"llm" mapstructure:"llm"`
	TTS      TTSConfig      `yaml:"tts" mapstructure:"tts"`
	ASR      ASRConfig      `yaml:"asr" mapstructure:"asr"`
	VLLLM    VLLLMConfig    `yaml:"vlllm" mapstructure:"vlllm"`
	MCP      MCPConfig      `yaml:"mcp" mapstructure:"mcp"`
	Selected SelectedConfig `yaml:"selected_module" mapstructure:"selected_module"`
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
	Type     string        `yaml:"type" mapstructure:"type"`
	Expiry   time.Duration `yaml:"expiry" mapstructure:"expiry"`
	Cleanup  time.Duration `yaml:"cleanup" mapstructure:"cleanup"`
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
	Websocket   string `yaml:"websocket" mapstructure:"websocket"`
	VisionURL   string `yaml:"vision" mapstructure:"vision"`
	ActivateText string `yaml:"activate_text" mapstructure:"activate_text"`
}

type LLMConfig struct {
	Type        string  `yaml:"type" mapstructure:"type"`
	Model       string  `yaml:"model_name" mapstructure:"model_name"`
	APIKey      string  `yaml:"api_key" mapstructure:"api_key"`
	BaseURL     string  `yaml:"url" mapstructure:"url"`
	Temperature float64 `yaml:"temperature" mapstructure:"temperature"`
	MaxTokens   int     `yaml:"max_tokens" mapstructure:"max_tokens"`
	TopP        float64 `yaml:"top_p" mapstructure:"top_p"`
}

type TTSConfig struct {
	Type            string            `yaml:"type" mapstructure:"type"`
	Voice           string            `yaml:"voice" mapstructure:"voice"`
	OutputDir       string            `yaml:"output_dir" mapstructure:"output_dir"`
	Format          string            `yaml:"format" mapstructure:"format"`
	SupportedVoices []VoiceInfo       `yaml:"supported_voices" mapstructure:"supported_voices"`
}

type VoiceInfo struct {
	Name        string `yaml:"name" mapstructure:"name"`
	DisplayName string `yaml:"display_name" mapstructure:"display_name"`
	Sex         string `yaml:"sex" mapstructure:"sex"`
	Description string `yaml:"description" mapstructure:"description"`
	AudioURL    string `yaml:"audio_url" mapstructure:"audio_url"`
}

type ASRConfig struct {
	Type        string `yaml:"type" mapstructure:"type"`
	AppID       string `yaml:"appid" mapstructure:"appid"`
	AccessToken string `yaml:"access_token" mapstructure:"access_token"`
	OutputDir   string `yaml:"output_dir" mapstructure:"output_dir"`
}

type VLLLMConfig struct {
	Type        string  `yaml:"type" mapstructure:"type"`
	Model       string  `yaml:"model_name" mapstructure:"model_name"`
	APIKey      string  `yaml:"api_key" mapstructure:"api_key"`
	BaseURL     string  `yaml:"url" mapstructure:"url"`
	MaxTokens   int     `yaml:"max_tokens" mapstructure:"max_tokens"`
	Temperature float64 `yaml:"temperature" mapstructure:"temperature"`
	TopP        float64 `yaml:"top_p" mapstructure:"top_p"`
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