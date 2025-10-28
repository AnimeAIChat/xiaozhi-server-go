package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/sashabaranov/go-openai"

	"xiaozhi-server-go/src/core/types"
)

type testLogger struct {
	mu sync.Mutex
}

func (l *testLogger) logf(_ string, _ ...any) {
	l.mu.Lock()
	l.mu.Unlock()
}

func (l *testLogger) Debug(format string, args ...any) { l.logf(format, args...) }
func (l *testLogger) Info(format string, args ...any)  { l.logf(format, args...) }
func (l *testLogger) Warn(format string, args ...any)  { l.logf(format, args...) }
func (l *testLogger) Error(format string, args ...any) { l.logf(format, args...) }

type fakeClient struct {
	tools       []openai.Tool
	results     map[string]any
	startCalled bool
	stopCalled  bool
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		tools: []openai.Tool{{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name: "ping",
			},
		}},
		results: map[string]any{"ping": "pong"},
	}
}

func (c *fakeClient) Start(context.Context) error {
	c.startCalled = true
	return nil
}

func (c *fakeClient) Stop() {
	c.stopCalled = true
}

func (c *fakeClient) HasTool(name string) bool {
	for _, tool := range c.tools {
		if tool.Function != nil && tool.Function.Name == name {
			return true
		}
	}
	return false
}

func (c *fakeClient) GetAvailableTools() []openai.Tool {
	return c.tools
}

func (c *fakeClient) CallTool(context.Context, string, map[string]any) (any, error) {
	return c.results["ping"], nil
}

func (c *fakeClient) IsReady() bool { return true }

func (c *fakeClient) ResetConnection() error { return nil }

func TestManagerRegisterAndExecute(t *testing.T) {
	logger := &testLogger{}
	manager, err := NewManager(Options{
		Logger:    logger,
		AutoStart: true,
	})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	client := newFakeClient()
	if err := manager.RegisterClient("fake", client, true); err != nil {
		t.Fatalf("RegisterClient error: %v", err)
	}
	if !client.startCalled {
		t.Fatalf("expected client to be started")
	}

	if !manager.IsMCPTool("ping") {
		t.Fatalf("expected tool to be registered")
	}

	res, err := manager.ExecuteTool(context.Background(), "ping", map[string]any{})
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if res.(string) != "pong" {
		t.Fatalf("unexpected tool result: %v", res)
	}

	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if !client.stopCalled {
		t.Fatalf("expected client stop to be invoked")
	}
}

type mockLegacy struct {
	bindCalls    int
	cleanupCalls int
	autoReturn   bool
	lastMsg      map[string]any
}

func (m *mockLegacy) ExecuteTool(context.Context, string, map[string]any) (any, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLegacy) ToolNames() []string { return nil }

func (m *mockLegacy) BindConnection(conn Conn, _ types.FunctionRegistryInterface, _ any) error {
	if conn == nil {
		return fmt.Errorf("connection nil")
	}
	m.bindCalls++
	return nil
}

func (m *mockLegacy) Cleanup() error {
	m.cleanupCalls++
	return nil
}

func (m *mockLegacy) CleanupAll(context.Context) {
	m.cleanupCalls++
}

func (m *mockLegacy) Reset() error {
	return nil
}

func (m *mockLegacy) AutoReturn() bool {
	return m.autoReturn
}

func (m *mockLegacy) IsMCPTool(string) bool { return false }

func (m *mockLegacy) HandleXiaoZhiMCPMessage(msg map[string]any) error {
	m.lastMsg = msg
	return nil
}

type dummyConn struct{ writes int }

func (d *dummyConn) WriteMessage(int, []byte) error {
	d.writes++
	return nil
}

func TestManagerWithLegacyBridge(t *testing.T) {
	logger := &testLogger{}
	legacy := &mockLegacy{autoReturn: true}
	manager, err := NewManager(Options{
		Logger: logger,
		Legacy: legacy,
	})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	conn := &dummyConn{}
	if err := manager.BindConnection(conn, nil, nil); err != nil {
		t.Fatalf("BindConnection error: %v", err)
	}
	if legacy.bindCalls != 1 {
		t.Fatalf("expected bind to propagate to legacy")
	}

	if !manager.AutoReturn() {
		t.Fatalf("expected auto return to reflect legacy flag")
	}

	if err := manager.Cleanup(); err != nil {
		t.Fatalf("Cleanup error: %v", err)
	}
	manager.CleanupAll(context.Background())

	if legacy.cleanupCalls == 0 {
		t.Fatalf("expected cleanup to trigger legacy")
	}

	if err := manager.HandleXiaoZhiMCPMessage(map[string]any{"ok": true}); err != nil {
		t.Fatalf("HandleXiaoZhiMCPMessage error: %v", err)
	}
	if legacy.lastMsg == nil {
		t.Fatalf("expected legacy handler to capture message")
	}
}
