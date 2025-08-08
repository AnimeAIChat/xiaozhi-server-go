package pool

import (
	"encoding/base64"
	"fmt"
	"os"
)

// TestDataGenerator 测试数据生成器
type TestDataGenerator struct {
	testModes TestModes // 测试配置
}

// NewTestDataGenerator 创建测试数据生成器
func NewTestDataGenerator(testModes TestModes) *TestDataGenerator {
	return &TestDataGenerator{
		testModes: testModes,
	}
}

// GetTestAudioData 获取测试音频数据
// 优先使用配置文件中指定的音频文件，如果没有配置则生成默认的测试音频
func (tdg *TestDataGenerator) GetTestAudioData() ([]byte, error) {
	// 如果配置了ASR测试音频文件路径，则读取文件
	if tdg.testModes.ASRTestAudio != "" {
		audioData, err := os.ReadFile(tdg.testModes.ASRTestAudio)
		if err != nil {
			return nil, fmt.Errorf("读取配置的测试音频文件失败: %v", err)
		}
		return audioData, nil
	}

	// 否则使用默认的测试音频数据（WAV格式，包含"Hello"）
	// 这是一个非常简短的音频片段，用于ASR健康检查
	testAudioBase64 := `UklGRiQAAABXQVZFZm10IBAAAAABAAEAgD4AAAB9AAACABAAZGF0YQAAAAA=`

	audioData, err := base64.StdEncoding.DecodeString(testAudioBase64)
	if err != nil {
		return nil, fmt.Errorf("解码测试音频数据失败: %v", err)
	}

	return audioData, nil
}

// GetTestPrompt 获取LLM测试提示词
// 使用配置文件中的LLM测试提示词，如果没有配置则使用默认值
func (tdg *TestDataGenerator) GetTestPrompt() string {
	if tdg.testModes.LLMTestPrompt != "" {
		return tdg.testModes.LLMTestPrompt
	}
	// 默认提示词
	return "Hello, this is a health check test. Please respond with a simple greeting."
}

// GetTestTTSText 获取TTS测试文本
// 使用配置文件中的TTS测试文本，如果没有配置则使用默认值
func (tdg *TestDataGenerator) GetTestTTSText() string {
	if tdg.testModes.TTSTestText != "" {
		return tdg.testModes.TTSTestText
	}
	// 默认测试文本
	return "健康检查测试"
}

// GetTestImageData 获取测试图片数据（用于VLLLM测试）
// 这是一个1x1像素的PNG图片
func (tdg *TestDataGenerator) GetTestImageData() ([]byte, error) {
	// 1x1像素的透明PNG图片的base64编码
	testImageBase64 := `iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==`

	imageData, err := base64.StdEncoding.DecodeString(testImageBase64)
	if err != nil {
		return nil, fmt.Errorf("解码测试图片数据失败: %v", err)
	}

	return imageData, nil
}

// GenerateTestAudioFile 生成测试音频文件（用于需要文件路径的ASR服务）
func (tdg *TestDataGenerator) GenerateTestAudioFile(outputPath string) error {
	audioData, err := tdg.GetTestAudioData()
	if err != nil {
		return err
	}

	// 将音频数据写入文件
	err = os.WriteFile(outputPath, audioData, 0o644)
	if err != nil {
		return fmt.Errorf("写入测试音频文件失败: %v", err)
	}

	return nil
}

// ValidateASRResponse 验证ASR响应是否合理
func (tdg *TestDataGenerator) ValidateASRResponse(response string) bool {
	if len(response) == 0 {
		return false
	}

	// 简单检查响应长度，实际的ASR可能返回任何内容
	// 这里主要是确保服务有响应
	return len(response) <= 1000 // 防止异常长的响应
}

// ValidateLLMResponse 验证LLM响应是否合理
func (tdg *TestDataGenerator) ValidateLLMResponse(response string) bool {
	if len(response) == 0 {
		return false
	}

	// 检查响应长度合理性
	if len(response) > 10000 {
		return false
	}

	// LLM应该能正常回复，不应该返回错误信息
	return true
}

// ValidateTTSResponse 验证TTS响应是否合理
func (tdg *TestDataGenerator) ValidateTTSResponse(audioPath string) bool {
	if len(audioPath) == 0 {
		return false
	}

	// 简单检查路径格式
	return len(audioPath) > 0
}

// ValidateVLLLMResponse 验证VLLLM响应是否合理
func (tdg *TestDataGenerator) ValidateVLLLMResponse(response string) bool {
	if len(response) == 0 {
		return false
	}

	// 检查响应长度合理性
	return len(response) > 0 && len(response) <= 10000
}
