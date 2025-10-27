package config

import (
	"fmt"

	"github.com/joho/godotenv"

	"xiaozhi-server-go/internal/platform/storage"
	"xiaozhi-server-go/src/configs"
)

// Loader bridges legacy config loading logic into the new internal layer.
type Loader struct {
	useDotEnv bool
	source    configs.ConfigDBInterface
}

// NewLoader creates a loader that reads from the default database-backed source.
func NewLoader() *Loader {
	return &Loader{
		useDotEnv: true,
		source:    storage.ConfigStore(),
	}
}

// WithDotEnv toggles loading variables from a .env file before reading config.
func (l *Loader) WithDotEnv(enabled bool) *Loader {
	l.useDotEnv = enabled
	return l
}

// WithSource overrides the configuration data source (useful for tests).
func (l *Loader) WithSource(src configs.ConfigDBInterface) *Loader {
	if src != nil {
		l.source = src
	}
	return l
}

// Result captures the loaded configuration and its origin path.
type Result struct {
	Config *configs.Config
	Path   string
}

// Load retrieves configuration by delegating to the legacy configs package.
func (l *Loader) Load() (*Result, error) {
	if l.useDotEnv {
		if err := godotenv.Load(); err != nil {
			fmt.Println("未找到 .env 文件，使用系统环境变量")
		}
	}

	if l.source == nil {
		l.source = storage.ConfigStore()
	}

	cfg, path, err := configs.LoadConfig(l.source)
	if err != nil {
		return nil, err
	}

	return &Result{
		Config: cfg,
		Path:   path,
	}, nil
}
