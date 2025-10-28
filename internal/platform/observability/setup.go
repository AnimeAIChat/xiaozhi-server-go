package observability

import (
	"context"
	"log/slog"
	"sync"
)

// Config captures observability toggles. Future fields (OTel endpoint, etc.) can be added here.
type Config struct {
	Enabled bool
}

// ShutdownFunc allows callers to tear down any observability exporters.
type ShutdownFunc func(context.Context) error

var (
	loggerMu             sync.RWMutex
	instrumentationLog   *slog.Logger
	instrumentationState Config
)

func currentLogger() (*slog.Logger, Config) {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return instrumentationLog, instrumentationState
}

// Setup wires observability stubs. Instrumentation will be added in later milestones.
func Setup(ctx context.Context, cfg Config, logger *slog.Logger) (ShutdownFunc, error) {
	loggerMu.Lock()
	instrumentationLog = logger
	instrumentationState = cfg
	loggerMu.Unlock()

	if logger != nil {
		if cfg.Enabled {
			logger.InfoContext(ctx, "[OBSERVABILITY][SETUP] scaffolding enabled")
		} else {
			logger.InfoContext(ctx, "[OBSERVABILITY][SETUP] disabled")
		}
	}
	return func(context.Context) error { return nil }, nil
}
