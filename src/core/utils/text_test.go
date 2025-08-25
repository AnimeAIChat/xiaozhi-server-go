package utils

import (
	"fmt"
	"testing"
)

func TestRemoveAllPunctuation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "测试退出句号",
			input:    "退出。",
			expected: "退出",
		},
		{
			name:     "中文标点符号",
			input:    "你好，世界！这是一个测试。",
			expected: "你好世界这是一个测试",
		},
		{
			name:     "英文标点符号",
			input:    "Hello, world! This is a test.",
			expected: "Hello world This is a test",
		},
		{
			name:     "混合标点符号",
			input:    "测试：English, 中文！Mixed?",
			expected: "测试English 中文Mixed",
		},
		{
			name:     "特殊符号",
			input:    "符号@#$%^&*()测试",
			expected: "符号测试",
		},
		{
			name:     "引号和括号",
			input:    `"引号"、'单引号'（括号）【方括号】`,
			expected: "引号单引号括号方括号",
		},
		{
			name:     "书名号和破折号",
			input:    "《书名》——作者",
			expected: "书名作者",
		},
		{
			name:     "空字符串",
			input:    "",
			expected: "",
		},
		{
			name:     "纯标点符号",
			input:    "！@#$%^&*(),.?;:",
			expected: "",
		},
		{
			name:     "无标点符号",
			input:    "纯文本没有标点符号",
			expected: "纯文本没有标点符号",
		},
		{
			name:     "数字和字母",
			input:    "abc123测试!@#",
			expected: "abc123测试",
		},
		{
			name:     "省略号和连字符",
			input:    "测试…省略号—连字符-普通",
			expected: "测试省略号连字符普通",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveAllPunctuation(tt.input)
			if result != tt.expected {
				t.Errorf("RemoveAllPunctuation(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRemoveAllPunctuation_EdgeCases(t *testing.T) {
	t.Run("只有空格", func(t *testing.T) {
		result := RemoveAllPunctuation("   ")
		expected := "   " // 空格不是标点符号，应该保留
		if result != expected {
			t.Errorf("RemoveAllPunctuation(%q) = %q, want %q", "   ", result, expected)
		}
	})

	t.Run("混合空格和标点", func(t *testing.T) {
		result := RemoveAllPunctuation("测试 , 空格！")
		expected := "测试  空格" // 保留空格，移除标点
		if result != expected {
			t.Errorf("RemoveAllPunctuation(%q) = %q, want %q", "测试 , 空格！", result, expected)
		}
	})

	t.Run("连续标点符号", func(t *testing.T) {
		result := RemoveAllPunctuation("测试！！！。。。？？？")
		expected := "测试"
		if result != expected {
			t.Errorf("RemoveAllPunctuation(%q) = %q, want %q", "测试！！！。。。？？？", result, expected)
		}
	})
}

// 基准测试
func BenchmarkRemoveAllPunctuation(b *testing.B) {
	testString := "这是一个测试字符串，包含各种标点符号！@#$%^&*()，。？；：\"\"''「」『』（）【】[]{}《》〈〉—–-_~·…‖|\\/*&^%$#@+=<>"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RemoveAllPunctuation(testString)
	}
}

func BenchmarkRemoveAllPunctuation_Short(b *testing.B) {
	testString := "退出。"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RemoveAllPunctuation(testString)
	}
}

func TestSplitAtLastPunctuation(t *testing.T) {
	msg := ""
	cnt := 0
	strings := []string{}
	for len(msg) > 20 {
		text, n := SplitAtLastPunctuation(msg)
		// 检查是否找到分割点
		if n == 0 || len(text) == 0 {
			fmt.Println("没有找到分割点，结束分割")
			break
		}
		strings = append(strings, msg[n:])
		msg = text[:len(text)-1]
		cnt++
		if cnt > 210 {
			fmt.Println("... 结束 ...")
			break
		}
	}
	// 倒序打印
	for i := len(strings) - 1; i >= 0; i-- {
		fmt.Printf("第 %d 段: %s\n", i+1, strings[i])
	}
}
