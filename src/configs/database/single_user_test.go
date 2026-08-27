package database

import (
	"testing"

	"xiaozhi-server-go/src/models"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNormalizeSingleUserDataAssignsDefaultUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:single-user-normalize-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Agent{}, &models.AgentDialog{}, &models.Device{}, &models.ASRConfig{}, &models.TTSConfig{}, &models.LLMConfig{}, &models.VLLLMConfig{}); err != nil {
		t.Fatal(err)
	}
	legacyUserID := uint(42)
	fixtures := []interface{}{
		&models.Agent{Name: "旧智能体", UserID: legacyUserID},
		&models.AgentDialog{Conversationid: "dialog", AgentID: 1, UserID: legacyUserID},
		&models.Device{Name: "未归属设备", DeviceID: "device-01", ClientID: "client-01"},
		&models.ASRConfig{UserID: legacyUserID, Name: "asr", Type: "edge", Data: datatypes.JSON([]byte(`{}`))},
		&models.TTSConfig{UserID: legacyUserID, Name: "tts", Type: "edge", Data: datatypes.JSON([]byte(`{}`))},
		&models.LLMConfig{UserID: legacyUserID, Name: "llm", Type: "openai", Data: datatypes.JSON([]byte(`{}`))},
		&models.VLLLMConfig{UserID: legacyUserID, Name: "vllm", Type: "openai", Data: datatypes.JSON([]byte(`{}`))},
	}
	for _, fixture := range fixtures {
		if err := db.Create(fixture).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := NormalizeSingleUserData(db); err != nil {
		t.Fatal(err)
	}

	var agent models.Agent
	var dialog models.AgentDialog
	var device models.Device
	var asr models.ASRConfig
	var tts models.TTSConfig
	var llm models.LLMConfig
	var vllm models.VLLLMConfig
	for _, result := range []interface{}{&agent, &dialog, &device, &asr, &tts, &llm, &vllm} {
		if err := db.First(result).Error; err != nil {
			t.Fatal(err)
		}
	}
	if agent.UserID != AdminUserID || dialog.UserID != AdminUserID || device.UserID == nil || *device.UserID != AdminUserID || asr.UserID != AdminUserID || tts.UserID != AdminUserID || llm.UserID != AdminUserID || vllm.UserID != AdminUserID {
		t.Fatalf("single-user normalization failed: agent=%d dialog=%d device=%v asr=%d tts=%d llm=%d vllm=%d", agent.UserID, dialog.UserID, device.UserID, asr.UserID, tts.UserID, llm.UserID, vllm.UserID)
	}
}
