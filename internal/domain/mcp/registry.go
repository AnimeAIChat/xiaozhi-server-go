package mcp

import (
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/sashabaranov/go-openai"
)

type toolRegistry struct {
	mu    sync.RWMutex
	tools map[string]openai.Tool
}

func newToolRegistry() *toolRegistry {
	return &toolRegistry{
		tools: make(map[string]openai.Tool),
	}
}

func (r *toolRegistry) register(tools []openai.Tool) error {
	if len(tools) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.tools == nil {
		r.tools = make(map[string]openai.Tool, len(tools))
	}

	for _, tool := range tools {
		name := tool.Function.Name
		if name == "" {
			return fmt.Errorf("tool name cannot be empty")
		}
		r.tools[name] = tool
	}
	return nil
}

func (r *toolRegistry) unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

func (r *toolRegistry) clone() map[string]openai.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return maps.Clone(r.tools)
}

func (r *toolRegistry) list() []string {
	tools := r.clone()
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (r *toolRegistry) get(name string) (openai.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}
