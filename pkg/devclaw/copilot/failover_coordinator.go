// Package copilot – failover_coordinator.go provides a unified coordinator
// for profile authentication and model failover. This replaces the fragmented
// approach of 3 independent systems (profile cooldown, model failover, LLM inline cooldown)
// with a single consistent error classification and response strategy.
package copilot

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/jholhewres/devclaw/pkg/devclaw/auth/profiles"
)

// FailoverCoordinator unifies profile management and model failover into a single
// system with consistent error classification and cooldown management.
type FailoverCoordinator struct {
	profileMgr profiles.ProfileManager
	modelMgr   *ModelFailoverManager
	logger     *slog.Logger
	mu         sync.RWMutex
}

// NewFailoverCoordinator creates a coordinator that wraps profile and model managers.
func NewFailoverCoordinator(profileMgr profiles.ProfileManager, modelCfg ModelFallbackConfig, logger *slog.Logger) *FailoverCoordinator {
	var modelMgr *ModelFailoverManager
	if modelCfg.Primary != "" {
		modelMgr = NewModelFailoverManager(modelCfg, logger)
	}
	return &FailoverCoordinator{
		profileMgr: profileMgr,
		modelMgr:   modelMgr,
		logger:     logger.With("component", "failover-coordinator"),
	}
}

// SelectModelAndProfile resolves the best model and profile for a request.
// Returns the selected model, profile ID, API key, and any error.
// Optional fallbacksOverride temporarily overrides the fallback chain for this
// single request, useful for per-channel or per-skill model preferences.
func (fc *FailoverCoordinator) SelectModelAndProfile(provider, modelOverride string, fallbacksOverride ...string) (model string, profileID profiles.ProfileID, apiKey string, err error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	// Resolve model (failover chain).
	if modelOverride != "" {
		model = modelOverride
	} else if fc.modelMgr != nil {
		// If fallbacksOverride is provided, try those first before the standard chain.
		if len(fallbacksOverride) > 0 {
			// Use the override chain: try each in order, fall back to standard selection.
			model = fc.modelMgr.SelectFromChain(fallbacksOverride)
		}
		if model == "" {
			model, _ = fc.modelMgr.SelectModel()
		}
	}

	// Resolve profile for credentials.
	// When no profiles exist for the provider, fall through silently
	// so the caller can use the API key from config/env instead.
	if fc.profileMgr != nil {
		result := fc.profileMgr.Resolve(profiles.ResolutionOptions{
			Provider: provider,
		})
		if result != nil {
			if result.Error != nil {
				// Only treat as fatal if profiles were actually attempted
				// (i.e., profiles exist but all failed). When no profiles
				// exist at all, this is expected — not an error.
				if len(result.AttemptedProfiles) > 0 {
					return model, "", "", fmt.Errorf("resolving profile for %s: %w", provider, result.Error)
				}
				// No profiles for this provider — fall through to use
				// API key from config or environment.
			}
			if result.Profile != nil {
				profileID = result.Profile.ID
				// Use the resolved credential (handles vault KeyRef resolution),
				// not the raw APIKey.Key field.
				apiKey = result.Credential
			}
		}
	}

	return model, profileID, apiKey, nil
}

// ReportSuccess records a successful call for both model and profile.
func (fc *FailoverCoordinator) ReportSuccess(model string, profileID profiles.ProfileID) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.modelMgr != nil && model != "" {
		fc.modelMgr.ReportSuccess(model)
	}
	if fc.profileMgr != nil && profileID != "" {
		fc.profileMgr.MarkUsed(profileID)
	}
}

// ReportFailure classifies an error ONCE and applies consistent cooldowns
// to both the model and profile systems. This eliminates the inconsistency
// where the same error could be classified differently by each system.
func (fc *FailoverCoordinator) ReportFailure(model string, profileID profiles.ProfileID, statusCode int, errMsg string) FailoverReason {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Classify once — single source of truth for both systems.
	reason := ClassifyError(statusCode, errMsg)

	// Apply pre-classified reason to model failover (avoids re-classification).
	if fc.modelMgr != nil && model != "" {
		fc.modelMgr.ApplyClassifiedFailure(model, reason)
	}

	// Map failover reason to profile failure reason and apply.
	if fc.profileMgr != nil && profileID != "" {
		profileReason := mapFailoverToProfileReason(reason)
		fc.profileMgr.MarkFailure(profileID, profileReason)
	}

	fc.logger.Info("unified failure reported",
		"model", model,
		"profile", profileID,
		"status_code", statusCode,
		"reason", reason,
	)

	return reason
}

// mapFailoverToProfileReason converts a FailoverReason to a profiles.FailureReason
// for consistent cross-system classification.
func mapFailoverToProfileReason(reason FailoverReason) profiles.FailureReason {
	switch reason {
	case FailoverBilling:
		return profiles.FailureBilling
	case FailoverRateLimit:
		return profiles.FailureRateLimit
	case FailoverAuth:
		return profiles.FailureAuth
	case FailoverAuthPermanent:
		return profiles.FailureAuthPermanent
	case FailoverSessionExpired:
		return profiles.FailureSessionExpired
	case FailoverTimeout:
		return profiles.FailureTimeout
	case FailoverFormat:
		return profiles.FailureFormat
	case FailoverModelNotFound:
		return profiles.FailureModelNotFound
	case FailoverServer:
		return profiles.FailureUnknown // Server errors don't affect profile
	default:
		return profiles.FailureUnknown
	}
}

// GetModelManager returns the underlying model failover manager (for direct access when needed).
func (fc *FailoverCoordinator) GetModelManager() *ModelFailoverManager {
	return fc.modelMgr
}

// HasProfileManager returns true if a profile manager is configured.
func (fc *FailoverCoordinator) HasProfileManager() bool {
	return fc.profileMgr != nil
}
