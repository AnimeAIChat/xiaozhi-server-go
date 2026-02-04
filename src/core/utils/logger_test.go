package utils

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "debug",
		LogDir:   tmpDir,
		LogFile:  "test.log",
	})

	assert.NoError(t, err)
	assert.NotNil(t, logger)

	err = logger.Close()
	assert.NoError(t, err)
}

func TestNewLogger_DefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogFile: "default.log",
		LogDir:  tmpDir,
	})

	assert.NoError(t, err)
	assert.NotNil(t, logger)

	err = logger.Close()
	assert.NoError(t, err)
}

func TestLogger_Info(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "info",
		LogDir:   tmpDir,
		LogFile:  "info.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	testMsg := "test info message"
	logger.Info(testMsg)

	time.Sleep(10 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "info.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), testMsg)
}

func TestLogger_Warn(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "warn",
		LogDir:   tmpDir,
		LogFile:  "warn.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	testMsg := "test warn message"
	logger.Warn(testMsg)

	time.Sleep(10 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "warn.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), testMsg)
}

func TestLogger_Error(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "error",
		LogDir:   tmpDir,
		LogFile:  "error.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	testMsg := "test error message"
	logger.Error(testMsg)

	time.Sleep(10 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "error.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), testMsg)
}

func TestLogger_InfoWithArgs(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "debug",
		LogDir:   tmpDir,
		LogFile:  "info_args.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	logger.Info("user testuser logged in from 192.168.1.1")

	time.Sleep(10 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "info_args.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "testuser")
	assert.Contains(t, string(content), "192.168.1.1")
}

func TestLogger_InfoASR(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "debug",
		LogDir:   tmpDir,
		LogFile:  "asr.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	logger.InfoASR("processing audio")

	time.Sleep(10 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "asr.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "[ASR]")
	assert.Contains(t, string(content), "processing audio")
}

func TestLogger_InfoLLM(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "debug",
		LogDir:   tmpDir,
		LogFile:  "llm.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	logger.InfoLLM("generating response")

	time.Sleep(10 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "llm.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "[LLM]")
	assert.Contains(t, string(content), "generating response")
}

func TestLogger_InfoTTS(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "debug",
		LogDir:   tmpDir,
		LogFile:  "tts.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	logger.InfoTTS("synthesizing speech")

	time.Sleep(10 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "tts.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "[TTS]")
	assert.Contains(t, string(content), "synthesizing speech")
}

func TestLogger_InfoTiming(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "debug",
		LogDir:   tmpDir,
		LogFile:  "timing.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	logger.InfoTiming("operation took 100ms")

	time.Sleep(10 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "timing.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "[TIMING]")
	assert.Contains(t, string(content), "operation took 100ms")
}

func TestLogger_MultipleMessages(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "debug",
		LogDir:   tmpDir,
		LogFile:  "multi.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	logger.Info("message one")
	logger.Info("message two")
	logger.Info("message three")

	time.Sleep(50 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "multi.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "message one")
	assert.Contains(t, string(content), "message two")
	assert.Contains(t, string(content), "message three")
}

func TestLogger_LogLevelFiltering(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "error",
		LogDir:   tmpDir,
		LogFile:  "filter.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	logger.Debug("this should not appear")
	logger.Info("this should not appear either")
	logger.Warn("this should not appear")
	logger.Error("this should appear")

	time.Sleep(10 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "filter.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "this should not appear")
	assert.Contains(t, string(content), "this should appear")
}

func TestContainsFormatPlaceholders(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"hello world", false},
		{"hello %s", true},
		{"value is %d", true},
		{"no placeholders here", false},
		{"%[1]s argument", true},
	}

	for _, tt := range tests {
		result := containsFormatPlaceholders(tt.input)
		assert.Equal(t, tt.expected, result, "input: %s", tt.input)
	}
}

func TestCustomTextHandler_Enabled(t *testing.T) {
	handler := &CustomTextHandler{
		writer: &strings.Builder{},
		level:  slog.LevelInfo,
	}

	assert.True(t, handler.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelWarn))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelError))
	assert.False(t, handler.Enabled(context.Background(), slog.LevelDebug))
}

func TestConfigLogLevelToSlogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		result := configLogLevelToSlogLevel(tt.input)
		assert.Equal(t, tt.expected, result, "input: %s", tt.input)
	}
}

func TestLogger_ConcurrentLogging(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "debug",
		LogDir:   tmpDir,
		LogFile:  "concurrent.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			logger.Info("concurrent message number", idx)
		}(i)
	}

	wg.Wait()

	time.Sleep(50 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "concurrent.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)

	count := strings.Count(string(content), "concurrent message number")
	assert.Equal(t, 10, count)
}

func TestLogger_LongMessage(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(&LogCfg{
		LogLevel: "debug",
		LogDir:   tmpDir,
		LogFile:  "long.log",
	})
	require.NoError(t, err)
	defer logger.Close()

	longMsg := strings.Repeat("A", 10000)
	logger.Info(longMsg)

	time.Sleep(10 * time.Millisecond)

	logFile := filepath.Join(tmpDir, "long.log")
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), longMsg)
}
