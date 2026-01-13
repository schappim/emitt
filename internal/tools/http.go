package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPTool makes HTTP requests
type HTTPTool struct {
	client *http.Client
}

// NewHTTPTool creates a new HTTP tool
func NewHTTPTool() *HTTPTool {
	return &HTTPTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *HTTPTool) Name() string {
	return "http_request"
}

func (t *HTTPTool) Description() string {
	return "Makes HTTP requests to external APIs and webhooks. Use this to call REST APIs, trigger webhooks, or fetch data from web services."
}

func (t *HTTPTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"method": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
				"description": "HTTP method to use",
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to send the request to",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "HTTP headers to include in the request",
				"additionalProperties": map[string]interface{}{
					"type": "string",
				},
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Request body (for POST/PUT/PATCH requests)",
			},
			"json_body": map[string]interface{}{
				"type":        "object",
				"description": "JSON body (alternative to body, will be serialized)",
			},
		},
		"required": []string{"method", "url"},
	}
}

// HTTPArgs represents the arguments for the HTTP tool
type HTTPArgs struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers"`
	Body     string            `json:"body"`
	JSONBody json.RawMessage   `json:"json_body"`
}

// HTTPResponse represents the HTTP response
type HTTPResponse struct {
	StatusCode int               `json:"status_code"`
	Status     string            `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

func (t *HTTPTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params HTTPArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return NewErrorResult(fmt.Errorf("invalid arguments: %w", err))
	}

	// Validate URL
	if params.URL == "" {
		return NewErrorResult(fmt.Errorf("url is required"))
	}
	if !strings.HasPrefix(params.URL, "http://") && !strings.HasPrefix(params.URL, "https://") {
		return NewErrorResult(fmt.Errorf("url must start with http:// or https://"))
	}

	// Prepare request body
	var bodyReader io.Reader
	if len(params.JSONBody) > 0 {
		bodyReader = bytes.NewReader(params.JSONBody)
		if params.Headers == nil {
			params.Headers = make(map[string]string)
		}
		if _, ok := params.Headers["Content-Type"]; !ok {
			params.Headers["Content-Type"] = "application/json"
		}
	} else if params.Body != "" {
		bodyReader = strings.NewReader(params.Body)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, params.Method, params.URL, bodyReader)
	if err != nil {
		return NewErrorResult(fmt.Errorf("failed to create request: %w", err))
	}

	// Add headers
	for key, value := range params.Headers {
		req.Header.Set(key, value)
	}

	// Execute request
	resp, err := t.client.Do(req)
	if err != nil {
		return NewErrorResult(fmt.Errorf("request failed: %w", err))
	}
	defer resp.Body.Close()

	// Read response body (limit to 1MB)
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return NewErrorResult(fmt.Errorf("failed to read response: %w", err))
	}

	// Build response
	response := HTTPResponse{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    make(map[string]string),
		Body:       string(bodyBytes),
	}

	for key := range resp.Header {
		response.Headers[key] = resp.Header.Get(key)
	}

	return NewSuccessResult(response)
}
