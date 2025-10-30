package utils

import (
	"regexp"
)

// EmotionEmoji 定义情绪到表情的映射
var EmotionEmoji = map[string]string{
	"neutral":     "😐",
	"happy":       "😊",
	"laughing":    "😂",
	"funny":       "🤡",
	"sad":         "😢",
	"angry":       "😠",
	"crying":      "😭",
	"loving":      "🥰",
	"embarrassed": "😳",
	"surprised":   "😮",
	"shocked":     "😱",
	"thinking":    "🤔",
	"winking":     "😉",
	"cool":        "😎",
	"relaxed":     "😌",
	"delicious":   "😋",
	"kissy":       "😘",
	"confident":   "😏",
	"sleepy":      "😴",
	"silly":       "🤪",
	"confused":    "😕",
}

// GetEmotionEmoji 根据情绪返回对应的表情
func GetEmotionEmoji(emotion string) string {
	if emoji, ok := EmotionEmoji[emotion]; ok {
		return emoji
	}
	return EmotionEmoji["neutral"] // 默认返回中性表情
}

// 更全面的表情符号正则表达式，覆盖所有主要的表情符号范围
var SimpleEmojiRegex = regexp.MustCompile(`[\x{1F000}-\x{1FFFF}]|[\x{2600}-\x{27BF}]`)

func RemoveAllEmoji(text string) string {
	return SimpleEmojiRegex.ReplaceAllString(text, "")
}
