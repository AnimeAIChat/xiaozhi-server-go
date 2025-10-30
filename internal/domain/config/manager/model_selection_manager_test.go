package manager

import (
	"testing"

	"xiaozhi-server-go/internal/platform/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestModelSelectionManager(t *testing.T) {
	// 创建内存数据库用于测试
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// 自动迁移
	err = db.AutoMigrate(&storage.ModelSelection{})
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	// 创建管理器
	manager := NewModelSelectionManager(db)

	// 测试初始化默认选择 (使用用户ID 1作为管理员)
	adminUserID := 1
	err = manager.InitDefaultSelection(adminUserID)
	if err != nil {
		t.Fatalf("Failed to init default selection: %v", err)
	}

	// 测试获取选择
	selection, err := manager.GetModelSelection(adminUserID)
	if err != nil {
		t.Fatalf("Failed to get model selection: %v", err)
	}

	if selection.UserID != adminUserID {
		t.Errorf("Expected user ID %d, got %d", adminUserID, selection.UserID)
	}

	if selection.ASRProvider != "DoubaoASR" {
		t.Errorf("Expected ASR provider 'DoubaoASR', got '%s'", selection.ASRProvider)
	}

	// 测试更新选择
	err = manager.UpdateModelSelection(adminUserID, "GoSherpaASR", "DoubaoTTS", "OllamaLLM", "ChatGLMVLLM")
	if err != nil {
		t.Fatalf("Failed to update model selection: %v", err)
	}

	// 重新获取选择
	selection, err = manager.GetModelSelection(adminUserID)
	if err != nil {
		t.Fatalf("Failed to get updated model selection: %v", err)
	}

	if selection.ASRProvider != "GoSherpaASR" {
		t.Errorf("Expected updated ASR provider 'GoSherpaASR', got '%s'", selection.ASRProvider)
	}

	// 测试可用提供商列表
	providers := manager.GetAvailableProviders()
	if len(providers["asr"]) == 0 {
		t.Error("Expected ASR providers list to be non-empty")
	}

	if len(providers["tts"]) == 0 {
		t.Error("Expected TTS providers list to be non-empty")
	}
}