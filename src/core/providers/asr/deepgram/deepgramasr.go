package deepgram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xiaozhi-server-go/src/core/providers/asr"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gorilla/websocket"
)

// Ensure Provider implements asr.Provider interface
var _ asr.Provider = (*Provider)(nil)

// Provider Deepgram ASR provider implementation
type Provider struct {
	*asr.BaseProvider
	apiKey    string
	language  string
	outputDir string
	wsURL     string
	logger    *utils.Logger

	// Streaming related fields
	conn        *websocket.Conn
	isStreaming bool
	reqID       string
	result      string
	err         error
	connMutex   sync.Mutex

	sendDataCnt int
}

// NewProvider creates a new Deepgram ASR provider instance
func NewProvider(config *asr.Config, deleteFile bool, logger *utils.Logger) (*Provider, error) {
	base := asr.NewBaseProvider(config, deleteFile)

	// Get configuration from config.Data
	apiKey, ok := config.Data["api_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing api_key configuration")
	}

	language, ok := config.Data["lang"].(string)
	if !ok {
		language = "en" // Default to English US
	}

	// Ensure output directory exists
	outputDir, _ := config.Data["output_dir"].(string)
	if outputDir == "" {
		outputDir = "tmp/"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %v", err)
	}

	// Get model from config, default to nova
	// model, _ := config.Data["model"].(string)
	// if model == "" {
	// 	model = "nova"
	// }

	provider := &Provider{
		BaseProvider: base,
		apiKey:       apiKey,
		language:     language,
		outputDir:    outputDir,
		wsURL:        "wss://api.deepgram.com/v1/listen",
		// model:        model,
		// punctuate: false, // Default to true for punctuation
		logger: logger,
	}

	// Initialize audio processing
	provider.InitAudioProcessing()

	return provider, nil
}

// Transcribe implements the asr.Provider interface transcription method
func (p *Provider) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	if p.isStreaming {
		return "", fmt.Errorf("streaming transcription in progress, please call Reset first")
	}

	// Create temporary file
	tempFile := filepath.Join(p.outputDir, fmt.Sprintf("temp_%d.wav", time.Now().UnixNano()))
	if err := os.WriteFile(tempFile, audioData, 0644); err != nil {
		return "", fmt.Errorf("failed to save temporary file: %v", err)
	}
	defer func() {
		if p.DeleteFile() {
			os.Remove(tempFile)
		}
	}()

	// Initialize connection
	if err := p.Initialize(); err != nil {
		return "", err
	}
	defer p.Cleanup()

	// Add audio data
	if err := p.AddAudioWithContext(ctx, audioData); err != nil {
		return "", err
	}

	return p.result, nil
}

// AddAudio adds audio data to the buffer
func (p *Provider) AddAudio(data []byte) error {
	return p.AddAudioWithContext(context.Background(), data)
}

// AddAudioWithContext adds audio data with context
func (p *Provider) AddAudioWithContext(ctx context.Context, data []byte) error {
	p.connMutex.Lock()
	isStreaming := p.isStreaming
	p.connMutex.Unlock()

	if !isStreaming {
		err := p.StartStreaming(ctx)
		if err != nil {
			return err
		}
	}

	if len(data) > 0 && p.isStreaming {
		if err := p.sendAudioData(data, false); err != nil {
			return err
		} else {
			p.sendDataCnt += 1
			if p.sendDataCnt%20 == 0 {
				p.logger.Debug("Audio data sent successfully, length: %d bytes", len(data))
			}
		}
	}

	return nil
}

// StartStreaming starts the streaming transcription
func (p *Provider) StartStreaming(ctx context.Context) error {
	p.logger.Info("----Starting streaming transcription----")
	p.ResetStartListenTime()

	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	if p.isStreaming {
		return nil
	}

	p.InitAudioProcessing()
	p.result = ""
	p.err = nil

	if p.conn != nil {
		p.closeConnection()
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Add query parameters
	queryParams := fmt.Sprintf("?language=%s&sample_rate=%v&encoding=%v",
		p.language, 16000, "linear16")

	headers := http.Header{
		"Authorization": []string{"token " + p.apiKey},
	}

	var conn *websocket.Conn
	var resp *http.Response
	var err error
	maxRetries := 2

	for i := 0; i <= maxRetries; i++ {
		conn, resp, err = dialer.DialContext(ctx, p.wsURL+queryParams, headers)
		if err == nil {
			break
		}

		if i < maxRetries {
			backoffTime := time.Duration(500*(i+1)) * time.Millisecond
			p.logger.Debug("WebSocket connection failed (attempt %d/%d): %v, retrying in %v",
				i+1, maxRetries+1, err, backoffTime)
			time.Sleep(backoffTime)
		}
	}

	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return fmt.Errorf("WebSocket connection failed (status code:%d): %v", statusCode, err)
	}

	p.conn = conn
	p.isStreaming = true
	p.reqID = fmt.Sprintf("%d", time.Now().UnixNano())

	p.logger.Debug("[DEBUG] Streaming initialized successfully, reqID=%s", p.reqID)

	go p.ReadMessage()
	return nil
}

// ReadMessage reads messages from the WebSocket connection
func (p *Provider) ReadMessage() {
	p.logger.Info("Deepgram streaming thread started")
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("Streaming thread error: %v", r)
		}
		p.connMutex.Lock()
		p.isStreaming = false
		if p.conn != nil {
			p.closeConnection()
		}
		p.connMutex.Unlock()
		p.logger.Info("Deepgram streaming thread ended")
	}()

	for {
		p.connMutex.Lock()
		if !p.isStreaming || p.conn == nil {
			p.connMutex.Unlock()
			p.logger.Info("Streaming ended or connection closed, exiting read loop")
			return
		}
		conn := p.conn
		p.connMutex.Unlock()

		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		_, response, err := conn.ReadMessage()
		if err != nil {
			p.setErrorAndStop(err)
			return
		}

		result, err := p.parseResponse(response)
		if err != nil {
			p.setErrorAndStop(fmt.Errorf("failed to parse response: %v", err))
			return
		}

		// Handle error response
		if resultType, ok := result["type"].(string); ok && resultType == "Error" {
			description := "unknown error"
			if desc, ok := result["description"].(string); ok {
				description = desc
			}
			p.setErrorAndStop(fmt.Errorf("Deepgram API error: %s", description))
			return
		}

		// Handle successful transcription
		if resultType, ok := result["type"].(string); ok && resultType == "Results" {
			// Check if this is a final result
			isFinal, _ := result["is_final"].(bool)

			if channel, ok := result["channel"].(map[string]interface{}); ok {
				if alternatives, ok := channel["alternatives"].([]interface{}); ok && len(alternatives) > 0 {
					if firstAlt, ok := alternatives[0].(map[string]interface{}); ok {
						if transcript, ok := firstAlt["transcript"].(string); ok {
							transcript = strings.TrimSpace(transcript)

							// Only update result for final transcripts
							if isFinal {
								p.connMutex.Lock()
								p.result = transcript
								p.connMutex.Unlock()

								if listener := p.BaseProvider.GetListener(); listener != nil {
									if p.result == "" && p.SilenceTime() > 30*time.Second {
										p.BaseProvider.SilenceCount += 1
										p.result = "U r not listen to me!"
									} else if p.result != "" {
										p.BaseProvider.SilenceCount = 0
									}
									if finished := listener.OnAsrResult(p.result); finished {
										return
									}
								}
							// } else if p.interim {
							// 	// For interim results, notify listener but don't update final result
							// 	if listener := p.BaseProvider.GetListener(); listener != nil {
							// 		listener.OnAsrInterimResult(transcript)
							// 	}
							}
						}
					}
				}
			}
		}
	}
}

// parseResponse parses the Deepgram response
func (p *Provider) parseResponse(data []byte) (map[string]interface{}, error) {
	var response map[string]interface{}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	p.logger.Debug("[DEBUG] parseResponse: JSON parsed successfully, data=%v", response)

	// Log additional debug info for error responses
	if responseType, ok := response["type"].(string); ok && responseType == "Error" {
		p.logger.Debug("[DEBUG] Received error response: %v", response)
		if desc, ok := response["description"].(string); ok {
			p.logger.Debug("[DEBUG] Error description: %s", desc)
		}
		if msg, ok := response["message"].(string); ok {
			p.logger.Debug("[DEBUG] Error message: %s", msg)
		}
	}

	return response, nil
}

func (p *Provider) setErrorAndStop(err error) {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()
	p.err = err
	p.isStreaming = false
	errMsg := err.Error()
	if strings.Contains(errMsg, "use of closed network connection") {
		p.logger.Debug("setErrorAndStop: %v, sendDataCnt=%d", err, p.sendDataCnt)
	} else {
		p.logger.Error("setErrorAndStop: %v, sendDataCnt=%d", err, p.sendDataCnt)
	}

	if p.conn != nil {
		p.closeConnection()
	}
}

func (p *Provider) closeConnection() {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("Error closing connection: %v", r)
		}
	}()

	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

// sendAudioData sends audio data to Deepgram
func (p *Provider) sendAudioData(data []byte, isLast bool) error {
	p.logger.Debug("[DEBUG] sendAudioData: data length=%d, isLast=%t, sendDataCnt=%d", len(data), isLast, p.sendDataCnt)
	if len(data) == 0 && !isLast {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("Panic while sending audio data: %v", r)
		}
	}()

	if p.conn == nil {
		return fmt.Errorf("WebSocket connection not established")
	}

	if err := p.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("failed to send audio data: %v", err)
	}

	return nil
}

// Reset resets the ASR state
func (p *Provider) Reset() error {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	p.isStreaming = false
	p.closeConnection()

	p.reqID = ""
	p.result = ""
	p.err = nil

	p.InitAudioProcessing()

	p.logger.Info("ASR state reset")

	return nil
}

// Initialize implements Provider interface Initialize method
func (p *Provider) Initialize() error {
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to initialize output directory: %v", err)
	}
	return nil
}

// Cleanup implements Provider interface Cleanup method
func (p *Provider) Cleanup() error {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	p.closeConnection()

	p.logger.Info("ASR resources cleaned up")

	return nil
}

func init() {
	// Register Deepgram ASR provider
	asr.Register("deepgram", func(config *asr.Config, deleteFile bool, logger *utils.Logger) (asr.Provider, error) {
		return NewProvider(config, deleteFile, logger)
	})
}
