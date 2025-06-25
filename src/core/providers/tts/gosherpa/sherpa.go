package gosherpa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"xiaozhi-server-go/src/core/providers/tts"

	"github.com/gorilla/websocket"
)

// Provider Sherpa TTS提供者实现
type Provider struct {
	*tts.BaseProvider
	conn *websocket.Conn
}

// NewProvider 创建Sherpa TTS提供者
func NewProvider(config *tts.Config, deleteFile bool) (*Provider, error) {
	base := tts.NewBaseProvider(config, deleteFile)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second, // 设置握手超时
	}
	conn, _, err := dialer.DialContext(context.Background(), config.Cluster, map[string][]string{})
	if err != nil {
		return nil, err
	}

	return &Provider{
		BaseProvider: base,
		conn:         conn,
	}, nil
}

// ToTTS 将文本转换为音频文件，并返回文件路径
func (p *Provider) ToTTS(text string) (string, error) {
	// 获取配置的声音，如果未配置则使用默认值
	SherpaTTSStartTime := time.Now()

	// 创建临时文件路径用于保存 SherpaTTS 生成的 MP3
	outputDir := p.BaseProvider.Config().OutputDir
	if outputDir == "" {
		outputDir = os.TempDir() // Use system temp dir if not configured
	}
	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("创建输出目录失败 '%s': %v", outputDir, err)
	}
	// Use a unique filename
	tempFile := filepath.Join(outputDir, fmt.Sprintf("go_sherpa_tts_%d.wav", time.Now().UnixNano()))

	p.conn.WriteMessage(websocket.TextMessage, []byte(text))
	_, bytes, err := p.conn.ReadMessage()

	if err != nil {
		return "", fmt.Errorf("go-sherpa-tts 获取音频流失败: %v", err)
	}

	ttsDuration := time.Since(SherpaTTSStartTime)
	fmt.Println(fmt.Sprintf("go-sherpa-tts 语音合成完成，耗时: %s", ttsDuration))

	// 将音频数据写入临时文件
	err = os.WriteFile(tempFile, bytes, 0644)
	if err != nil {
		return "", fmt.Errorf("写入音频文件 '%s' 失败: %v", tempFile, err)
	}

	// 检查文件是否成功创建
	if _, err := os.Stat(tempFile); os.IsNotExist(err) {
		return "", fmt.Errorf("go-sherpa-tts 未能创建音频文件: %s", tempFile)
	}

	// Return the path to the generated audio file
	return tempFile, nil
}

func init() {
	// 注册Sherpa TTS提供者
	tts.Register("gosherpa", func(config *tts.Config, deleteFile bool) (tts.Provider, error) {
		return NewProvider(config, deleteFile)
	})
}
