package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
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

// githubRepoPattern matches GitHub releases URLs to extract owner/repo.
var githubRepoPattern = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)`)

// CheckNow performs an immediate update check.
// Supports both GitHub releases URLs and plain latest.txt endpoints.
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

	var latestStr string
	var err error

	if m := githubRepoPattern.FindStringSubmatch(c.assetsURL); len(m) == 3 {
		latestStr, err = c.fetchGitHubLatest(m[1], m[2])
	} else {
		latestStr, err = c.fetchLatestTxt()
	}
	if err != nil {
		return c.LastCheck(), err
	}

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

// fetchGitHubLatest gets the latest version from GitHub releases API.
func (c *Checker) fetchGitHubLatest(owner, repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch GitHub release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch GitHub release: HTTP %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&release); err != nil {
		return "", fmt.Errorf("parse GitHub release: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

// fetchLatestTxt gets the latest version from a plain-text latest.txt endpoint.
func (c *Checker) fetchLatestTxt() (string, error) {
	url := c.assetsURL + "/latest.txt"
	resp, err := c.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetch latest.txt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch latest.txt: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", fmt.Errorf("read latest.txt: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
}

// LastCheck returns the result of the most recent update check (thread-safe).
func (c *Checker) LastCheck() UpdateInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastInfo
}
