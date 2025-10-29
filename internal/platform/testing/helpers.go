package testing

import (
	"testing"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/platform/logging"
)

func SetupTestConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{
			IP:   "127.0.0.1",
			Port: 8080,
		},
		Log: config.LogConfig{
			Level:  "DEBUG",
			Dir:    "/tmp/test_logs",
			File:   "test.log",
			Format: "{time} - {level} - {message}",
		},
		Web: config.WebConfig{
			Enabled:   true,
			Port:      8081,
			Websocket: "ws://127.0.0.1:8080",
		},
	}

	return cfg
}

func SetupTestLogger(t *testing.T) *logging.Logger {
	t.Helper()

	cfg := SetupTestConfig(t)
	logger, err := logging.New(logging.Config{
		Level:    cfg.Log.Level,
		Dir:      cfg.Log.Dir,
		Filename: cfg.Log.File,
	})

	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}

	return logger
}

func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error but got nil")
	}
}

func AssertEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if expected != actual {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}