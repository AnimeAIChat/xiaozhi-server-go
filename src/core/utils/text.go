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
	reMarkdownChars        = regexp.MustCompile(`[\*#\-+=>` + "`" + `~_\[\](){}|\\]`)
	reRemoveAllPunctuation = regexp.MustCompile(
		`[.,!?;:，。！？、；：""''「」『』（）\(\)【】\[\]{}《》〈〉—–\-_~·…‖\|\\/*&\^%\$#@\+=<>]`,
	)
	reWakeUpWord = regexp.MustCompile(`^你好.+`)
)

// splitAtLastPunctuation 在最后一个标点符号处分割文本
func SplitAtLastPunctuation(text string) (string, int) {
	punctuations := []string{"。", "？", "！", "；", "：", ".", "?", "!", ";", ":"}
	lastIndex := -1
	foundPunctuation := ""

	for _, punct := range punctuations {
		if idx := strings.LastIndex(text, punct); idx > lastIndex {
			lastIndex = idx
			foundPunctuation = punct
		}
	}

	if lastIndex == -1 {
		return "", 0
	}

	endPos := lastIndex + len(foundPunctuation)
	return text[:endPos], endPos
}

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
