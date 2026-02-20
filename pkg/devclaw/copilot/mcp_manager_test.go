package copilot

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestNewMCPManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := DefaultMCPConfig()
	m := NewMCPManager(cfg, nil, logger)

	if m == nil {
		t.Fatal("expected manager, got nil")
	}

	if len(m.ListServers()) != 0 {
		t.Errorf("expected 0 servers, got %d", len(m.ListServers()))
	}
}

func TestMCPManager_AddServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewMCPManager(DefaultMCPConfig(), nil, logger)

	// First add a server to test duplicate
	_ = m.AddServer(ManagedMCPServerConfig{
		Name:    "filesystem",
		Type:    MCPTypeStdio,
		Command: "mcp-filesystem",
	})

	tests := []struct {
		name    string
		cfg     ManagedMCPServerConfig
		wantErr bool
	}{
		{
			name: "valid stdio server",
			cfg: ManagedMCPServerConfig{
				Name:    "filesystem2",
				Type:    MCPTypeStdio,
				Command: "mcp-filesystem",
				Args:    []string{"/workspace"},
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "valid http server",
			cfg: ManagedMCPServerConfig{
				Name:    "api-server",
				Type:    MCPTypeHTTP,
				URL:     "http://localhost:8080/mcp",
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			cfg: ManagedMCPServerConfig{
				Type:    MCPTypeStdio,
				Command: "mcp-server",
			},
			wantErr: true,
		},
		{
			name: "duplicate name",
			cfg: ManagedMCPServerConfig{
				Name:    "filesystem",
				Type:    MCPTypeStdio,
				Command: "mcp-filesystem2",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.AddServer(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddServer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMCPManager_GetServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewMCPManager(DefaultMCPConfig(), nil, logger)

	// Add a server
	_ = m.AddServer(ManagedMCPServerConfig{
		Name:    "test-server",
		Type:    MCPTypeStdio,
		Command: "test-cmd",
		Enabled: true,
	})

	// Get existing
	info := m.GetServer("test-server")
	if info == nil {
		t.Fatal("expected server info, got nil")
	}
	if info.Config.Name != "test-server" {
		t.Errorf("name = %q, want %q", info.Config.Name, "test-server")
	}

	// Get non-existing
	info = m.GetServer("nonexistent")
	if info != nil {
		t.Error("expected nil for nonexistent server")
	}
}

func TestMCPManager_UpdateServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewMCPManager(DefaultMCPConfig(), nil, logger)

	// Add a server first
	_ = m.AddServer(ManagedMCPServerConfig{
		Name:    "update-test",
		Type:    MCPTypeStdio,
		Command: "old-cmd",
		Enabled: true,
	})

	// Update it
	err := m.UpdateServer("update-test", ManagedMCPServerConfig{
		Name:    "update-test",
		Type:    MCPTypeStdio,
		Command: "new-cmd",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("UpdateServer() error = %v", err)
	}

	// Verify update
	info := m.GetServer("update-test")
	if info == nil {
		t.Fatal("server not found after update")
	}
	if info.Config.Command != "new-cmd" {
		t.Errorf("command = %q, want %q", info.Config.Command, "new-cmd")
	}
	if info.Config.Enabled {
		t.Error("expected server to be disabled")
	}

	// Update non-existing
	err = m.UpdateServer("nonexistent", ManagedMCPServerConfig{Name: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent server")
	}
}

func TestMCPManager_RemoveServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewMCPManager(DefaultMCPConfig(), nil, logger)

	// Add a server
	_ = m.AddServer(ManagedMCPServerConfig{
		Name:    "remove-test",
		Type:    MCPTypeStdio,
		Command: "test-cmd",
	})

	// Remove it
	err := m.RemoveServer("remove-test")
	if err != nil {
		t.Fatalf("RemoveServer() error = %v", err)
	}

	// Verify removed
	if m.GetServer("remove-test") != nil {
		t.Error("expected server to be removed")
	}

	// Remove non-existing
	err = m.RemoveServer("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent server")
	}
}

func TestMCPManager_SetEnabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewMCPManager(DefaultMCPConfig(), nil, logger)

	// Add a server
	_ = m.AddServer(ManagedMCPServerConfig{
		Name:    "toggle-test",
		Type:    MCPTypeStdio,
		Command: "test-cmd",
		Enabled: true,
	})

	// Disable
	err := m.SetEnabled("toggle-test", false)
	if err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

	info := m.GetServer("toggle-test")
	if info.Config.Enabled {
		t.Error("expected server to be disabled")
	}

	// Enable
	_ = m.SetEnabled("toggle-test", true)
	info = m.GetServer("toggle-test")
	if !info.Config.Enabled {
		t.Error("expected server to be enabled")
	}

	// Non-existing
	err = m.SetEnabled("nonexistent", true)
	if err == nil {
		t.Error("expected error for nonexistent server")
	}
}

func TestMCPManager_TestServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewMCPManager(DefaultMCPConfig(), nil, logger)

	// Add a stdio server
	_ = m.AddServer(ManagedMCPServerConfig{
		Name:    "test-stdio",
		Type:    MCPTypeStdio,
		Command: "mcp-filesystem",
		Enabled: true,
	})

	// Test the server
	status, err := m.TestServer(context.Background(), "test-stdio")
	if err != nil {
		t.Fatalf("TestServer() error = %v", err)
	}
	if status.Name != "test-stdio" {
		t.Errorf("status name = %q, want %q", status.Name, "test-stdio")
	}

	// Test non-existing
	_, err = m.TestServer(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent server")
	}
}

func TestMCPManager_ListServers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewMCPManager(DefaultMCPConfig(), nil, logger)

	// Initially empty
	if len(m.ListServers()) != 0 {
		t.Error("expected empty list initially")
	}

	// Add servers
	_ = m.AddServer(ManagedMCPServerConfig{Name: "server-a", Type: MCPTypeStdio, Command: "a"})
	_ = m.AddServer(ManagedMCPServerConfig{Name: "server-b", Type: MCPTypeHTTP, URL: "http://localhost"})

	list := m.ListServers()
	if len(list) != 2 {
		t.Errorf("expected 2 servers, got %d", len(list))
	}
}

func TestMCPManager_Reload(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	m := NewMCPManager(DefaultMCPConfig(), nil, logger)

	// Reload with new config
	newCfg := MCPConfig{
		Enabled: true,
		Servers: []ManagedMCPServerConfig{
			{Name: "reloaded-server", Type: MCPTypeStdio, Command: "test"},
		},
	}

	m.Reload(newCfg)

	list := m.ListServers()
	if len(list) != 1 {
		t.Errorf("expected 1 server after reload, got %d", len(list))
	}
	if list[0].Config.Name != "reloaded-server" {
		t.Errorf("name = %q, want %q", list[0].Config.Name, "reloaded-server")
	}
}

func TestMCPManager_GetConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := MCPConfig{
		Enabled: true,
		Servers: []ManagedMCPServerConfig{
			{Name: "test", Type: MCPTypeStdio, Command: "cmd"},
		},
	}
	m := NewMCPManager(cfg, nil, logger)

	got := m.GetConfig()
	if !got.Enabled {
		t.Error("expected config to be enabled")
	}
	if len(got.Servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(got.Servers))
	}
}
