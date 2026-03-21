// Package gateway – channel_health_monitor.go monitors the health of active
// channels (WebSocket connections, long-running integrations) and restarts
// stale ones with cooldown and rate limiting.
package gateway

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// ChannelHealthMonitorConfig configures the health monitor.
type ChannelHealthMonitorConfig struct {
	Enabled            bool          `yaml:"enabled"`
	CheckInterval      time.Duration `yaml:"check_interval"`        // default 60s
	StaleThreshold     time.Duration `yaml:"stale_threshold"`       // default 5m
	RestartCooldown    time.Duration `yaml:"restart_cooldown"`      // default 2m
	MaxRestartsPerHour int           `yaml:"max_restarts_per_hour"` // default 5
	RestartTimeout     time.Duration `yaml:"restart_timeout"`       // default 30s
}

// DefaultChannelHealthMonitorConfig returns sensible defaults.
func DefaultChannelHealthMonitorConfig() ChannelHealthMonitorConfig {
	return ChannelHealthMonitorConfig{
		Enabled:            false,
		CheckInterval:      60 * time.Second,
		StaleThreshold:     5 * time.Minute,
		RestartCooldown:    2 * time.Minute,
		MaxRestartsPerHour: 5,
		RestartTimeout:     30 * time.Second,
	}
}

// ChannelHealth represents the health state of a single channel.
type ChannelHealth struct {
	Name         string    `json:"name"`
	LastActivity time.Time `json:"last_activity"`
	Restarts     int       `json:"restarts"`
	Healthy      bool      `json:"healthy"`
}

// MonitoredChannel is the interface that channels must implement to be monitored.
type MonitoredChannel interface {
	// ChannelName returns the channel identifier (e.g. "whatsapp", "telegram").
	ChannelName() string
	// LastActivityTime returns the time of the last received or sent message.
	LastActivityTime() time.Time
	// Restart attempts to reconnect/restart the channel. Returns error if failed.
	Restart(ctx context.Context) error
}

// ChannelHealthMonitor checks registered channels periodically and restarts
// stale connections with rate limiting.
type ChannelHealthMonitor struct {
	config   ChannelHealthMonitorConfig
	logger   *slog.Logger

	mu             sync.Mutex
	channels       []MonitoredChannel
	restartHistory []time.Time          // timestamps of successful restarts (sliding window)
	lastRestart    map[string]time.Time // per-channel cooldown (includes failures)
	restartCounts  map[string]int       // per-channel successful restart count
}

// NewChannelHealthMonitor creates a new monitor.
func NewChannelHealthMonitor(cfg ChannelHealthMonitorConfig, logger *slog.Logger) *ChannelHealthMonitor {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 60 * time.Second
	}
	if cfg.StaleThreshold <= 0 {
		cfg.StaleThreshold = 5 * time.Minute
	}
	if cfg.RestartCooldown <= 0 {
		cfg.RestartCooldown = 2 * time.Minute
	}
	if cfg.MaxRestartsPerHour <= 0 {
		cfg.MaxRestartsPerHour = 5
	}
	if cfg.RestartTimeout <= 0 {
		cfg.RestartTimeout = 30 * time.Second
	}
	return &ChannelHealthMonitor{
		config:        cfg,
		logger:        logger.With("component", "channel_health_monitor"),
		lastRestart:   make(map[string]time.Time),
		restartCounts: make(map[string]int),
	}
}

// Register adds a channel to the monitor. Nil channels and duplicates
// (by ChannelName) are silently ignored.
func (m *ChannelHealthMonitor) Register(ch MonitoredChannel) {
	if ch == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	name := ch.ChannelName()
	for _, existing := range m.channels {
		if existing.ChannelName() == name {
			return // duplicate
		}
	}
	m.channels = append(m.channels, ch)
}

// Start begins the periodic health check loop. Blocks until ctx is cancelled.
func (m *ChannelHealthMonitor) Start(ctx context.Context) {
	if !m.config.Enabled {
		m.logger.Info("channel health monitor disabled")
		return
	}

	m.logger.Info("channel health monitor started",
		"interval", m.config.CheckInterval,
		"stale_threshold", m.config.StaleThreshold,
	)

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkAll(ctx)
		case <-ctx.Done():
			m.logger.Info("channel health monitor stopped")
			return
		}
	}
}

// Health returns the current health status of all monitored channels.
// Does not hold the mutex during channel interface calls.
func (m *ChannelHealthMonitor) Health() []ChannelHealth {
	m.mu.Lock()
	channels := make([]MonitoredChannel, len(m.channels))
	copy(channels, m.channels)
	counts := make(map[string]int, len(m.restartCounts))
	for k, v := range m.restartCounts {
		counts[k] = v
	}
	staleThreshold := m.config.StaleThreshold
	m.mu.Unlock()

	now := time.Now()
	result := make([]ChannelHealth, len(channels))
	for i, ch := range channels {
		name := ch.ChannelName()
		lastActivity := ch.LastActivityTime()
		result[i] = ChannelHealth{
			Name:         name,
			LastActivity: lastActivity,
			Restarts:     counts[name],
			Healthy:      now.Sub(lastActivity) < staleThreshold,
		}
	}
	return result
}

// checkAll iterates all channels and restarts stale ones.
func (m *ChannelHealthMonitor) checkAll(ctx context.Context) {
	m.mu.Lock()
	channels := make([]MonitoredChannel, len(m.channels))
	copy(channels, m.channels)
	m.mu.Unlock()

	now := time.Now()

	for _, ch := range channels {
		name := ch.ChannelName()
		lastActivity := ch.LastActivityTime()

		if now.Sub(lastActivity) < m.config.StaleThreshold {
			continue // Healthy.
		}

		// Check cooldown for this specific channel.
		m.mu.Lock()
		lastRestart := m.lastRestart[name]
		m.mu.Unlock()

		if now.Sub(lastRestart) < m.config.RestartCooldown {
			m.logger.Debug("skipping restart (cooldown)",
				"channel", name,
				"last_restart", lastRestart,
			)
			continue
		}

		// Check global hourly restart cap (only successful restarts count).
		if !m.canRestart(now) {
			m.logger.Warn("skipping restart (hourly cap reached)",
				"channel", name,
				"max_per_hour", m.config.MaxRestartsPerHour,
			)
			continue
		}

		m.logger.Warn("channel stale, attempting restart",
			"channel", name,
			"last_activity", lastActivity,
			"stale_for", now.Sub(lastActivity),
		)

		// Use a per-restart timeout to prevent a hanging Restart from blocking
		// the entire check loop.
		restartCtx, restartCancel := context.WithTimeout(ctx, m.config.RestartTimeout)
		err := ch.Restart(restartCtx)
		restartCancel()

		if err != nil {
			m.logger.Error("channel restart failed",
				"channel", name,
				"error", err,
			)
			// Record per-channel cooldown to prevent hammering, but do NOT
			// count failures against the global hourly cap.
			m.mu.Lock()
			m.lastRestart[name] = now
			m.mu.Unlock()
			continue
		}

		m.logger.Info("channel restarted successfully", "channel", name)

		// Record successful restart against both per-channel cooldown and
		// global hourly cap.
		m.mu.Lock()
		m.lastRestart[name] = now
		m.restartHistory = append(m.restartHistory, now)
		m.restartCounts[name]++
		m.mu.Unlock()
	}
}

// canRestart checks if we haven't exceeded the hourly restart cap.
// Only successful restarts are counted.
func (m *ChannelHealthMonitor) canRestart(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	hourAgo := now.Add(-1 * time.Hour)

	// Prune old entries.
	pruned := m.restartHistory[:0]
	for _, t := range m.restartHistory {
		if t.After(hourAgo) {
			pruned = append(pruned, t)
		}
	}
	m.restartHistory = pruned

	return len(m.restartHistory) < m.config.MaxRestartsPerHour
}
