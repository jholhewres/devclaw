package plugins

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestLoader_Discover(t *testing.T) {
	dir := t.TempDir()

	// Create a plugin directory with plugin.yaml.
	pluginDir := filepath.Join(dir, "test-plugin")
	if err := os.Mkdir(pluginDir, 0700); err != nil {
		t.Fatal(err)
	}
	yamlContent := `id: test-plugin
name: Test Plugin
version: 1.0.0
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(yamlContent), 0600); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(PluginsConfig{Dirs: []string{dir}}, slog.Default())
	discovered, err := loader.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(discovered) != 1 {
		t.Fatalf("discovered = %d, want 1", len(discovered))
	}
	if discovered[0].Manifest.ID != "test-plugin" {
		t.Errorf("ID = %q, want %q", discovered[0].Manifest.ID, "test-plugin")
	}
	if discovered[0].State != StateDiscovered {
		t.Errorf("State = %q, want %q", discovered[0].State, StateDiscovered)
	}
}

func TestLoader_Discover_Disabled(t *testing.T) {
	dir := t.TempDir()

	pluginDir := filepath.Join(dir, "disabled-plugin")
	if err := os.Mkdir(pluginDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte("id: disabled-plugin\n"), 0600); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(PluginsConfig{
		Dirs:     []string{dir},
		Disabled: []string{"disabled-plugin"},
	}, slog.Default())

	discovered, err := loader.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 0 {
		t.Errorf("disabled plugin should not be discovered, got %d", len(discovered))
	}
}

func TestLoader_LoadAll(t *testing.T) {
	dir := t.TempDir()

	pluginDir := filepath.Join(dir, "loaded-plugin")
	if err := os.Mkdir(pluginDir, 0700); err != nil {
		t.Fatal(err)
	}
	yamlContent := `id: loaded-plugin
name: Loaded Plugin
version: 1.0.0
config:
  fields:
    - key: greeting
      name: Greeting
      type: string
      default: hello
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(yamlContent), 0600); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(PluginsConfig{Dirs: []string{dir}}, slog.Default())
	if err := loader.LoadAll(context.Background(), nil); err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if loader.Count() != 1 {
		t.Fatalf("Count = %d, want 1", loader.Count())
	}

	inst := loader.Get("loaded-plugin")
	if inst == nil {
		t.Fatal("expected instance for loaded-plugin")
	}
	if inst.State != StateLoaded {
		t.Errorf("State = %q, want %q", inst.State, StateLoaded)
	}
	if inst.Config["greeting"] != "hello" {
		t.Errorf("Config[greeting] = %v, want %q", inst.Config["greeting"], "hello")
	}
}

func TestLoader_EmptyConfig(t *testing.T) {
	loader := NewLoader(PluginsConfig{}, slog.Default())
	if err := loader.LoadAll(context.Background(), nil); err != nil {
		t.Fatalf("LoadAll with empty config should not fail: %v", err)
	}
	if loader.Count() != 0 {
		t.Errorf("expected 0 plugins, got %d", loader.Count())
	}
}

func TestPluginsConfig_EffectiveDirs(t *testing.T) {
	cfg := PluginsConfig{
		Dirs: []string{"/a", "/b"},
		Dir:  "/c",
	}
	dirs := cfg.EffectiveDirs()
	if len(dirs) != 3 {
		t.Fatalf("EffectiveDirs = %d, want 3", len(dirs))
	}

	// Deduplication.
	cfg = PluginsConfig{
		Dirs: []string{"/a", "/b"},
		Dir:  "/a",
	}
	dirs = cfg.EffectiveDirs()
	if len(dirs) != 2 {
		t.Errorf("EffectiveDirs should deduplicate, got %d", len(dirs))
	}
}
