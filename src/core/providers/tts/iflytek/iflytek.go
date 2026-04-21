package iflytek

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"xiaozhi-server-go/src/core/providers/tts"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gorilla/websocket"
)

const defaultBaseURL = "wss://tts-api.xfyun.cn/v2/tts"

type Provider struct {
	*tts.BaseProvider
	baseURL string
}

type ttsResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Sid     string `json:"sid"`
	Data    struct {
		Audio  string `json:"audio"`
		Status int    `json:"status"`
	} `json:"data"`
}

func init() {
	tts.Register("iflytek", func(config *tts.Config, deleteFile bool) (tts.Provider, error) {
		return NewProvider(config, deleteFile)
	})
}

func NewProvider(config *tts.Config, deleteFile bool) (*Provider, error) {
	base := tts.NewBaseProvider(config, deleteFile)
	return &Provider{
		BaseProvider: base,
		baseURL:      defaultBaseURL,
	}, nil
}

func (p *Provider) ToTTS(text string) (string, error) {
	if p.Config().AppID == "" {
		return "", fmt.Errorf("missing iFlytek appid")
	}
	if p.Config().Token == "" {
		return "", fmt.Errorf("missing iFlytek api_key in token field")
	}
	if p.Config().Cluster == "" {
		return "", fmt.Errorf("missing iFlytek api_secret in cluster field")
	}

	authURL, err := utils.BuildIFlytekAuthURL(p.baseURL, p.Config().Token, p.Config().Cluster)
	if err != nil {
		return "", err
	}

	conn, _, err := websocket.DefaultDialer.Dial(authURL, nil)
	if err != nil {
		return "", fmt.Errorf("connect iFlytek TTS websocket failed: %w", err)
	}
	defer conn.Close()

	audioEncoding, fileExt := resolveAudioFormat(p.Config().Format)
	request := map[string]interface{}{
		"common": map[string]interface{}{
			"app_id": p.Config().AppID,
		},
		"business": map[string]interface{}{
			"aue":    audioEncoding,
			"auf":    "audio/L16;rate=24000",
			"vcn":    p.Config().Voice,
			"tte":    "UTF8",
			"speed":  50,
			"pitch":  50,
			"volume": 50,
		},
		"data": map[string]interface{}{
			"status": 2,
			"text":   base64.StdEncoding.EncodeToString([]byte(text)),
		},
	}

	if p.Config().Voice == "" {
		request["business"].(map[string]interface{})["vcn"] = "xiaoyan"
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal iFlytek TTS request failed: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, requestBytes); err != nil {
		return "", fmt.Errorf("send iFlytek TTS request failed: %w", err)
	}

	var audioData []byte
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return "", fmt.Errorf("read iFlytek TTS response failed: %w", err)
		}

		var response ttsResponse
		if err := json.Unmarshal(message, &response); err != nil {
			return "", fmt.Errorf("parse iFlytek TTS response failed: %w", err)
		}
		if response.Code != 0 {
			return "", fmt.Errorf("iFlytek TTS error %d: %s", response.Code, response.Message)
		}

		if response.Data.Audio != "" {
			chunk, err := base64.StdEncoding.DecodeString(response.Data.Audio)
			if err != nil {
				return "", fmt.Errorf("decode iFlytek TTS audio failed: %w", err)
			}
			audioData = append(audioData, chunk...)
		}

		if response.Data.Status == 2 {
			break
		}
	}

	outputDir := p.Config().OutputDir
	if outputDir == "" {
		outputDir = "tmp"
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir failed: %w", err)
	}

	tempFile := filepath.Join(outputDir, fmt.Sprintf("iflytek_tts_%d.%s", time.Now().UnixNano(), fileExt))
	if fileExt == "wav" {
		audioData = buildWAV(audioData, 24000, 1, 16)
	}
	if err := os.WriteFile(tempFile, audioData, 0o644); err != nil {
		return "", fmt.Errorf("write iFlytek TTS audio failed: %w", err)
	}

	return tempFile, nil
}

func resolveAudioFormat(format string) (string, string) {
	switch format {
	case "wav", "pcm", "raw":
		return "raw", "wav"
	default:
		return "lame", "mp3"
	}
}

func buildWAV(pcm []byte, sampleRate, channels, bitsPerSample int) []byte {
	header := make([]byte, 44)
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8

	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+len(pcm)))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(header[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(header[34:36], uint16(bitsPerSample))
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(len(pcm)))

	wav := make([]byte, 0, len(header)+len(pcm))
	wav = append(wav, header...)
	wav = append(wav, pcm...)
	return wav
}
