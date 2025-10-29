package logging

import (
	"fmt"
	"log/slog"

	"xiaozhi-server-go/src/core/utils"
)

// Config captures logging configuration options.
type Config struct {
	Level    string
	Dir      string
	Filename string
}

// Logger provides access to both slog and legacy logging APIs.
type Logger struct {
	legacy *utils.Logger
}

// New creates a new Logger instance backed by the legacy utils logger.
func New(cfg Config) (*Logger, error) {
	logCfg := &utils.LogCfg{
		LogLevel: cfg.Level,
		LogDir:   cfg.Dir,
		LogFile:  cfg.Filename,
	}
	legacy, err := utils.NewLogger(logCfg)
	if err != nil {
		return nil, fmt.Errorf("初始化日志失败: %w", err)
	}
	return &Logger{legacy: legacy}, nil
}

// Legacy exposes the underlying legacy logger for backward compatibility.
func (l *Logger) Legacy() *utils.Logger {
	return l.legacy
}

// Slog exposes the structured logger for new integrations.
func (l *Logger) Slog() *slog.Logger {
	return l.legacy.Slog()
}
