package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MAX_FILENAME_LENGTH = 250 // 文件名最大长度限制
	USER_FILE_SUBSIZE   = 20  // 用户文件名中，需要减去的固定长度，年月日和后缀
)

// AudioCache 音频缓存类
type AudioCache struct {
	CacheDir      string // 缓存目录
	TTSProvider   string // TTS提供商名称
	VoiceName     string // 音色名称
	AudioFormat   string // 音频格式，默认为 "mp3"
	SampleRate    int    // 采样率
	Channels      int    // 声道数
	BitsPerSample int    // 采样位数
	DeviceID      string // 设备ID，用于区分不同设备的缓存

	CacheFileSubSize int // 文件名中需要减去的固定长度
}

// NewAudioCache 创建音频缓存配置
func NewAudioCache(ttsProvider, cacheDir, voiceName, audioFormat string) *AudioCache {
	return &AudioCache{
		CacheDir:      cacheDir,
		TTSProvider:   ttsProvider,
		VoiceName:     voiceName,
		AudioFormat:   audioFormat,
		SampleRate:    24000,
		Channels:      1,
		BitsPerSample: 16,

		CacheFileSubSize: len(ttsProvider) + len(voiceName) + 10, // 计算文件名中需要减去的固定长度
	}
}

func (ac *AudioCache) SetAudioInfo(sampleRate int, channels int, bitsPerSample int) {
	ac.SampleRate = sampleRate
	ac.Channels = channels
	ac.BitsPerSample = bitsPerSample
}

func (ac *AudioCache) SetDeviceID(deviceID string) {
	ac.DeviceID = strings.Replace(deviceID, ":", "_", -1)
}

// FindCachedAudio 查找已缓存的音频文件
func (ac *AudioCache) FindCachedAudio(text string) string {
	// 检查目录是否存在
	if _, err := os.Stat(ac.CacheDir); os.IsNotExist(err) {
		return ""
	}

	// 生成文件名
	filename := ac.generateFilename(text, "mp3")

	// 构建完整文件路径
	fullPath := fmt.Sprintf("%s/%s", ac.CacheDir, filename)

	// 检查文件是否存在
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath
	}

	// 如果不存在，替换后缀名检查wav
	fullPath = strings.Replace(fullPath, ".mp3", ".wav", 1)
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath
	}

	return ""
}

// SaveCachedAudio 保存音频到缓存目录
func (ac *AudioCache) SaveCachedAudio(text string, data []byte) (string, error) {
	suffix := "wav"
	dir := ac.CacheDir

	// 生成目标文件名
	filename := ""
	targetPath := ""
	if ac.VoiceName == "user" && ac.DeviceID != "" {
		filename = ac.generateUserFileName(text, suffix)
		dir = fmt.Sprintf("%s/%s", ac.CacheDir, ac.DeviceID)
		targetPath = fmt.Sprintf("%s/%s", dir, filename)
	} else {
		filename = ac.generateFilename(text, suffix)
		targetPath = fmt.Sprintf("%s/%s", ac.CacheDir, filename)
	}

	// 创建缓存目录
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建缓存目录失败: %v", err)
	}

	// 检查目标文件是否已存在
	if _, err := os.Stat(targetPath); err == nil {
		//DefaultLogger.Info("音频文件已存在，跳过保存: %s", targetPath)
		return targetPath, nil // 文件已存在，跳过保存
	}

	return SaveAudioToWavFile(data, targetPath, ac.SampleRate, ac.Channels, ac.BitsPerSample, false)
}

func (ac *AudioCache) generateUserFileName(text, suffix string) string {
	// 对文本进行安全化处理
	safeText := ac.sanitizeFilename(text, USER_FILE_SUBSIZE)
	if suffix == "" {
		suffix = ac.AudioFormat
	}

	// 生成文件名格式: xxYxxMXXD_text.format
	filename := fmt.Sprintf("%02d%02d%02d%02d%02d%02d_%s.%s",
		time.Now().Year(), time.Now().Month(), time.Now().Day(),
		time.Now().Hour(), time.Now().Minute(), time.Now().Second(),
		safeText, suffix)
	return filename
}

// generateFilename 生成音频文件名
func (ac *AudioCache) generateFilename(text, suffix string) string {
	// 对文本进行安全化处理
	safeText := ac.sanitizeFilename(text, ac.CacheFileSubSize)

	if suffix == "" {
		suffix = ac.AudioFormat
	}
	// 生成文件名格式: text_provider_voice.format
	filename := fmt.Sprintf(
		"%s_%s_%s.%s",
		safeText,
		ac.TTSProvider,
		ac.VoiceName,
		suffix,
	)

	return filename
}

// sanitizeFilename 清理文件名，移除不安全的字符
func (ac *AudioCache) sanitizeFilename(text string, l int) string {

	// 移除或替换文件名中不安全的字符
	unsafe := regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)
	safe := unsafe.ReplaceAllString(text, "_")

	// 移除首尾的下划线，点和空格
	safe = strings.Trim(safe, "_. ")

	maxTextLen := MAX_FILENAME_LENGTH - l // 限制文本长度，避免文件名过长
	if len(safe) > maxTextLen {
		safe = safe[:maxTextLen]
		// 检查最后一个字符是否被截断
		for len(safe) > 0 {
			r, size := utf8.DecodeLastRuneInString(safe)
			if r == utf8.RuneError && size == 1 {
				safe = safe[:len(safe)-1]
				fmt.Println("截断最后一个不完整的字符, 新长度:", len(safe), "字符:", safe)
			} else {
				break
			}
		}
	}

	// 如果清理后为空，使用默认名称
	if safe == "" {
		safe = "audio"
	}

	return safe
}

// IsAudioCacheHit 检查文本是否为音频缓存命中
func IsAudioCacheHit(text string, audioCacheWords []string) bool {
	return IsInArray(text, audioCacheWords)
}

// IsCachedFile 判断指定文件路径是否为缓存文件
func (ac *AudioCache) IsCachedFile(filePath string) bool {
	if filePath == "" {
		return false
	}

	// 获取文件的目录部分
	dir := filepath.Dir(filePath)

	// 简单判断文件的上一层目录是否是缓存目录
	return dir == ac.CacheDir ||
		strings.HasSuffix(dir, "/"+ac.CacheDir) ||
		strings.HasSuffix(dir, "\\"+ac.CacheDir)
}
