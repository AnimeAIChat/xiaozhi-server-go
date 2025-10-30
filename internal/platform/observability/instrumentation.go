package observability

import (
	"context"
	"log/slog"
	"time"
)

// Enabled reports whether observability has been toggled on.
func Enabled() bool {
	_, cfg := currentLogger()
	return cfg.Enabled
}

// StartSpan records a lightweight span lifecycle around an operation.
func StartSpan(ctx context.Context, component, operation string) (context.Context, func(error)) {
	logger, _ := currentLogger()
	if logger == nil {
		return ctx, func(error) {}
	}

	start := time.Now()
	logger.LogAttrs(ctx, slog.LevelDebug, "obs span start",
		slog.String("component", component),
		slog.String("operation", operation),
	)

	return ctx, func(err error) {
		level := slog.LevelDebug
		if err != nil {
			level = slog.LevelError
		}

		attrs := []slog.Attr{
			slog.String("component", component),
			slog.String("operation", operation),
			slog.Duration("duration", time.Since(start)),
		}
		if err != nil {
			attrs = append(attrs, slog.Any("error", err))
		}

		logger.LogAttrs(ctx, level, "obs span end", attrs...)
	}
}

// RecordMetric emits a best-effort metric datapoint via the configured logger.
func RecordMetric(ctx context.Context, name string, value float64, labels map[string]string) {
	logger, _ := currentLogger()
	if logger == nil {
		return
	}

	attrs := []slog.Attr{
		slog.String("metric", name),
		slog.Float64("value", value),
	}
	for k, v := range labels {
		attrs = append(attrs, slog.String(k, v))
	}

	logger.LogAttrs(ctx, slog.LevelDebug, "obs metric", attrs...)
}
