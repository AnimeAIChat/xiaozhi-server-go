package service

import (
	"xiaozhi-server-go/internal/domain/config/types"
)

// ConfigService 配置服务
type ConfigService struct {
	repo types.Repository
}

// NewConfigService 创建配置服务
func NewConfigService(repo types.Repository) *ConfigService {
	return &ConfigService{
		repo: repo,
	}
}

// GetQuickReplyConfig 获取快速回复配置
func (s *ConfigService) GetQuickReplyConfig() (enabled bool, words []string, err error) {
	// 获取启用状态
	enabled, err = s.repo.GetBoolConfigValue("QuickReply.Enabled")
	if err != nil {
		// 如果配置不存在或出错，返回默认值
		return false, []string{"在呢", "您好", "我在听", "请讲"}, nil
	}

	// 获取回复词列表
	words, err = s.repo.GetStringArrayConfigValue("QuickReply.Words")
	if err != nil {
		// 如果配置不存在或出错，使用默认回复词
		words = []string{"在呢", "您好", "我在听", "请讲"}
		err = nil // 重置错误，因为我们提供了默认值
	}

	return enabled, words, nil
}

// IsQuickReplyEnabled 检查快速回复是否启用
func (s *ConfigService) IsQuickReplyEnabled() bool {
	enabled, _, err := s.GetQuickReplyConfig()
	if err != nil {
		return false
	}
	return enabled
}

// GetQuickReplyWords 获取快速回复词列表
func (s *ConfigService) GetQuickReplyWords() []string {
	_, words, err := s.GetQuickReplyConfig()
	if err != nil {
		return []string{"在呢", "您好", "我在听", "请讲"}
	}
	return words
}