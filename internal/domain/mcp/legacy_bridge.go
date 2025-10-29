package mcp

import (
	"context"
	"errors"

	"xiaozhi-server-go/src/configs"
	coremcp "xiaozhi-server-go/src/core/mcp"
	"xiaozhi-server-go/src/core/types"
	"xiaozhi-server-go/src/core/utils"
)

type coreLegacyAdapter struct {
	manager *coremcp.Manager
}

func newCoreLegacyAdapter(manager *coremcp.Manager) legacyBridge {
	if manager == nil {
		return nil
	}
	return &coreLegacyAdapter{manager: manager}
}

func (a *coreLegacyAdapter) ExecuteTool(
	ctx context.Context,
	name string,
	args map[string]any,
) (any, error) {
	if a.manager == nil {
		return nil, errors.New("legacy MCP manager not configured")
	}
	return a.manager.ExecuteTool(ctx, name, args)
}

func (a *coreLegacyAdapter) ToolNames() []string {
	if a.manager == nil {
		return nil
	}
	return a.manager.GetAllToolsNames()
}

func (a *coreLegacyAdapter) BindConnection(
	conn Conn,
	fh types.FunctionRegistryInterface,
	params any,
) error {
	if a.manager == nil {
		return errors.New("legacy MCP manager not configured")
	}
	if conn == nil {
		return errors.New("connection is nil")
	}
	return a.manager.BindConnection(wrapLegacyConn(conn), fh, params)
}

func (a *coreLegacyAdapter) Cleanup() error {
	if a.manager == nil {
		return nil
	}
	return a.manager.Cleanup()
}

func (a *coreLegacyAdapter) CleanupAll(ctx context.Context) {
	if a.manager == nil {
		return
	}
	a.manager.CleanupAll(ctx)
}

func (a *coreLegacyAdapter) Reset() error {
	if a.manager == nil {
		return nil
	}
	return a.manager.Reset()
}

func (a *coreLegacyAdapter) AutoReturn() bool {
	if a.manager == nil {
		return false
	}
	return a.manager.AutoReturnToPool
}

func (a *coreLegacyAdapter) IsMCPTool(name string) bool {
	if a.manager == nil {
		return false
	}
	return a.manager.IsMCPTool(name)
}

func (a *coreLegacyAdapter) HandleXiaoZhiMCPMessage(msg map[string]any) error {
	if a.manager == nil {
		return errors.New("legacy MCP manager not configured")
	}
	return a.manager.HandleXiaoZhiMCPMessage(msg)
}

type legacyConnAdapter struct {
	Conn
}

func (a legacyConnAdapter) WriteMessage(messageType int, data []byte) error {
	return a.Conn.WriteMessage(messageType, data)
}

func wrapLegacyConn(conn Conn) coremcp.Conn {
	if c, ok := any(conn).(coremcp.Conn); ok {
		return c
	}
	return legacyConnAdapter{Conn: conn}
}

// NewLegacyFromConfig constructs a bridge over the legacy MCP manager using the supplied config.
func NewLegacyFromConfig(cfg *configs.Config, logger *utils.Logger) legacyBridge {
	if cfg == nil || logger == nil {
		return nil
	}
	return newCoreLegacyAdapter(coremcp.NewManagerForPool(logger, cfg))
}

// NewLegacyFromManager constructs a bridge over an existing legacy MCP manager.
func NewLegacyFromManager(manager *coremcp.Manager) legacyBridge {
	if manager == nil {
		return nil
	}
	return newCoreLegacyAdapter(manager)
}

// NewFromConfig creates a manager instance pre-wired with the legacy bridge.
func NewFromConfig(cfg *configs.Config, logger *utils.Logger) (*Manager, error) {
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	legacy := NewLegacyFromConfig(cfg, logger)

	return NewManager(Options{
		Logger:     logger,
		Legacy:     legacy,
		AutoReturn: legacy != nil && legacy.AutoReturn(),
	})
}
