package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func InitProviders(db *gorm.DB, config *configs.Config) error {
	if err := InitLLMProviders(db, config); err != nil {
		return fmt.Errorf("初始化LLM提供者失败: %v", err)
	}
	if err := InitASRProviders(db, config); err != nil {
		return fmt.Errorf("初始化ASR提供者失败: %v", err)
	}
	if err := InitTTSProviders(db, config); err != nil {
		return fmt.Errorf("初始化TTS提供者失败: %v", err)
	}
	if err := InitVLLLMProviders(db, config); err != nil {
		return fmt.Errorf("初始化VLLLM提供者失败: %v", err)
	}
	return nil
}

func InitTTSProviders(db *gorm.DB, config *configs.Config) error {
	if config == nil || config.TTS == nil {
		return nil
	}

	for name, provider := range config.TTS {
		providerJson, err := json.Marshal(provider)
		if err != nil {
			return fmt.Errorf("序列化TTS提供者 %s 配置失败: %v", name, err)
		}
		ttsConfig := models.TTSConfig{
			Name: name,
			Type: provider.Type,
			Data: datatypes.JSON(providerJson),
		}

		if err := db.Where("name = ?", name).FirstOrCreate(&ttsConfig).Error; err != nil {
			return fmt.Errorf("插入或更新TTS提供者 %s 失败: %v", name, err)
		}
	}

	return nil
}

func InitLLMProviders(db *gorm.DB, config *configs.Config) error {
	if config == nil || config.LLM == nil {
		return nil
	}
	for name, provider := range config.LLM {
		providerJson, err := json.Marshal(provider)
		if err != nil {
			return fmt.Errorf("序列化LLM提供者 %s 配置失败: %v", name, err)
		}
		llmConfig := models.LLMConfig{
			Name: name,
			Type: provider.Type,
			Data: datatypes.JSON(providerJson),
		}
		// 比较，然后确认创建还是更新，name相同就更新
		if err := db.Where("name = ?", name).FirstOrCreate(&llmConfig).Error; err != nil {
			return fmt.Errorf("插入或更新LLM提供者 %s 失败: %v", name, err)
		}
	}

	return nil
}

func InitASRProviders(db *gorm.DB, config *configs.Config) error {
	if config == nil || config.ASR == nil {
		return nil
	}

	for name, provider := range config.ASR {
		providerJson, err := json.Marshal(provider)
		if err != nil {
			return fmt.Errorf("序列化ASR提供者 %s 配置失败: %v", name, err)
		}
		providerType := ""
		if typeVal, ok := provider["type"]; ok {
			if typeStr, ok := typeVal.(string); ok {
				providerType = typeStr
			}
		}

		asrConfig := models.ASRConfig{
			Name: name,
			Type: providerType,
			Data: datatypes.JSON(providerJson),
		}

		if err := db.Where("name = ?", name).FirstOrCreate(&asrConfig).Error; err != nil {
			return fmt.Errorf("插入或更新ASR提供者 %s 失败: %v", name, err)
		}
	}

	return nil
}

func InitVLLLMProviders(db *gorm.DB, config *configs.Config) error {
	if config == nil || config.VLLLM == nil {
		return nil
	}
	for name, provider := range config.VLLLM {
		providerJson, err := json.Marshal(provider)
		if err != nil {
			return fmt.Errorf("序列化VLLLM提供者 %s 配置失败: %v", name, err)
		}

		vlllmConfig := models.VLLLMConfig{
			Name: name,
			Type: provider.Type, // 复用LLM的类型
			Data: datatypes.JSON(providerJson),
		}

		if err := db.Where("name = ?", name).FirstOrCreate(&vlllmConfig).Error; err != nil {
			return fmt.Errorf("插入或更新VLLLM提供者 %s 失败: %v", name, err)
		}
	}

	return nil
}

func GetAllProviders(userID uint) map[string]string {
	providersMap := make(map[string]string)
	asr, _ := GetProviderByType("ASR", userID)
	tts, _ := GetProviderByType("TTS", userID)
	llm, _ := GetProviderByType("LLM", userID)
	vlllm, _ := GetProviderByType("VLLLM", userID)
	for name, data := range asr {
		providersMap["ASR:"+name] = data
	}
	for name, data := range tts {
		providersMap["TTS:"+name] = data
	}
	for name, data := range llm {
		providersMap["LLM:"+name] = data
	}
	for name, data := range vlllm {
		providersMap["VLLLM:"+name] = data
	}

	return providersMap
}

func RemoveSensitiveFields(data datatypes.JSON) (string, error) {
	configData := make(map[string]interface{})
	if err := json.Unmarshal(data, &configData); err != nil {
		return "", fmt.Errorf("反序列化ASR提供者数据失败: %v", err)
	}
	// 移除敏感字段
	delete(configData, "token")
	delete(configData, "access_token")
	delete(configData, "api_key")
	delete(configData, "appid")

	// 将处理后的数据重新序列化为JSON字符串
	configJson, err := json.Marshal(configData)
	if err != nil {
		return "", fmt.Errorf("序列化处理后的ASR提供者数据失败: %v", err)
	}
	return string(configJson), nil
}

func GetProviderNameList(userID uint) (asrList, ttsList, llmList, vlllmList []string) {
	asrList = make([]string, 0)
	var asrConfigs []models.ASRConfig
	if err := DB.Find(&asrConfigs).Error; err == nil {
		for _, asr := range asrConfigs {
			if asr.UserID != 0 && asr.UserID != AdminUserID && asr.UserID != userID {
				continue
			}
			asrList = append(asrList, asr.Name)
		}
	}

	ttsList = make([]string, 0)
	var ttsConfigs []models.TTSConfig
	if err := DB.Find(&ttsConfigs).Error; err == nil {
		for _, tts := range ttsConfigs {
			if tts.UserID != 0 && tts.UserID != AdminUserID && tts.UserID != userID {
				continue
			}
			ttsList = append(ttsList, tts.Name)
		}
	}

	llmList = make([]string, 0)
	var llmConfigs []models.LLMConfig
	if err := DB.Find(&llmConfigs).Error; err == nil {
		for _, llm := range llmConfigs {
			if llm.UserID != 0 && llm.UserID != AdminUserID && llm.UserID != userID {
				continue
			}
			llmList = append(llmList, llm.Name)
		}
	}

	vlllmList = make([]string, 0)
	var vlllmConfigs []models.VLLLMConfig
	if err := DB.Find(&vlllmConfigs).Error; err == nil {
		for _, vlllm := range vlllmConfigs {
			if vlllm.UserID != 0 && vlllm.UserID != AdminUserID && vlllm.UserID != userID {
				continue
			}
			vlllmList = append(vlllmList, vlllm.Name)
		}
	}
	return asrList, ttsList, llmList, vlllmList
}

func GetProviderByType(providerType string, userID uint) (map[string]string, error) {
	return GetProviderByTypeInternal(providerType, userID, true)
}

func GetProviderByTypeInternal(providerType string, userID uint, bRemoveSensitive bool) (map[string]string, error) {
	// 根据提供者类型返回对应的提供者列表
	switch providerType {
	case "ASR":
		// 取数据库中ASR提供者的名称列表
		var asrConfigs []models.ASRConfig
		if err := DB.Find(&asrConfigs).Error; err != nil {
			return nil, fmt.Errorf("查询ASR提供者失败: %v", err)
		}
		asrs := make(map[string]string)
		for _, config := range asrConfigs {
			if config.UserID != 0 && config.UserID != AdminUserID && config.UserID != userID {
				continue
			}
			if bRemoveSensitive {
				asrs[config.Name], _ = RemoveSensitiveFields(config.Data)
			} else {
				asrs[config.Name] = string(config.Data)
			}
		}
		return asrs, nil
	case "TTS":
		// 取数据库中TTS提供者的名称列表
		var ttsConfigs []models.TTSConfig
		if err := DB.Find(&ttsConfigs).Error; err != nil {
			return nil, fmt.Errorf("查询TTS提供者失败: %v", err)
		}
		tts := make(map[string]string)
		for _, config := range ttsConfigs {
			if config.UserID != 0 && config.UserID != AdminUserID && config.UserID != userID {
				continue
			}
			if bRemoveSensitive {
				tts[config.Name], _ = RemoveSensitiveFields(config.Data) // string(config.Data)
			} else {
				tts[config.Name] = string(config.Data)
			}
		}
		return tts, nil
	case "LLM":
		// 取数据库中LLM提供者的名称列表
		var llmConfigs []models.LLMConfig
		if err := DB.Find(&llmConfigs).Error; err != nil {
			return nil, fmt.Errorf("查询LLM提供者失败: %v", err)
		}
		llms := make(map[string]string)
		for _, config := range llmConfigs {
			if config.UserID != 0 && config.UserID != AdminUserID && config.UserID != userID {
				continue
			}
			if bRemoveSensitive {
				llms[config.Name], _ = RemoveSensitiveFields(config.Data)
			} else {
				llms[config.Name] = string(config.Data)
			}
		}
		return llms, nil
	case "VLLLM":
		// 取数据库中VLLLM提供者的名称列表
		var vlllmConfigs []models.VLLLMConfig
		if err := DB.Find(&vlllmConfigs).Error; err != nil {
			return nil, fmt.Errorf("查询VLLLM提供者失败: %v", err)
		}
		vlllms := make(map[string]string)
		for _, config := range vlllmConfigs {
			if config.UserID != 0 && config.UserID != AdminUserID && config.UserID != userID {
				continue
			}
			if bRemoveSensitive {
				vlllms[config.Name], _ = RemoveSensitiveFields(config.Data)
			} else {
				vlllms[config.Name] = string(config.Data)
			}
		}
		return vlllms, nil
	default:
		return nil, fmt.Errorf("未知的提供者类型: %s", providerType)
	}
}

// CreateProvider 创建新的提供者
func CreateProvider(providerType, name string, data interface{}, createUserID uint) error {
	providerJson, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化提供者数据失败: %v", err)
	}

	// 从providerJson中获取type字段
	var providerSubType string
	if typeVal, ok := data.(map[string]interface{})["type"]; ok {
		if typeStr, ok := typeVal.(string); ok {
			providerSubType = typeStr
		}
	} else {
		return fmt.Errorf("提供者数据中缺少type字段")
	}

	switch providerType {
	case "ASR":
		config := models.ASRConfig{
			UserID: createUserID,
			Name:   name,
			Type:   providerSubType,
			Data:   datatypes.JSON(providerJson),
		}
		return DB.Create(&config).Error
	case "TTS":
		config := models.TTSConfig{
			UserID: createUserID,
			Name:   name,
			Type:   providerSubType,
			Data:   datatypes.JSON(providerJson),
		}
		return DB.Create(&config).Error
	case "LLM":
		config := models.LLMConfig{
			UserID: createUserID,
			Name:   name,
			Type:   providerSubType,
			Data:   datatypes.JSON(providerJson),
		}
		return DB.Create(&config).Error
	case "VLLLM":
		config := models.VLLLMConfig{
			UserID: createUserID,
			Name:   name,
			Type:   providerSubType,
			Data:   datatypes.JSON(providerJson),
		}
		return DB.Create(&config).Error
	default:
		return fmt.Errorf("未知的提供者类型: %s", providerType)
	}
}

// UpdateProvider 更新提供者
func UpdateProvider(providerType, name string, data interface{}, userID uint) error {
	providerJson, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化提供者数据失败: %v", err)
	}

	switch providerType {
	case "ASR":
		result := DB.Model(&models.ASRConfig{}).
			Where(&models.ASRConfig{Name: name, UserID: userID}).
			Update("data", datatypes.JSON(providerJson))

		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("ASR提供者 %s 不存在或无权限更新", name)
		}
		return nil
	case "TTS":
		result := DB.Model(&models.TTSConfig{}).
			Where(&models.TTSConfig{Name: name, UserID: userID}).
			Update("data", datatypes.JSON(providerJson))

		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("TTS提供者 %s 不存在或无权限更新", name)
		}
		return nil
	case "LLM":
		result := DB.Model(&models.LLMConfig{}).
			Where(&models.LLMConfig{Name: name, UserID: userID}).
			Update("data", datatypes.JSON(providerJson))

		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("LLM提供者 %s 不存在或无权限更新", name)
		}
		return nil
	case "VLLLM":
		result := DB.Model(&models.VLLLMConfig{}).
			Where(&models.VLLLMConfig{Name: name, UserID: userID}).
			Update("data", datatypes.JSON(providerJson))

		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("VLLLM提供者 %s 不存在或无权限更新", name)
		}
		return nil
	default:
		return fmt.Errorf("未知的提供者类型: %s", providerType)
	}
}

// DeleteProvider 删除提供者
var ErrNoPermission = errors.New("没有权限执行此操作")

func DeleteProvider(providerType, name string, userID uint) error {
	switch providerType {
	case "ASR":
		var cfg models.ASRConfig
		if err := DB.Where("name = ?", name).First(&cfg).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}
		if cfg.UserID != userID && userID != AdminUserID {
			return ErrNoPermission
		}
		return DB.Delete(&cfg).Error
	case "TTS":
		var cfg models.TTSConfig
		if err := DB.Where("name = ?", name).First(&cfg).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}
		if cfg.UserID != userID && userID != AdminUserID {
			return ErrNoPermission
		}
		return DB.Delete(&cfg).Error
	case "LLM":
		var cfg models.LLMConfig
		if err := DB.Where("name = ?", name).First(&cfg).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}
		if cfg.UserID != userID && userID != AdminUserID {
			return ErrNoPermission
		}
		return DB.Delete(&cfg).Error
	case "VLLLM":
		var cfg models.VLLLMConfig
		if err := DB.Where("name = ?", name).First(&cfg).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}
		if cfg.UserID != userID && userID != AdminUserID {
			return ErrNoPermission
		}
		return DB.Delete(&cfg).Error
	default:
		return fmt.Errorf("未知的提供者类型: %s", providerType)
	}
}

// GetProviderByName 根据名称获取特定提供者
func GetProviderByName(providerType, name string) (string, error) {
	switch providerType {
	case "ASR":
		var config models.ASRConfig
		if err := DB.Where(&models.ASRConfig{Name: name}).First(&config).Error; err != nil {
			return "", err
		}
		return RemoveSensitiveFields(config.Data)
	case "TTS":
		var config models.TTSConfig
		if err := DB.Where(&models.TTSConfig{Name: name}).First(&config).Error; err != nil {
			return "", err
		}
		return RemoveSensitiveFields(config.Data)
	case "LLM":
		var config models.LLMConfig
		if err := DB.Where(&models.LLMConfig{Name: name}).First(&config).Error; err != nil {
			return "", err
		}
		return RemoveSensitiveFields(config.Data)
	case "VLLLM":
		var config models.VLLLMConfig
		if err := DB.Where(&models.VLLLMConfig{Name: name}).First(&config).Error; err != nil {
			return "", err
		}
		return RemoveSensitiveFields(config.Data)
	default:
		return "", fmt.Errorf("未知的提供者类型: %s", providerType)
	}
}
