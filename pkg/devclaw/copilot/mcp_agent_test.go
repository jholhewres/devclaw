package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestMCPAssistant builds a minimal Assistant wired for MCP management tests:
// a temp config file, a real ToolExecutor and an MCP bridge.
func newTestMCPAssistant(t *testing.T) (*Assistant, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := DefaultConfig()
	if err := SaveConfigToFile(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	te := NewToolExecutor(logger)
	a := &Assistant{
		config:       cfg,
		configPath:   path,
		toolExecutor: te,
		logger:       logger,
	}
	a.mcpBridge = NewMCPToolsBridge(te, logger)
	a.mcpBridge.SetBaseContext(context.Background())
	return a, path
}

func TestMCPAdd_PersistsAndValidates(t *testing.T) {
	a, path := newTestMCPAssistant(t)

	// Missing command is rejected for stdio.
	if _, err := a.mcpAdd(map[string]any{"name": "x"}); err == nil {
		t.Error("stdio server without command should be rejected")
	}

	// Valid add (start=false to avoid launching a process).
	msg, err := a.mcpAdd(map[string]any{
		"name":    "fs",
		"command": "mcp-filesystem",
		"args":    []any{"/workspace"},
		"start":   false,
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(msg, "fs") {
		t.Errorf("unexpected message: %q", msg)
	}

	// Persisted to disk with the subsystem enabled.
	onDisk, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !onDisk.MCP.Enabled {
		t.Error("adding a server should enable the MCP subsystem")
	}
	if len(onDisk.MCP.Servers) != 1 || onDisk.MCP.Servers[0].Name != "fs" {
		t.Fatalf("server not persisted: %+v", onDisk.MCP.Servers)
	}
	if onDisk.MCP.Servers[0].Timeout != 30 {
		t.Errorf("default timeout should be 30, got %d", onDisk.MCP.Servers[0].Timeout)
	}

	// In-memory config mirrors disk.
	if len(a.config.MCP.Servers) != 1 {
		t.Error("in-memory config not updated")
	}

	// Duplicate rejected.
	if _, err := a.mcpAdd(map[string]any{"name": "fs", "command": "x", "start": false}); err == nil {
		t.Error("duplicate name should be rejected")
	}
}

func TestMCPRemoveAndDisable(t *testing.T) {
	a, _ := newTestMCPAssistant(t)
	if _, err := a.mcpAdd(map[string]any{"name": "fs", "command": "x", "start": false}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Disabling a stopped server must succeed (no connection to tear down).
	if _, err := a.mcpSetEnabled("fs", false); err != nil {
		t.Errorf("disable stopped server: %v", err)
	}

	// Remove unknown → error; remove known → ok.
	if _, err := a.mcpRemove("ghost"); err == nil {
		t.Error("removing unknown server should error")
	}
	if _, err := a.mcpRemove("fs"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(a.config.MCP.Servers) != 0 {
		t.Error("server not removed from memory")
	}
}

// TestMCPBridge_StdioLifecycle drives ConnectOne/DisconnectOne against a fake
// MCP server (this test binary re-invoked as a helper process). It verifies
// tools are registered as mcp_<server>_<tool> and unregistered on disconnect.
func TestMCPBridge_StdioLifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	te := NewToolExecutor(logger)
	b := NewMCPToolsBridge(te, logger)

	srv := ManagedMCPServerConfig{
		Name:    "fake",
		Type:    MCPTypeStdio,
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPHelperProcess"},
		Env:     map[string]string{"GO_WANT_MCP_HELPER": "1"},
		Timeout: 10,
	}

	if err := b.ConnectOne(context.Background(), srv); err != nil {
		t.Fatalf("ConnectOne: %v", err)
	}
	if !b.IsConnected("fake") {
		t.Fatal("server should be connected")
	}
	tools := b.ServerTools("fake")
	if len(tools) != 1 || tools[0] != "mcp_fake_echo" {
		t.Fatalf("expected [mcp_fake_echo], got %v", tools)
	}
	if !te.HasTool("mcp_fake_echo") {
		t.Error("tool not registered in executor")
	}

	if err := b.DisconnectOne("fake"); err != nil {
		t.Fatalf("DisconnectOne: %v", err)
	}
	if b.IsConnected("fake") {
		t.Error("server should be disconnected")
	}
	if te.HasTool("mcp_fake_echo") {
		t.Error("tool should be unregistered after disconnect")
	}
}

// TestMCPHelperProcess is not a real test: when GO_WANT_MCP_HELPER=1 it acts as
// a minimal stdio MCP server speaking just enough JSON-RPC for the lifecycle test.
func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER") != "1" {
		return
	}
	dec := json.NewDecoder(os.Stdin)
	for {
		var req struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		if err := dec.Decode(&req); err != nil {
			return // stdin closed → exit
		}
		// Notifications carry no id and expect no response.
		if req.ID == nil {
			continue
		}
		var result string
		switch req.Method {
		case "initialize":
			result = `{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"fake","version":"0"}}`
		case "tools/list":
			result = `{"tools":[{"name":"echo","description":"echo back","inputSchema":{"type":"object","properties":{}}}]}`
		case "tools/call":
			result = `{"content":[{"type":"text","text":"ok"}]}`
		default:
			result = `{}`
		}
		fmt.Fprintf(os.Stdout, `{"jsonrpc":"2.0","id":%d,"result":%s}`+"\n", *req.ID, result)
	}
}
