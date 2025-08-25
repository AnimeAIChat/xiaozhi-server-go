package utils

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
)

var (
	// 预编译正则表达式
	reSplitString          = regexp.MustCompile(`[.,!?;。！？；：]+`)
	reMarkdownChars        = regexp.MustCompile(`(\*\*|__|\*|_|#{1,6}\s|` + "`" + `{1,3}|~~|>\s|\[.*?\]\(.*?\)|\!\[.*?\]\(.*?\)|\|.*?\|)`)
	reRemoveAllPunctuation = regexp.MustCompile(
		`[.,!?;:，。！？、；：""''「」『』（）\(\)【】\[\]{}《》〈〉—–\-_~·…‖\|\\/*&\^%\$#@\+=<>]`,
	)
	reWakeUpWord = regexp.MustCompile(`^你好.+`)
)

// splitAtLastPunctuation 在最后一个标点符号处分割文本，优化聊天场景下的分句逻辑
func SplitAtLastPunctuation(text string) (string, int) {
	if len(text) == 0 {
		return "", 0
	}

	// 定义不同优先级的分句标点符号
	// 优先级1：强制停顿的标点（句号、问号、感叹号等）
	strongPunctuations := []string{"。", "？", "！", "；", ".", "?", "!", ";"}

	// 优先级2：中等停顿的标点（逗号、冒号等）
	mediumPunctuations := []string{"，", "：", ",", ":"}

	// 优先级3：轻微停顿的标点（顿号、括号等）
	lightPunctuations := []string{"、", "）", ")", "】", "]", "》", ">", "`", "'"}

	// 动态调整最小分句长度，避免超出文本长度
	minLength := 2
	if len(text) < minLength {
		minLength = 1
	}

	// 优先查找强停顿标点
	if segment, pos := findLastPunctuationWithMinLength(text, strongPunctuations, minLength); pos > 0 {
		return segment, pos
	}

	// 如果文本较长（超过30字符），考虑中等停顿标点
	if len(text) > 30 {
		minLength = 8
		if len(text) < minLength {
			minLength = len(text) / 2
		}
		if segment, pos := findLastPunctuationWithMinLength(text, mediumPunctuations, minLength); pos > 0 {
			return segment, pos
		}
	}

	// 如果文本很长（超过50字符），考虑轻微停顿标点
	if len(text) > 50 {
		minLength = 8
		if len(text) < minLength {
			minLength = len(text) / 2
		}
		if segment, pos := findLastPunctuationWithMinLength(text, lightPunctuations, minLength); pos > 0 {
			return segment, pos
		}
	}

	// 如果没有找到合适的标点，且文本过长（超过80字符），强制在空格处分割
	if len(text) > 80 {
		minLength = 8
		if len(text) < minLength {
			minLength = len(text) / 2
		}
		if segment, pos := findLastSpaceWithMinLength(text, minLength); pos > 0 {
			return segment, pos
		}
	}

	// 如果文本过长（超过100字符），强制分割
	if len(text) > 100 {
		cutPos := 80
		if len(text) < cutPos {
			cutPos = len(text) / 2
		}
		return text[:cutPos], cutPos
	}

	return "", 0
}

// findLastPunctuationWithMinLength 查找最后一个标点符号位置，确保最小长度
func findLastPunctuationWithMinLength(text string, punctuations []string, minLength int) (string, int) {
	// 安全检查：确保 minLength 不超过文本长度
	if minLength >= len(text) {
		minLength = len(text) - 1
		if minLength < 0 {
			return "", 0
		}
	}

	lastIndex := -1
	foundPunctuation := ""

	for _, punct := range punctuations {
		// 从最小长度位置开始查找
		searchText := text[minLength:]
		if idx := strings.LastIndex(searchText, punct); idx != -1 {
			actualIdx := idx + minLength
			if actualIdx > lastIndex {
				lastIndex = actualIdx
				foundPunctuation = punct
			}
		}
	}

	if lastIndex == -1 {
		return "", 0
	}

	endPos := lastIndex + len(foundPunctuation)
	// 确保不超出文本长度
	if endPos > len(text) {
		endPos = len(text)
	}
	return text[:endPos], endPos
}

// findLastSpaceWithMinLength 查找最后一个空格位置，确保最小长度
func findLastSpaceWithMinLength(text string, minLength int) (string, int) {
	// 安全检查：确保 minLength 不超过文本长度
	if minLength >= len(text) {
		minLength = len(text) - 1
		if minLength < 0 {
			return "", 0
		}
	}

	// 从最小长度位置开始查找空格
	searchText := text[minLength:]
	if idx := strings.LastIndex(searchText, " "); idx != -1 {
		actualIdx := idx + minLength
		return text[:actualIdx], actualIdx
	}
	return "", 0
}

// SplitByPunctuation 使用正则表达式分割文本
func SplitByPunctuation(text string) []string {
	// 使用正则表达式分割文本
	parts := reSplitString.Split(text, -1)

	// 过滤掉空字符串
	var result []string
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			result = append(result, part)
		}
	}

	return result
}

func RemoveMarkdownSyntax(text string) string {
	// 替换Markdown符号为空格
	cleaned := reMarkdownChars.ReplaceAllString(text, "")

	return cleaned
}

// RemoveAllPunctuation 移除所有标点符号
func RemoveAllPunctuation(text string) string {
	// 替换标点符号为空字符串
	cleaned := reRemoveAllPunctuation.ReplaceAllString(text, "")
	return cleaned
}

// extract_json_from_string 提取字符串中的 JSON 部分
func Extract_json_from_string(input string) map[string]interface{} {
	// 提取最外层的{}
	start := strings.Index(input, "{")
	if start == -1 {
		fmt.Println("没有找到JSON起始符号")
		return nil
	}
	bracketCount := 0
	end := -1
outer:
	for i := start; i < len(input); i++ {
		switch input[i] {
		case '{':
			bracketCount++
		case '}':
			bracketCount--
			if bracketCount == 0 {
				end = i
				break outer
			}
		}
	}
	if end == -1 {
		fmt.Println("没有找到完整的JSON结构")
		return nil
	}
	jsonStr := input[start : end+1]
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &jsonData); err != nil {
		fmt.Println("JSON解析错误:", err)
		return nil
	}
	return jsonData
}

// joinStrings 连接字符串切片
func JoinStrings(strs []string) string {
	var result string
	for _, s := range strs {
		result += s
	}
	return result
}

// IsWakeUpWord 判断是否是唤醒词，格式为"你好xx"
func IsWakeUpWord(text string) bool {
	// 检测是否匹配
	return reWakeUpWord.MatchString(text)
}

// IsInArray 判断text是否在字符串数组中
func IsInArray(text string, array []string) bool {
	for _, item := range array {
		if item == text {
			return true
		}
	}
	return false
}

// RandomSelectFromArray 从字符串数组中随机选择一个返回
func RandomSelectFromArray(array []string) string {
	if len(array) == 0 {
		return ""
	}

	// 生成随机索引
	index := rand.Intn(len(array))

	return array[index]
}

func GenerateSecurePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}|;:,.<>?/~`"
	password := make([]byte, length)
	for i := range password {
		password[i] = charset[rand.Intn(len(charset))]
	}
	return string(password)
}
