package plugins

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// mockToolRegistrar records tool registrations for testing.
type mockToolRegistrar struct {
	registered   map[string]ToolRegistration
	unregistered []string
}

func newMockToolRegistrar() *mockToolRegistrar {
	return &mockToolRegistrar{registered: make(map[string]ToolRegistration)}
}

func (m *mockToolRegistrar) RegisterPluginTool(reg ToolRegistration) {
	m.registered[reg.Name] = reg
}

func (m *mockToolRegistrar) UnregisterTool(name string) bool {
	m.unregistered = append(m.unregistered, name)
	_, found := m.registered[name]
	delete(m.registered, name)
	return found
}

func TestRegistry_RegisterAll(t *testing.T) {
	dir := t.TempDir()

	// Create a plugin instance with tools.
	inst := &PluginInstance{
		Manifest: &PluginManifest{
			ID:      "test",
			Name:    "Test",
			Version: "1.0.0",
			Tools: []ToolDef{
				{
					Name:        "greet",
					Description: "Greet someone",
					Script:      "echo hello",
				},
			},
		},
		Dir:     dir,
		State:   StateLoaded,
		Enabled: true,
		Config:  map[string]any{},
	}

	registry := NewRegistry(slog.Default())
	mock := newMockToolRegistrar()
	registry.SetToolRegistrar(mock)

	registry.mu.Lock()
	registry.plugins["test"] = inst
	registry.mu.Unlock()

	if err := registry.RegisterAll(); err != nil {
		t.Fatalf("RegisterAll failed: %v", err)
	}

	// Check tool was registered with namespaced name.
	if _, ok := mock.registered["test_greet"]; !ok {
		t.Error("expected tool test_greet to be registered")
	}

	if inst.State != StateRegistered {
		t.Errorf("State = %q, want %q", inst.State, StateRegistered)
	}

	if len(inst.RegisteredTools) != 1 || inst.RegisteredTools[0] != "test_greet" {
		t.Errorf("RegisteredTools = %v, want [test_greet]", inst.RegisteredTools)
	}
}

func TestRegistry_StartStopAll(t *testing.T) {
	inst := &PluginInstance{
		Manifest: &PluginManifest{ID: "test", Name: "Test", Version: "1.0.0"},
		State:    StateRegistered,
		Enabled:  true,
		Config:   map[string]any{},
		RegisteredTools: []string{"test_tool"},
	}

	registry := NewRegistry(slog.Default())
	mock := newMockToolRegistrar()
	registry.SetToolRegistrar(mock)
	mock.registered["test_tool"] = ToolRegistration{Name: "test_tool"}

	registry.mu.Lock()
	registry.plugins["test"] = inst
	registry.mu.Unlock()

	if err := registry.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if inst.State != StateStarted {
		t.Errorf("State after start = %q, want %q", inst.State, StateStarted)
	}

	registry.StopAll()
	if inst.State != StateStopped {
		t.Errorf("State after stop = %q, want %q", inst.State, StateStopped)
	}
	if len(mock.unregistered) != 1 || mock.unregistered[0] != "test_tool" {
		t.Errorf("expected tool test_tool to be unregistered, got %v", mock.unregistered)
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry(slog.Default())

	if registry.HasPlugins() {
		t.Error("empty registry should not have plugins")
	}

	registry.mu.Lock()
	registry.plugins["a"] = &PluginInstance{
		Manifest: &PluginManifest{ID: "a", Name: "A", Version: "1.0.0"},
		State:    StateLoaded,
		Enabled:  true,
	}
	registry.mu.Unlock()

	if !registry.HasPlugins() {
		t.Error("registry should have plugins")
	}

	infos := registry.List()
	if len(infos) != 1 {
		t.Fatalf("List = %d, want 1", len(infos))
	}
	if infos[0].ID != "a" {
		t.Errorf("ID = %q, want %q", infos[0].ID, "a")
	}
}

func TestRegistry_EnableDisable(t *testing.T) {
	registry := NewRegistry(slog.Default())
	registry.mu.Lock()
	registry.plugins["test"] = &PluginInstance{
		Manifest: &PluginManifest{ID: "test"},
		Enabled:  true,
	}
	registry.mu.Unlock()

	if err := registry.Disable("test"); err != nil {
		t.Fatal(err)
	}
	if registry.Get("test").Enabled {
		t.Error("should be disabled")
	}

	if err := registry.Enable("test"); err != nil {
		t.Fatal(err)
	}
	if !registry.Get("test").Enabled {
		t.Error("should be enabled")
	}

	if err := registry.Enable("nonexistent"); err == nil {
		t.Error("expected error for nonexistent plugin")
	}
}

func TestRegistry_MatchTrigger(t *testing.T) {
	registry := NewRegistry(slog.Default())

	// Add a plugin with an agent that has triggers.
	registry.mu.Lock()
	registry.plugins["hello"] = &PluginInstance{
		Manifest: &PluginManifest{
			ID: "hello",
			Agents: []AgentDef{
				{
					ID:       "greeter",
					Triggers: []string{"hello", "hi"},
				},
			},
		},
		State:   StateRegistered,
		Enabled: true,
	}
	registry.agentIndex["hello/greeter"] = &resolvedAgent{
		AgentDef: AgentDef{
			ID:       "greeter",
			Triggers: []string{"hello", "hi"},
		},
		pluginID: "hello",
	}
	registry.mu.Unlock()

	// Match.
	match := registry.MatchTrigger("hello there!", "webui")
	if match == nil {
		t.Fatal("expected trigger match")
	}
	if match.PluginID != "hello" || match.AgentID != "greeter" {
		t.Errorf("match = %+v, want hello/greeter", match)
	}

	// No match.
	match = registry.MatchTrigger("goodbye", "webui")
	if match != nil {
		t.Errorf("expected no match, got %+v", match)
	}
}

func TestRegistry_MatchTrigger_ChannelFilter(t *testing.T) {
	registry := NewRegistry(slog.Default())

	registry.mu.Lock()
	registry.agentIndex["test/agent"] = &resolvedAgent{
		AgentDef: AgentDef{
			ID:       "agent",
			Triggers: []string{"test"},
			Channels: []string{"telegram"},
		},
		pluginID: "test",
	}
	registry.mu.Unlock()

	// Should match on telegram.
	match := registry.MatchTrigger("test message", "telegram")
	if match == nil {
		t.Error("expected match on telegram")
	}

	// Should not match on webui (channel filter).
	match = registry.MatchTrigger("test message", "webui")
	if match != nil {
		t.Error("expected no match on webui due to channel filter")
	}
}

func TestResolveConfig(t *testing.T) {
	schema := &PluginConfigSchema{
		Fields: []PluginConfigField{
			{Key: "name", Type: "string", Default: "world"},
			{Key: "required_field", Type: "string", Required: true},
		},
	}

	// Missing required field.
	_, err := ResolveConfig(schema, map[string]any{}, nil)
	if err == nil {
		t.Error("expected error for missing required field")
	}

	// With override.
	resolved, err := ResolveConfig(schema, map[string]any{
		"required_field": "provided",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["name"] != "world" {
		t.Errorf("name = %v, want world", resolved["name"])
	}
	if resolved["required_field"] != "provided" {
		t.Errorf("required_field = %v, want provided", resolved["required_field"])
	}
}

func TestRegistry_ToolHandlerExecution(t *testing.T) {
	dir := t.TempDir()

	inst := &PluginInstance{
		Manifest: &PluginManifest{
			ID: "exec-test",
			Tools: []ToolDef{
				{
					Name:        "echo_tool",
					Description: "Echo a value",
					Script:      `echo "result: ${PLUGIN_VALUE}"`,
				},
			},
		},
		Dir:     dir,
		State:   StateLoaded,
		Enabled: true,
		Config:  map[string]any{},
	}

	registry := NewRegistry(slog.Default())
	mock := newMockToolRegistrar()
	registry.SetToolRegistrar(mock)

	registry.mu.Lock()
	registry.plugins["exec-test"] = inst
	registry.mu.Unlock()

	if err := registry.RegisterAll(); err != nil {
		t.Fatal(err)
	}

	// Get the registered handler and execute it.
	reg, ok := mock.registered["exec-test_echo_tool"]
	if !ok {
		t.Fatal("tool not registered")
	}

	result, err := reg.Handler(context.Background(), map[string]any{"value": "test123"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	resultStr, _ := result.(string)
	if resultStr != "result: test123" {
		t.Errorf("result = %q, want %q", resultStr, "result: test123")
	}
}

func TestRegistry_ScriptHandlerReceivesConfig(t *testing.T) {
	dir := t.TempDir()

	inst := &PluginInstance{
		Manifest: &PluginManifest{
			ID: "cfg-test",
			Tools: []ToolDef{
				{
					Name:        "check_cfg",
					Description: "Check config env vars",
					Script:      `echo "stage=${PLUGIN_CFG_STAGE_PLANNING} max=${PLUGIN_CFG_MAX_FILE_CHANGES}"`,
				},
			},
		},
		Dir:     dir,
		State:   StateLoaded,
		Enabled: true,
		Config: map[string]any{
			"stage_planning":   true,
			"max_file_changes": 20,
		},
	}

	registry := NewRegistry(slog.Default())
	mock := newMockToolRegistrar()
	registry.SetToolRegistrar(mock)

	registry.mu.Lock()
	registry.plugins["cfg-test"] = inst
	registry.mu.Unlock()

	if err := registry.RegisterAll(); err != nil {
		t.Fatal(err)
	}

	reg, ok := mock.registered["cfg-test_check_cfg"]
	if !ok {
		t.Fatal("tool not registered")
	}

	result, err := reg.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	resultStr, _ := result.(string)
	if resultStr != "stage=true max=20" {
		t.Errorf("result = %q, want %q", resultStr, "stage=true max=20")
	}
}

func TestPluginInfo_JSON(t *testing.T) {
	inst := &PluginInstance{
		Manifest: &PluginManifest{
			ID:          "json-test",
			Name:        "JSON Test",
			Version:     "1.0.0",
			Description: "Test JSON serialization",
		},
		State:           StateRegistered,
		Enabled:         true,
		RegisteredTools: []string{"tool1"},
	}

	info := inst.Info()
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}

	var parsed PluginInfo
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed.ID != "json-test" {
		t.Errorf("ID = %q, want %q", parsed.ID, "json-test")
	}
	if len(parsed.Tools) != 1 {
		t.Errorf("Tools = %d, want 1", len(parsed.Tools))
	}
}

func TestEndToEnd_DiscoverLoadRegister(t *testing.T) {
	dir := t.TempDir()

	// Create a plugin directory.
	pluginDir := filepath.Join(dir, "e2e-plugin")
	if err := os.Mkdir(pluginDir, 0700); err != nil {
		t.Fatal(err)
	}

	yamlContent := `id: e2e-plugin
name: E2E Plugin
version: 1.0.0
tools:
  - name: ping
    description: Ping
    script: echo pong
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(yamlContent), 0600); err != nil {
		t.Fatal(err)
	}

	// 1. Load
	loader := NewLoader(PluginsConfig{Dirs: []string{dir}}, slog.Default())
	if err := loader.LoadAll(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if loader.Count() != 1 {
		t.Fatalf("Count = %d, want 1", loader.Count())
	}

	// 2. Registry
	registry := NewRegistry(slog.Default())
	mock := newMockToolRegistrar()
	registry.SetToolRegistrar(mock)
	registry.AddLoadedPlugins(loader)

	// 3. RegisterAll
	if err := registry.RegisterAll(); err != nil {
		t.Fatal(err)
	}

	// 4. Verify
	if !registry.HasPlugins() {
		t.Error("registry should have plugins")
	}
	if _, ok := mock.registered["e2e-plugin_ping"]; !ok {
		t.Error("tool e2e-plugin_ping should be registered")
	}

	inst := registry.Get("e2e-plugin")
	if inst.State != StateRegistered {
		t.Errorf("State = %q, want %q", inst.State, StateRegistered)
	}

	// 5. Start
	if err := registry.StartAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if inst.State != StateStarted {
		t.Errorf("State = %q, want %q", inst.State, StateStarted)
	}

	// 6. Stop
	registry.StopAll()
	if inst.State != StateStopped {
		t.Errorf("State = %q, want %q", inst.State, StateStopped)
	}
}
