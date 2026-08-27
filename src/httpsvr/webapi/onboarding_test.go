package webapi

import (
	"testing"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/models"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOnboardingProviderSetupRequiresConfiguredASRLLMAndTTS(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:onboarding-provider-setup-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ASRConfig{}, &models.LLMConfig{}, &models.TTSConfig{}, &models.Agent{}, &models.Device{}); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	configsToCreate := []interface{}{
		&models.ASRConfig{UserID: database.AdminUserID, Name: "asr", Type: "doubao", Data: datatypes.JSON([]byte(`{"type":"doubao","appid":"your_appid","access_token":"your_token"}`))},
		&models.LLMConfig{UserID: database.AdminUserID, Name: "llm", Type: "openai", Data: datatypes.JSON([]byte(`{"type":"openai","url":"https://example.test/v1","api_key":"valid-key","model_name":"model"}`))},
		&models.TTSConfig{UserID: database.AdminUserID, Name: "tts", Type: "edge", Data: datatypes.JSON([]byte(`{"type":"edge","voice":"zh-CN-XiaoxiaoNeural"}`))},
	}
	for _, provider := range configsToCreate {
		if err := db.Create(provider).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := &DefaultAdminService{config: &configs.Config{SelectedModule: map[string]string{"ASR": "asr", "LLM": "llm", "TTS": "tts"}}}
	if _, _, issues := service.onboardingProviderSetup(); len(issues) == 0 {
		t.Fatal("placeholder ASR credentials were accepted")
	}

	if err := db.Model(&models.ASRConfig{}).Where("name = ?", "asr").Update("data", datatypes.JSON([]byte(`{"type":"doubao","appid":"configured-app","access_token":"configured-token"}`))).Error; err != nil {
		t.Fatal(err)
	}
	llmName, voice, issues := service.onboardingProviderSetup()
	if len(issues) > 0 || llmName != "llm" || voice != "zh-CN-XiaoxiaoNeural" {
		t.Fatalf("configured providers were rejected: llm=%q voice=%q issues=%v", llmName, voice, issues)
	}
}
