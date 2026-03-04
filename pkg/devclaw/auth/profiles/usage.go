package profiles

import (
	"math"
	"time"
)

const (
	// rateLimitCooldown is a flat cooldown for rate limit errors.
	// Aligned with PR #31962: providers typically clear rate limits within ~60s,
	// so exponential backoff (which could lock for 25+ min) is excessive.
	rateLimitCooldown = 30 * time.Second

	// billingBaseBackoff is the base backoff for billing/auth_permanent failures.
	billingBaseBackoff = 5 * time.Hour
	// billingMaxBackoff caps billing disable duration.
	billingMaxBackoff = 24 * time.Hour

	// failureWindowDuration is the window after which error counters decay to zero.
	failureWindowDuration = 24 * time.Hour
)

// CalculateRateLimitCooldown returns the cooldown for rate limit errors.
// If cfg is non-nil and RateLimitSeconds > 0, uses that value; otherwise falls back to 30s.
func CalculateRateLimitCooldown(cfg *ProfileCooldownConfig) time.Duration {
	if cfg != nil && cfg.RateLimitSeconds > 0 {
		return time.Duration(cfg.RateLimitSeconds) * time.Second
	}
	return rateLimitCooldown
}

// CalculateBillingDisable returns the disable duration for billing/auth_permanent errors.
// Exponential: base * 2^(n-1), capped at max.
// If cfg is non-nil, uses configured backoff/max hours; otherwise uses defaults.
func CalculateBillingDisable(errorCount int, cfg *ProfileCooldownConfig) time.Duration {
	base := billingBaseBackoff
	maxBackoff := billingMaxBackoff
	if cfg != nil {
		if cfg.BillingBackoffHours > 0 {
			base = time.Duration(cfg.BillingBackoffHours * float64(time.Hour))
		}
		if cfg.BillingMaxHours > 0 {
			maxBackoff = time.Duration(cfg.BillingMaxHours * float64(time.Hour))
		}
	}

	n := max(1, errorCount)
	exp := min(n-1, 10)
	raw := float64(base) * math.Pow(2, float64(exp))
	d := time.Duration(raw)
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}

// failureWindowFromConfig returns the failure window duration from config or default.
func failureWindowFromConfig(cfg *ProfileCooldownConfig) time.Duration {
	if cfg != nil && cfg.FailureWindowHours > 0 {
		return time.Duration(cfg.FailureWindowHours * float64(time.Hour))
	}
	return failureWindowDuration
}

// MarkProfileFailure records a categorized failure for a profile.
// Rate limit -> short flat cooldown. Billing/auth_permanent -> exponential disable.
// Other transient errors -> no cooldown (just tracked).
// cfg is optional — pass nil to use hardcoded defaults.
func MarkProfileFailure(store *ProfileStore, id ProfileID, reason FailureReason, cfg *ProfileCooldownConfig) {
	stats := store.GetUsageStats(id)
	now := time.Now()

	// Decay: reset counters if last failure was outside the window.
	window := failureWindowFromConfig(cfg)
	if stats.LastFailureAt != nil && now.Sub(*stats.LastFailureAt) > window {
		stats.ErrorCount = 0
		stats.FailureCounts = nil
	}

	stats.ErrorCount++
	stats.LastFailureAt = &now
	if stats.FailureCounts == nil {
		stats.FailureCounts = make(map[FailureReason]int)
	}
	stats.FailureCounts[reason]++

	switch reason {
	case FailureRateLimit:
		cd := now.Add(CalculateRateLimitCooldown(cfg))
		stats.CooldownUntil = keepActiveWindowOrSet(stats.CooldownUntil, now, cd)

	case FailureSessionExpired:
		// Session expired is auth-transient: short cooldown to allow token refresh.
		cd := now.Add(CalculateRateLimitCooldown(cfg))
		stats.CooldownUntil = keepActiveWindowOrSet(stats.CooldownUntil, now, cd)

	case FailureBilling, FailureAuthPermanent:
		dd := now.Add(CalculateBillingDisable(stats.ErrorCount, cfg))
		stats.DisabledUntil = keepActiveWindowOrSet(stats.DisabledUntil, now, dd)
		stats.DisabledReason = reason
	}
}

// IsProfileInCooldown returns true if the profile is in cooldown or disabled.
// OpenRouter profiles bypass auth cooldowns because OpenRouter handles
// authentication routing internally across multiple providers.
func IsProfileInCooldown(store *ProfileStore, id ProfileID) bool {
	if IsAuthCooldownBypassedForProfile(store, id) {
		return false
	}
	return ResolveProfileUnusableUntil(store, id) != nil
}

// IsAuthCooldownBypassedForProfile returns true if the profile's provider
// should bypass auth-related cooldowns. Currently only OpenRouter qualifies
// because it routes internally and auth errors for one backend don't affect others.
func IsAuthCooldownBypassedForProfile(store *ProfileStore, id ProfileID) bool {
	if store == nil || store.Profiles == nil {
		return false
	}
	for _, p := range store.Profiles {
		if p.ID == id {
			return isAuthCooldownBypassedProvider(p.Provider)
		}
	}
	return false
}

// isAuthCooldownBypassedProvider returns true for providers that should bypass
// auth-related cooldowns.
func isAuthCooldownBypassedProvider(provider string) bool {
	return provider == "openrouter"
}

// ResolveProfileUnusableUntil returns the timestamp until which the profile is unusable,
// or nil if the profile is available.
func ResolveProfileUnusableUntil(store *ProfileStore, id ProfileID) *time.Time {
	if store.UsageStats == nil {
		return nil
	}
	stats, ok := store.UsageStats[string(id)]
	if !ok {
		return nil
	}

	now := time.Now()
	var latest *time.Time

	if stats.CooldownUntil != nil && stats.CooldownUntil.After(now) {
		latest = stats.CooldownUntil
	}
	if stats.DisabledUntil != nil && stats.DisabledUntil.After(now) {
		if latest == nil || stats.DisabledUntil.After(*latest) {
			latest = stats.DisabledUntil
		}
	}

	return latest
}

// ClearExpiredCooldowns resets cooldown/disabled state for profiles whose windows have passed.
// Returns true if any state was cleared.
func ClearExpiredCooldowns(store *ProfileStore) bool {
	if store.UsageStats == nil {
		return false
	}

	now := time.Now()
	anyChanged := false

	for _, stats := range store.UsageStats {
		cleared := false
		if stats.CooldownUntil != nil && !stats.CooldownUntil.After(now) {
			stats.CooldownUntil = nil
			cleared = true
		}
		if stats.DisabledUntil != nil && !stats.DisabledUntil.After(now) {
			stats.DisabledUntil = nil
			stats.DisabledReason = ""
			cleared = true
		}
		if cleared {
			anyChanged = true
			if stats.CooldownUntil == nil && stats.DisabledUntil == nil {
				stats.ErrorCount = 0
				stats.FailureCounts = nil
			}
		}
	}

	return anyChanged
}

// keepActiveWindowOrSet preserves an existing active window, or sets the new one.
// This prevents retries from extending cooldown windows.
func keepActiveWindowOrSet(existing *time.Time, now time.Time, computed time.Time) *time.Time {
	if existing != nil && existing.After(now) {
		return existing
	}
	return &computed
}
