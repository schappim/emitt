package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/emitt/emitt/internal/config"
	"github.com/emitt/emitt/internal/tools"
)

// LLMClient wraps the OpenAI Responses API
type LLMClient struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
	logger  zerolog.Logger
}

// NewLLMClient creates a new LLM client
func NewLLMClient(cfg *config.LLMConfig, logger zerolog.Logger) *LLMClient {
	return &LLMClient{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: "https://api.openai.com/v1",
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
		logger: logger.With().Str("component", "llm").Logger(),
	}
}

// ResponseRequest represents a request to the Responses API
type ResponseRequest struct {
	Model           string                   `json:"model"`
	Input           interface{}              `json:"input"`
	Instructions    string                   `json:"instructions,omitempty"`
	Tools           []Tool                   `json:"tools,omitempty"`
	ToolChoice      string                   `json:"tool_choice,omitempty"`
	MaxOutputTokens int                      `json:"max_output_tokens,omitempty"`
	Temperature     float32                  `json:"temperature,omitempty"`
	Store           bool                     `json:"store"`
}

// Tool represents a tool definition for the Responses API
type Tool struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ResponseObject represents the response from the Responses API
type ResponseObject struct {
	ID          string       `json:"id"`
	Object      string       `json:"object"`
	Status      string       `json:"status"`
	Output      []OutputItem `json:"output"`
	Error       *ErrorObject `json:"error"`
	Usage       *Usage       `json:"usage"`
}

// OutputItem represents an item in the response output
type OutputItem struct {
	Type      string        `json:"type"`
	ID        string        `json:"id"`
	Status    string        `json:"status"`
	Role      string        `json:"role"`
	Content   []ContentItem `json:"content,omitempty"`
	Name      string        `json:"name,omitempty"`
	Arguments string        `json:"arguments,omitempty"`
	CallID    string        `json:"call_id,omitempty"`
}

// ContentItem represents content within an output item
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ErrorObject represents an error from the API
type ErrorObject struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Usage represents token usage
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// InputMessage represents an input message
type InputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// FunctionCallInput represents a function call result input
type FunctionCallInput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// Chat sends a request to the Responses API
func (c *LLMClient) Chat(ctx context.Context, systemPrompt string, input interface{}, apiTools []Tool) (*ResponseObject, error) {
	req := ResponseRequest{
		Model:        c.model,
		Input:        input,
		Instructions: systemPrompt,
		Tools:        apiTools,
		ToolChoice:   "auto",
		Temperature:  0.7,
		Store:        false,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	c.logger.Debug().
		Str("model", c.model).
		RawJSON("request", reqBody).
		Msg("Sending request to Responses API")

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/responses", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("API error")
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result ResponseObject
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}

	c.logger.Debug().
		Str("status", result.Status).
		Int("output_items", len(result.Output)).
		Msg("Received response")

	return &result, nil
}

// ProcessWithTools runs a conversation loop with tool calling
func (c *LLMClient) ProcessWithTools(
	ctx context.Context,
	systemPrompt string,
	userMessage string,
	registry *tools.Registry,
	toolNames []string,
	maxIterations int,
) (string, error) {
	if maxIterations <= 0 {
		maxIterations = 10
	}

	// Convert registry tools to API tools
	apiTools := c.convertTools(registry, toolNames)

	// Start with user message
	input := []interface{}{
		InputMessage{Role: "user", Content: userMessage},
	}

	for i := 0; i < maxIterations; i++ {
		resp, err := c.Chat(ctx, systemPrompt, input, apiTools)
		if err != nil {
			return "", err
		}

		// Check for completion
		if resp.Status == "completed" {
			// Look for text output
			for _, item := range resp.Output {
				if item.Type == "message" && item.Role == "assistant" {
					for _, content := range item.Content {
						if content.Type == "output_text" {
							return content.Text, nil
						}
					}
				}
			}
		}

		// Check for function calls
		var functionCalls []OutputItem
		for _, item := range resp.Output {
			if item.Type == "function_call" {
				functionCalls = append(functionCalls, item)
			}
		}

		if len(functionCalls) == 0 {
			// No function calls and completed - extract text
			for _, item := range resp.Output {
				if item.Type == "message" {
					for _, content := range item.Content {
						if content.Type == "output_text" {
							return content.Text, nil
						}
					}
				}
			}
			return "", nil
		}

		// Execute function calls and add results to input
		for _, fc := range functionCalls {
			c.logger.Info().
				Str("tool", fc.Name).
				Str("call_id", fc.CallID).
				Msg("Executing tool call")

			result, err := registry.Execute(ctx, fc.Name, json.RawMessage(fc.Arguments))
			if err != nil {
				result, _ = tools.NewErrorResult(err)
			}

			// Add function call output to input for next iteration
			input = append(input, map[string]interface{}{
				"type":    "function_call_output",
				"call_id": fc.CallID,
				"output":  string(result),
			})
		}
	}

	return "", fmt.Errorf("max iterations reached without completion")
}

// convertTools converts registry tools to API tool format
func (c *LLMClient) convertTools(registry *tools.Registry, names []string) []Tool {
	var regTools []tools.Tool
	if len(names) == 0 {
		regTools = registry.GetAll()
	} else {
		regTools = registry.GetByNames(names)
	}

	apiTools := make([]Tool, len(regTools))
	for i, t := range regTools {
		apiTools[i] = Tool{
			Type:        "function",
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		}
	}
	return apiTools
}
