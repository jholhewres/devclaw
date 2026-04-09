// Package memory — layer_identity.go implements the L0 IdentityLayer.
//
// The IdentityLayer loads a user-curated identity fragment from disk and
// caches it in memory. It hot-reloads on file change via fsnotify, with a
// 30-second polling fallback when fsnotify is unavailable.
//
// Until Room 2.4 wires this layer into the prompt stack, the layer is dead
// code at runtime — Render() is safe to call from tests but no production
// caller exists yet.
package memory

import (
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"
)

// defaultIdentityBudget caps how many UTF-8 bytes the rendered identity
// fragment can occupy. Room 2.4 may later replace this with a token-based
// budget from HierarchyConfig. Until then, 800 bytes ≈ 200 tokens is a
// safe ceiling that fits any reasonable identity blurb.
const defaultIdentityBudget = 800

// pollInterval is how often the fallback poller checks for file changes.
const pollInterval = 30 * time.Second

// IdentityLayer loads a user-curated identity fragment from disk and
// caches it in memory. Hot-reloads on file change via fsnotify, with a
// 30s polling fallback when fsnotify is unavailable. Thread-safe.
//
// Until Room 2.4 wires this layer into the prompt stack, the layer is
// dead code at runtime — Render() is safe to call from tests but no
// production caller exists yet.
type IdentityLayer struct {
	path   string
	logger *slog.Logger
	budget int

	mu      sync.RWMutex
	content string
	loaded  bool
	modTime time.Time

	// watcher is non-nil only when fsnotify setup succeeded.
	watcher *fsnotify.Watcher

	// pollDone signals the polling goroutine to stop. Only used when
	// watcher is nil. Closed by Stop().
	pollDone chan struct{}

	// started guards against double-Start.
	started bool
}

// NewIdentityLayer constructs a new IdentityLayer. The path is the
// absolute file path to load (caller resolves "~"). If logger is nil,
// slog.Default() is used. Pass budget=0 to use defaultIdentityBudget.
//
// The constructor does NOT touch the filesystem — call Start() to begin
// watching and Render() to read content.
func NewIdentityLayer(path string, logger *slog.Logger, budget int) *IdentityLayer {
	if logger == nil {
		logger = slog.Default()
	}
	if budget <= 0 {
		budget = defaultIdentityBudget
	}
	return &IdentityLayer{
		path:     path,
		logger:   logger,
		budget:   budget,
		pollDone: make(chan struct{}),
	}
}

// Start begins the file watcher (or polling fallback) and performs an
// initial load. Returns nil even if the file does not exist — a missing
// identity file is a valid state (renders to empty string). Errors only
// for unrecoverable conditions (e.g. cannot create temp dir).
//
// Safe to call multiple times — subsequent calls are no-ops.
func (l *IdentityLayer) Start() error {
	l.mu.Lock()
	if l.started {
		l.mu.Unlock()
		return nil
	}
	l.started = true
	l.mu.Unlock()

	// Initial load — errors are non-fatal.
	if err := l.reload(); err != nil {
		l.logger.Warn("identity layer: initial load failed", "path", l.path, "err", err)
	}

	// Try to set up fsnotify watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		l.logger.Info("identity layer: fsnotify unavailable, using poll fallback",
			"path", l.path, "err", err)
		go l.pollLoop()
		return nil
	}

	// Watch the file if it exists; if not, we'll watch when it appears.
	if watchErr := watcher.Add(l.path); watchErr != nil {
		// File may not exist yet — that's fine. Log at INFO and start
		// poll fallback instead of returning an error.
		l.logger.Info("identity layer: cannot watch path (file may not exist yet), using poll fallback",
			"path", l.path, "err", watchErr)
		_ = watcher.Close()
		go l.pollLoop()
		return nil
	}

	l.mu.Lock()
	l.watcher = watcher
	l.mu.Unlock()

	go l.watchLoop(watcher)
	return nil
}

// Stop closes the watcher and signals the polling goroutine to exit.
// Idempotent.
func (l *IdentityLayer) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.watcher != nil {
		_ = l.watcher.Close()
		l.watcher = nil
	}

	// Close pollDone only once; recover from double-close panic.
	func() {
		defer func() { recover() }() //nolint:errcheck
		close(l.pollDone)
	}()
}

// Render returns the cached identity content, truncated to the budget.
// If the file has not been loaded yet, returns empty string. Never
// returns an error — file-read failures are logged at WARN and result
// in empty content.
func (l *IdentityLayer) Render() string {
	l.mu.RLock()
	content := l.content
	budget := l.budget
	l.mu.RUnlock()

	return truncateAtBoundary(content, budget)
}

// Reload forces an immediate re-read from disk, bypassing the watcher
// or poll cadence. Used by tests and the CLI edit subcommand.
func (l *IdentityLayer) Reload() error {
	return l.reload()
}

// reload reads the file from disk and updates the cached content.
// Caller does not need to hold any lock.
func (l *IdentityLayer) reload() error {
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing file is valid — render as empty.
			l.mu.Lock()
			l.content = ""
			l.loaded = true
			l.modTime = time.Time{}
			l.mu.Unlock()
			return nil
		}
		return err
	}

	info, statErr := os.Stat(l.path)
	var modTime time.Time
	if statErr == nil {
		modTime = info.ModTime()
	}

	l.mu.Lock()
	l.content = string(data)
	l.loaded = true
	l.modTime = modTime
	l.mu.Unlock()
	return nil
}

// watchLoop runs in a goroutine and reloads on fsnotify events.
func (l *IdentityLayer) watchLoop(watcher *fsnotify.Watcher) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				if err := l.reload(); err != nil {
					l.logger.Warn("identity layer: reload on watch event failed",
						"path", l.path, "err", err)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			l.logger.Warn("identity layer: fsnotify error", "path", l.path, "err", err)
		}
	}
}

// pollLoop runs in a goroutine when fsnotify is unavailable. It checks
// the file's ModTime every 30 seconds and reloads if it has changed.
func (l *IdentityLayer) pollLoop() {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.pollDone:
			return
		case <-ticker.C:
			info, err := os.Stat(l.path)
			if err != nil {
				// File may not exist yet — not an error.
				continue
			}

			l.mu.RLock()
			lastMod := l.modTime
			l.mu.RUnlock()

			if info.ModTime().After(lastMod) {
				if reloadErr := l.reload(); reloadErr != nil {
					l.logger.Warn("identity layer: poll reload failed",
						"path", l.path, "err", reloadErr)
				}
			}
		}
	}
}

// truncateAtBoundary truncates s to at most maxBytes, ending at a clean
// word boundary (space or newline) when possible. If the cut point falls
// inside a word, the truncation is moved back to the previous boundary.
// Multi-byte UTF-8 sequences are never split.
func truncateAtBoundary(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}

	// Walk back from the byte limit to find a valid UTF-8 rune boundary.
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}

	// Walk back further to find a word boundary (space or newline).
	boundary := cut
	for boundary > 0 {
		c := s[boundary-1]
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' {
			break
		}
		boundary--
	}

	// If we found a word boundary, prefer it. Otherwise use the rune boundary.
	if boundary > 0 {
		// Include the trailing whitespace character for clean output.
		return strings.TrimRight(s[:boundary], " \t\r\n")
	}
	return s[:cut]
}
