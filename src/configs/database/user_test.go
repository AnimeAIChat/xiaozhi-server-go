package database

import (
	"errors"
	"os"
	"strings"
	"testing"

	"xiaozhi-server-go/src/configs"
)

func TestGenerateInitialAdminPassword(t *testing.T) {
	password, err := generateInitialAdminPassword()
	if err != nil {
		t.Fatalf("generateInitialAdminPassword() error = %v", err)
	}
	if len(password) != initialAdminPasswordLength {
		t.Fatalf("password length = %d, want %d", len(password), initialAdminPasswordLength)
	}
	for _, character := range password {
		if !strings.ContainsRune(initialAdminPasswordAlphabet, character) {
			t.Fatalf("password contains character outside the allowed alphabet: %q", character)
		}
	}
}

func TestFirstStartDoesNotPersistTemplateConfig(t *testing.T) {
	directory := t.TempDir()
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDirectory) })

	previousDB := DB
	previousConfigDB := serverConfigDB
	t.Cleanup(func() {
		DB = previousDB
		serverConfigDB = previousConfigDB
	})

	if _, _, err := InitDB(); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	sqlDB, err := DB.DB()
	if err != nil {
		t.Fatalf("DB.DB() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	_, path, err := configs.LoadConfig(GetServerConfigDB())
	if !errors.Is(err, configs.ErrInitialConfigCreated) {
		t.Fatalf("LoadConfig() error = %v, want ErrInitialConfigCreated", err)
	}
	if path != ".config.yaml" {
		t.Fatalf("config path = %q, want .config.yaml", path)
	}
	if _, err := os.Stat(".config.yaml"); err != nil {
		t.Fatalf("private config was not created: %v", err)
	}
	configString, err := GetServerConfigDB().LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig() error = %v", err)
	}
	if configString != "" {
		t.Fatalf("template config was persisted before it was completed: %q", configString)
	}
}
