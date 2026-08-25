package database

import (
	"strings"
	"testing"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureOnboardingAgentCreatesOneProtectedAgent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:onboarding-agent-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.Agent{}, &models.Device{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	config := &configs.Config{
		SelectedModule: map[string]string{"LLM": "primary-llm", "TTS": "primary-tts"},
		LLM:            map[string]configs.LLMConfig{"primary-llm": {}},
		TTS: map[string]configs.TTSConfig{
			"primary-tts": {Voice: "zh-CN-XiaoxiaoNeural"},
		},
	}

	first, err := EnsureOnboardingAgent(db, config)
	if err != nil {
		t.Fatalf("EnsureOnboardingAgent() error: %v", err)
	}
	if !first.IsOnboarding || first.Name != OnboardingAgentName {
		t.Fatalf("unexpected onboarding agent: %#v", first)
	}
	if first.LLM != "primary-llm" || first.Voice != "zh-CN-XiaoxiaoNeural" {
		t.Fatalf("unexpected default provider settings: llm=%q voice=%q", first.LLM, first.Voice)
	}
	if !strings.Contains(first.Prompt, "初始设置助手") {
		t.Fatal("onboarding prompt does not identify the assistant")
	}

	second, err := EnsureOnboardingAgent(db, config)
	if err != nil {
		t.Fatalf("second EnsureOnboardingAgent() error: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("created duplicate onboarding agent: %d != %d", first.ID, second.ID)
	}
	if !IsOnboardingAgent(db, first.ID) {
		t.Fatal("IsOnboardingAgent() = false, want true")
	}
	if err := DeleteAgent(db, first.ID, AdminUserID); err == nil {
		t.Fatal("DeleteAgent() allowed deleting the onboarding agent")
	}
}
