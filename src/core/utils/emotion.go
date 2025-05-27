package utils

import "regexp"

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

func RemoveAllEmoji(text string) string {
	// 简化版本，匹配主要的emoji范围
	emojiRegex := regexp.MustCompile(`[\x{1F000}-\x{1FFFF}]`)
	return emojiRegex.ReplaceAllString(text, "")
}
