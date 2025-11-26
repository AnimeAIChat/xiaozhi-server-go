package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sashabaranov/go-openai"
	"xiaozhi-server-go/internal/utils"
)

// ToolInputSchema describes the JSON schema for tool arguments.
type ToolInputSchema struct {
	Type       string                   `json:"type"`
	Properties map[string]any           `json:"properties,omitempty"`
	Required   []string                 `json:"required,omitempty"`
	Examples   []map[string]interface{} `json:"examples,omitempty"`
}

// Tool holds the metadata necessary to expose a tool to clients.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema ToolInputSchema `json:"input_schema"`
}

// Client encapsulates the lifecycle of a MCP client implementation.
type Client interface {
	Start(ctx context.Context) error
	Stop()
	HasTool(name string) bool
	GetAvailableTools() []openai.Tool
	CallTool(ctx context.Context, name string, args map[string]any) (any, error)
	IsReady() bool
	ResetConnection() error
}

// ToolExecutor represents an execution primitive that can be plugged into the manager.
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args map[string]any) (any, error)
}

// Logger captures the logging interface consumed by the domain.
type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	DebugTag(tag string, format string, args ...any)
	InfoTag(tag string, format string, args ...any)
	WarnTag(tag string, format string, args ...any)
	ErrorTag(tag string, format string, args ...any)
}

// Config defines the MCP client configuration
type Config struct {
	Enabled       bool     `yaml:"enabled"`
	ServerAddress string   `yaml:"server_address"`
	ServerPort    int      `yaml:"server_port"`
	Namespace     string   `yaml:"namespace"`
	NodeID        string   `yaml:"node_id"`
	ResourceTypes []string `yaml:"resource_types"`
	Command       string   `yaml:"command,omitempty"` // Command line connection
	Args          []string `yaml:"args,omitempty"`    // Command line arguments
	Env           []string `yaml:"env,omitempty"`     // Environment variables
	URL           string   `yaml:"url,omitempty"`     // SSE connection URL
}

// ConfigLoader handles MCP server configuration loading
type ConfigLoader struct {
	logger Logger
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader(logger Logger) *ConfigLoader {
	return &ConfigLoader{
		logger: logger,
	}
}

// LoadConfig loads MCP server configuration from file
func (c *ConfigLoader) LoadConfig() (map[string]*Config, error) {
	projectDir := utils.GetProjectDir()
	configPath := filepath.Join(projectDir, "data", ".mcp_server_settings.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.logger.Debug("MCP config file not found: %s", configPath)
		return nil, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error loading MCP config from %s: %w", configPath, err)
	}

	var config struct {
		MCPServers map[string]interface{} `json:"mcpServers"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing MCP config: %w", err)
	}

	result := make(map[string]*Config)
	for name, srvConfig := range config.MCPServers {
		srvConfigMap, ok := srvConfig.(map[string]interface{})
		if !ok {
			c.logger.Warn("Invalid configuration format for server %s", name)
			continue
		}

		clientConfig, err := convertConfig(srvConfigMap)
		if err != nil {
			c.logger.Error("Failed to convert config for server %s: %v", name, err)
			continue
		}

		result[name] = clientConfig
	}

	return result, nil
}

// convertConfig converts map configuration to Config structure
func convertConfig(cfg map[string]interface{}) (*Config, error) {
	config := &Config{
		Enabled: true, // Default to enabled
	}

	// Server address
	if addr, ok := cfg["server_address"].(string); ok {
		config.ServerAddress = addr
	}

	// Server port
	if port, ok := cfg["server_port"].(float64); ok {
		config.ServerPort = int(port)
	}

	// Namespace
	if ns, ok := cfg["namespace"].(string); ok {
		config.Namespace = ns
	}

	// Node ID
	if nodeID, ok := cfg["node_id"].(string); ok {
		config.NodeID = nodeID
	}

	// Command line connection
	if cmd, ok := cfg["command"].(string); ok {
		config.Command = cmd
	}

	// Command line arguments
	if args, ok := cfg["args"].([]interface{}); ok {
		for _, arg := range args {
			if argStr, ok := arg.(string); ok {
				config.Args = append(config.Args, argStr)
			}
		}
	}

	// Enabled parameter
	if enabled, ok := cfg["enabled"].(bool); ok {
		config.Enabled = enabled
	}

	// Environment variables
	if env, ok := cfg["env"].(map[string]interface{}); ok {
		config.Env = make([]string, 0)
		for k, v := range env {
			if vStr, ok := v.(string); ok {
				config.Env = append(config.Env, fmt.Sprintf("%s=%s", k, vStr))
			}
		}
	}

	// SSE connection URL
	if url, ok := cfg["url"].(string); ok {
		config.URL = url
	}

	return config, nil
}

func (t Tool) toOpenAITool() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema.toParameters(),
		},
	}
}

func (s ToolInputSchema) toParameters() map[string]any {
	params := map[string]any{
		"type": s.Type,
	}
	if len(s.Properties) > 0 {
		params["properties"] = s.Properties
	}
	if len(s.Required) > 0 {
		params["required"] = s.Required
	}
	if len(s.Examples) > 0 {
		params["examples"] = s.Examples
	}
	return params
}
