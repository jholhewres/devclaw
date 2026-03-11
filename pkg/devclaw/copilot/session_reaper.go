// Package copilot – session_reaper.go periodically removes stale persisted sessions
// that haven't been active within a configurable time window.
// This prevents unbounded storage growth from long-lived deployments.
package copilot

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// SessionReaperConfig configures the session reaper.
type SessionReaperConfig struct {
	Enabled    bool `yaml:"enabled" json:"enabled"`
	MaxAgeDays int  `yaml:"max_age_days" json:"max_age_days"`
}

// StartPersistentSessionReaper runs a background goroutine that deletes session
// files older than maxAgeDays. It runs once daily and on startup.
// storePath is the directory containing session JSONL/SQLite files.
func StartPersistentSessionReaper(ctx context.Context, storePath string, maxAgeDays int, logger *slog.Logger) {
	if maxAgeDays <= 0 {
		maxAgeDays = 90
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "session-reaper")

	reap := func() {
		if storePath == "" {
			return
		}
		cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
		var deleted int

		entries, err := os.ReadDir(storePath)
		if err != nil {
			logger.Warn("cannot read sessions directory", "path", storePath, "error", err)
			return
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := filepath.Ext(entry.Name())
			if ext != ".jsonl" && ext != ".json" && ext != ".meta" {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				fullPath := filepath.Join(storePath, entry.Name())
				if err := os.Remove(fullPath); err != nil {
					logger.Warn("failed to remove stale session file",
						"path", fullPath, "error", err)
				} else {
					deleted++
				}
			}
		}

		if deleted > 0 {
			logger.Info("reaped stale session files",
				"deleted", deleted,
				"max_age_days", maxAgeDays,
			)
		}
	}

	// Initial reap on startup.
	reap()

	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				reap()
			case <-ctx.Done():
				return
			}
		}
	}()

	logger.Info("session reaper started", "max_age_days", maxAgeDays, "store_path", storePath)
}
