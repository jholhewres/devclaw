// Package copilot – mcp_http.go implements the MCP Streamable HTTP transport
// (the 2025 standard, which also covers servers advertised as "sse"). A single
// endpoint receives JSON-RPC requests via POST; the response is either a single
// application/json body or a text/event-stream carrying the JSON-RPC reply as
// an SSE event. The Mcp-Session-Id header returned by initialize is echoed on
// subsequent requests.
package copilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// errMCPUnauthorized is returned when an MCP HTTP endpoint replies 401. Phase 3
// (OAuth) detects this to trigger an authorization flow.
var errMCPUnauthorized = errors.New("mcp: unauthorized (401)")

// authProvider supplies (and refreshes) an Authorization header value for an
// MCP HTTP server. Injected by the OAuth layer (Phase 3); nil = no auth.
type authProvider interface {
	// AuthHeader returns the full Authorization header value (e.g. "Bearer x").
	AuthHeader(ctx context.Context) (string, error)
}

type mcpHTTPClient struct {
	name    string
	url     string
	headers map[string]string
	httpc   *http.Client
	auth    authProvider
	logger  *slog.Logger

	nextID    atomic.Int64
	mu        sync.Mutex
	sessionID string
}

func (b *MCPToolsBridge) connectHTTP(ctx context.Context, srv ManagedMCPServerConfig) error {
	if srv.URL == "" {
		return fmt.Errorf("url is required for %s servers", srv.Type)
	}
	timeout := time.Duration(srv.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client := &mcpHTTPClient{
		name:    srv.Name,
		url:     srv.URL,
		headers: srv.Headers,
		httpc:   &http.Client{Timeout: timeout},
		auth:    b.authFor(srv),
		logger:  b.logger.With("server", srv.Name),
	}
	return b.finishConnect(ctx, srv, client)
}

// authFor returns the auth provider for a server, if any. Phase 3 overrides
// this; the base bridge has no OAuth providers.
func (b *MCPToolsBridge) authFor(srv ManagedMCPServerConfig) authProvider {
	if b.authResolver != nil {
		return b.authResolver(srv)
	}
	return nil
}

func (c *mcpHTTPClient) initialize(ctx context.Context) error {
	if _, err := c.sendRequest(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "devclaw", "version": "1.0.0"},
	}); err != nil {
		return err
	}
	_ = c.sendNotification(ctx, "notifications/initialized", nil)
	return nil
}

func (c *mcpHTTPClient) listTools(ctx context.Context) ([]mcpToolDef, error) {
	resp, err := c.sendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	return parseToolsList(resp)
}

func (c *mcpHTTPClient) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	resp, err := c.sendRequest(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	return parseToolCallResult(resp)
}

// Close is a no-op for HTTP (no persistent process). Implements mcpClient.
func (c *mcpHTTPClient) Close() {}

func (c *mcpHTTPClient) sendNotification(ctx context.Context, method string, params any) error {
	body, err := json.Marshal(mcpRequest{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return err
	}
	req, resp, err := c.do(ctx, body)
	if err != nil {
		return err
	}
	_ = req
	resp.Body.Close()
	return nil
}

// sendRequest posts a JSON-RPC request and returns the matching result. It
// handles both a single application/json reply and a text/event-stream reply.
func (c *mcpHTTPClient) sendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	body, err := json.Marshal(mcpRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	_, resp, err := c.do(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Capture the session id from initialize so later requests reuse it.
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.mu.Lock()
		c.sessionID = sid
		c.mu.Unlock()
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errMCPUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return readSSEResponse(resp.Body, id)
	}
	// Plain JSON response.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return decodeRPC(data)
}

// do builds and executes a POST with the standard MCP headers + session + auth.
func (c *mcpHTTPClient) do(ctx context.Context, body []byte) (*http.Request, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()
	if sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	if c.auth != nil {
		if h, err := c.auth.AuthHeader(ctx); err != nil {
			return nil, nil, fmt.Errorf("auth: %w", err)
		} else if h != "" {
			req.Header.Set("Authorization", h)
		}
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	return req, resp, nil
}

// decodeRPC parses a single JSON-RPC response object and returns its result.
func decodeRPC(data []byte) (json.RawMessage, error) {
	var resp mcpResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

// readSSEResponse scans an SSE stream and returns the result of the JSON-RPC
// response whose id matches wantID (server-sent events deliver the reply as a
// `data:` payload). Stops at the first matching response.
func readSSEResponse(r io.Reader, wantID int64) (json.RawMessage, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var data strings.Builder
	flush := func() (json.RawMessage, bool, error) {
		if data.Len() == 0 {
			return nil, false, nil
		}
		payload := data.String()
		data.Reset()
		var probe struct {
			ID    *int64    `json:"id"`
			Error *mcpError `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &probe); err != nil {
			return nil, false, nil // not a JSON-RPC frame; ignore
		}
		if probe.ID == nil || *probe.ID != wantID {
			return nil, false, nil // a different message (e.g. server notification)
		}
		res, err := decodeRPC([]byte(payload))
		return res, true, err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" { // event boundary
			if res, ok, err := flush(); ok || err != nil {
				return res, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read sse: %w", err)
	}
	// Stream ended; try a final flush in case there was no trailing blank line.
	if res, ok, err := flush(); ok || err != nil {
		return res, err
	}
	return nil, fmt.Errorf("sse stream ended without a response for id %d", wantID)
}
