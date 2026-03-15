// Package copilot – session_lock.go implements cross-process session write locks
// using advisory file locking. This prevents two DevClaw processes from
// writing to the same session simultaneously, which could corrupt session state.
//
// Architecture:
//   - Lock file: <sessions_dir>/<session_id>.lock
//   - Contains JSON: {"pid": <PID>, "acquired_at": "<RFC3339>"}
//   - Uses syscall.Flock for advisory locking (safe on Linux/macOS)
//   - Watchdog goroutine refreshes the lock file mtime every 60s
//   - Stale detection: locks older than 30 min are considered abandoned
package copilot

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// sessionLockStaleThreshold is how long a lock can go without refresh
	// before being considered stale/abandoned.
	sessionLockStaleThreshold = 30 * time.Minute

	// sessionLockRefreshInterval is how often the watchdog refreshes the
	// lock file mtime to signal liveness.
	sessionLockRefreshInterval = 60 * time.Second
)

// sessionLockInfo is the JSON payload stored inside lock files.
type sessionLockInfo struct {
	PID        int       `json:"pid"`
	AcquiredAt time.Time `json:"acquired_at"`
}

// SessionLock represents an active cross-process lock on a session.
type SessionLock struct {
	file      *os.File
	path      string
	sessionID string
	stopCh    chan struct{}
	stopped   sync.Once
	logger    *slog.Logger
}

// AcquireSessionLock attempts to acquire an advisory file lock for the given
// session. If the lock is held by another live process, it returns an error.
// Stale locks (older than sessionLockStaleThreshold) are automatically broken.
func AcquireSessionLock(sessionsDir, sessionID string, logger *slog.Logger) (*SessionLock, error) {
	// Sanitize sessionID to prevent path traversal (e.g. "../../etc/passwd").
	sessionID = filepath.Base(sessionID)
	lockPath := filepath.Join(sessionsDir, sessionID+".lock")

	// Ensure parent directory exists.
	if err := os.MkdirAll(sessionsDir, 0700); err != nil {
		return nil, fmt.Errorf("session lock: mkdir %s: %w", sessionsDir, err)
	}

	// Check for stale lock before attempting to acquire.
	if info, err := os.Stat(lockPath); err == nil {
		age := time.Since(info.ModTime())
		if age > sessionLockStaleThreshold {
			logger.Warn("breaking stale session lock",
				"session", sessionID,
				"age", age.Round(time.Second),
			)
			_ = os.Remove(lockPath)
		}
	}

	// Open (or create) the lock file.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("session lock: open %s: %w", lockPath, err)
	}

	// Try non-blocking advisory lock.
	if err := flockExclusive(f); err != nil {
		f.Close()
		// Read existing lock info for a better error message.
		holder := readLockHolder(lockPath)
		return nil, fmt.Errorf("session %s is locked by PID %d (acquired %s)",
			sessionID, holder.PID, holder.AcquiredAt.Format(time.RFC3339))
	}

	// Write our lock info.
	info := sessionLockInfo{
		PID:        os.Getpid(),
		AcquiredAt: time.Now(),
	}
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	_ = json.NewEncoder(f).Encode(info)
	_ = f.Sync()

	sl := &SessionLock{
		file:      f,
		path:      lockPath,
		sessionID: sessionID,
		stopCh:    make(chan struct{}),
		logger:    logger,
	}

	// Start watchdog to refresh mtime periodically.
	go sl.watchdog()

	return sl, nil
}

// Release releases the session lock and stops the watchdog.
func (sl *SessionLock) Release() {
	sl.stopped.Do(func() {
		close(sl.stopCh)
		_ = flockUnlock(sl.file)
		sl.file.Close()
		_ = os.Remove(sl.path)
		sl.logger.Debug("session lock released", "session", sl.sessionID)
	})
}

// watchdog refreshes the lock file mtime periodically so other processes
// can distinguish a live lock from a stale/crashed one.
func (sl *SessionLock) watchdog() {
	ticker := time.NewTicker(sessionLockRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sl.stopCh:
			return
		case now := <-ticker.C:
			// Touch the file mtime to signal liveness.
			if err := os.Chtimes(sl.path, now, now); err != nil {
				sl.logger.Warn("session lock refresh failed",
					"session", sl.sessionID, "error", err)
			}
		}
	}
}

// readLockHolder reads the lock info from an existing lock file.
// Returns zero-value info on any error.
func readLockHolder(path string) sessionLockInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionLockInfo{}
	}
	var info sessionLockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return sessionLockInfo{} // corrupted lock file
	}
	return info
}
