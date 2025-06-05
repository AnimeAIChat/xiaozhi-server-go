package utils

import (
	"encoding/json"
	"regexp"
	"strings"
)

// splitAtLastPunctuation 在最后一个标点符号处分割文本
func SplitAtLastPunctuation(text string) (string, int) {
	punctuations := []string{"。", "？", "！", "；", "："}
	lastIndex := -1

	for _, punct := range punctuations {
		if idx := strings.LastIndex(text, punct); idx > lastIndex {
			lastIndex = idx
		}
	}

	if lastIndex == -1 {
		return "", 0
	}

	return text[:lastIndex+len("。")], lastIndex + len("。")
}

func RemoveMarkdownSyntax(text string) string {
	// 定义需要移除的Markdown语法符号,包括空格
	markdownChars := `[\*#\-+=>` + "`" + `~_\[\](){}|\\\ ]`

	// 编译正则表达式
	re := regexp.MustCompile(markdownChars)

	// 替换Markdown符号为空格
	cleaned := re.ReplaceAllString(text, "")

	return cleaned
}

// RemoveAllPunctuation 移除所有标点符号
func RemoveAllPunctuation(text string) string {
	// 定义所有标点符号（中文、英文标点）- 修复转义问题
	punctuation := `[.,!?;:，。！？、；：""''「」『』（）\(\)【】\[\]{}《》〈〉—–\-_~·…‖\|\\/*&\^%\$#@\+=<>]`

	// 编译正则表达式
	re := regexp.MustCompile(punctuation)

	// 替换标点符号为空字符串
	cleaned := re.ReplaceAllString(text, "")

	return cleaned
}

// extract_json_from_string 提取字符串中的 JSON 部分
func Extract_json_from_string(input string) map[string]interface{} {
	pattern := `(\{.*\})`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(input)
	if len(matches) > 1 {
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(matches[1]), &result); err == nil {
			return result
		}
	}
	return nil
}

// joinStrings 连接字符串切片
func JoinStrings(strs []string) string {
	var result string
	for _, s := range strs {
		result += s
	}
	return result
}
