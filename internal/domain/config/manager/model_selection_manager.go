package manager

import (
	"xiaozhi-server-go/internal/platform/errors"
	"xiaozhi-server-go/internal/platform/storage"

	"gorm.io/gorm"
)

// ModelSelectionManager 模型选择管理器
type ModelSelectionManager struct {
	db *gorm.DB
}

// NewModelSelectionManager 创建模型选择管理器
func NewModelSelectionManager(db *gorm.DB) *ModelSelectionManager {
	return &ModelSelectionManager{db: db}
}

// GetModelSelection 获取用户的模型选择
func (m *ModelSelectionManager) GetModelSelection(userID int) (*storage.ModelSelection, error) {
	var selection storage.ModelSelection
	if err := m.db.Where("user_id = ? AND is_active = ?", userID, true).First(&selection).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 如果没有找到，返回默认选择
			return m.getDefaultSelection(userID), nil
		}
		return nil, errors.Wrap(errors.KindStorage, "model_selection.get", "failed to get model selection", err)
	}

	return &selection, nil
}

// SaveModelSelection 保存用户的模型选择
func (m *ModelSelectionManager) SaveModelSelection(selection *storage.ModelSelection) error {
	if selection == nil {
		return errors.Wrap(errors.KindDomain, "model_selection.save", "selection cannot be nil", nil)
	}

	// 先将该用户的所有选择标记为非活跃
	if err := m.db.Model(&storage.ModelSelection{}).Where("user_id = ?", selection.UserID).Update("is_active", false).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "model_selection.save", "failed to deactivate existing selections", err)
	}

	// 保存新的选择
	selection.IsActive = true
	if err := m.db.Create(selection).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "model_selection.save", "failed to save model selection", err)
	}

	return nil
}

// UpdateModelSelection 更新用户的模型选择
func (m *ModelSelectionManager) UpdateModelSelection(userID int, asrProvider, ttsProvider, llmProvider, vllmProvider string) error {
	// 首先尝试获取现有选择
	existing, err := m.GetModelSelection(userID)
	if err != nil {
		return errors.Wrap(errors.KindStorage, "model_selection.get_existing", "failed to get existing selection", err)
	}

	if existing.ID == 0 {
		// 不存在，创建新的
		selection := &storage.ModelSelection{
			UserID:       userID,
			ASRProvider:  asrProvider,
			TTSProvider:  ttsProvider,
			LLMProvider:  llmProvider,
			VLLMProvider: vllmProvider,
		}
		return m.SaveModelSelection(selection)
	} else {
		// 存在，更新现有的
		existing.ASRProvider = asrProvider
		existing.TTSProvider = ttsProvider
		existing.LLMProvider = llmProvider
		existing.VLLMProvider = vllmProvider
		existing.IsActive = true

		if err := m.db.Save(existing).Error; err != nil {
			return errors.Wrap(errors.KindStorage, "model_selection.update", "failed to update model selection", err)
		}
		return nil
	}
}

// GetAvailableProviders 获取所有可用的提供商列表
func (m *ModelSelectionManager) GetAvailableProviders() map[string][]string {
	return map[string][]string{
		"asr":  {"DoubaoASR", "GoSherpaASR", "DeepgramSST", "StepASR"},
		"tts":  {"EdgeTTS", "DoubaoTTS", "GoSherpaTTS", "DeepgramTTS"},
		"llm":  {"ChatGLMLLM", "OllamaLLM", "DoubaoLLM", "CozeLLM"},
		"vllm": {"ChatGLMVLLM", "OllamaVLLM"},
	}
}

// InitDefaultSelection 初始化默认的模型选择
func (m *ModelSelectionManager) InitDefaultSelection(userID int) error {
	// 检查是否已存在选择
	var count int64
	if err := m.db.Model(&storage.ModelSelection{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "model_selection.init", "failed to check existing selections", err)
	}

	if count > 0 {
		return nil // 已存在，不需要初始化
	}

	// 创建默认选择
	defaultSelection := m.getDefaultSelection(userID)
	return m.SaveModelSelection(defaultSelection)
}

// getDefaultSelection 获取默认的模型选择
func (m *ModelSelectionManager) getDefaultSelection(userID int) *storage.ModelSelection {
	return &storage.ModelSelection{
		UserID:       userID,
		ASRProvider:  "DoubaoASR",
		TTSProvider:  "EdgeTTS",
		LLMProvider:  "ChatGLMLLM",
		VLLMProvider: "ChatGLMVLLM",
		IsActive:     true,
	}
}