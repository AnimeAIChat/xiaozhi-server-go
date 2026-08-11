package configs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
)

type memoryConfigDB struct {
	serverConfig string
}

func (db *memoryConfigDB) GetDB() *gorm.DB { return nil }

func (db *memoryConfigDB) InitServerConfig(config string) error {
	db.serverConfig = config
	return nil
}

func (db *memoryConfigDB) UpdateServerConfig(config string) error {
	db.serverConfig = config
	return nil
}

func (db *memoryConfigDB) LoadServerConfig() (string, error) { return db.serverConfig, nil }

func (db *memoryConfigDB) LoadProviderData(string, uint) map[string]string { return nil }

func TestLoadConfigCreatesPrivateConfigFromTemplate(t *testing.T) {
	directory := t.TempDir()
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDirectory) })

	const template = "server:\n  port: 8000\ntransport:\n  websocket:\n    enabled: true\nselected_module: {}\nASR: {}\nTTS: {}\nLLM: {}\nVLLLM: {}\n"
	if err := os.WriteFile("config.yaml", []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}

	db := &memoryConfigDB{}
	config, path, err := LoadConfig(db)
	if !errors.Is(err, ErrInitialConfigCreated) {
		t.Fatalf("LoadConfig() error = %v, want ErrInitialConfigCreated", err)
	}
	if path != ".config.yaml" {
		t.Fatalf("config path = %q, want .config.yaml", path)
	}
	if config != nil {
		t.Fatal("first run should stop before returning an active config")
	}
	privateConfig, err := os.ReadFile(filepath.Join(directory, ".config.yaml"))
	if err != nil {
		t.Fatalf("private config was not created: %v", err)
	}
	if string(privateConfig) != template {
		t.Fatalf("private config content = %q, want template content", string(privateConfig))
	}
	if strings.Contains(db.serverConfig, "port: 8000") {
		t.Fatalf("initial config must not be saved to config DB before it is completed: %q", db.serverConfig)
	}
}
