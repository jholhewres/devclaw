// Package copilot – config_watcher.go polls config.yaml for changes and
// triggers hot-reload of safe-to-update fields without restarting the daemon.
package copilot

import (
	"context"
	"crypto/sha256"
	"log/slog"
	"os"
	"time"
)

// ConfigWatcher monitors a config file for changes and invokes a callback
// when the file is modified. Uses polling (mtime + sha256) to avoid platform-specific
// file watchers.
type ConfigWatcher struct {
	path     string
	lastMod  time.Time
	lastHash [32]byte
	interval time.Duration
	onChange func(newCfg *Config)
	logger   *slog.Logger
}

// NewConfigWatcher creates a new config watcher.
// interval is the polling interval (e.g. 5 * time.Second).
// onChange is called when a valid config change is detected.
func NewConfigWatcher(path string, interval time.Duration, onChange func(*Config), logger *slog.Logger) *ConfigWatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &ConfigWatcher{
		path:     path,
		interval: interval,
		onChange: onChange,
		logger:   logger.With("component", "config_watcher"),
	}
}

// Start begins polling in a goroutine. Exits when ctx is cancelled.
func (w *ConfigWatcher) Start(ctx context.Context) {
	// Initial check to set baseline (avoid triggering onChange on first tick).
	w.check()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("config watcher stopped")
			return
		case <-ticker.C:
			w.check()
		}
	}
}

// check reads the config file, compares mtime and hash, and calls onChange if changed.
func (w *ConfigWatcher) check() {
	info, err := os.Stat(w.path)
	if err != nil {
		// File may not exist yet or be temporarily unavailable.
		return
	}

	mod := info.ModTime()
	// Fast path: same mtime as last time.
	if !mod.After(w.lastMod) && !w.lastMod.IsZero() {
		return
	}

	data, err := os.ReadFile(w.path)
	if err != nil {
		w.logger.Warn("config watcher: failed to read file", "error", err)
		return
	}

	hash := sha256.Sum256(data)
	// Same hash = no actual change (e.g. touch without edit).
	if hash == w.lastHash {
		w.lastMod = mod
		return
	}

	// First run: set baseline without triggering onChange.
	var zeroHash [32]byte
	if w.lastHash == zeroHash {
		w.lastMod = mod
		w.lastHash = hash
		return
	}

	// Load and validate config (same as LoadConfigFromFile: env expansion, secrets).
	cfg, err := LoadConfigFromFile(w.path)
	if err != nil {
		w.logger.Warn("config watcher: invalid config, skipping hot-reload",
			"error", err)
		return
	}

	w.lastMod = mod
	w.lastHash = hash

	w.logger.Info("config file changed, applying hot-reload")
	if w.onChange != nil {
		w.onChange(cfg)
	}
}
