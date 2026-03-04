// Package copilot – mcp_tools.go bridges MCP (Model Context Protocol) servers
// with the ToolExecutor, allowing the LLM to call tools exposed by external
// MCP servers as if they were native DevClaw tools.
//
// Workflow:
//  1. For each enabled+auto_start MCP server, launch the process (stdio) or
//     connect to the endpoint (SSE/HTTP).
//  2. Send tools/list to discover available tools.
//  3. Register each tool in the ToolExecutor with a handler that forwards
//     tool calls via MCP's tools/call JSON-RPC.
//  4. On shutdown, send a graceful close and terminate processes.
//
// Currently supports stdio transport (most common for MCP).
package copilot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// MCP JSON-RPC types
// ---------------------------------------------------------------------------

type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// mcpToolDef represents a tool definition returned by tools/list.
type mcpToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type mcpToolsListResult struct {
	Tools []mcpToolDef `json:"tools"`
}

type mcpToolCallResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ---------------------------------------------------------------------------
// MCPToolsBridge manages MCP server connections and tool registration
// ---------------------------------------------------------------------------

// MCPToolsBridge connects MCP servers to the ToolExecutor.
type MCPToolsBridge struct {
	executor *ToolExecutor
	logger   *slog.Logger

	mu      sync.Mutex
	clients map[string]*mcpStdioClient // key: server name
}

// NewMCPToolsBridge creates a bridge that will register MCP tools.
func NewMCPToolsBridge(executor *ToolExecutor, logger *slog.Logger) *MCPToolsBridge {
	return &MCPToolsBridge{
		executor: executor,
		logger:   logger.With("component", "mcp-tools"),
		clients:  make(map[string]*mcpStdioClient),
	}
}

// ConnectAll launches all enabled auto-start MCP servers, discovers their
// tools, and registers them in the ToolExecutor.
func (b *MCPToolsBridge) ConnectAll(ctx context.Context, servers []ManagedMCPServerConfig) {
	for _, srv := range servers {
		if !srv.Enabled || !srv.AutoStart {
			continue
		}
		if srv.Type != MCPTypeStdio {
			b.logger.Warn("mcp tools bridge: only stdio supported",
				"server", srv.Name, "type", srv.Type)
			continue
		}

		if err := b.connectStdio(ctx, srv); err != nil {
			b.logger.Error("mcp tools bridge: connect failed",
				"server", srv.Name, "error", err)
		}
	}
}

// Shutdown gracefully closes all MCP server connections.
func (b *MCPToolsBridge) Shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for name, client := range b.clients {
		b.logger.Debug("shutting down mcp server", "server", name)
		client.Close()
	}
	b.clients = make(map[string]*mcpStdioClient)
}

// ---------------------------------------------------------------------------
// stdio transport
// ---------------------------------------------------------------------------

type mcpStdioClient struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	nextID  atomic.Int64
	mu      sync.Mutex // serializes requests
	logger  *slog.Logger
}

func (b *MCPToolsBridge) connectStdio(ctx context.Context, srv ManagedMCPServerConfig) error {
	b.logger.Info("launching mcp stdio server", "server", srv.Name, "command", srv.Command)

	cmd := exec.CommandContext(ctx, srv.Command, srv.Args...)

	// Set environment.
	cmd.Env = os.Environ()
	for k, v := range srv.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	// Discard stderr (MCP protocol uses stdout).
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", srv.Command, err)
	}

	client := &mcpStdioClient{
		name:   srv.Name,
		cmd:    cmd,
		stdin:  stdinPipe,
		stdout: bufio.NewReader(stdoutPipe),
		logger: b.logger.With("server", srv.Name),
	}

	// Initialize the MCP connection.
	timeout := time.Duration(srv.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	initCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := client.initialize(initCtx); err != nil {
		client.Close()
		return fmt.Errorf("initialize: %w", err)
	}

	// Discover tools.
	tools, err := client.listTools(initCtx)
	if err != nil {
		client.Close()
		return fmt.Errorf("list tools: %w", err)
	}

	b.mu.Lock()
	b.clients[srv.Name] = client
	b.mu.Unlock()

	// Register each tool in the ToolExecutor.
	for _, tool := range tools {
		b.registerMCPTool(srv.Name, tool, client)
	}

	b.logger.Info("mcp server connected",
		"server", srv.Name,
		"tools", len(tools),
	)

	return nil
}

func (b *MCPToolsBridge) registerMCPTool(serverName string, tool mcpToolDef, client *mcpStdioClient) {
	// Prefix tool name with server name to avoid collisions.
	fullName := sanitizeToolName("mcp_" + serverName + "_" + tool.Name)

	schema := tool.InputSchema
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}

	def := ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        fullName,
			Description: fmt.Sprintf("[MCP:%s] %s", serverName, tool.Description),
			Parameters:  schema,
		},
	}

	originalName := tool.Name

	handler := func(ctx context.Context, args map[string]any) (any, error) {
		callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		result, err := client.callTool(callCtx, originalName, args)
		if err != nil {
			return nil, fmt.Errorf("mcp tool %s/%s: %w", serverName, originalName, err)
		}
		return result, nil
	}

	b.executor.Register(def, handler)
}

// ---------------------------------------------------------------------------
// MCP protocol methods
// ---------------------------------------------------------------------------

func (c *mcpStdioClient) initialize(ctx context.Context) error {
	resp, err := c.sendRequest(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "devclaw",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return err
	}

	// Send initialized notification (no response expected).
	_ = c.sendNotification("notifications/initialized", nil)

	c.logger.Debug("mcp initialized", "result", string(resp))
	return nil
}

func (c *mcpStdioClient) listTools(ctx context.Context) ([]mcpToolDef, error) {
	resp, err := c.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var result mcpToolsListResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list: %w", err)
	}

	return result.Tools, nil
}

func (c *mcpStdioClient) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	resp, err := c.sendRequest(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}

	var result mcpToolCallResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parse tools/call: %w", err)
	}

	// Concatenate text content blocks.
	var sb strings.Builder
	for _, c := range result.Content {
		if c.Type == "text" || c.Type == "" {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(c.Text)
		}
	}

	text := sb.String()
	if result.IsError {
		return "", fmt.Errorf("mcp tool error: %s", text)
	}

	return text, nil
}

// ---------------------------------------------------------------------------
// JSON-RPC transport
// ---------------------------------------------------------------------------

func (c *mcpStdioClient) sendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Write request as a single line followed by newline.
	if _, err := fmt.Fprintf(c.stdin, "%s\n", data); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read response synchronously. The mutex already serializes
	// request/response pairs, so no goroutine is needed. Timeout is
	// enforced by the process's CommandContext — when cancelled, the
	// process is killed and stdout returns io.EOF.
	line, err := c.stdout.ReadBytes('\n')
	if err != nil {
		// Check if context was cancelled while reading.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp mcpResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

func (c *mcpStdioClient) sendNotification(method string, params any) error {
	req := mcpRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	return err
}

// Close terminates the MCP server process.
func (c *mcpStdioClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
}
