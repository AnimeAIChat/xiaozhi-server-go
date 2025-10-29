package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xiaozhi-server-go/src/core/utils"
)

func TestSmokeLoadConfigAndLogger(t *testing.T) {
	config, logger, err := loadConfigAndLogger()
	if err != nil {
		t.Fatalf("loadConfigAndLogger failed: %v", err)
	}
	if config == nil {
		t.Fatal("config is nil")
	}
	if logger == nil {
		t.Fatal("logger is nil")
	}
	logger.Close()
}

func TestInitGraphOrder(t *testing.T) {
	steps := InitGraph()
	want := []string{
		"storage:init-config-store",
		"config:load-runtime",
		"logging:init-provider",
		"observability:setup-hooks",
		"auth:init-manager",
	}
	if len(steps) != len(want) {
		t.Fatalf("unexpected step count: got %d want %d", len(steps), len(want))
	}
	for i, step := range steps {
		if step.ID != want[i] {
			t.Fatalf("step %d mismatch: got %s want %s", i, step.ID, want[i])
		}
	}
}

func TestExecuteInitGraph(t *testing.T) {
	state := &appState{}
	if err := executeInitSteps(context.Background(), InitGraph(), state); err != nil {
		t.Fatalf("executeInitSteps failed: %v", err)
	}
	if state.config == nil {
		t.Fatal("config is nil after init")
	}
	if state.logger == nil {
		t.Fatal("logger is nil after init")
	}
	if state.authManager == nil {
		t.Fatal("auth manager is nil after init")
	}
	if state.observabilityShutdown == nil {
		t.Fatal("observability shutdown hook not set")
	}
	defer state.logger.Close()
	defer state.authManager.Close()
	defer state.observabilityShutdown(context.Background())
}

func TestLogBootstrapGraphOutput(t *testing.T) {
	tmp := t.TempDir()
	logCfg := &utils.LogCfg{
		LogLevel: "info",
		LogDir:   tmp,
		LogFile:  "graph.log",
	}
	logger, err := utils.NewLogger(logCfg)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	logBootstrapGraph(logger, InitGraph())
	logger.Close()

	data, err := os.ReadFile(filepath.Join(tmp, logCfg.LogFile))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "初始化依赖关系概览") {
		t.Fatalf("graph header missing in log output: %s", content)
	}
	for _, id := range []string{
		"storage:init-config-store",
		"config:load-runtime",
		"logging:init-provider",
		"observability:setup-hooks",
		"auth:init-manager",
	} {
		if !strings.Contains(content, id) {
			t.Fatalf("expected graph output to contain %q, got: %s", id, content)
		}
	}
}
