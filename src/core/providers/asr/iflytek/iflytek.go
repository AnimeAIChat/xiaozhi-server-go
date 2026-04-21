package iflytek

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"xiaozhi-server-go/src/core/providers/asr"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gorilla/websocket"
)

const (
	defaultBaseURL   = "wss://iat-api.xfyun.cn/v2/iat"
	defaultFrameSize = 1280
)

type Provider struct {
	*asr.BaseProvider

	appID       string
	apiKey      string
	apiSecret   string
	baseURL     string
	language    string
	domain      string
	accent      string
	encoding    string
	audioFmt    string
	vadEos      int
	dwa         string
	frameSize   int
	logger      *utils.Logger
	conn        *websocket.Conn
	isStreaming bool
	result      string
	err         error
	frameSeq    int
	segments    map[int]string
	connMutex   sync.Mutex
	finalCh     chan struct{}
	finalOnce   sync.Once
}

type asrResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Sid     string `json:"sid"`
	Data    struct {
		Status int `json:"status"`
		Result struct {
			Sn  int    `json:"sn"`
			Ls  bool   `json:"ls"`
			Pgs string `json:"pgs"`
			Rg  []int  `json:"rg"`
			Ws  []struct {
				Cw []struct {
					W string `json:"w"`
				} `json:"cw"`
			} `json:"ws"`
		} `json:"result"`
	} `json:"data"`
}

func init() {
	asr.Register("iflytek", func(config *asr.Config, deleteFile bool, logger *utils.Logger) (asr.Provider, error) {
		return NewProvider(config, deleteFile, logger)
	})
}

func NewProvider(config *asr.Config, deleteFile bool, logger *utils.Logger) (*Provider, error) {
	base := asr.NewBaseProvider(config, deleteFile)

	appID, _ := getString(config.Data, "appid")
	apiKey, _ := getString(config.Data, "api_key")
	if apiKey == "" {
		apiKey, _ = getString(config.Data, "access_token")
	}
	apiSecret, _ := getString(config.Data, "api_secret")
	if apiSecret == "" {
		apiSecret, _ = getString(config.Data, "cluster")
	}
	if appID == "" {
		return nil, fmt.Errorf("missing appid")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("missing api_key")
	}
	if apiSecret == "" {
		return nil, fmt.Errorf("missing api_secret")
	}

	baseURL, _ := getString(config.Data, "asr_url")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	language, _ := getString(config.Data, "language")
	if language == "" {
		language = "zh_cn"
	}
	domain, _ := getString(config.Data, "domain")
	if domain == "" {
		domain = "iat"
	}
	accent, _ := getString(config.Data, "accent")
	if accent == "" {
		accent = "mandarin"
	}
	encoding, _ := getString(config.Data, "encoding")
	if encoding == "" {
		encoding = "raw"
	}
	audioFmt, _ := getString(config.Data, "format")
	if audioFmt == "" {
		audioFmt = "audio/L16;rate=16000"
	}
	dwa, _ := getString(config.Data, "dwa")
	vadEos := getInt(config.Data, "vad_eos", 2000)
	frameSize := getInt(config.Data, "frame_size", defaultFrameSize)

	provider := &Provider{
		BaseProvider: base,
		appID:        appID,
		apiKey:       apiKey,
		apiSecret:    apiSecret,
		baseURL:      baseURL,
		language:     language,
		domain:       domain,
		accent:       accent,
		encoding:     encoding,
		audioFmt:     audioFmt,
		vadEos:       vadEos,
		dwa:          dwa,
		frameSize:    frameSize,
		logger:       logger,
		segments:     map[int]string{},
	}
	provider.InitAudioProcessing()

	return provider, nil
}

func (p *Provider) Initialize() error {
	return nil
}

func (p *Provider) Cleanup() error {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()
	p.closeConnection()
	return nil
}

func (p *Provider) CloseConnection() error {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()
	p.closeConnection()
	return nil
}

func (p *Provider) AddAudio(data []byte) error {
	return p.AddAudioWithContext(context.Background(), data)
}

func (p *Provider) AddAudioWithContext(ctx context.Context, data []byte) error {
	p.connMutex.Lock()
	isStreaming := p.isStreaming
	p.connMutex.Unlock()

	if !isStreaming {
		if err := p.StartStreaming(ctx); err != nil {
			return err
		}
	}

	if len(data) == 0 {
		return nil
	}

	status := 1
	p.connMutex.Lock()
	if p.frameSeq == 0 {
		status = 0
	}
	p.connMutex.Unlock()

	if err := p.sendFrame(data, status); err != nil {
		return err
	}

	p.connMutex.Lock()
	p.frameSeq++
	p.connMutex.Unlock()
	return nil
}

func (p *Provider) SendLastAudio(data []byte) error {
	p.connMutex.Lock()
	frameSeq := p.frameSeq
	p.connMutex.Unlock()

	if frameSeq == 0 && len(data) > 0 {
		if err := p.sendFrame(data, 0); err != nil {
			return err
		}
		p.connMutex.Lock()
		p.frameSeq++
		p.connMutex.Unlock()
		return p.sendFrame(nil, 2)
	}

	if len(data) > 0 {
		return p.sendFrame(data, 2)
	}
	return p.sendFrame(nil, 2)
}

func (p *Provider) StartStreaming(ctx context.Context) error {
	p.logger.Info("[ASR] [iflytek] [流式识别] 开始")
	p.ResetStartListenTime()

	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	if p.isStreaming {
		return nil
	}
	if p.conn != nil {
		p.closeConnection()
	}

	authURL, err := utils.BuildIFlytekAuthURL(p.baseURL, p.apiKey, p.apiSecret)
	if err != nil {
		return err
	}

	p.logger.Debug("[ASR] [iflytek] 开始连接讯飞 ASR WebSocket: endpoint=%s appid=%s api_key=%s language=%s domain=%s accent=%s format=%s encoding=%s vad_eos=%d",
		sanitizeIFlytekAuthURL(authURL),
		maskSecret(p.appID),
		maskSecret(p.apiKey),
		p.language,
		p.domain,
		p.accent,
		p.audioFmt,
		p.encoding,
		p.vadEos,
	)

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, authURL, nil)
	if err != nil {
		diagnostics := describeIFlytekHandshakeFailure(resp, authURL)
		p.logError("[ASR] [iflytek] 连接讯飞 ASR WebSocket 失败: err=%v %s", err, diagnostics)
		return fmt.Errorf("connect iFlytek ASR websocket failed: %w (%s)", err, diagnostics)
	}
	p.logger.Debug("[ASR] [iflytek] 讯飞 ASR WebSocket 连接成功: endpoint=%s status=%s", sanitizeIFlytekAuthURL(authURL), responseStatus(resp))

	p.conn = conn
	p.isStreaming = true
	p.err = nil
	p.result = ""
	p.frameSeq = 0
	p.segments = map[int]string{}
	p.finalCh = make(chan struct{})
	p.finalOnce = sync.Once{}

	go p.readLoop()
	return nil
}

func (p *Provider) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	if err := p.StartStreaming(ctx); err != nil {
		return "", err
	}

	for offset := 0; offset < len(audioData); offset += p.frameSize {
		end := offset + p.frameSize
		if end > len(audioData) {
			end = len(audioData)
		}
		chunk := audioData[offset:end]
		if end == len(audioData) {
			if err := p.SendLastAudio(chunk); err != nil {
				return "", err
			}
		} else if err := p.AddAudioWithContext(ctx, chunk); err != nil {
			return "", err
		}
	}

	if len(audioData) == 0 {
		if err := p.SendLastAudio(nil); err != nil {
			return "", err
		}
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-p.finalCh:
		if p.err != nil {
			return "", p.err
		}
		return p.result, nil
	case <-time.After(30 * time.Second):
		return "", fmt.Errorf("iFlytek ASR timeout")
	}
}

func (p *Provider) Reset() error {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	p.isStreaming = false
	p.result = ""
	p.err = nil
	p.frameSeq = 0
	p.segments = map[int]string{}
	p.closeConnection()
	p.InitAudioProcessing()
	p.signalFinal()
	return nil
}

func (p *Provider) readLoop() {
	defer func() {
		p.connMutex.Lock()
		p.isStreaming = false
		p.closeConnection()
		p.connMutex.Unlock()
		p.signalFinal()
	}()

	for {
		p.connMutex.Lock()
		if !p.isStreaming || p.conn == nil {
			p.connMutex.Unlock()
			return
		}
		conn := p.conn
		p.connMutex.Unlock()

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, message, err := conn.ReadMessage()
		if err != nil {
			p.setError(err)
			return
		}

		var response asrResponse
		if err := json.Unmarshal(message, &response); err != nil {
			p.setError(fmt.Errorf("parse iFlytek ASR response failed: %w", err))
			return
		}
		if response.Code != 0 {
			p.setError(fmt.Errorf("iFlytek ASR error %d: %s", response.Code, response.Message))
			return
		}

		result := response.Data.Result
		if len(result.Ws) > 0 {
			text := buildText(result.Ws)
			p.connMutex.Lock()
			if result.Pgs == "rpl" && len(result.Rg) == 2 {
				for i := result.Rg[0]; i <= result.Rg[1]; i++ {
					delete(p.segments, i)
				}
			}
			p.segments[result.Sn] = text
			p.result = p.joinSegmentsLocked()
			finalText := p.result
			p.connMutex.Unlock()

			if result.Ls || response.Data.Status == 2 {
				if listener := p.BaseProvider.GetListener(); listener != nil {
					listener.OnAsrResult(finalText, true)
				}
				return
			}
		} else if response.Data.Status == 2 {
			if listener := p.BaseProvider.GetListener(); listener != nil {
				listener.OnAsrResult(p.result, true)
			}
			return
		}
	}
}

func (p *Provider) sendFrame(audio []byte, status int) error {
	p.connMutex.Lock()
	conn := p.conn
	p.connMutex.Unlock()
	if conn == nil {
		return fmt.Errorf("iFlytek ASR websocket not connected")
	}

	payload := map[string]interface{}{
		"data": map[string]interface{}{
			"status":   status,
			"format":   p.audioFmt,
			"encoding": p.encoding,
			"audio":    base64.StdEncoding.EncodeToString(audio),
		},
	}

	if status == 0 {
		payload["common"] = map[string]interface{}{
			"app_id": p.appID,
		}
		business := map[string]interface{}{
			"language": p.language,
			"domain":   p.domain,
			"accent":   p.accent,
			"vad_eos":  p.vadEos,
		}
		if p.dwa != "" {
			business["dwa"] = p.dwa
		}
		payload["business"] = business
	}

	return conn.WriteJSON(payload)
}

func (p *Provider) closeConnection() {
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

func (p *Provider) setError(err error) {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()
	p.err = err
	p.isStreaming = false
}

func (p *Provider) signalFinal() {
	p.finalOnce.Do(func() {
		if p.finalCh != nil {
			close(p.finalCh)
		}
	})
}

func (p *Provider) joinSegmentsLocked() string {
	keys := make([]int, 0, len(p.segments))
	for key := range p.segments {
		keys = append(keys, key)
	}
	sort.Ints(keys)

	text := ""
	for _, key := range keys {
		text += p.segments[key]
	}
	return text
}

func buildText(ws []struct {
	Cw []struct {
		W string `json:"w"`
	} `json:"cw"`
}) string {
	text := ""
	for _, item := range ws {
		if len(item.Cw) == 0 {
			continue
		}
		text += item.Cw[0].W
	}
	return text
}

func getString(data map[string]interface{}, key string) (string, bool) {
	value, ok := data[key]
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return typed, true
	default:
		return fmt.Sprintf("%v", typed), true
	}
}

func getInt(data map[string]interface{}, key string, fallback int) int {
	value, ok := data[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func (p *Provider) logInfo(msg string, args ...interface{}) {
	if p.logger != nil {
		p.logger.Info(msg, args...)
	}
}

func (p *Provider) logError(msg string, args ...interface{}) {
	if p.logger != nil {
		p.logger.Error(msg, args...)
	}
}

func sanitizeIFlytekAuthURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-url>"
	}
	query := parsedURL.Query()
	if query.Has("authorization") {
		query.Set("authorization", "<redacted>")
	}
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String()
}

func describeIFlytekHandshakeFailure(resp *http.Response, rawURL string) string {
	details := []string{
		"endpoint=" + sanitizeIFlytekAuthURL(rawURL),
	}

	if parsedURL, err := url.Parse(rawURL); err == nil {
		query := parsedURL.Query()
		if date := query.Get("date"); date != "" {
			details = append(details, "auth_date="+date)
		}
		details = append(details, "host="+parsedURL.Host, "path="+parsedURL.EscapedPath())
	}

	if resp == nil {
		details = append(details, "http_status=<none>", "hint=未收到 HTTP 响应，优先检查网络/DNS/代理")
		return strings.Join(details, " ")
	}

	details = append(details,
		"http_status="+resp.Status,
		"response_headers="+formatHeader(resp.Header),
	)

	body := readResponseBody(resp.Body, 4096)
	if body != "" {
		details = append(details, "response_body="+body)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		details = append(details, "hint=401 通常表示 api_key/api_secret/appid/asr_url 或服务器时间签名不匹配")
	}

	return strings.Join(details, " ")
}

func responseStatus(resp *http.Response) string {
	if resp == nil {
		return "<none>"
	}
	return resp.Status
}

func formatHeader(header http.Header) string {
	if len(header) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", key, header.Values(key)))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func readResponseBody(body io.ReadCloser, limit int64) string {
	if body == nil {
		return ""
	}
	defer body.Close()

	data, err := io.ReadAll(io.LimitReader(body, limit))
	if err != nil {
		return fmt.Sprintf("<read failed: %v>", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return ""
	}
	if int64(len(data)) == limit {
		text += "...<truncated>"
	}
	return strconvQuoteForLog(text)
}

func strconvQuoteForLog(value string) string {
	quoted, _ := json.Marshal(value)
	return string(quoted)
}

func maskSecret(value string) string {
	if value == "" {
		return "<empty>"
	}
	if len(value) <= 6 {
		return strings.Repeat("*", len(value))
	}
	return value[:3] + strings.Repeat("*", len(value)-6) + value[len(value)-3:]
}
