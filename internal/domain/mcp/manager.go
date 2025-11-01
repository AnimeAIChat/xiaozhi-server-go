package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/sashabaranov/go-openai"

	"xiaozhi-server-go/internal/domain/llm"
	"xiaozhi-server-go/src/core/mcp"
	"xiaozhi-server-go/src/core/utils"
)

// Options configures the manager instance.
type Options struct {
	Logger     Logger
	Clients    map[string]Client
	AutoStart  bool
	AutoReturn bool
	Legacy     legacyBridge
}

// Conn captures the minimal connection behaviour required by MCP clients.
type Conn interface {
	WriteMessage(messageType int, data []byte) error
}

type legacyBridge interface {
	ExecuteTool(ctx context.Context, name string, args map[string]any) (any, error)
	ToolNames() []string
	BindConnection(conn Conn, fh llm.FunctionRegistryInterface, params any) error
	Cleanup() error
	CleanupAll(ctx context.Context)
	Reset() error
	AutoReturn() bool
	IsMCPTool(name string) bool
	HandleXiaoZhiMCPMessage(msg map[string]any) error
}

// Manager coordinates MCP clients and tool execution.
type Manager struct {
	logger Logger

	registry *toolRegistry

	clientsMu sync.RWMutex
	clients   map[string]Client

	autoReturn bool
	legacy     legacyBridge
}

// NewManager constructs a new manager instance.
func NewManager(opts Options) (*Manager, error) {
	if opts.Logger == nil {
		return nil, errors.New("mcp manager requires logger")
	}
	manager := &Manager{
		logger:     opts.Logger,
		registry:   newToolRegistry(),
		clients:    make(map[string]Client),
		autoReturn: opts.AutoReturn,
		legacy:     opts.Legacy,
	}

	if !manager.autoReturn && manager.legacy != nil {
		manager.autoReturn = manager.legacy.AutoReturn()
	}

	if len(opts.Clients) > 0 {
		if err := manager.addClients(opts.Clients, opts.AutoStart); err != nil {
			return nil, err
		}
	}

	manager.refreshToolRegistry()

	return manager, nil
}

func (m *Manager) addClients(clients map[string]Client, autoStart bool) error {
	if len(clients) == 0 {
		return nil
	}

	for name, client := range maps.Clone(clients) {
		if err := m.registerClient(name, client, autoStart); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) registerClient(name string, client Client, autoStart bool) error {
	if name == "" {
		return fmt.Errorf("client name cannot be empty")
	}
	if client == nil {
		return fmt.Errorf("client %s is nil", name)
	}

	if autoStart {
		if err := client.Start(context.Background()); err != nil {
			return fmt.Errorf("start client %s: %w", name, err)
		}
	}

	if err := m.registry.register(client.GetAvailableTools()); err != nil {
		return err
	}

	m.clientsMu.Lock()
	m.clients[name] = client
	m.clientsMu.Unlock()

	m.logger.InfoTag("MCP", "注册客户端 %s（工具数量=%d）", name, len(client.GetAvailableTools()))
	m.refreshToolRegistry()
	return nil
}

// RegisterClient attaches a new client to the manager.
func (m *Manager) RegisterClient(name string, client Client, autoStart bool) error {
	return m.registerClient(name, client, autoStart)
}

// RemoveClient detaches a client and stops it.
func (m *Manager) RemoveClient(name string) {
	m.clientsMu.Lock()
	client, ok := m.clients[name]
	if ok {
		delete(m.clients, name)
	}
	m.clientsMu.Unlock()

	if ok && client != nil {
		client.Stop()
	}
}

// ListClients returns the registered client names.
func (m *Manager) ListClients() []string {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()
	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// RegisterTools merges the supplied tools into the registry.
func (m *Manager) RegisterTools(tools []Tool) error {
	if len(tools) == 0 {
		return nil
	}
	openAITools := make([]openai.Tool, 0, len(tools))
	for _, tool := range tools {
		openAITools = append(openAITools, tool.toOpenAITool())
	}
	return m.registry.register(openAITools)
}

// ExecuteTool executes a tool by name across known clients.
func (m *Manager) ExecuteTool(ctx context.Context, name string, args map[string]any) (any, error) {
	if len(m.clients) == 0 && m.legacy != nil {
		return m.legacy.ExecuteTool(ctx, name, args)
	}
	if name == "" {
		return nil, errors.New("tool name cannot be empty")
	}

	m.clientsMu.RLock()
	clients := maps.Clone(m.clients)
	m.clientsMu.RUnlock()

	if len(clients) == 0 {
		return nil, errors.New("no MCP clients registered")
	}

	for clientName, client := range clients {
		if client == nil || !client.HasTool(name) {
			continue
		}
		m.logger.DebugTag("MCP", "执行工具 %s，来自客户端 %s", name, clientName)
		result, err := client.CallTool(ctx, name, args)
		if err != nil {
			return nil, fmt.Errorf("client %s failed: %w", clientName, err)
		}
		return result, nil
	}

	return nil, fmt.Errorf("tool %s not found in clients %v", name, maps.Keys(clients))
}

// ToolNames returns the registered tool names sorted alphabetically.
func (m *Manager) ToolNames() []string {
	if m.registry != nil {
		return m.registry.list()
	}
	if m.legacy != nil {
		return m.legacy.ToolNames()
	}
	return nil
}

// Close stops every registered client.
func (m *Manager) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	var errs []error
	m.clientsMu.Lock()
	for name, client := range m.clients {
		if client == nil {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					errs = append(errs, fmt.Errorf("panic closing client %s: %v", name, r))
				}
			}()

			client.Stop()
		}()
		delete(m.clients, name)
	}
	m.clientsMu.Unlock()

	if m.legacy != nil {
		m.legacy.CleanupAll(ctx)
	}

	return errors.Join(errs...)
}

// AutoReturn reports whether the manager should be returned to its pool automatically.
func (m *Manager) AutoReturn() bool {
	if m.autoReturn {
		return true
	}
	if m.legacy != nil {
		return m.legacy.AutoReturn()
	}
	return false
}

// BindConnection attaches the websocket connection to the MCP clients.
func (m *Manager) BindConnection(
	conn Conn,
	fh llm.FunctionRegistryInterface,
	params any,
) error {
	if m.legacy == nil {
		return errors.New("legacy MCP manager not configured")
	}
	if err := m.legacy.BindConnection(conn, fh, params); err != nil {
		return err
	}
	m.refreshToolRegistry()
	return nil
}

// Cleanup calls the underlying cleanup routine.
func (m *Manager) Cleanup() error {
	if m.legacy == nil {
		return nil
	}
	return m.legacy.Cleanup()
}

// CleanupAll closes all MCP clients.
func (m *Manager) CleanupAll(ctx context.Context) {
	if m.legacy == nil {
		return
	}
	m.legacy.CleanupAll(ctx)
}

// Reset clears internal state for reuse.
func (m *Manager) Reset() error {
	m.clientsMu.Lock()
	m.clients = make(map[string]Client)
	m.clientsMu.Unlock()

	if m.registry != nil {
		m.registry = newToolRegistry()
	}

	if m.legacy == nil {
		return nil
	}
	if err := m.legacy.Reset(); err != nil {
		return err
	}
	m.refreshToolRegistry()
	return nil
}

// IsMCPTool reports whether the tool comes from any MCP client.
func (m *Manager) IsMCPTool(name string) bool {
	if name == "" {
		return false
	}
	if m.registry != nil {
		if _, ok := m.registry.get(name); ok {
			return true
		}
	}
	if m.legacy != nil {
		return m.legacy.IsMCPTool(name)
	}
	return false
}

func (m *Manager) refreshToolRegistry() {
	if m.registry == nil {
		return
	}
	if m.legacy == nil {
		return
	}
	names := m.legacy.ToolNames()
	if len(names) == 0 {
		return
	}
	tools := make([]openai.Tool, 0, len(names))
	for _, name := range names {
		tools = append(tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name: name,
			},
		})
	}
	_ = m.registry.register(tools)
}

// HandleXiaoZhiMCPMessage delegates message handling to the legacy manager.
func (m *Manager) HandleXiaoZhiMCPMessage(msg map[string]interface{}) error {
	if m.legacy == nil {
		return errors.New("legacy MCP manager not configured")
	}
	return m.legacy.HandleXiaoZhiMCPMessage(msg)
}

// NewFromManager creates a manager instance using an existing legacy MCP manager.
func NewFromManager(manager *mcp.Manager, logger *utils.Logger) (*Manager, error) {
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	if manager == nil {
		return nil, errors.New("mcp manager is required")
	}
	legacy := NewLegacyFromManager(manager)

	return NewManager(Options{
		Logger:     logger,
		Legacy:     legacy,
		AutoReturn: legacy != nil && legacy.AutoReturn(),
	})
}
