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
	// ID is a pointer so notifications (no id) omit it entirely. Per JSON-RPC
	// 2.0 a notification MUST NOT carry an id; sending "id":0 makes strict MCP
	// servers treat it as a request and reply, desyncing the stream.
	ID     *int64 `json:"id,omitempty"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
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

// mcpClient is the transport-agnostic interface a connected MCP server exposes
// to the bridge. Implemented by mcpStdioClient (stdio) and mcpHTTPClient
// (Streamable HTTP / SSE).
type mcpClient interface {
	initialize(ctx context.Context) error
	listTools(ctx context.Context) ([]mcpToolDef, error)
	callTool(ctx context.Context, name string, args map[string]any) (string, error)
	Close()
}

// MCPToolsBridge connects MCP servers to the ToolExecutor.
type MCPToolsBridge struct {
	executor *ToolExecutor
	logger   *slog.Logger
	baseCtx  context.Context // lifetime ctx for runtime (re)connects

	// authResolver, when set, supplies an OAuth-backed auth provider for HTTP
	// servers. Wired by the OAuth layer (Phase 3); nil = headers-only auth.
	authResolver func(ManagedMCPServerConfig) authProvider

	mu            sync.Mutex
	clients       map[string]mcpClient // key: server name
	toolsByServer map[string][]string  // key: server name -> registered tool names
}

// NewMCPToolsBridge creates a bridge that will register MCP tools.
func NewMCPToolsBridge(executor *ToolExecutor, logger *slog.Logger) *MCPToolsBridge {
	return &MCPToolsBridge{
		executor:      executor,
		logger:        logger.With("component", "mcp-tools"),
		baseCtx:       context.Background(),
		clients:       make(map[string]mcpClient),
		toolsByServer: make(map[string][]string),
	}
}

// SetBaseContext sets the long-lived context used for runtime (re)connects
// triggered after startup (e.g. the agent `mcp` tool). MCP server processes
// are tied to this context, so it should be the assistant's lifetime context.
func (b *MCPToolsBridge) SetBaseContext(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.baseCtx = ctx
}

// ConnectAll launches all enabled auto-start MCP servers, discovers their
// tools, and registers them in the ToolExecutor.
func (b *MCPToolsBridge) ConnectAll(ctx context.Context, servers []ManagedMCPServerConfig) {
	for _, srv := range servers {
		if !srv.Enabled || !srv.AutoStart {
			continue
		}
		if err := b.ConnectOne(ctx, srv); err != nil {
			b.logger.Error("mcp tools bridge: connect failed",
				"server", srv.Name, "error", err)
		}
	}
}

// ConnectOne connects a single MCP server now and registers its tools,
// regardless of the server's Enabled/AutoStart flags (the caller decides).
// If the server is already connected it is first disconnected so tools are
// refreshed cleanly. Currently only the stdio transport is supported.
func (b *MCPToolsBridge) ConnectOne(ctx context.Context, srv ManagedMCPServerConfig) error {
	if b.IsConnected(srv.Name) {
		_ = b.DisconnectOne(srv.Name)
	}
	switch srv.Type {
	case "", MCPTypeStdio:
		return b.connectStdio(ctx, srv)
	case MCPTypeHTTP, MCPTypeSSE:
		return b.connectHTTP(ctx, srv)
	default:
		return fmt.Errorf("transport %q not supported (use stdio, http or sse)", srv.Type)
	}
}

// finishConnect runs the shared post-connect steps: initialize the session,
// discover tools, register them in the executor and record the client. Used by
// every transport.
func (b *MCPToolsBridge) finishConnect(ctx context.Context, srv ManagedMCPServerConfig, client mcpClient) error {
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
	tools, err := client.listTools(initCtx)
	if err != nil {
		client.Close()
		return fmt.Errorf("list tools: %w", err)
	}

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, b.registerMCPTool(srv.Name, tool, client))
	}

	b.mu.Lock()
	b.clients[srv.Name] = client
	b.toolsByServer[srv.Name] = names
	b.mu.Unlock()

	b.logger.Info("mcp server connected", "server", srv.Name, "transport", mcpTypeOrDefault(srv.Type), "tools", len(tools))
	return nil
}

// DisconnectOne closes a single MCP server connection and unregisters all of
// its tools from the executor. Returns an error if the server is not connected.
func (b *MCPToolsBridge) DisconnectOne(name string) error {
	b.mu.Lock()
	client, ok := b.clients[name]
	tools := b.toolsByServer[name]
	delete(b.clients, name)
	delete(b.toolsByServer, name)
	b.mu.Unlock()

	if !ok {
		return fmt.Errorf("server %q is not connected", name)
	}

	client.Close()
	for _, tn := range tools {
		b.executor.UnregisterTool(tn)
	}
	b.logger.Info("mcp server disconnected", "server", name, "tools_removed", len(tools))
	return nil
}

// IsConnected reports whether the named MCP server currently has a live client.
func (b *MCPToolsBridge) IsConnected(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.clients[name]
	return ok
}

// ServerTools returns the tool names currently registered for a connected
// server (empty if not connected).
func (b *MCPToolsBridge) ServerTools(name string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.toolsByServer[name]...)
}

// BaseContext returns the long-lived context used for runtime connects.
func (b *MCPToolsBridge) BaseContext() context.Context {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.baseCtx
}

// Shutdown gracefully closes all MCP server connections.
func (b *MCPToolsBridge) Shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for name, client := range b.clients {
		b.logger.Debug("shutting down mcp server", "server", name)
		client.Close()
	}
	b.clients = make(map[string]mcpClient)
	b.toolsByServer = make(map[string][]string)
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

	// MCP protocol uses stdout; the server's stderr is its log channel. Discard
	// it by default, but pass it through when DEVCLAW_MCP_STDERR is set so MCP
	// servers can be debugged.
	if os.Getenv("DEVCLAW_MCP_STDERR") != "" {
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stderr = io.Discard
	}

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

	return b.finishConnect(ctx, srv, client)
}

// registerMCPTool registers a single MCP tool in the executor and returns the
// full (prefixed) tool name under which it was registered.
func (b *MCPToolsBridge) registerMCPTool(serverName string, tool mcpToolDef, client mcpClient) string {
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
	return fullName
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
	return parseToolsList(resp)
}

func (c *mcpStdioClient) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	resp, err := c.sendRequest(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	return parseToolCallResult(resp)
}

// parseToolsList decodes a tools/list result. Shared across transports.
func parseToolsList(raw json.RawMessage) ([]mcpToolDef, error) {
	var result mcpToolsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list: %w", err)
	}
	return result.Tools, nil
}

// parseToolCallResult decodes a tools/call result, concatenating text content
// blocks. Shared across transports.
func parseToolCallResult(raw json.RawMessage) (string, error) {
	var result mcpToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("parse tools/call: %w", err)
	}

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
		ID:      &id,
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
