package mcp

import (
	"context"
	"sync"
	"testing"

	"github.com/sashabaranov/go-openai"

	"xiaozhi-server-go/internal/platform/config"
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
func (l *testLogger) DebugTag(tag string, format string, args ...any) { l.logf("["+tag+"] "+format, args...) }
func (l *testLogger) InfoTag(tag string, format string, args ...any)  { l.logf("["+tag+"] "+format, args...) }
func (l *testLogger) WarnTag(tag string, format string, args ...any)  { l.logf("["+tag+"] "+format, args...) }
func (l *testLogger) ErrorTag(tag string, format string, args ...any) { l.logf("["+tag+"] "+format, args...) }

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
	cfg := config.DefaultConfig() // Use default config for testing
	manager, err := NewManager(Options{
		Logger:    logger,
		Config:    cfg,
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
