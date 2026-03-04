package updater

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// UpdateInfo holds the result of an update check.
type UpdateInfo struct {
	Available      bool      `json:"available"`
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version"`
	CheckedAt      time.Time `json:"checked_at"`
}

// Checker periodically checks for new versions against a remote assets URL.
type Checker struct {
	currentVersion string
	assetsURL      string
	interval       time.Duration
	logger         *slog.Logger
	client         *http.Client

	mu       sync.RWMutex
	lastInfo UpdateInfo
}

// NewChecker creates a new update checker.
// If currentVersion is "dev", the checker will not perform checks.
func NewChecker(currentVersion, assetsURL string, interval time.Duration, logger *slog.Logger) *Checker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Checker{
		currentVersion: currentVersion,
		assetsURL:      strings.TrimRight(assetsURL, "/"),
		interval:       interval,
		logger:         logger.With("component", "updater"),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		lastInfo: UpdateInfo{
			CurrentVersion: currentVersion,
		},
	}
}

// Start begins periodic update checking in a background goroutine.
// It performs an initial check immediately, then repeats at the configured interval.
func (c *Checker) Start(ctx context.Context) {
	if c.currentVersion == "dev" {
		c.logger.Info("update checker disabled for dev build")
		return
	}

	go func() {
		// Initial check.
		if _, err := c.CheckNow(); err != nil {
			c.logger.Warn("initial update check failed", "error", err)
		}

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := c.CheckNow(); err != nil {
					c.logger.Warn("periodic update check failed", "error", err)
				}
			}
		}
	}()
}

// CheckNow performs an immediate update check against the remote latest.txt.
func (c *Checker) CheckNow() (UpdateInfo, error) {
	if c.currentVersion == "dev" {
		info := UpdateInfo{
			CurrentVersion: "dev",
			CheckedAt:      time.Now(),
		}
		c.mu.Lock()
		c.lastInfo = info
		c.mu.Unlock()
		return info, nil
	}

	url := c.assetsURL + "/latest.txt"
	resp, err := c.client.Get(url)
	if err != nil {
		return c.LastCheck(), fmt.Errorf("fetch latest.txt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.LastCheck(), fmt.Errorf("fetch latest.txt: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return c.LastCheck(), fmt.Errorf("read latest.txt: %w", err)
	}

	latestStr := strings.TrimSpace(string(body))
	latest, err := ParseVersion(latestStr)
	if err != nil {
		return c.LastCheck(), fmt.Errorf("parse remote version %q: %w", latestStr, err)
	}

	current, err := ParseVersion(c.currentVersion)
	if err != nil {
		return c.LastCheck(), fmt.Errorf("parse current version %q: %w", c.currentVersion, err)
	}

	info := UpdateInfo{
		Available:      latest.IsNewerThan(current),
		CurrentVersion: c.currentVersion,
		LatestVersion:  latestStr,
		CheckedAt:      time.Now(),
	}

	c.mu.Lock()
	c.lastInfo = info
	c.mu.Unlock()

	if info.Available {
		c.logger.Info("update available", "current", c.currentVersion, "latest", latestStr)
	} else {
		c.logger.Debug("no update available", "current", c.currentVersion, "latest", latestStr)
	}

	return info, nil
}

// LastCheck returns the result of the most recent update check (thread-safe).
func (c *Checker) LastCheck() UpdateInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastInfo
}
