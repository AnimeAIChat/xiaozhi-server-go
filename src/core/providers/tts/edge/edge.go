package edge

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
	"xiaozhi-server-go/src/core/providers/tts"

	"github.com/wujunwei928/edge-tts-go/edge_tts"
)

// Provider Edge TTS提供者实现
type Provider struct {
	*tts.BaseProvider
}

// NewProvider 创建Edge TTS提供者
func NewProvider(config *tts.Config, deleteFile bool) (*Provider, error) {
	base := tts.NewBaseProvider(config, deleteFile)
	return &Provider{
		BaseProvider: base,
	}, nil
}

// ToTTS 将文本转换为音频文件，并返回文件路径
// 使用的edge库是github.com/wujunwei928/edge-tts-go，默认使用24k采样率
func (p *Provider) ToTTS(text string) (string, error) {
	// 获取配置的声音，如果未配置则使用默认值
	edgeTTSStartTime := time.Now()
	voice := p.BaseProvider.Config().Voice
	if voice == "" {
		voice = "zh-CN-XiaoxiaoNeural" // 默认声音
	}

	// 创建临时文件路径用于保存 edgeTTS 生成的 MP3
	outputDir := p.BaseProvider.Config().OutputDir
	if outputDir == "" {
		outputDir = os.TempDir() // Use system temp dir if not configured
	}
	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("创建输出目录失败 '%s': %v", outputDir, err)
	}
	// Use a unique filename
	tempFile := filepath.Join(outputDir, fmt.Sprintf("edge_tts_go_%d.mp3", time.Now().UnixNano()))

	// 配置 edge-tts-go 连接选项
	connOptions := []edge_tts.CommunicateOption{
		edge_tts.SetVoice(voice),
	}

	// 创建 Communicate 实例
	conn, err := edge_tts.NewCommunicate(text, connOptions...)
	if err != nil {
		return "", fmt.Errorf("创建 edge-tts-go Communicate 失败: %v", err)
	}

	// 获取音频流数据
	audioData, err := conn.Stream()
	if err != nil {
		return "", fmt.Errorf("edge-tts-go 获取音频流失败: %v", err)
	}

	ttsDuration := time.Since(edgeTTSStartTime)
	_ = ttsDuration
	//fmt.Println(fmt.Sprintf("edge-tts-go 语音合成完成，耗时: %s", ttsDuration))

	// 将音频数据写入临时文件
	err = os.WriteFile(tempFile, audioData, 0644)
	if err != nil {
		return "", fmt.Errorf("写入音频文件 '%s' 失败: %v", tempFile, err)
	}

	// 检查文件是否成功创建
	if _, err := os.Stat(tempFile); os.IsNotExist(err) {
		return "", fmt.Errorf("edge-tts-go 未能创建音频文件: %s", tempFile)
	}
	//fmt.Printf("音频文件已生成: %s\n", tempFile)

	// Return the path to the generated audio file
	return tempFile, nil
}

func init() {
	// 注册Edge TTS提供者
	tts.Register("edge", func(config *tts.Config, deleteFile bool) (tts.Provider, error) {
		return NewProvider(config, deleteFile)
	})
}
