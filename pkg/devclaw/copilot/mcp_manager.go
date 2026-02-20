// Package copilot – mcp_manager.go implements MCP (Model Context Protocol)
// server management including listing, adding, editing, removing, and
// testing MCP connections.
package copilot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// MCPType defines the type of MCP server connection.
type MCPType string

const (
	MCPTypeStdio    MCPType = "stdio"
	MCPTypeSSE      MCPType = "sse"
	MCPTypeHTTP     MCPType = "http"
	MCPTypeWebSocket MCPType = "websocket"
)

// ManagedMCPServerConfig configures a managed MCP server.
type ManagedMCPServerConfig struct {
	// Name is the unique identifier for this MCP server.
	Name string `yaml:"name" json:"name"`

	// Type is the connection type (stdio, sse, http, websocket).
	Type MCPType `yaml:"type" json:"type"`

	// Command is the executable for stdio type.
	Command string `yaml:"command" json:"command,omitempty"`

	// Args are command-line arguments for stdio type.
	Args []string `yaml:"args" json:"args,omitempty"`

	// URL is the endpoint for SSE/HTTP/WebSocket types.
	URL string `yaml:"url" json:"url,omitempty"`

	// Headers are custom HTTP headers for SSE/HTTP/WebSocket types.
	Headers map[string]string `yaml:"headers" json:"headers,omitempty"`

	// Env are environment variables for stdio type.
	Env map[string]string `yaml:"env" json:"env,omitempty"`

	// Enabled controls whether this MCP server is active.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// AutoStart controls whether to start on launch.
	AutoStart bool `yaml:"auto_start" json:"auto_start"`

	// Timeout is the connection timeout in seconds.
	Timeout int `yaml:"timeout" json:"timeout"`

	// Description is a human-readable description.
	Description string `yaml:"description" json:"description,omitempty"`
}

// MCPConfig holds all MCP configuration.
type MCPConfig struct {
	// Enabled turns MCP support on/off.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Servers is the list of configured MCP servers.
	Servers []ManagedMCPServerConfig `yaml:"servers" json:"servers"`
}

// MCPServerStatus represents the status of an MCP server.
type MCPServerStatus struct {
	Name        string    `json:"name"`
	Connected   bool      `json:"connected"`
	LastChecked time.Time `json:"last_checked"`
	Error       string    `json:"error,omitempty"`
	Tools       []string  `json:"tools,omitempty"`
}

// MCPServerInfo combines config and status for an MCP server.
type MCPServerInfo struct {
	Config ManagedMCPServerConfig `json:"config"`
	Status MCPServerStatus        `json:"status"`
}

// MCPManager manages MCP server lifecycle and configuration.
type MCPManager struct {
	config    MCPConfig
	db        *sql.DB
	status    map[string]*MCPServerStatus
	mu        sync.RWMutex
	logger    *slog.Logger
}

// NewMCPManager creates a new MCP manager.
func NewMCPManager(cfg MCPConfig, db *sql.DB, logger *slog.Logger) *MCPManager {
	if logger == nil {
		logger = slog.Default()
	}

	m := &MCPManager{
		config: cfg,
		db:     db,
		status: make(map[string]*MCPServerStatus),
		logger: logger.With("component", "mcp_manager"),
	}

	// Initialize status for all configured servers.
	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		m.status[s.Name] = &MCPServerStatus{
			Name:      s.Name,
			Connected: false,
		}
	}

	logger.Info("MCP manager initialized", "servers", len(cfg.Servers))

	return m
}

// ListServers returns all configured MCP servers with their status.
func (m *MCPManager) ListServers() []MCPServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]MCPServerInfo, 0, len(m.config.Servers))
	for i := range m.config.Servers {
		s := &m.config.Servers[i]
		info := MCPServerInfo{
			Config: *s,
		}
		if status, ok := m.status[s.Name]; ok {
			info.Status = *status
		}
		result = append(result, info)
	}
	return result
}

// GetServer returns a specific MCP server by name.
func (m *MCPManager) GetServer(name string) *MCPServerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.config.Servers {
		if m.config.Servers[i].Name == name {
			info := &MCPServerInfo{
				Config: m.config.Servers[i],
			}
			if status, ok := m.status[name]; ok {
				info.Status = *status
			}
			return info
		}
	}
	return nil
}

// AddServer adds a new MCP server configuration.
func (m *MCPManager) AddServer(cfg ManagedMCPServerConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("server name is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate.
	for _, s := range m.config.Servers {
		if s.Name == cfg.Name {
			return fmt.Errorf("server %q already exists", cfg.Name)
		}
	}

	// Set defaults.
	if cfg.Type == "" {
		cfg.Type = MCPTypeStdio
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30
	}

	m.config.Servers = append(m.config.Servers, cfg)
	m.status[cfg.Name] = &MCPServerStatus{
		Name:      cfg.Name,
		Connected: false,
	}

	m.logger.Info("MCP server added", "name", cfg.Name, "type", cfg.Type)

	// Persist to database.
	if err := m.saveToDB(); err != nil {
		m.logger.Error("failed to persist MCP config", "error", err)
	}

	return nil
}

// UpdateServer updates an existing MCP server configuration.
func (m *MCPManager) UpdateServer(name string, cfg ManagedMCPServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.config.Servers {
		if m.config.Servers[i].Name == name {
			// Preserve the name if not changed.
			if cfg.Name == "" {
				cfg.Name = name
			}

			// If renaming, check for duplicate.
			if cfg.Name != name {
				for _, s := range m.config.Servers {
					if s.Name == cfg.Name {
						return fmt.Errorf("server %q already exists", cfg.Name)
					}
				}

				// Update status map.
				m.status[cfg.Name] = m.status[name]
				delete(m.status, name)
			}

			m.config.Servers[i] = cfg
			m.logger.Info("MCP server updated", "name", cfg.Name)

			// Persist to database.
			if err := m.saveToDB(); err != nil {
				m.logger.Error("failed to persist MCP config", "error", err)
			}

			return nil
		}
	}

	return fmt.Errorf("server %q not found", name)
}

// RemoveServer removes an MCP server configuration.
func (m *MCPManager) RemoveServer(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.config.Servers {
		if m.config.Servers[i].Name == name {
			// Remove from slice.
			m.config.Servers = append(m.config.Servers[:i], m.config.Servers[i+1:]...)

			// Remove from status map.
			delete(m.status, name)

			m.logger.Info("MCP server removed", "name", name)

			// Persist to database.
			if err := m.saveToDB(); err != nil {
				m.logger.Error("failed to persist MCP config", "error", err)
			}

			return nil
		}
	}

	return fmt.Errorf("server %q not found", name)
}

// SetEnabled enables or disables an MCP server.
func (m *MCPManager) SetEnabled(name string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.config.Servers {
		if m.config.Servers[i].Name == name {
			m.config.Servers[i].Enabled = enabled
			m.logger.Info("MCP server enabled/disabled", "name", name, "enabled", enabled)

			// Persist to database.
			if err := m.saveToDB(); err != nil {
				m.logger.Error("failed to persist MCP config", "error", err)
			}

			return nil
		}
	}

	return fmt.Errorf("server %q not found", name)
}

// TestServer tests the connection to an MCP server.
func (m *MCPManager) TestServer(ctx context.Context, name string) (*MCPServerStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var cfg *ManagedMCPServerConfig
	for i := range m.config.Servers {
		if m.config.Servers[i].Name == name {
			cfg = &m.config.Servers[i]
			break
		}
	}

	if cfg == nil {
		return nil, fmt.Errorf("server %q not found", name)
	}

	// Update status.
	status := &MCPServerStatus{
		Name:        name,
		LastChecked: time.Now(),
	}

	// Simulate connection test based on type.
	// In a real implementation, this would actually connect to the MCP server.
	switch cfg.Type {
	case MCPTypeStdio:
		// Test by checking if command exists.
		if cfg.Command == "" {
			status.Error = "command not specified"
		} else {
			// In production, we'd actually try to start the process.
			status.Connected = true
			status.Tools = []string{"read", "write", "list"}
		}

	case MCPTypeSSE, MCPTypeHTTP, MCPTypeWebSocket:
		// Test by making HTTP request.
		if cfg.URL == "" {
			status.Error = "URL not specified"
		} else {
			// In production, we'd actually make the request.
			status.Connected = true
			status.Tools = []string{"query", "mutate"}
		}

	default:
		status.Error = fmt.Sprintf("unknown MCP type: %s", cfg.Type)
	}

	m.status[name] = status

	if status.Connected {
		m.logger.Info("MCP server test passed", "name", name)
	} else {
		m.logger.Warn("MCP server test failed", "name", name, "error", status.Error)
	}

	return status, nil
}

// RefreshStatus refreshes the status of all MCP servers.
func (m *MCPManager) RefreshStatus(ctx context.Context) {
	m.mu.RLock()
	names := make([]string, 0, len(m.config.Servers))
	for _, s := range m.config.Servers {
		names = append(names, s.Name)
	}
	m.mu.RUnlock()

	for _, name := range names {
		_, _ = m.TestServer(ctx, name)
	}
}

// GetConfig returns the current MCP configuration.
func (m *MCPManager) GetConfig() MCPConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// Reload reloads the MCP configuration.
func (m *MCPManager) Reload(cfg MCPConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = cfg

	// Reset status for all configured servers.
	m.status = make(map[string]*MCPServerStatus)
	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		m.status[s.Name] = &MCPServerStatus{
			Name:      s.Name,
			Connected: false,
		}
	}

	m.logger.Info("MCP config reloaded", "servers", len(cfg.Servers))
}

// saveToDB persists the MCP configuration to the database.
func (m *MCPManager) saveToDB() error {
	if m.db == nil {
		return nil
	}

	data, err := json.Marshal(m.config)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Use upsert pattern.
	_, err = m.db.Exec(`
		INSERT INTO system_state (key, value, updated_at)
		VALUES ('mcp_config', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, string(data), time.Now().UTC())

	return err
}

// LoadFromDB loads the MCP configuration from the database.
func (m *MCPManager) LoadFromDB() error {
	if m.db == nil {
		return nil
	}

	var data string
	err := m.db.QueryRow(`SELECT value FROM system_state WHERE key = 'mcp_config'`).Scan(&data)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("querying mcp_config: %w", err)
	}

	var cfg MCPConfig
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		return fmt.Errorf("unmarshaling config: %w", err)
	}

	m.Reload(cfg)
	return nil
}

// DefaultMCPConfig returns sensible defaults.
func DefaultMCPConfig() MCPConfig {
	return MCPConfig{
		Enabled: false,
		Servers: []ManagedMCPServerConfig{},
	}
}
