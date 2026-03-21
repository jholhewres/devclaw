// Package copilot – model_failover.go implements automatic model failover
// with cooldowns and reason classification. When the primary LLM returns
// persistent errors, the system rotates through fallback models automatically.
package copilot

import (
	"errors"
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
	FailoverBilling        FailoverReason = "billing"         // 402 Payment Required
	FailoverRateLimit      FailoverReason = "rate_limit"      // 429 Too Many Requests, 529 Overloaded
	FailoverAuth           FailoverReason = "auth"            // 401 (transient)
	FailoverAuthPermanent  FailoverReason = "auth_permanent"  // 403, revoked keys, deactivated accounts
	FailoverSessionExpired FailoverReason = "session_expired" // Token expired/revoked
	FailoverTimeout        FailoverReason = "timeout"         // 408, ETIMEDOUT, empty chunks, Cloudflare 521-530
	FailoverFormat         FailoverReason = "format"          // 400 Bad Request
	FailoverServer         FailoverReason = "server"          // 5xx (excluding Cloudflare CDN codes)
	FailoverModelNotFound  FailoverReason = "model_not_found" // Model doesn't exist for this provider
	FailoverUnknown        FailoverReason = "unknown"
)

// symbolicErrorCodes maps provider-specific error code identifiers to failover
// reasons. Checked case-insensitively against the raw error message before
// heuristic matching in ClassifyError.
var symbolicErrorCodes = map[string]FailoverReason{
	// Google Cloud / gRPC
	"resource_exhausted": FailoverRateLimit,
	"deadline_exceeded":  FailoverTimeout,
	"unauthenticated":    FailoverAuth,

	// AWS
	"throttlingexception":          FailoverRateLimit,
	"toomanyrequestsexception":     FailoverRateLimit,
	"serviceunavailableexception":  FailoverServer,
	"accessdeniedexception":        FailoverAuthPermanent,

	// Anthropic
	"overloaded_error": FailoverRateLimit,

	// Generic / cross-provider
	"rate_limit_error":       FailoverRateLimit,
	"internal_server_error":  FailoverServer,
}

// ModelCooldown tracks a model's cooldown state.
type ModelCooldown struct {
	Model      string
	Until      time.Time
	Reason     FailoverReason
	ErrorCount int
	LastError  time.Time
}

// probeMinInterval is the minimum time between recovery probes for the same model.
const probeMinInterval = 30 * time.Second

// probeMargin is how close to cooldown expiry we start probing.
const probeMargin = 2 * time.Minute

// ModelFailoverManager handles automatic model rotation on failures.
type ModelFailoverManager struct {
	config      ModelFallbackConfig
	cooldowns   map[string]*ModelCooldown // model → cooldown state
	lastProbeAt map[string]time.Time      // model → last probe attempt time
	mu          sync.RWMutex
	logger      *slog.Logger
}

// NewModelFailoverManager creates a failover manager.
func NewModelFailoverManager(config ModelFallbackConfig, logger *slog.Logger) *ModelFailoverManager {
	if config.Cooldowns == (CooldownConfig{}) {
		config.Cooldowns = DefaultCooldownConfig()
	}
	if len(config.Fallbacks) == 0 {
		logger.Warn("no fallback models configured — rate limits will cause downtime", "primary", config.Primary)
	}

	return &ModelFailoverManager{
		config:      config,
		cooldowns:   make(map[string]*ModelCooldown),
		lastProbeAt: make(map[string]time.Time),
		logger:      logger,
	}
}

// SelectModel returns the best available model. It checks the primary first,
// then iterates through fallbacks, skipping any that are in cooldown.
// Returns the model name and whether it's the primary.
//
// When the primary is in cooldown but near expiry (within probeMargin),
// the caller can check ShouldProbe() to decide whether to attempt a
// recovery probe before committing to a fallback model.
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

// SelectFromChain tries models from a custom chain, skipping those in cooldown.
// Returns the first available model, or empty string if all are in cooldown.
func (m *ModelFailoverManager) SelectFromChain(chain []string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, model := range chain {
		if model != "" && !m.isInCooldownLocked(model) {
			return model
		}
	}
	return ""
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
	return m.ReportFailureWithCause(model, statusCode, errMsg, nil)
}

// ReportFailureWithCause is like ReportFailure but also traverses the error
// cause chain for more accurate classification of wrapped errors.
func (m *ModelFailoverManager) ReportFailureWithCause(model string, statusCode int, errMsg string, cause error) FailoverReason {
	reason := ClassifyErrorFull(statusCode, errMsg, cause)
	m.ApplyClassifiedFailure(model, reason)
	return reason
}

// ApplyClassifiedFailure applies a pre-classified failure reason to a model.
// Use this when the error has already been classified (e.g., by the FailoverCoordinator)
// to avoid redundant re-classification.
func (m *ModelFailoverManager) ApplyClassifiedFailure(model string, reason FailoverReason) {
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

	case FailoverAuthPermanent:
		// Permanent auth errors (revoked keys, deactivated accounts) — same as billing.
		hours := cfg.BillingBackoffHours
		if hours <= 0 {
			hours = 5
		}
		duration = time.Duration(hours * float64(time.Hour))
		maxHours := cfg.BillingMaxHours
		if maxHours <= 0 {
			maxHours = 24
		}
		if duration > time.Duration(maxHours*float64(time.Hour)) {
			duration = time.Duration(maxHours * float64(time.Hour))
		}

	case FailoverAuth:
		// Transient auth errors — moderate cooldown.
		duration = 1 * time.Hour

	case FailoverSessionExpired:
		// Session/token expired — short cooldown to allow token refresh.
		duration = 5 * time.Minute

	case FailoverModelNotFound:
		// Model doesn't exist for this provider — skip until restart.
		// Use a very long cooldown (48h) to effectively disable the model.
		duration = 48 * time.Hour

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
}

// ShouldProbe returns true if a model is in cooldown but near expiry (within
// probeMargin) and hasn't been probed too recently. This enables auto-recovery
// without waiting for the full cooldown to expire.
func (m *ModelFailoverManager) ShouldProbe(model string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cd, ok := m.cooldowns[model]
	if !ok {
		return false // Not in cooldown.
	}

	now := time.Now()

	// Only probe if cooldown is near expiry (within probeMargin) or already expired.
	timeUntilExpiry := cd.Until.Sub(now)
	if timeUntilExpiry > probeMargin {
		return false
	}

	// Throttle probes to avoid storms.
	if lastProbe, ok := m.lastProbeAt[model]; ok {
		if now.Sub(lastProbe) < probeMinInterval {
			return false
		}
	}

	return true
}

// MarkProbed records that a probe was attempted for a model.
func (m *ModelFailoverManager) MarkProbed(model string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastProbeAt[model] = time.Now()
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

// unwrapCauseChain traverses the Go error chain via errors.Unwrap, returning
// all errors in the chain from outermost to innermost. This handles both
// single-wrap (errors.Unwrap) and multi-error (Unwrap() []error) patterns.
func unwrapCauseChain(err error) []error {
	if err == nil {
		return nil
	}
	var chain []error
	chain = append(chain, err)

	// Single-wrap chain.
	inner := errors.Unwrap(err)
	for inner != nil {
		chain = append(chain, inner)
		inner = errors.Unwrap(inner)
	}

	// Multi-error pattern (Go 1.20+ errors.Join).
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range multi.Unwrap() {
			chain = append(chain, unwrapCauseChain(e)...)
		}
	}

	return chain
}

// ClassifyErrorFull determines the failover reason by examining the HTTP status
// code, error message, AND the full error cause chain. It walks inner causes
// before applying timeout heuristics, so a RESOURCE_EXHAUSTED wrapped in an
// AbortError is correctly classified as rate_limit, not timeout.
func ClassifyErrorFull(statusCode int, errMsg string, cause error) FailoverReason {
	// First try the surface-level classification.
	reason := ClassifyError(statusCode, errMsg)
	if reason != FailoverUnknown {
		return reason
	}

	// If surface classification failed, walk the error cause chain.
	if cause != nil {
		for _, inner := range unwrapCauseChain(cause) {
			if inner == nil {
				continue
			}
			innerReason := ClassifyError(0, inner.Error())
			if innerReason != FailoverUnknown {
				return innerReason
			}
		}
	}

	return FailoverUnknown
}

// ClassifyError determines the failover reason from an HTTP status code and error message.
func ClassifyError(statusCode int, errMsg string) FailoverReason {
	switch statusCode {
	case 402:
		return FailoverBilling
	case 429:
		return FailoverRateLimit
	case 529:
		// Anthropic overloaded — treat as rate limit (not server error).
		return FailoverRateLimit
	case 401:
		return FailoverAuth
	case 403:
		return FailoverAuthPermanent
	case 408:
		return FailoverTimeout
	case 400:
		return FailoverFormat
	}

	// Cloudflare CDN errors (521-530) — transient, treat as timeout.
	if statusCode >= 521 && statusCode <= 530 {
		return FailoverTimeout
	}

	if statusCode >= 500 {
		return FailoverServer
	}

	// Check error message patterns.
	lower := strings.ToLower(errMsg)

	// Check for provider-specific symbolic error codes before heuristic matching.
	// These are identifiers like RESOURCE_EXHAUSTED (Google/gRPC), ThrottlingException (AWS),
	// etc. that may appear in error messages without a corresponding HTTP status code.
	for code, reason := range symbolicErrorCodes {
		if strings.Contains(lower, code) {
			return reason
		}
	}

	switch {
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
		return FailoverTimeout
	case strings.Contains(lower, "empty") && strings.Contains(lower, "chunk"):
		return FailoverTimeout
	case strings.Contains(lower, "ended without sending any chunks"):
		return FailoverTimeout
	case strings.Contains(lower, "overloaded") || strings.Contains(lower, "service unavailable") ||
		strings.Contains(lower, "high demand"):
		return FailoverRateLimit
	case strings.Contains(lower, "billing") || strings.Contains(lower, "payment"):
		return FailoverBilling
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "rate_limit") ||
		strings.Contains(lower, "too many requests"):
		return FailoverRateLimit
	case strings.Contains(lower, "session expired") || strings.Contains(lower, "session_expired") ||
		strings.Contains(lower, "token expired") || strings.Contains(lower, "token has been revoked") ||
		strings.Contains(lower, "token revoked") || strings.Contains(lower, "refresh token") ||
		strings.Contains(lower, "session invalid"):
		return FailoverSessionExpired
	case strings.Contains(lower, "model not found") || strings.Contains(lower, "model_not_found") ||
		strings.Contains(lower, "does not exist") || strings.Contains(lower, "no such model") ||
		strings.Contains(lower, "unknown model"):
		return FailoverModelNotFound
	// Auth permanent — API key revoked, account deactivated, permission denied.
	case strings.Contains(lower, "api key revoked") || strings.Contains(lower, "key has been disabled") ||
		strings.Contains(lower, "key has been revoked") || strings.Contains(lower, "account deactivated") ||
		strings.Contains(lower, "account has been deactivated") ||
		strings.Contains(lower, "permission_error") || strings.Contains(lower, "not allowed for this organization"):
		return FailoverAuthPermanent
	}

	return FailoverUnknown
}

// IsLikelyContextOverflowError returns true if the error message indicates a context
// window overflow. Context overflow errors should NOT trigger model failover because
// rotating to a model with a potentially smaller context window makes things worse.
// Aligned with OpenClaw's isLikelyContextOverflowError + isContextOverflowError.
func IsLikelyContextOverflowError(errMsg string) bool {
	lower := strings.ToLower(errMsg)

	// Exclude rate limit TPM errors that mention "tokens" — these are NOT overflow.
	if strings.Contains(lower, "rate limit") || strings.Contains(lower, "rate_limit") ||
		strings.Contains(lower, "tokens per minute") || strings.Contains(lower, "tpm") {
		return false
	}

	// Exclude reasoning constraint errors (e.g. "reasoning effort too high").
	if strings.Contains(lower, "reasoning") && (strings.Contains(lower, "constraint") || strings.Contains(lower, "budget")) {
		return false
	}

	// Exclude "context window too small" (model selection issue, not overflow).
	if strings.Contains(lower, "too small") {
		return false
	}

	// Strict matches (high confidence).
	if strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "context length exceeded") ||
		strings.Contains(lower, "request_too_large") ||
		strings.Contains(lower, "prompt is too long") ||
		strings.Contains(lower, "exceeds model context window") ||
		strings.Contains(lower, "maximum context length") ||
		strings.Contains(lower, "request size exceeds") ||
		strings.Contains(lower, "context_window_exceeded") {
		return true
	}

	// Chinese provider messages.
	if strings.Contains(errMsg, "上下文过长") || strings.Contains(errMsg, "超出最大上下文") {
		return true
	}

	// Heuristic: "context window" + overflow-like term.
	if strings.Contains(lower, "context window") && (strings.Contains(lower, "too large") ||
		strings.Contains(lower, "too long") || strings.Contains(lower, "exceed") ||
		strings.Contains(lower, "overflow") || strings.Contains(lower, "limit")) {
		return true
	}

	// Heuristic: "input" or "request" + "context" + overflow-like term.
	if (strings.Contains(lower, "input") || strings.Contains(lower, "request")) &&
		strings.Contains(lower, "context") &&
		(strings.Contains(lower, "too large") || strings.Contains(lower, "exceed") || strings.Contains(lower, "limit")) {
		return true
	}

	return false
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
