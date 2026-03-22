// Package plugins implements the unified YAML-first plugin system for DevClaw.
// Plugins extend the runtime with agents, tools, hooks, services, channels,
// and skills — all declared in a plugin.yaml manifest.
//
// Legacy .so plugins (without plugin.yaml) are supported via synthetic
// PluginInstance wrappers for backward compatibility.
package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"plugin"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

// Plugin is the interface for generic DevClaw native plugins (.so).
// Channel plugins can also implement this for lifecycle hooks,
// but it's not required — exporting var Channel is enough.
type Plugin interface {
	Name() string
	Version() string
	Init(ctx context.Context, config map[string]any) error
	Shutdown() error
}

// Loader discovers, loads, and manages plugin instances.
type Loader struct {
	cfg       PluginsConfig
	logger    *slog.Logger
	instances map[string]*PluginInstance
	mu        sync.RWMutex
}

// NewLoader creates a new plugin loader.
func NewLoader(cfg PluginsConfig, logger *slog.Logger) *Loader {
	if logger == nil {
		logger = slog.Default()
	}
	return &Loader{
		cfg:       cfg,
		logger:    logger.With("component", "plugins"),
		instances: make(map[string]*PluginInstance),
	}
}

// Discover scans all configured directories for plugin.yaml files
// and returns discovered PluginInstance entries (state = Discovered).
func (l *Loader) Discover() ([]*PluginInstance, error) {
	var discovered []*PluginInstance

	disabled := make(map[string]bool, len(l.cfg.Disabled))
	for _, d := range l.cfg.Disabled {
		disabled[d] = true
	}
	enabled := make(map[string]bool, len(l.cfg.Enabled))
	for _, e := range l.cfg.Enabled {
		enabled[e] = true
	}

	for _, dir := range l.cfg.EffectiveDirs() {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			l.logger.Debug("plugins: directory does not exist", "dir", dir)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("stat plugins dir %s: %w", dir, err)
		}
		if !info.IsDir() {
			l.logger.Warn("plugins: path is not a directory", "path", dir)
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("reading plugins dir %s: %w", dir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			manifestPath := filepath.Join(dir, entry.Name(), "plugin.yaml")
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				continue
			}

			manifest, err := ParseManifest(manifestPath)
			if err != nil {
				l.logger.Error("plugins: invalid manifest",
					"path", manifestPath, "error", err)
				continue
			}

			// Check enabled/disabled lists.
			if disabled[manifest.ID] {
				l.logger.Debug("plugins: skipping disabled plugin", "id", manifest.ID)
				continue
			}
			if len(enabled) > 0 && !enabled[manifest.ID] {
				l.logger.Debug("plugins: not in enabled list", "id", manifest.ID)
				continue
			}

			inst := &PluginInstance{
				Manifest: manifest,
				Dir:      filepath.Join(dir, entry.Name()),
				State:    StateDiscovered,
				Enabled:  true,
			}
			discovered = append(discovered, inst)

			l.logger.Info("plugins: discovered",
				"id", manifest.ID, "name", manifest.Name, "dir", inst.Dir)
		}

		// Also scan for legacy .so files in this directory.
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".so") {
				continue
			}
			soName := strings.TrimSuffix(entry.Name(), ".so")
			if disabled[soName] {
				continue
			}
			if len(enabled) > 0 && !enabled[soName] {
				continue
			}

			soPath := filepath.Join(dir, entry.Name())
			if trusted, reason := isTrustedPlugin(soPath, dir); !trusted {
				l.logger.Warn("plugins: rejecting untrusted .so",
					"path", soPath, "reason", reason)
				continue
			}

			// Create a synthetic manifest for the legacy .so.
			inst := &PluginInstance{
				Manifest: &PluginManifest{
					ID:        soName,
					Name:      soName,
					Version:   "0.0.0",
					NativeLib: entry.Name(),
				},
				Dir:     dir,
				State:   StateDiscovered,
				Enabled: true,
			}
			discovered = append(discovered, inst)

			l.logger.Info("plugins: discovered legacy .so", "name", soName, "path", soPath)
		}
	}

	return discovered, nil
}

// LoadAll discovers and loads all plugins. Config is resolved using the
// vault and any overrides from cfg.Overrides.
func (l *Loader) LoadAll(ctx context.Context, vault VaultReader) error {
	discovered, err := l.Discover()
	if err != nil {
		return fmt.Errorf("discovery: %w", err)
	}

	for _, inst := range discovered {
		id := inst.Manifest.ID

		// Check requirements.
		if inst.Manifest.Requires != nil && !inst.Manifest.Requires.IsEligible() {
			l.logger.Warn("plugins: requirements not met, skipping",
				"id", id)
			inst.State = StateError
			inst.ErrorMsg = "requirements not met"
			l.storeInstance(id, inst)
			continue
		}

		// Resolve config.
		overrides := l.cfg.Overrides[id]
		if overrides == nil {
			overrides = make(map[string]any)
		}
		resolved, err := ResolveConfig(inst.Manifest.Config, overrides, vault)
		if err != nil {
			l.logger.Error("plugins: config resolution failed",
				"id", id, "error", err)
			inst.State = StateError
			inst.ErrorMsg = fmt.Sprintf("config: %v", err)
			l.storeInstance(id, inst)
			continue
		}
		if err := ValidateConfig(inst.Manifest.Config, resolved); err != nil {
			l.logger.Error("plugins: config validation failed",
				"id", id, "error", err)
			inst.State = StateError
			inst.ErrorMsg = fmt.Sprintf("config validation: %v", err)
			l.storeInstance(id, inst)
			continue
		}
		inst.Config = resolved

		// Load native library if specified.
		if inst.Manifest.NativeLib != "" {
			if err := l.loadNativeLib(ctx, inst); err != nil {
				l.logger.Error("plugins: failed to load native lib",
					"id", id, "error", err)
				inst.State = StateError
				inst.ErrorMsg = fmt.Sprintf("native: %v", err)
				l.storeInstance(id, inst)
				continue
			}
		}

		provider := NewManifestProvider(inst.Manifest, inst.Dir)
		// Wire native lifecycle hooks if .so loaded.
		if inst.nativeHandle != nil && inst.nativeHandle.plugin != nil {
			np := inst.nativeHandle.plugin
			provider.SetInitFunc(np.Init)
			provider.SetShutdownFunc(np.Shutdown)
		}
		inst.Provider = provider
		inst.State = StateLoaded
		inst.LoadedAt = time.Now()

		l.storeInstance(id, inst)

		l.logger.Info("plugins: loaded",
			"id", id, "name", inst.Manifest.Name, "state", inst.State)
	}

	l.logger.Info("plugins: loading complete", "total", l.Count())
	return nil
}

// loadNativeLib opens a .so file and extracts Channel and/or Plugin symbols.
func (l *Loader) loadNativeLib(ctx context.Context, inst *PluginInstance) error {
	soPath := filepath.Join(inst.Dir, inst.Manifest.NativeLib)

	p, err := plugin.Open(soPath)
	if err != nil {
		return fmt.Errorf("opening .so: %w", err)
	}

	native := &nativePlugin{raw: p}

	// Look up "Channel" symbol.
	if sym, err := p.Lookup("Channel"); err == nil {
		if ch, ok := sym.(*channels.Channel); ok && ch != nil {
			native.channel = *ch
		}
	}

	// Look up "Plugin" symbol.
	if sym, err := p.Lookup("Plugin"); err == nil {
		if pl, ok := sym.(*Plugin); ok && pl != nil {
			native.plugin = *pl
		}
	}

	// Legacy .so without plugin.yaml must export at least one symbol.
	if native.channel == nil && native.plugin == nil && inst.Manifest.NativeLib == inst.Manifest.ID+".so" {
		return fmt.Errorf("plugin exports neither Channel nor Plugin symbol")
	}

	// Initialize generic plugin if present.
	if native.plugin != nil {
		if err := native.plugin.Init(ctx, inst.Config); err != nil {
			return fmt.Errorf("initializing plugin: %w", err)
		}
	}

	inst.nativeHandle = native
	return nil
}

// storeInstance safely stores a plugin instance under the loader's lock.
func (l *Loader) storeInstance(id string, inst *PluginInstance) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.instances[id] = inst
}

// Get returns a plugin instance by ID.
func (l *Loader) Get(id string) *PluginInstance {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.instances[id]
}

// All returns all loaded plugin instances.
func (l *Loader) All() []*PluginInstance {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*PluginInstance, 0, len(l.instances))
	for _, inst := range l.instances {
		result = append(result, inst)
	}
	return result
}

// Count returns the number of loaded plugins.
func (l *Loader) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.instances)
}

// Shutdown gracefully stops all loaded plugins with native handlers.
func (l *Loader) Shutdown() {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, inst := range l.instances {
		if inst.nativeHandle != nil && inst.nativeHandle.plugin != nil {
			if err := inst.nativeHandle.plugin.Shutdown(); err != nil {
				l.logger.Error("plugins: shutdown error",
					"id", inst.Manifest.ID, "error", err)
			}
		}
	}
}

// Channels returns all loaded native channel implementations.
func (l *Loader) Channels() []channels.Channel {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var chs []channels.Channel
	for _, inst := range l.instances {
		if inst.nativeHandle != nil {
			if ch, ok := inst.nativeHandle.channel.(channels.Channel); ok {
				chs = append(chs, ch)
			}
		}
	}
	return chs
}

// RegisterChannels registers all loaded native channel plugins with a Manager.
func (l *Loader) RegisterChannels(mgr *channels.Manager) error {
	for _, ch := range l.Channels() {
		if err := mgr.Register(ch); err != nil {
			l.logger.Error("plugins: failed to register channel",
				"channel", ch.Name(), "error", err)
			return err
		}
	}
	return nil
}

// isTrustedPlugin checks whether a plugin .so file is safe to load.
// Returns (true, "") if trusted, or (false, reason) if not.
func isTrustedPlugin(pluginPath, pluginDir string) (bool, string) {
	// Resolve symlinks and verify the real path stays within pluginDir.
	realPath, err := filepath.EvalSymlinks(pluginPath)
	if err != nil {
		return false, fmt.Sprintf("cannot resolve symlinks: %v", err)
	}
	cleanDir := filepath.Clean(pluginDir)
	if !strings.HasPrefix(filepath.Clean(realPath), cleanDir+string(filepath.Separator)) {
		return false, fmt.Sprintf("plugin symlink escapes plugin directory: %s → %s", pluginPath, realPath)
	}

	// Unix-specific: check directory permissions.
	if runtime.GOOS != "windows" {
		dirInfo, err := os.Stat(pluginDir)
		if err != nil {
			return false, fmt.Sprintf("cannot stat plugin dir: %v", err)
		}
		if dirInfo.Mode().Perm()&0o002 != 0 {
			return false, fmt.Sprintf("plugin directory is world-writable: %s", pluginDir)
		}
	}

	return true, ""
}
