package dsh

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/types"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestProviderStreamsBridgeResponse(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer bridge-token", request.Header.Get("Authorization"))
		connection, err := upgrader.Upgrade(writer, request, nil)
		require.NoError(t, err)
		defer connection.Close()

		require.NoError(t, connection.WriteJSON(bridgeMessage{Type: "ready"}))
		var turn bridgeMessage
		require.NoError(t, connection.ReadJSON(&turn))
		require.Equal(t, "turn", turn.Type)
		require.Equal(t, "device-01", turn.DeviceID)
		require.Equal(t, "最新问题", turn.Text)
		require.NotEmpty(t, turn.RequestID)

		require.NoError(t, connection.WriteJSON(bridgeMessage{Type: "accepted", RequestID: turn.RequestID}))
		require.NoError(t, connection.WriteJSON(bridgeMessage{Type: "assistant_delta", RequestID: turn.RequestID, Text: "第一段。"}))
		require.NoError(t, connection.WriteJSON(bridgeMessage{Type: "assistant_fallback", RequestID: turn.RequestID, Text: "我在继续处理。"}))
		require.NoError(t, connection.WriteJSON(bridgeMessage{Type: "assistant_end", RequestID: turn.RequestID}))
	}))
	defer server.Close()

	provider := newTestProvider(t, websocketURL(server.URL))
	responses, err := provider.ResponseWithFunctions(context.Background(), "device-01", []types.Message{
		{Role: "user", Content: "旧问题"},
		{Role: "assistant", Content: "旧回答"},
		{Role: "user", Content: "最新问题"},
	}, nil)
	require.NoError(t, err)

	var got []types.Response
	for response := range responses {
		got = append(got, response)
	}
	require.Equal(t, []types.Response{{Content: "第一段。"}, {Content: "我在继续处理。"}}, got)
}

func TestProviderReturnsBridgeError(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		require.NoError(t, err)
		defer connection.Close()
		require.NoError(t, connection.WriteJSON(bridgeMessage{Type: "ready"}))
		var turn bridgeMessage
		require.NoError(t, connection.ReadJSON(&turn))
		require.NoError(t, connection.WriteJSON(bridgeMessage{Type: "error", RequestID: turn.RequestID, Code: "busy", Message: "agent busy"}))
	}))
	defer server.Close()

	provider := newTestProvider(t, websocketURL(server.URL))
	responses, err := provider.ResponseWithFunctions(context.Background(), "device-01", []types.Message{{Role: "user", Content: "测试"}}, nil)
	require.NoError(t, err)

	response, ok := <-responses
	require.True(t, ok)
	require.Equal(t, "DSH bridge error: agent busy (busy)", response.Error)
	_, ok = <-responses
	require.False(t, ok)
}

func TestInitializeRejectsInvalidBridgeConfiguration(t *testing.T) {
	provider, err := NewProvider(&llm.Config{BaseURL: "http://127.0.0.1:17980/xiaozhi", APIKey: "bridge-token"})
	require.NoError(t, err)
	require.ErrorContains(t, provider.Initialize(), "must use ws:// or wss://")

	provider, err = NewProvider(&llm.Config{BaseURL: "ws://127.0.0.1:17980/xiaozhi"})
	require.NoError(t, err)
	require.ErrorContains(t, provider.Initialize(), "missing DSH bridge token")
}

func TestBridgeDeviceIDHashesUnsupportedSessionID(t *testing.T) {
	deviceID, err := bridgeDeviceID("client session with spaces")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(deviceID, "xiaozhi-"))
	require.True(t, isBridgeDeviceID(deviceID))
}

func newTestProvider(t *testing.T, endpoint string) *Provider {
	t.Helper()
	provider, err := NewProvider(&llm.Config{BaseURL: endpoint, APIKey: "bridge-token"})
	require.NoError(t, err)
	require.NoError(t, provider.Initialize())
	return provider.(*Provider)
}

func websocketURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}
