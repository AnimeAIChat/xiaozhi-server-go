package doubao

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"xiaozhi-server-go/src/core/providers/tts"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var (
	enumMessageType = map[byte]string{
		11: "audio-only server response",
		12: "frontend server response",
		15: "error message from server",
	}
	enumMessageTypeSpecificFlags = map[byte]string{
		0: "no sequence number",
		1: "sequence number > 0",
		2: "last message from server (seq < 0)",
		3: "sequence number < 0",
	}
)

// 默认的二进制消息头
// version: b0001 (4 bits)
// header size: b0001 (4 bits)
// message type: b0001 (Full client request) (4bits)
// message type specific flags: b0000 (none) (4bits)
// message serialization method: b0001 (JSON) (4bits)
// message compression: b0001 (gzip) (4bits)
// reserved data: 0x00 (1 byte)
var defaultHeader = []byte{0x11, 0x10, 0x11, 0x00}

type synResp struct {
	Audio  []byte
	IsLast bool
}

// Provider 豆包 TTS 提供者
type Provider struct {
	*tts.BaseProvider
	baseURL string
}

// NewProvider 创建豆包 TTS 提供者
func NewProvider(config *tts.Config, deleteFile bool) (*Provider, error) {
	base := tts.NewBaseProvider(config, deleteFile)
	u := url.URL{Scheme: "wss", Host: "openspeech.bytedance.com", Path: "/api/v1/tts/ws_binary"}

	return &Provider{
		BaseProvider: base,
		baseURL:      u.String(),
	}, nil
}

// ToTTS 实现文本到语音的转换
func (p *Provider) ToTTS(text string) (string, error) {
	// 创建WebSocket连接
	header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer;%s", p.Config().Token)}}
	conn, _, err := websocket.DefaultDialer.Dial(p.baseURL, header)
	if err != nil {
		return "", fmt.Errorf("连接WebSocket服务器失败: %v", err)
	}
	defer conn.Close()

	// 准备请求参数
	reqParams := map[string]map[string]interface{}{
		"app": {
			"appid":   p.Config().AppID,
			"token":   p.Config().Token,
			"cluster": p.Config().Cluster,
		},
		"user": {
			"uid": "uid",
		},
		"audio": {
			"voice_type":   p.Config().Voice,
			"encoding":     "mp3",
			"speed_ratio":  1.0,
			"volume_ratio": 1.0,
			"pitch_ratio":  1.0,
		},
		"request": {
			"reqid":     uuid.New().String(),
			"text":      text,
			"text_type": "plain",
			"operation": "submit", // 使用流式合成
		},
	}

	// 序列化并压缩请求参数
	jsonData, err := json.Marshal(reqParams)
	if err != nil {
		return "", fmt.Errorf("序列化请求参数失败: %v", err)
	}

	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(jsonData); err != nil {
		return "", fmt.Errorf("压缩请求数据失败: %v", err)
	}
	w.Close()
	compressed := b.Bytes()

	// 构建完整的二进制请求
	payloadSize := make([]byte, 4)
	binary.BigEndian.PutUint32(payloadSize, uint32(len(compressed)))
	request := make([]byte, len(defaultHeader))
	copy(request, defaultHeader)
	request = append(request, payloadSize...)
	request = append(request, compressed...)

	// 发送请求
	if err := conn.WriteMessage(websocket.BinaryMessage, request); err != nil {
		return "", fmt.Errorf("发送请求失败: %v", err)
	}

	// 创建临时文件
	outputDir := p.Config().OutputDir
	if outputDir == "" {
		outputDir = "tmp"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("创建输出目录失败: %v", err)
	}

	tempFile := filepath.Join(outputDir, fmt.Sprintf("doubao_tts_%d.mp3", time.Now().UnixNano()))
	var audioData []byte

	// 接收音频数据
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return "", fmt.Errorf("接收响应失败: %v", err)
		}

		resp, err := p.parseResponse(message)
		if err != nil {
			return "", fmt.Errorf("解析响应失败: %v", err)
		}

		audioData = append(audioData, resp.Audio...)
		if resp.IsLast {
			break
		}
	}

	// 写入音频文件
	if err := os.WriteFile(tempFile, audioData, 0644); err != nil {
		return "", fmt.Errorf("写入音频文件失败: %v", err)
	}

	return tempFile, nil
}

// parseResponse 解析服务器响应
func (p *Provider) parseResponse(res []byte) (resp synResp, err error) {
	if len(res) < 4 {
		return resp, fmt.Errorf("响应数据长度不足")
	}

	messageType := res[1] >> 4
	messageTypeSpecificFlags := res[1] & 0x0f
	headSize := res[0] & 0x0f
	payload := res[headSize*4:]

	switch messageType {
	case 0xb: // audio-only server response
		if messageTypeSpecificFlags != 0 {
			// 有序列号的响应
			if len(payload) < 8 {
				return resp, fmt.Errorf("音频数据长度不足")
			}
			sequenceNumber := int32(binary.BigEndian.Uint32(payload[0:4]))
			payload = payload[8:]
			resp.Audio = append(resp.Audio, payload...)
			if sequenceNumber < 0 {
				resp.IsLast = true
			}
		}
       case 0xf: // error message
	       if len(payload) < 8 {
		       return resp, fmt.Errorf("错误消息数据长度不足")
	       }
	       code := int32(binary.BigEndian.Uint32(payload[0:4]))
	       errMsg := payload[8:]
	       // 总是尝试 gzip 解压
	       r, gzErr := gzip.NewReader(bytes.NewReader(errMsg))
	       if gzErr == nil {
		       if errMsg2, err2 := io.ReadAll(r); err2 == nil {
			       errMsg = errMsg2
		       }
		       r.Close()
	       }
	       return resp, fmt.Errorf("服务器错误 [%d]: %s", code, string(errMsg))
	default:
		return resp, fmt.Errorf("未知的消息类型: %d", messageType)
	}

	return resp, nil
}

func init() {
	tts.Register("doubao", func(config *tts.Config, deleteFile bool) (tts.Provider, error) {
		return NewProvider(config, deleteFile)
	})
}
