package processor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/sashabaranov/go-openai"

	"github.com/emitt/emitt/internal/config"
	"github.com/emitt/emitt/internal/tools"
)

// LLMClient wraps the OpenAI client
type LLMClient struct {
	client   *openai.Client
	model    string
	maxTokens int
	temp     float32
	logger   zerolog.Logger
}

// NewLLMClient creates a new LLM client
func NewLLMClient(cfg *config.LLMConfig, logger zerolog.Logger) *LLMClient {
	client := openai.NewClient(cfg.APIKey)

	return &LLMClient{
		client:    client,
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		temp:      cfg.Temperature,
		logger:    logger.With().Str("component", "llm").Logger(),
	}
}

// Message represents a chat message
type Message struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
}

// ToolCall represents a function call from the LLM
type ToolCall struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatRequest represents a chat completion request
type ChatRequest struct {
	SystemPrompt string
	Messages     []Message
	Tools        []openai.Tool
}

// ChatResponse represents a chat completion response
type ChatResponse struct {
	Message    Message
	ToolCalls  []ToolCall
	FinishReason string
	Usage      Usage
}

// Usage represents token usage
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Chat sends a chat completion request
func (c *LLMClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	messages := make([]openai.ChatCompletionMessage, 0, len(req.Messages)+1)

	// Add system prompt if provided
	if req.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: req.SystemPrompt,
		})
	}

	// Convert messages
	for _, m := range req.Messages {
		msg := openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
		if m.ToolCallID != "" {
			msg.ToolCallID = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]openai.ToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				msg.ToolCalls[i] = openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				}
			}
		}
		messages = append(messages, msg)
	}

	// Build request
	chatReq := openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		MaxTokens:   c.maxTokens,
		Temperature: c.temp,
	}

	if len(req.Tools) > 0 {
		chatReq.Tools = req.Tools
	}

	c.logger.Debug().
		Str("model", c.model).
		Int("message_count", len(messages)).
		Int("tool_count", len(req.Tools)).
		Msg("Sending chat request")

	// Send request
	resp, err := c.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := resp.Choices[0]

	// Build response
	response := &ChatResponse{
		Message: Message{
			Role:    choice.Message.Role,
			Content: choice.Message.Content,
		},
		FinishReason: string(choice.FinishReason),
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	// Extract tool calls
	if len(choice.Message.ToolCalls) > 0 {
		response.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		response.Message.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			toolCall := ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
			response.ToolCalls[i] = toolCall
			response.Message.ToolCalls[i] = toolCall
		}
	}

	c.logger.Debug().
		Str("finish_reason", response.FinishReason).
		Int("tool_calls", len(response.ToolCalls)).
		Int("total_tokens", response.Usage.TotalTokens).
		Msg("Received chat response")

	return response, nil
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

	messages := []Message{
		{Role: openai.ChatMessageRoleUser, Content: userMessage},
	}

	openaiTools := registry.ToOpenAITools(toolNames)

	for i := 0; i < maxIterations; i++ {
		resp, err := c.Chat(ctx, ChatRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        openaiTools,
		})
		if err != nil {
			return "", err
		}

		// Add assistant message
		messages = append(messages, resp.Message)

		// Check if done
		if resp.FinishReason == "stop" || len(resp.ToolCalls) == 0 {
			return resp.Message.Content, nil
		}

		// Execute tool calls
		for _, tc := range resp.ToolCalls {
			c.logger.Info().
				Str("tool", tc.Name).
				Str("call_id", tc.ID).
				Msg("Executing tool call")

			result, err := registry.Execute(ctx, tc.Name, json.RawMessage(tc.Arguments))
			if err != nil {
				result, _ = tools.NewErrorResult(err)
			}

			messages = append(messages, Message{
				Role:       openai.ChatMessageRoleTool,
				Content:    string(result),
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("max iterations reached without completion")
}
