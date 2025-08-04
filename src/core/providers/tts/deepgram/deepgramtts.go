package deepgram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"xiaozhi-server-go/src/core/providers/tts"

	"github.com/gorilla/websocket"
)

// Provider Deepgram TTS 提供者
type Provider struct {
	*tts.BaseProvider
	baseURL string
}

// NewProvider 创建Deepgram TTS提供者
func NewProvider(config *tts.Config, deleteFile bool) (*Provider, error) {
	base := tts.NewBaseProvider(config, deleteFile)

	// 构造带参数的URL
	// u := fmt.Sprintf("%v?model=%s&encoding=%s&sample_rate=%d",
	u := fmt.Sprintf("%v?model=%s", config.Cluster, config.Voice)
	return &Provider{
		BaseProvider: base,
		baseURL:      u,
	}, nil
}

// ToTTS 实现文本到语音的转换
func (p *Provider) ToTTS(text string) (string, error) {
	// 创建WebSocket连接
	header := http.Header{"Authorization": []string{fmt.Sprintf("token %s", p.Config().Token)}}
	conn, _, err := websocket.DefaultDialer.Dial(p.baseURL, header)
	if err != nil {
		return "", fmt.Errorf("连接Deepgram TTS服务器失败: %v", err)
	}
	defer conn.Close()

	// 发送文本消息
	speakRequest := map[string]string{
		"type": "Speak",
		"text": text,
	}
	requestBytes, err := json.Marshal(speakRequest)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, requestBytes); err != nil {
		return "", fmt.Errorf("发送speak请求失败: %v", err)
	}

	// 发送Flush控制消息确保所有音频数据返回
	flushRequest := map[string]string{"type": "Flush"}
	if err := conn.WriteJSON(flushRequest); err != nil {
		return "", fmt.Errorf("发送Flush请求失败: %v", err)
	}

	// 创建临时文件
	outputDir := p.Config().OutputDir
	if outputDir == "" {
		outputDir = "tmp"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("创建输出目录失败: %v", err)
	}

	// ext := getFileExtension(p.Config().Encoding)
	ext := "mp3"
	tempFile := filepath.Join(outputDir, fmt.Sprintf("deepgram_tts_%d.%s", time.Now().UnixNano(), ext))
	// 接收音频数据
	var lastSeqID int
	// 接收音频数据
	var audioBuffer bytes.Buffer
loop:
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
				return "", fmt.Errorf("接收响应异常: %v", err)
			}
			break // 正常关闭
		}

		switch messageType {
		case websocket.TextMessage:
			// 处理控制消息响应
			var response struct {
				Type       string `json:"type"`
				SequenceID int    `json:"sequence_id,omitempty"`
				Error      string `json:"error,omitempty"`
			}

			if err := json.Unmarshal(message, &response); err != nil {
				return "", fmt.Errorf("解析控制消息失败: %v", err)
			}

			switch response.Type {
			case "Flushed":
				// 记录最后序列ID
				lastSeqID = response.SequenceID
				break loop
			case "close":
				// 服务器确认关闭
				break loop
			case "error":
				return "", fmt.Errorf("Deepgram TTS错误: %s", response.Error)
			}
		case websocket.BinaryMessage:
			// 二进制音频数据
			// 直接写入二进制音频数据到文件
			// 同时缓冲到内存以备完整性检查
			audioBuffer.Write(message)
		case websocket.CloseMessage:
			break loop
		}
	}

	// 验证音频完整性（可选）
	// 检查是否接收到音频数据
	if lastSeqID > 0 && audioBuffer.Len() == 0 {
		return "", fmt.Errorf("音频数据不完整，最后接收序列号: %d", lastSeqID)
	}

	// 写入音频文件
	if err := os.WriteFile(tempFile, audioBuffer.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("写入音频文件失败: %v", err)
	}

	return tempFile, nil
}

// getFileExtension 根据编码获取文件扩展名
func getFileExtension(encoding string) string {
	switch encoding {
	case "linear16":
		return "wav"
	case "opus":
		return "opus"
	case "flac":
		return "flac"
	case "aac":
		return "aac"
	case "alaw", "mulaw":
		return "wav"
	default: // mp3
		return "mp3"
	}
}

func init() {
	tts.Register("deepgram", func(config *tts.Config, deleteFile bool) (tts.Provider, error) {
		return NewProvider(config, deleteFile)
	})
}
