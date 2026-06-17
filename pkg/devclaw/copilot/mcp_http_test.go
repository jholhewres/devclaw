package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeMCPHTTPServer is a minimal Streamable HTTP MCP server: it replies to
// initialize/tools/call with plain JSON and to tools/list with an SSE event,
// exercising both response paths.
func fakeMCPHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Notifications (no id) just get a 202.
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		writeJSON := func(result string) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, *req.ID, result)
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "sess-123")
			writeJSON(`{"protocolVersion":"2024-11-05","capabilities":{}}`)
		case "tools/list":
			// Reply over SSE to exercise readSSEResponse.
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"tools\":[{\"name\":\"search\",\"description\":\"d\",\"inputSchema\":{\"type\":\"object\"}}]}}\n\n", *req.ID)
		case "tools/call":
			// Verify the session id round-trips.
			if r.Header.Get("Mcp-Session-Id") != "sess-123" {
				http.Error(w, "missing session", http.StatusBadRequest)
				return
			}
			writeJSON(`{"content":[{"type":"text","text":"hit"}]}`)
		default:
			writeJSON(`{}`)
		}
	}))
}

func TestMCPBridge_HTTPLifecycle(t *testing.T) {
	srv := fakeMCPHTTPServer(t)
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	te := NewToolExecutor(logger)
	b := NewMCPToolsBridge(te, logger)

	cfg := ManagedMCPServerConfig{
		Name:    "remote",
		Type:    MCPTypeHTTP,
		URL:     srv.URL,
		Timeout: 10,
	}
	if err := b.ConnectOne(context.Background(), cfg); err != nil {
		t.Fatalf("ConnectOne(http): %v", err)
	}
	tools := b.ServerTools("remote")
	if len(tools) != 1 || tools[0] != "mcp_remote_search" {
		t.Fatalf("expected [mcp_remote_search], got %v", tools)
	}

	// Invoke the registered tool end-to-end (also asserts session-id reuse).
	out, err := te.executeByName(context.Background(), "mcp_remote_search", map[string]any{"q": "x"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if out != "hit" {
		t.Errorf("tool result = %q, want %q", out, "hit")
	}

	if err := b.DisconnectOne("remote"); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if te.HasTool("mcp_remote_search") {
		t.Error("tool should be unregistered")
	}
}

func TestReadSSEResponse_MatchesID(t *testing.T) {
	// A stream with an unrelated notification first, then the real reply.
	stream := "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n" +
		"data: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n\n"
	res, err := readSSEResponse(strings.NewReader(stream), 7)
	if err != nil {
		t.Fatalf("readSSEResponse: %v", err)
	}
	if string(res) != `{"ok":true}` {
		t.Errorf("result = %s", res)
	}
}
