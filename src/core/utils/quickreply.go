package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// QuickReplyCache 快速回复缓存配置
type QuickReplyCache struct {
	CacheDir    string // 缓存目录，默认为 "wake_replay"
	TTSProvider string // TTS提供商名称
	VoiceName   string // 音色名称
	AudioFormat string // 音频格式，默认为 "mp3"
}

// NewQuickReplyCache 创建快速回复缓存配置
func NewQuickReplyCache(ttsProvider, voiceName string) *QuickReplyCache {
	return &QuickReplyCache{
		CacheDir:    "wake_replay",
		TTSProvider: ttsProvider,
		VoiceName:   voiceName,
		AudioFormat: "mp3",
	}
}

// FindCachedAudio 查找已缓存的快速回复音频文件
func (qrc *QuickReplyCache) FindCachedAudio(text string) string {
	// 检查目录是否存在
	if _, err := os.Stat(qrc.CacheDir); os.IsNotExist(err) {
		return ""
	}

	// 生成文件名
	filename := qrc.generateFilename(text)

	// 构建完整文件路径
	fullPath := fmt.Sprintf("%s/%s", qrc.CacheDir, filename)

	// 检查文件是否存在
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath
	}

	return ""
}

// SaveCachedAudio 保存快速回复音频到缓存目录
func (qrc *QuickReplyCache) SaveCachedAudio(text, sourcePath string) error {
	// 创建缓存目录
	if err := os.MkdirAll(qrc.CacheDir, 0o755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %v", err)
	}

	// 生成目标文件名
	filename := qrc.generateFilename(text)
	targetPath := fmt.Sprintf("%s/%s", qrc.CacheDir, filename)

	// 检查目标文件是否已存在
	if _, err := os.Stat(targetPath); err == nil {
		return nil // 文件已存在，跳过保存
	}

	// 复制文件到目标位置
	return qrc.copyFile(sourcePath, targetPath)
}

// generateFilename 生成快速回复音频文件名
func (qrc *QuickReplyCache) generateFilename(text string) string {
	// 对文本进行安全化处理
	safeText := qrc.sanitizeFilename(text)

	// 生成文件名格式: text_provider_voice.format
	filename := fmt.Sprintf(
		"%s_%s_%s.%s",
		safeText,
		qrc.TTSProvider,
		qrc.VoiceName,
		qrc.AudioFormat,
	)

	return filename
}

// sanitizeFilename 清理文件名，移除不安全的字符
func (qrc *QuickReplyCache) sanitizeFilename(text string) string {
	// 移除或替换文件名中不安全的字符
	unsafe := regexp.MustCompile(`[<>:"/\\|?*\s]+`)
	safe := unsafe.ReplaceAllString(text, "_")

	// 限制文件名长度，避免过长
	if len(safe) > 50 {
		safe = safe[:50]
	}

	// 移除首尾的下划线
	safe = strings.Trim(safe, "_")

	// 如果清理后为空，使用默认名称
	if safe == "" {
		safe = "quick_reply"
	}

	return safe
}

// copyFile 复制文件
func (qrc *QuickReplyCache) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %v", err)
	}
	defer sourceFile.Close()

	targetFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %v", err)
	}
	defer targetFile.Close()

	// 复制文件内容
	if _, err := targetFile.ReadFrom(sourceFile); err != nil {
		return fmt.Errorf("复制文件内容失败: %v", err)
	}

	return nil
}

// IsQuickReplyHit 检查文本是否为快速回复词
func IsQuickReplyHit(text string, quickReplyWords []string) bool {
	return IsInArray(text, quickReplyWords)
}

// IsCachedFile 判断指定文件路径是否为缓存文件
func (qrc *QuickReplyCache) IsCachedFile(filePath string) bool {
	if filePath == "" {
		return false
	}

	// 获取文件的目录部分
	dir := filepath.Dir(filePath)

	// 简单判断文件的上一层目录是否是缓存目录
	return dir == qrc.CacheDir ||
		strings.HasSuffix(dir, "/"+qrc.CacheDir) ||
		strings.HasSuffix(dir, "\\"+qrc.CacheDir)
}
