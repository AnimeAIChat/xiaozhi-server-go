// Package dsh implements the LLM provider for the DSH Xiaozhi bridge.
package dsh

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/types"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
)

const providerName = "dsh"

// Provider forwards Xiaozhi turns to the DSH bridge WebSocket service.
// DSH owns the agent session, tools, and persistent conversation history.
type Provider struct {
	*llm.BaseProvider
	gatewayURL string
	dialer     *websocket.Dialer
}

type bridgeMessage struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id,omitempty"`
	DeviceID  string `json:"device_id,omitempty"`
	Text      string `json:"text,omitempty"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
}

func init() {
	llm.Register(providerName, NewProvider)
}

// NewProvider creates a DSH bridge provider. Initialization validates the
// endpoint and shared bridge token before the provider can serve requests.
func NewProvider(config *llm.Config) (llm.Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("DSH bridge configuration is required")
	}

	return &Provider{
		BaseProvider: llm.NewBaseProvider(config),
		dialer:       websocket.DefaultDialer,
	}, nil
}

// Initialize validates the WebSocket bridge endpoint and authentication token.
func (p *Provider) Initialize() error {
	config := p.Config()
	if config == nil {
		return fmt.Errorf("DSH bridge configuration is required")
	}

	endpoint := strings.TrimSpace(config.BaseURL)
	if endpoint == "" {
		return fmt.Errorf("missing DSH bridge WebSocket URL")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return fmt.Errorf("invalid DSH bridge WebSocket URL: it must use ws:// or wss:// and include a host")
	}
	if parsed.User != nil {
		return fmt.Errorf("DSH bridge WebSocket URL must not contain credentials")
	}

	if strings.TrimSpace(config.APIKey) == "" {
		return fmt.Errorf("missing DSH bridge token")
	}

	p.gatewayURL = parsed.String()
	return nil
}

func (p *Provider) Cleanup() error {
	return nil
}

// Response implements the legacy text-only interface.
func (p *Provider) Response(ctx context.Context, sessionID string, messages []types.Message) (<-chan string, error) {
	responses, err := p.ResponseWithFunctions(ctx, sessionID, messages, nil)
	if err != nil {
		return nil, err
	}

	output := make(chan string, 10)
	go func() {
		defer close(output)
		for response := range responses {
			if response.Error != "" {
				output <- response.Error
				return
			}
			if response.Content != "" {
				output <- response.Content
			}
		}
	}()
	return output, nil
}

// ResponseWithFunctions forwards the newest user message to DSH and maps its
// streaming frames into Xiaozhi responses. The tools argument is deliberately
// not forwarded: tool execution is performed by the DSH agent.
func (p *Provider) ResponseWithFunctions(ctx context.Context, sessionID string, messages []types.Message, _ []openai.Tool) (<-chan types.Response, error) {
	if p.gatewayURL == "" {
		return nil, fmt.Errorf("DSH bridge provider is not initialized")
	}

	text, err := latestUserMessage(messages)
	if err != nil {
		return nil, err
	}
	deviceID, err := bridgeDeviceID(sessionID)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	output := make(chan types.Response, 10)
	go p.stream(ctx, output, deviceID, text)
	return output, nil
}

func (p *Provider) stream(ctx context.Context, output chan<- types.Response, deviceID, text string) {
	defer close(output)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+strings.TrimSpace(p.Config().APIKey))
	conn, _, err := p.dialer.DialContext(ctx, p.gatewayURL, headers)
	if err != nil {
		p.sendError(output, "connect DSH bridge", err)
		return
	}
	defer conn.Close()

	stopCancellation := make(chan struct{})
	defer close(stopCancellation)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stopCancellation:
		}
	}()

	var ready bridgeMessage
	if err := conn.ReadJSON(&ready); err != nil {
		p.sendError(output, "read DSH bridge readiness", err)
		return
	}
	if ready.Type == "error" {
		p.sendBridgeError(output, ready)
		return
	}
	if ready.Type != "ready" {
		p.sendError(output, "read DSH bridge readiness", fmt.Errorf("expected ready frame, got %q", ready.Type))
		return
	}

	requestID := uuid.NewString()
	if err := conn.WriteJSON(bridgeMessage{
		Type:      "turn",
		RequestID: requestID,
		DeviceID:  deviceID,
		Text:      text,
	}); err != nil {
		p.sendError(output, "send DSH bridge turn", err)
		return
	}

	for {
		var frame bridgeMessage
		if err := conn.ReadJSON(&frame); err != nil {
			if ctx.Err() != nil {
				return
			}
			p.sendError(output, "read DSH bridge response", err)
			return
		}

		if frame.RequestID != "" && frame.RequestID != requestID {
			// A connection is private to this request. Ignore an unsolicited frame
			// instead of speaking a background notification in the wrong turn.
			continue
		}

		switch frame.Type {
		case "accepted", "turn_started":
			continue
		case "assistant_delta", "assistant_fallback":
			if frame.Text != "" {
				output <- types.Response{Content: frame.Text}
			}
		case "assistant_end":
			return
		case "error":
			p.sendBridgeError(output, frame)
			return
		}
	}
}

func (p *Provider) sendBridgeError(output chan<- types.Response, frame bridgeMessage) {
	message := strings.TrimSpace(frame.Message)
	if message == "" {
		message = "DSH bridge returned an unspecified error"
	}
	if frame.Code != "" {
		message = fmt.Sprintf("%s (%s)", message, frame.Code)
	}
	output <- types.Response{Error: "DSH bridge error: " + message}
}

func (p *Provider) sendError(output chan<- types.Response, stage string, err error) {
	output <- types.Response{Error: fmt.Sprintf("DSH bridge %s: %v", stage, err)}
}

func latestUserMessage(messages []types.Message) (string, error) {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role == "user" && strings.TrimSpace(message.Content) != "" {
			return message.Content, nil
		}
	}
	return "", fmt.Errorf("DSH bridge request has no user message")
}

func bridgeDeviceID(sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", fmt.Errorf("DSH bridge request has no session ID")
	}
	if len(sessionID) <= 128 && isBridgeDeviceID(sessionID) {
		return sessionID, nil
	}

	// The bridge only accepts a compact identifier. Hash unusual client-provided
	// session IDs deterministically so reconnections still resume the same DSH
	// conversation without leaking the original value to the bridge.
	digest := sha256.Sum256([]byte(sessionID))
	return "xiaozhi-" + base64.RawURLEncoding.EncodeToString(digest[:16]), nil
}

func isBridgeDeviceID(value string) bool {
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == ':' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}
