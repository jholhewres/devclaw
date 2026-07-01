package copilot

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// mcp_agent.go exposes the `mcp` tool, letting the main agent configure, start,
// stop and manage external MCP (Model Context Protocol) servers at runtime —
// no manual config editing and no restart. It mirrors the security and
// persistence model of settings_agent.go: owner/admin only, changes persisted
// to config.yaml (secrets sanitized to ${ENV}) and applied live via the
// MCPToolsBridge.

// registerMCPManagementTool registers the `mcp` tool. The actual work is
// delegated to the handler wired by the Assistant via SetMCPHandler.
func registerMCPManagementTool(executor *ToolExecutor) {
	executor.Register(
		MakeToolDefinition("mcp",
			"Manage external MCP (Model Context Protocol) servers at runtime — configure, "+
				"start, stop, enable/disable and test them. Changes persist to config and apply "+
				"immediately without a restart; a connected server's tools appear as mcp_<server>_<tool>. "+
				"Remote http/sse servers requiring OAuth: use action=authorize to get a consent URL. "+
				"Owner/admin only. Actions: list, add, remove, enable, disable, start, stop, test, authorize.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"list", "add", "remove", "enable", "disable", "start", "stop", "test", "authorize"},
						"description": "operation to perform (authorize starts the OAuth consent flow for an http/sse server)",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "server name (required for everything except list)",
					},
					"type": map[string]any{
						"type":        "string",
						"enum":        []string{"stdio", "http", "sse"},
						"description": "transport: stdio (local process), or http/sse (remote URL). Default stdio.",
					},
					"command": map[string]any{
						"type":        "string",
						"description": "executable to launch for stdio servers (e.g. npx)",
					},
					"args": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "command arguments for stdio servers",
					},
					"url": map[string]any{
						"type":        "string",
						"description": "endpoint URL for http/sse servers",
					},
					"headers": map[string]any{
						"type":        "object",
						"description": "custom HTTP headers for http/sse servers. For secrets, pass ${ENV_VAR} references — never raw keys.",
					},
					"env": map[string]any{
						"type":        "object",
						"description": "environment variables for the server process. For secrets, pass ${ENV_VAR} references — never raw keys.",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "human-readable description",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "connection timeout in seconds (default 30)",
					},
					"start": map[string]any{
						"type":        "boolean",
						"description": "for action=add: connect immediately after adding (default true)",
					},
				},
				"required": []string{"action"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			if lvl := CallerLevelFromContext(ctx); lvl != AccessOwner && lvl != AccessAdmin {
				return nil, fmt.Errorf("permission denied: mcp requires owner or admin")
			}
			handler := executor.mcpHandlerFn()
			if handler == nil {
				return nil, fmt.Errorf("mcp management is not available")
			}
			action, _ := args["action"].(string)
			action = strings.TrimSpace(strings.ToLower(action))
			if action == "" {
				action = "list"
			}
			return handler(ctx, action, args)
		},
	)
}

// handleMCPTool dispatches an `mcp` tool action. Wired into the executor by the
// Assistant. All mutating actions persist to config.yaml and apply live.
func (a *Assistant) handleMCPTool(ctx context.Context, action string, args map[string]any) (string, error) {
	switch action {
	case "list":
		return a.mcpList()
	case "add":
		return a.mcpAdd(args)
	case "remove":
		return a.mcpRemove(mcpArgString(args, "name"))
	case "enable":
		return a.mcpSetEnabled(mcpArgString(args, "name"), true)
	case "disable":
		return a.mcpSetEnabled(mcpArgString(args, "name"), false)
	case "start":
		return a.StartMCPServer(mcpArgString(args, "name"))
	case "stop":
		return a.StopMCPServer(mcpArgString(args, "name"))
	case "test":
		return a.mcpTest(mcpArgString(args, "name"))
	case "authorize":
		return a.mcpAuthorize(mcpArgString(args, "name"))
	default:
		return "", fmt.Errorf("unknown action %q (use list, add, remove, enable, disable, start, stop, test, authorize)", action)
	}
}

// mcpList returns the configured MCP servers and their live connection status.
func (a *Assistant) mcpList() (string, error) {
	a.configMu.RLock()
	servers := append([]ManagedMCPServerConfig(nil), a.config.MCP.Servers...)
	enabled := a.config.MCP.Enabled
	a.configMu.RUnlock()

	var b strings.Builder
	fmt.Fprintf(&b, "MCP subsystem enabled: %t\n", enabled)
	if len(servers) == 0 {
		b.WriteString("No MCP servers configured. Use action=add to create one.")
		return b.String(), nil
	}
	for _, s := range servers {
		connected := a.mcpBridge != nil && a.mcpBridge.IsConnected(s.Name)
		fmt.Fprintf(&b, "\n- %s (type=%s, enabled=%t, auto_start=%t) — %s",
			s.Name, mcpTypeOrDefault(s.Type), s.Enabled, s.AutoStart, mcpConnLabel(connected))
		if s.Command != "" {
			fmt.Fprintf(&b, "\n    command: %s %s", s.Command, strings.Join(s.Args, " "))
		}
		if connected {
			tools := a.mcpBridge.ServerTools(s.Name)
			fmt.Fprintf(&b, "\n    tools (%d): %s", len(tools), strings.Join(tools, ", "))
		}
	}
	return b.String(), nil
}

// mcpAdd validates, persists and (optionally) connects a new MCP server.
func (a *Assistant) mcpAdd(args map[string]any) (string, error) {
	srv := ManagedMCPServerConfig{
		Name:        mcpArgString(args, "name"),
		Type:        MCPType(mcpArgString(args, "type")),
		Command:     mcpArgString(args, "command"),
		Args:        mcpArgStringSlice(args, "args"),
		URL:         mcpArgString(args, "url"),
		Headers:     mcpArgStringMap(args, "headers"),
		Env:         mcpArgStringMap(args, "env"),
		Description: mcpArgString(args, "description"),
		Timeout:     mcpArgInt(args, "timeout"),
		Enabled:     true,
		AutoStart:   true,
	}
	if srv.Name == "" {
		return "", fmt.Errorf("name is required for action=add")
	}
	if srv.Type == "" {
		srv.Type = MCPTypeStdio
	}
	switch srv.Type {
	case MCPTypeStdio:
		if srv.Command == "" {
			return "", fmt.Errorf("command is required for stdio servers")
		}
	case MCPTypeHTTP, MCPTypeSSE:
		if srv.URL == "" {
			return "", fmt.Errorf("url is required for %s servers", srv.Type)
		}
	default:
		return "", fmt.Errorf("unsupported type %q (use stdio, http or sse)", srv.Type)
	}
	if srv.Timeout == 0 {
		srv.Timeout = 30
	}

	// Persist: load on-disk config, append (rejecting duplicates), flip the MCP
	// subsystem on, and save (secrets are sanitized to ${ENV} by SaveConfigToFile).
	if err := a.mutateMCPConfig(func(cfg *Config) error {
		for _, s := range cfg.MCP.Servers {
			if s.Name == srv.Name {
				return fmt.Errorf("server %q already exists", srv.Name)
			}
		}
		cfg.MCP.Enabled = true
		cfg.MCP.Servers = append(cfg.MCP.Servers, srv)
		return nil
	}); err != nil {
		return "", err
	}

	start := true
	if v, ok := args["start"].(bool); ok {
		start = v
	}
	msg := fmt.Sprintf("OK — MCP server %q added and persisted.", srv.Name)
	if start {
		if connMsg, err := a.StartMCPServer(srv.Name); err != nil {
			return msg + fmt.Sprintf(" Auto-start failed: %v", err), nil
		} else {
			msg += " " + connMsg
		}
	}
	return msg, nil
}

// mcpRemove disconnects (if connected) and removes an MCP server.
func (a *Assistant) mcpRemove(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required for action=remove")
	}
	if a.mcpBridge != nil && a.mcpBridge.IsConnected(name) {
		_ = a.mcpBridge.DisconnectOne(name)
	}
	if err := a.mutateMCPConfig(func(cfg *Config) error {
		found := false
		kept := cfg.MCP.Servers[:0]
		for _, s := range cfg.MCP.Servers {
			if s.Name == name {
				found = true
				continue
			}
			kept = append(kept, s)
		}
		if !found {
			return fmt.Errorf("server %q not found", name)
		}
		cfg.MCP.Servers = kept
		return nil
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("OK — MCP server %q removed and disconnected.", name), nil
}

// mcpSetEnabled toggles a server's enabled flag and connects/disconnects to match.
func (a *Assistant) mcpSetEnabled(name string, enabled bool) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if err := a.mutateMCPConfig(func(cfg *Config) error {
		for i := range cfg.MCP.Servers {
			if cfg.MCP.Servers[i].Name == name {
				cfg.MCP.Servers[i].Enabled = enabled
				return nil
			}
		}
		return fmt.Errorf("server %q not found", name)
	}); err != nil {
		return "", err
	}
	if enabled {
		return a.StartMCPServer(name)
	}
	// Disabling: only disconnect if currently running (no error otherwise).
	if a.mcpBridge != nil && a.mcpBridge.IsConnected(name) {
		return a.StopMCPServer(name)
	}
	return fmt.Sprintf("OK — MCP server %q disabled.", name), nil
}

// StartMCPServer connects a configured MCP server now and registers its tools.
// Exposed for the WebUI Start hook and the `mcp` tool.
func (a *Assistant) StartMCPServer(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if a.mcpBridge == nil {
		return "", fmt.Errorf("MCP bridge is not available")
	}
	a.configMu.RLock()
	var srv *ManagedMCPServerConfig
	for i := range a.config.MCP.Servers {
		if a.config.MCP.Servers[i].Name == name {
			s := a.config.MCP.Servers[i]
			srv = &s
			break
		}
	}
	path := a.configPath
	a.configMu.RUnlock()

	// Fallback: a server added through another path (e.g. the WebUI, which
	// persists to disk without touching the live in-memory config) may not be
	// in a.config yet. Reload from the config file before giving up.
	if srv == nil && path != "" {
		if cfg, err := LoadConfigFromFile(path); err == nil {
			for i := range cfg.MCP.Servers {
				if cfg.MCP.Servers[i].Name == name {
					s := cfg.MCP.Servers[i]
					srv = &s
					a.configMu.Lock()
					a.config.MCP = cfg.MCP
					a.configMu.Unlock()
					break
				}
			}
		}
	}
	if srv == nil {
		return "", fmt.Errorf("server %q not found", name)
	}
	if err := a.mcpBridge.ConnectOne(a.mcpBridge.BaseContext(), *srv); err != nil {
		return "", fmt.Errorf("connect %q: %w", name, err)
	}
	tools := a.mcpBridge.ServerTools(name)
	return fmt.Sprintf("Connected %q — %d tool(s) registered: %s", name, len(tools), strings.Join(tools, ", ")), nil
}

// StopMCPServer disconnects a running MCP server and unregisters its tools.
// Exposed for the WebUI Stop hook and the `mcp` tool.
func (a *Assistant) StopMCPServer(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if a.mcpBridge == nil {
		return "", fmt.Errorf("MCP bridge is not available")
	}
	if err := a.mcpBridge.DisconnectOne(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Disconnected %q — tools unregistered.", name), nil
}

// mcpAuthorize starts an OAuth flow for a server and returns the consent URL the
// user must open. After approval the provider redirects to the local callback,
// the token is stored in the vault, and the server connects automatically.
func (a *Assistant) mcpAuthorize(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required for action=authorize")
	}
	if a.mcpOAuth == nil {
		return "", fmt.Errorf("OAuth is unavailable (vault not unlocked)")
	}
	a.configMu.RLock()
	var srv *ManagedMCPServerConfig
	for i := range a.config.MCP.Servers {
		if a.config.MCP.Servers[i].Name == name {
			s := a.config.MCP.Servers[i]
			srv = &s
			break
		}
	}
	a.configMu.RUnlock()
	if srv == nil {
		return "", fmt.Errorf("server %q not found", name)
	}

	// Mark the server as OAuth-enabled so future (re)connects attach the token.
	if !srv.OAuth {
		_ = a.mutateMCPConfig(func(cfg *Config) error {
			for i := range cfg.MCP.Servers {
				if cfg.MCP.Servers[i].Name == name {
					cfg.MCP.Servers[i].OAuth = true
				}
			}
			return nil
		})
	}

	authURL, err := a.mcpOAuth.BeginAuthorization(context.Background(), *srv)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("To authorize %q, open this URL in a browser and approve access:\n%s\n\nAfter approving you'll be redirected back and the server will connect automatically.", name, authURL), nil
}

// HandleMCPOAuthCallback completes an OAuth flow from the redirect callback.
// Exposed for the gateway/WebUI callback route.
func (a *Assistant) HandleMCPOAuthCallback(ctx context.Context, state, code string) (string, error) {
	if a.mcpOAuth == nil {
		return "", fmt.Errorf("OAuth is unavailable")
	}
	return a.mcpOAuth.HandleCallback(ctx, state, code)
}

// mcpOAuthRedirectURI derives the local OAuth callback URL from the WebUI
// address (default localhost). Configurable per deployment.
func (a *Assistant) mcpOAuthRedirectURI() string {
	addr := a.config.WebUI.Address
	if addr == "" {
		addr = ":8085"
	}
	// addr is typically ":8085"; build a localhost URL for dev/test.
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr + "/oauth/mcp/callback"
	}
	return "http://" + addr + "/oauth/mcp/callback"
}

// mcpTest connects to a server (if not already), reports the discovered tools,
// and disconnects again if it was not previously connected. Non-destructive.
func (a *Assistant) mcpTest(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name is required for action=test")
	}
	if a.mcpBridge == nil {
		return "", fmt.Errorf("MCP bridge is not available")
	}
	wasConnected := a.mcpBridge.IsConnected(name)
	if !wasConnected {
		if _, err := a.StartMCPServer(name); err != nil {
			return "", fmt.Errorf("test failed: %w", err)
		}
	}
	tools := a.mcpBridge.ServerTools(name)
	if !wasConnected {
		_ = a.mcpBridge.DisconnectOne(name)
	}
	return fmt.Sprintf("OK — %q reachable, %d tool(s): %s", name, len(tools), strings.Join(tools, ", ")), nil
}

// mutateMCPConfig loads config.yaml, applies mutate to it, persists the result,
// and mirrors the new MCP config into the live in-memory config so listings and
// a subsequent restart stay consistent. Mirrors settings_agent's persistence.
func (a *Assistant) mutateMCPConfig(mutate func(cfg *Config) error) error {
	a.configMu.RLock()
	path := a.configPath
	a.configMu.RUnlock()
	if path == "" {
		return fmt.Errorf("no config file is in use, cannot persist MCP changes")
	}

	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if err := mutate(cfg); err != nil {
		return err
	}
	if err := SaveConfigToFile(cfg, path); err != nil {
		return fmt.Errorf("persisting config: %w", err)
	}

	a.configMu.Lock()
	a.config.MCP = cfg.MCP
	a.configMu.Unlock()

	a.logger.Info("mcp config changed by agent", "servers", len(cfg.MCP.Servers))
	return nil
}

// ---------------------------------------------------------------------------
// arg coercion helpers (LLM tool args arrive as map[string]any)
// ---------------------------------------------------------------------------

func mcpArgString(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return strings.TrimSpace(s)
}

func mcpArgInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func mcpArgStringSlice(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mcpArgStringMap(args map[string]any, key string) map[string]string {
	raw, ok := args[key].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func mcpTypeOrDefault(t MCPType) MCPType {
	if t == "" {
		return MCPTypeStdio
	}
	return t
}

func mcpConnLabel(connected bool) string {
	if connected {
		return "CONNECTED"
	}
	return "stopped"
}

// allowedMCPActions documents the supported actions (used in error messages and tests).
var allowedMCPActions = func() []string {
	a := []string{"list", "add", "remove", "enable", "disable", "start", "stop", "test", "authorize"}
	sort.Strings(a)
	return a
}()
