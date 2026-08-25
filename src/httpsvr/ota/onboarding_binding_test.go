package ota

import (
	"net/http/httptest"
	"testing"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNewDeviceAutomaticallyBindsToOnboardingAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:ota-onboarding-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Agent{}, &models.Device{}); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })
	previousLogger := utils.DefaultLogger
	logger, err := utils.NewLogger(&utils.LogCfg{LogDir: t.TempDir(), LogFile: "ota-test.log", LogLevel: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	utils.DefaultLogger = logger
	t.Cleanup(func() {
		_ = logger.Close()
		utils.DefaultLogger = previousLogger
	})

	config := &configs.Config{
		SelectedModule: map[string]string{"LLM": "test-llm", "TTS": "test-tts"},
		LLM:            map[string]configs.LLMConfig{"test-llm": {}},
		TTS:            map[string]configs.TTSConfig{"test-tts": {Voice: "test-voice"}},
	}
	probeTx := db.Begin()
	if _, err := database.EnsureOnboardingAgent(probeTx, config); err != nil {
		probeTx.Rollback()
		t.Fatalf("EnsureOnboardingAgent in transaction: %v", err)
	}
	probeTx.Rollback()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("POST", "/ota/", nil)
	request := OTARequestBody{}
	request.Board.Type = "test-board"
	request.Language = "zh-CN"

	device := (&DefaultOTAService{}).CheckAndUpdateDevice(context, config, request, "device-onboarding-001", "client-onboarding-001", "测试设备", "1.0.0")
	if device == nil || device.AgentID == nil || device.UserID == nil {
		t.Fatalf("device was not bound: %#v, response=%s", device, recorder.Body.String())
	}
	if device.AuthStatus != "onboarding" || *device.UserID != database.AdminUserID {
		t.Fatalf("unexpected initial binding: %#v", device)
	}
	if !database.IsOnboardingAgent(db, *device.AgentID) {
		t.Fatalf("device agent %d is not the onboarding agent", *device.AgentID)
	}
}
