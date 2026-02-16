// Package copilot – model_failover.go implements automatic model failover
// with cooldowns and reason classification. When the primary LLM returns
// persistent errors, the system rotates through fallback models automatically.
package copilot

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ModelFallbackConfig defines the primary model and fallback chain.
type ModelFallbackConfig struct {
	Primary   string   `yaml:"primary"`   // e.g. "claude-sonnet-4-20250514"
	Fallbacks []string `yaml:"fallbacks"` // e.g. ["gpt-4o", "glm-5"]

	// CooldownConfig controls how long a model is disabled after failures.
	Cooldowns CooldownConfig `yaml:"cooldowns"`
}

// CooldownConfig defines backoff parameters for model cooldowns.
type CooldownConfig struct {
	BillingBackoffHours  float64 `yaml:"billing_backoff_hours"`  // Default: 5
	BillingMaxHours      float64 `yaml:"billing_max_hours"`      // Default: 24
	FailureWindowHours   float64 `yaml:"failure_window_hours"`   // Default: 24
	InitialBackoffMinutes float64 `yaml:"initial_backoff_minutes"` // Default: 1
	MaxBackoffMinutes    float64 `yaml:"max_backoff_minutes"`     // Default: 60
}

// DefaultCooldownConfig returns sensible defaults.
func DefaultCooldownConfig() CooldownConfig {
	return CooldownConfig{
		BillingBackoffHours:   5,
		BillingMaxHours:       24,
		FailureWindowHours:    24,
		InitialBackoffMinutes: 1,
		MaxBackoffMinutes:     60,
	}
}

// FailoverReason classifies why a model failed.
type FailoverReason string

const (
	FailoverBilling   FailoverReason = "billing"    // 402 Payment Required
	FailoverRateLimit FailoverReason = "rate_limit"  // 429 Too Many Requests
	FailoverAuth      FailoverReason = "auth"        // 401/403
	FailoverTimeout   FailoverReason = "timeout"     // 408, ETIMEDOUT, empty chunks
	FailoverFormat    FailoverReason = "format"      // 400 Bad Request
	FailoverServer    FailoverReason = "server"      // 5xx
	FailoverUnknown   FailoverReason = "unknown"
)

// ModelCooldown tracks a model's cooldown state.
type ModelCooldown struct {
	Model      string
	Until      time.Time
	Reason     FailoverReason
	ErrorCount int
	LastError  time.Time
}

// ModelFailoverManager handles automatic model rotation on failures.
type ModelFailoverManager struct {
	config    ModelFallbackConfig
	cooldowns map[string]*ModelCooldown // model → cooldown state
	mu        sync.RWMutex
	logger    *slog.Logger
}

// NewModelFailoverManager creates a failover manager.
func NewModelFailoverManager(config ModelFallbackConfig, logger *slog.Logger) *ModelFailoverManager {
	if config.Cooldowns == (CooldownConfig{}) {
		config.Cooldowns = DefaultCooldownConfig()
	}
	return &ModelFailoverManager{
		config:    config,
		cooldowns: make(map[string]*ModelCooldown),
		logger:    logger,
	}
}

// SelectModel returns the best available model. It checks the primary first,
// then iterates through fallbacks, skipping any that are in cooldown.
// Returns the model name and whether it's the primary.
func (m *ModelFailoverManager) SelectModel() (model string, isPrimary bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try primary first.
	if !m.isInCooldownLocked(m.config.Primary) {
		return m.config.Primary, true
	}

	// Try fallbacks.
	for _, fb := range m.config.Fallbacks {
		if !m.isInCooldownLocked(fb) {
			m.logger.Info("using fallback model",
				"primary", m.config.Primary,
				"fallback", fb,
			)
			return fb, false
		}
	}

	// All models in cooldown — use primary anyway (best effort).
	m.logger.Warn("all models in cooldown, using primary anyway",
		"primary", m.config.Primary,
	)
	return m.config.Primary, true
}

// ReportSuccess resets the cooldown for a model after a successful call.
func (m *ModelFailoverManager) ReportSuccess(model string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cooldowns, model)
}

// ReportFailure records a failure for a model and applies cooldown if needed.
// Returns the reason classification.
func (m *ModelFailoverManager) ReportFailure(model string, statusCode int, errMsg string) FailoverReason {
	reason := ClassifyError(statusCode, errMsg)

	m.mu.Lock()
	defer m.mu.Unlock()

	cd, ok := m.cooldowns[model]
	if !ok {
		cd = &ModelCooldown{Model: model}
		m.cooldowns[model] = cd
	}

	cd.ErrorCount++
	cd.LastError = time.Now()
	cd.Reason = reason

	// Calculate cooldown duration based on reason.
	var duration time.Duration
	cfg := m.config.Cooldowns

	switch reason {
	case FailoverBilling:
		hours := cfg.BillingBackoffHours
		if hours <= 0 {
			hours = 5
		}
		duration = time.Duration(hours * float64(time.Hour))
		// Cap at billing max.
		maxHours := cfg.BillingMaxHours
		if maxHours <= 0 {
			maxHours = 24
		}
		if duration > time.Duration(maxHours*float64(time.Hour)) {
			duration = time.Duration(maxHours * float64(time.Hour))
		}

	case FailoverRateLimit:
		// Exponential backoff: 1min → 5min → 25min → 60min
		initial := cfg.InitialBackoffMinutes
		if initial <= 0 {
			initial = 1
		}
		maxMin := cfg.MaxBackoffMinutes
		if maxMin <= 0 {
			maxMin = 60
		}
		minutes := initial
		for i := 1; i < cd.ErrorCount; i++ {
			minutes *= 5
			if minutes > maxMin {
				minutes = maxMin
				break
			}
		}
		duration = time.Duration(minutes * float64(time.Minute))

	case FailoverAuth:
		// Auth errors are persistent — long cooldown.
		duration = 1 * time.Hour

	case FailoverTimeout, FailoverServer:
		// Transient — short cooldown with exponential backoff.
		initial := cfg.InitialBackoffMinutes
		if initial <= 0 {
			initial = 1
		}
		minutes := initial
		for i := 1; i < cd.ErrorCount; i++ {
			minutes *= 2
			if minutes > 30 {
				minutes = 30
				break
			}
		}
		duration = time.Duration(minutes * float64(time.Minute))

	default:
		duration = 1 * time.Minute
	}

	cd.Until = time.Now().Add(duration)

	m.logger.Warn("model cooldown applied",
		"model", model,
		"reason", reason,
		"error_count", cd.ErrorCount,
		"cooldown_until", cd.Until.Format(time.RFC3339),
	)

	return reason
}

// isInCooldownLocked checks if a model is in cooldown. Must be called with mu held.
func (m *ModelFailoverManager) isInCooldownLocked(model string) bool {
	cd, ok := m.cooldowns[model]
	if !ok {
		return false
	}
	if time.Now().After(cd.Until) {
		// Cooldown expired — remove it.
		delete(m.cooldowns, model)
		return false
	}
	return true
}

// ClassifyError determines the failover reason from an HTTP status code and error message.
func ClassifyError(statusCode int, errMsg string) FailoverReason {
	switch statusCode {
	case 402:
		return FailoverBilling
	case 429:
		return FailoverRateLimit
	case 401, 403:
		return FailoverAuth
	case 408:
		return FailoverTimeout
	case 400:
		return FailoverFormat
	}

	if statusCode >= 500 {
		return FailoverServer
	}

	// Check error message patterns.
	lower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
		return FailoverTimeout
	case strings.Contains(lower, "empty") && strings.Contains(lower, "chunk"):
		return FailoverTimeout
	case strings.Contains(lower, "ended without sending any chunks"):
		return FailoverTimeout
	case strings.Contains(lower, "billing") || strings.Contains(lower, "payment"):
		return FailoverBilling
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "rate_limit"):
		return FailoverRateLimit
	}

	return FailoverUnknown
}

// GetCooldownStatus returns the current cooldown state of all models.
func (m *ModelFailoverManager) GetCooldownStatus() map[string]*ModelCooldown {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ModelCooldown, len(m.cooldowns))
	for k, v := range m.cooldowns {
		cp := *v
		result[k] = &cp
	}
	return result
}
