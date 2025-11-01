package utils

import (
	"os"
	"regexp"
	"strings"
	"time"
)

// GetProjectDir 获取项目根目录
func GetProjectDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

// 辅助函数：返回两个时间间隔中较小的一个
func MinDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// RemoveAngleBracketContent 移除尖括号及其内容
func RemoveAngleBracketContent(text string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(text, "")
}

// RemoveControlCharacters 移除控制字符
func RemoveControlCharacters(text string) string {
	// 移除常见的控制字符，但保留换行符和制表符
	return strings.Map(func(r rune) rune {
		if r < 32 && r != 9 && r != 10 && r != 13 {
			return -1
		}
		return r
	}, text)
}
