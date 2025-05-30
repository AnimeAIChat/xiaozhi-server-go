package utils

import "strings"

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
