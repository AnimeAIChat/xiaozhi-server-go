package database

import (
	"testing"

	"xiaozhi-server-go/src/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestActivateOnboardingAgentCreatesOneProtectedAgent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:onboarding-agent-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.Agent{}, &models.Device{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	first, created, err := ActivateOnboardingAgent(db, "primary-llm", "zh-CN-XiaoxiaoNeural")
	if err != nil || !created {
		t.Fatalf("ActivateOnboardingAgent() = %#v, %t, %v", first, created, err)
	}
	if !first.IsOnboarding || first.Name != OnboardingAgentName {
		t.Fatalf("unexpected onboarding agent: %#v", first)
	}
	if first.LLM != "primary-llm" || first.Voice != "zh-CN-XiaoxiaoNeural" {
		t.Fatalf("unexpected default provider settings: llm=%q voice=%q", first.LLM, first.Voice)
	}
	if first.Prompt == "" {
		t.Fatal("onboarding prompt does not identify the assistant")
	}

	second, created, err := ActivateOnboardingAgent(db, "primary-llm", "zh-CN-XiaoxiaoNeural")
	if err != nil || created {
		t.Fatalf("second activation = %#v, %t, %v", second, created, err)
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

func TestActivateOnboardingAgentKeepsOneRecordAndRefreshesProviderChoice(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:onboarding-activate-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Agent{}, &models.Device{}); err != nil {
		t.Fatal(err)
	}
	first, created, err := ActivateOnboardingAgent(db, "first-llm", "first-voice")
	if err != nil || !created {
		t.Fatalf("first activation = %#v, %t, %v", first, created, err)
	}
	second, created, err := ActivateOnboardingAgent(db, "second-llm", "second-voice")
	if err != nil || created {
		t.Fatalf("second activation = %#v, %t, %v", second, created, err)
	}
	if first.ID != second.ID || second.LLM != "second-llm" || second.Voice != "second-voice" {
		t.Fatalf("activation created a duplicate or did not refresh config: first=%#v second=%#v", first, second)
	}
	var count int64
	if err := db.Model(&models.Agent{}).Where("is_onboarding = ?", true).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("onboarding agent count = %d, %v", count, err)
	}
}
