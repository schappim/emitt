package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"

	"github.com/emitt/emitt/internal/config"
	"github.com/emitt/emitt/internal/tools"
)

// Client manages connections to MCP servers
type Client struct {
	servers map[string]*ServerConnection
	logger  zerolog.Logger
	mu      sync.RWMutex
}

// NewClient creates a new MCP client
func NewClient(logger zerolog.Logger) *Client {
	return &Client{
		servers: make(map[string]*ServerConnection),
		logger:  logger.With().Str("component", "mcp").Logger(),
	}
}

// Connect establishes connections to all configured MCP servers
func (c *Client) Connect(ctx context.Context, configs []config.MCPServerConfig) error {
	for _, cfg := range configs {
		if err := c.ConnectServer(ctx, cfg); err != nil {
			c.logger.Error().
				Err(err).
				Str("server", cfg.Name).
				Msg("Failed to connect to MCP server")
			// Continue with other servers
		}
	}
	return nil
}

// ConnectServer connects to a single MCP server
func (c *Client) ConnectServer(ctx context.Context, cfg config.MCPServerConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Close existing connection if any
	if existing, ok := c.servers[cfg.Name]; ok {
		existing.Close()
	}

	conn, err := NewServerConnection(cfg, c.logger)
	if err != nil {
		return err
	}

	if err := conn.Initialize(ctx); err != nil {
		conn.Close()
		return err
	}

	c.servers[cfg.Name] = conn
	c.logger.Info().
		Str("server", cfg.Name).
		Int("tools", len(conn.tools)).
		Msg("Connected to MCP server")

	return nil
}

// GetTools returns all tools from all connected servers
func (c *Client) GetTools() []tools.Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []tools.Tool
	for _, server := range c.servers {
		result = append(result, server.GetTools()...)
	}
	return result
}

// RegisterTools registers all MCP tools with the tool registry
func (c *Client) RegisterTools(registry *tools.Registry) {
	for _, tool := range c.GetTools() {
		registry.Register(tool)
	}
}

// Close closes all server connections
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for name, server := range c.servers {
		if err := server.Close(); err != nil {
			c.logger.Error().Err(err).Str("server", name).Msg("Error closing server")
		}
	}
	c.servers = make(map[string]*ServerConnection)
	return nil
}

// ServerConnection represents a connection to an MCP server
type ServerConnection struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	tools   []*MCPTool
	logger  zerolog.Logger
	reqID   atomic.Int64
	pending map[int64]chan *JSONRPCResponse
	mu      sync.Mutex
}

// NewServerConnection creates a new server connection
func NewServerConnection(cfg config.MCPServerConfig, logger zerolog.Logger) (*ServerConnection, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %w", err)
	}

	conn := &ServerConnection{
		name:    cfg.Name,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		logger:  logger.With().Str("mcp_server", cfg.Name).Logger(),
		pending: make(map[int64]chan *JSONRPCResponse),
	}

	// Start reading responses
	go conn.readResponses()

	return conn, nil
}

// Initialize performs the MCP initialization handshake
func (c *ServerConnection) Initialize(ctx context.Context) error {
	// Send initialize request
	_, err := c.request(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "emitt",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	// Send initialized notification
	if err := c.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("initialized notification failed: %w", err)
	}

	// List tools
	resp, err := c.request(ctx, "tools/list", nil)
	if err != nil {
		return fmt.Errorf("tools/list failed: %w", err)
	}

	// Parse tools
	var toolsResult struct {
		Tools []struct {
			Name        string                 `json:"name"`
			Description string                 `json:"description"`
			InputSchema map[string]interface{} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &toolsResult); err != nil {
		return fmt.Errorf("failed to parse tools: %w", err)
	}

	for _, t := range toolsResult.Tools {
		c.tools = append(c.tools, &MCPTool{
			conn:        c,
			name:        fmt.Sprintf("%s:%s", c.name, t.Name),
			mcpName:     t.Name,
			description: t.Description,
			params:      t.InputSchema,
		})
	}

	return nil
}

// GetTools returns the tools provided by this server
func (c *ServerConnection) GetTools() []tools.Tool {
	result := make([]tools.Tool, len(c.tools))
	for i, t := range c.tools {
		result[i] = t
	}
	return result
}

// CallTool invokes a tool on the MCP server
func (c *ServerConnection) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	resp, err := c.request(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": json.RawMessage(args),
	})
	if err != nil {
		return nil, err
	}

	// Parse tool result
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return resp.Result, nil // Return raw result if parsing fails
	}

	if result.IsError {
		if len(result.Content) > 0 {
			return nil, fmt.Errorf("tool error: %s", result.Content[0].Text)
		}
		return nil, fmt.Errorf("tool returned error")
	}

	if len(result.Content) > 0 && result.Content[0].Type == "text" {
		return json.Marshal(map[string]interface{}{
			"success": true,
			"data":    result.Content[0].Text,
		})
	}

	return resp.Result, nil
}

// Close closes the server connection
func (c *ServerConnection) Close() error {
	c.stdin.Close()
	c.stdout.Close()
	return c.cmd.Process.Kill()
}

// JSONRPCRequest represents a JSON-RPC request
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      *int64      `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC response
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC error
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *ServerConnection) request(ctx context.Context, method string, params interface{}) (*JSONRPCResponse, error) {
	id := c.reqID.Add(1)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}

	// Create response channel
	respCh := make(chan *JSONRPCResponse, 1)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	// Send request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	c.logger.Debug().RawJSON("request", data).Msg("Sending MCP request")

	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *ServerConnection) notify(method string, params interface{}) error {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

func (c *ServerConnection) readResponses() {
	scanner := bufio.NewScanner(c.stdout)
	// Increase buffer size for large responses
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			c.logger.Error().Err(err).Msg("Failed to parse response")
			continue
		}

		if resp.ID != nil {
			c.mu.Lock()
			if ch, ok := c.pending[*resp.ID]; ok {
				ch <- &resp
			}
			c.mu.Unlock()
		}
	}

	if err := scanner.Err(); err != nil {
		c.logger.Error().Err(err).Msg("Error reading from MCP server")
	}
}

// MCPTool wraps an MCP server tool as a tools.Tool
type MCPTool struct {
	conn        *ServerConnection
	name        string
	mcpName     string
	description string
	params      map[string]interface{}
}

func (t *MCPTool) Name() string {
	return t.name
}

func (t *MCPTool) Description() string {
	return t.description
}

func (t *MCPTool) Parameters() map[string]interface{} {
	return t.params
}

func (t *MCPTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	return t.conn.CallTool(ctx, t.mcpName, args)
}
