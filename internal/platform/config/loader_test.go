package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoader_Load(t *testing.T) {
	// 创建临时配置文件
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, ".config.yaml")

	configContent := `
server:
  ip: "127.0.0.1"
  port: 8080
log:
  log_level: "DEBUG"
  log_dir: "/tmp/logs"
  log_file: "test.log"
web:
  enabled: true
  port: 8081
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// 切换到临时目录
	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	loader := NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.IP != "127.0.0.1" {
		t.Errorf("expected server IP 127.0.0.1, got %s", cfg.Server.IP)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected server port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Log.Level != "DEBUG" {
		t.Errorf("expected log level DEBUG, got %s", cfg.Log.Level)
	}
}

func TestLoader_Validate(t *testing.T) {
	loader := NewLoader()

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Server: ServerConfig{Port: 8080},
				Web:    WebConfig{Port: 8081},
			},
			wantErr: false,
		},
		{
			name: "invalid server port",
			config: &Config{
				Server: ServerConfig{Port: 70000},
				Web:    WebConfig{Port: 8081},
			},
			wantErr: true,
		},
		{
			name: "invalid web port",
			config: &Config{
				Server: ServerConfig{Port: 8080},
				Web:    WebConfig{Port: 70000},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loader.validate(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}