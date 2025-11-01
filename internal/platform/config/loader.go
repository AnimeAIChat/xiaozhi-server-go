package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"xiaozhi-server-go/internal/platform/errors"
)

type Loader struct {
	viper *viper.Viper
}

func NewLoader() *Loader {
	v := viper.New()
	v.SetConfigName(".config")
	v.SetConfigType("yaml")

	// 搜索路径
	searchPaths := []string{
		".",
		"./config",
		filepath.Dir(os.Args[0]), // 可执行文件目录
	}

	for _, path := range searchPaths {
		v.AddConfigPath(path)
	}

	return &Loader{viper: v}
}

func (l *Loader) Load() (*Config, error) {
	// 设置默认值
	l.setDefaults()

	// 读取配置文件
	if err := l.viper.ReadInConfig(); err != nil {
		return nil, errors.Wrap(errors.KindConfig, "load", "failed to read config file", err)
	}

	var cfg Config
	if err := l.viper.Unmarshal(&cfg); err != nil {
		return nil, errors.Wrap(errors.KindConfig, "load", "failed to unmarshal config", err)
	}

	// 验证配置
	if err := l.validate(&cfg); err != nil {
		return nil, errors.Wrap(errors.KindConfig, "load", "config validation failed", err)
	}

	return &cfg, nil
}

func (l *Loader) setDefaults() {
	l.viper.SetDefault("server.ip", "0.0.0.0")
	l.viper.SetDefault("server.port", 8000)
	l.viper.SetDefault("web.port", 8080)
	l.viper.SetDefault("log.level", "INFO")
	l.viper.SetDefault("log.dir", "logs")
	l.viper.SetDefault("log.file", "server.log")
}

func (l *Loader) validate(cfg *Config) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return errors.New(errors.KindConfig, "validate", "invalid server port")
	}

	if cfg.Web.Port <= 0 || cfg.Web.Port > 65535 {
		return errors.New(errors.KindConfig, "validate", "invalid web port")
	}

	return nil
}
