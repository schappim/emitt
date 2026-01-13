package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
	"github.com/sashabaranov/go-openai"
)

// Tool represents a callable tool/function
type Tool interface {
	// Name returns the tool's unique identifier
	Name() string

	// Description returns a description of what the tool does
	Description() string

	// Parameters returns the JSON schema for the tool's parameters
	Parameters() map[string]interface{}

	// Execute runs the tool with the given arguments
	Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
}

// Registry manages available tools
type Registry struct {
	tools  map[string]Tool
	logger zerolog.Logger
	mu     sync.RWMutex
}

// NewRegistry creates a new tool registry
func NewRegistry(logger zerolog.Logger) *Registry {
	return &Registry{
		tools:  make(map[string]Tool),
		logger: logger.With().Str("component", "tools").Logger(),
	}
}

// Register adds a tool to the registry
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[tool.Name()] = tool
	r.logger.Debug().Str("tool", tool.Name()).Msg("Registered tool")
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

// GetAll returns all registered tools
func (r *Registry) GetAll() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// GetByNames returns tools matching the given names
func (r *Registry) GetByNames(names []string) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tools []Tool
	for _, name := range names {
		if tool, ok := r.tools[name]; ok {
			tools = append(tools, tool)
		}
	}
	return tools
}

// Execute runs a tool by name with the given arguments
func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	tool, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}

	r.logger.Debug().
		Str("tool", name).
		RawJSON("args", args).
		Msg("Executing tool")

	result, err := tool.Execute(ctx, args)
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("tool", name).
			Msg("Tool execution failed")
		return nil, err
	}

	r.logger.Debug().
		Str("tool", name).
		Msg("Tool execution completed")

	return result, nil
}

// ToOpenAITools converts registry tools to OpenAI function definitions
func (r *Registry) ToOpenAITools(names []string) []openai.Tool {
	var tools []Tool
	if len(names) == 0 {
		tools = r.GetAll()
	} else {
		tools = r.GetByNames(names)
	}

	result := make([]openai.Tool, len(tools))
	for i, t := range tools {
		result[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		}
	}
	return result
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// NewSuccessResult creates a successful tool result
func NewSuccessResult(data interface{}) (json.RawMessage, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	result := ToolResult{
		Success: true,
		Data:    dataJSON,
	}
	return json.Marshal(result)
}

// NewErrorResult creates an error tool result
func NewErrorResult(err error) (json.RawMessage, error) {
	result := ToolResult{
		Success: false,
		Error:   err.Error(),
	}
	return json.Marshal(result)
}
