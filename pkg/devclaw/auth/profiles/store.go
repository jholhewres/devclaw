package profiles

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/oauth"
)

// vaultStore implements ProfileManager using the encrypted vault for storage.
type vaultStore struct {
	mu        sync.RWMutex
	profiles  *ProfileStore
	vault     VaultInterface
	oauthMgr  OAuthManager
	logger    *slog.Logger
	cachePath string // Optional: cache decrypted profiles for faster access
}

// VaultInterface defines the interface for vault operations.
type VaultInterface interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Has(name string) bool
	Delete(name string) error
	IsUnlocked() bool
}

// OAuthManager defines the interface for OAuth operations.
type OAuthManager interface {
	GetValidToken(provider string) (*oauth.OAuthCredential, error)
	SaveCredential(cred *oauth.OAuthCredential) error
}

// StoreConfig contains configuration for the profile store.
type StoreConfig struct {
	// Vault is the encrypted vault interface.
	Vault VaultInterface

	// OAuthManager is the OAuth token manager.
	OAuthManager OAuthManager

	// Logger for logging operations.
	Logger *slog.Logger

	// CachePath is an optional path to cache decrypted profiles.
	// If empty, no disk cache is used.
	CachePath string
}

// NewStore creates a new profile store backed by the encrypted vault.
func NewStore(cfg StoreConfig) (*vaultStore, error) {
	if cfg.Vault == nil {
		return nil, fmt.Errorf("vault is required")
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	s := &vaultStore{
		profiles:  NewProfileStore(),
		vault:     cfg.Vault,
		oauthMgr:  cfg.OAuthManager,
		logger:    cfg.Logger.With("component", "auth-profiles"),
		cachePath: cfg.CachePath,
	}

	// Load existing profiles
	if err := s.load(); err != nil {
		// If vault is locked, we'll start with empty profiles
		if s.vault.IsUnlocked() {
			return nil, fmt.Errorf("failed to load profiles: %w", err)
		}
		s.logger.Warn("vault is locked, starting with empty profiles")
	}

	return s, nil
}

// vaultKey is the key used to store profiles in the vault.
const vaultKey = "auth:profiles:store"

// load loads profiles from the vault.
func (s *vaultStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.vault.IsUnlocked() {
		return fmt.Errorf("vault is locked")
	}

	// Try to load from vault
	data, err := s.vault.Get(vaultKey)
	if err != nil {
		return fmt.Errorf("failed to read from vault: %w", err)
	}

	if data == "" {
		// No profiles stored yet
		s.profiles = NewProfileStore()
		return nil
	}

	// Parse profiles
	var store ProfileStore
	if err := json.Unmarshal([]byte(data), &store); err != nil {
		return fmt.Errorf("failed to parse profiles: %w", err)
	}

	// Ensure providers map is initialized
	if store.Profiles == nil {
		store.Profiles = make(map[string]*AuthProfile)
	}

	s.profiles = &store
	s.logger.Debug("loaded profiles from vault", "count", len(store.Profiles))

	return nil
}

// save saves profiles to the vault.
func (s *vaultStore) save() error {
	if !s.vault.IsUnlocked() {
		return fmt.Errorf("vault is locked")
	}

	data, err := json.Marshal(s.profiles)
	if err != nil {
		return fmt.Errorf("failed to marshal profiles: %w", err)
	}

	if err := s.vault.Set(vaultKey, string(data)); err != nil {
		return fmt.Errorf("failed to write to vault: %w", err)
	}

	// Optionally write to cache
	if s.cachePath != "" {
		if err := s.writeCache(data); err != nil {
			s.logger.Warn("failed to write profile cache", "error", err)
		}
	}

	return nil
}

// writeCache writes profiles to the cache file.
func (s *vaultStore) writeCache(data []byte) error {
	dir := filepath.Dir(s.cachePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	if err := os.WriteFile(s.cachePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// Get retrieves a profile by ID.
func (s *vaultStore) Get(id ProfileID) (*AuthProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	profile, ok := s.profiles.Get(id)
	return profile, ok
}

// GetByProvider retrieves all profiles for a provider, sorted by preference.
func (s *vaultStore) GetByProvider(provider string, preference ProfilePreference) []*AuthProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()

	profiles := s.profiles.ListByProvider(provider)

	// Sort based on preference
	switch preference {
	case PreferValid:
		sort.Slice(profiles, func(i, j int) bool {
			// Valid profiles first
			iValid := profiles[i].IsValid()
			jValid := profiles[j].IsValid()
			if iValid != jValid {
				return iValid && !jValid
			}
			// Then by priority (higher first)
			if profiles[i].Priority != profiles[j].Priority {
				return profiles[i].Priority > profiles[j].Priority
			}
			// Then by last used (recent first)
			if profiles[i].LastUsedAt != nil && profiles[j].LastUsedAt != nil {
				return profiles[i].LastUsedAt.After(*profiles[j].LastUsedAt)
			}
			return profiles[i].LastUsedAt != nil
		})
	case PreferRecent:
		sort.Slice(profiles, func(i, j int) bool {
			// Recent first
			if profiles[i].LastUsedAt != nil && profiles[j].LastUsedAt != nil {
				return profiles[i].LastUsedAt.After(*profiles[j].LastUsedAt)
			}
			return profiles[i].LastUsedAt != nil
		})
	default: // PreferDefault
		sort.Slice(profiles, func(i, j int) bool {
			// Default profile first
			if profiles[i].Name == "default" {
				return true
			}
			if profiles[j].Name == "default" {
				return false
			}
			// Then by priority (higher first)
			if profiles[i].Priority != profiles[j].Priority {
				return profiles[i].Priority > profiles[j].Priority
			}
			return profiles[i].Name < profiles[j].Name
		})
	}

	return profiles
}

// Save stores a profile.
func (s *vaultStore) Save(profile *AuthProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profile.UpdatedAt = time.Now()
	s.profiles.Set(profile)

	if err := s.save(); err != nil {
		return fmt.Errorf("failed to save profile %s: %w", profile.ID, err)
	}

	s.logger.Info("saved profile", "id", profile.ID, "provider", profile.Provider, "mode", profile.Mode)
	return nil
}

// Delete removes a profile.
func (s *vaultStore) Delete(id ProfileID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profile, ok := s.profiles.Get(id)
	if !ok {
		return fmt.Errorf("profile %s not found", id)
	}

	s.profiles.Delete(id)

	if err := s.save(); err != nil {
		return fmt.Errorf("failed to delete profile %s: %w", id, err)
	}

	// Also delete from OAuth manager if OAuth profile
	if profile.Mode == ModeOAuth && profile.OAuth != nil && s.oauthMgr != nil {
		// Note: OAuthManager doesn't have a delete method, but we could add one
	}

	s.logger.Info("deleted profile", "id", id)
	return nil
}

// List returns all profiles.
func (s *vaultStore) List() []*AuthProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.profiles.List()
}

// Resolve resolves a profile based on options, with automatic fallback.
func (s *vaultStore) Resolve(opts ResolutionOptions) *ProfileResolutionResult {
	result := &ProfileResolutionResult{
		Provider: opts.Provider,
	}

	// If a specific profile is requested, try that first
	if opts.PreferredProfile != "" {
		id := NewProfileID(opts.Provider, opts.PreferredProfile)
		result.AttemptedProfiles = append(result.AttemptedProfiles, id)

		if profile, ok := s.Get(id); ok {
			if resolved := s.resolveProfile(profile, opts); resolved.Error == nil {
				return resolved
			} else {
				result.Error = resolved.Error
			}
		}
	}

	// Get all profiles for the provider
	profiles := s.GetByProvider(opts.Provider, opts.Preference)

	// Try each profile
	for _, profile := range profiles {
		// Skip if we already tried this one
		if slices.Contains(result.AttemptedProfiles, profile.ID) {
			continue
		}

		result.AttemptedProfiles = append(result.AttemptedProfiles, profile.ID)

		// Check mode restriction
		if opts.Mode != "" && profile.Mode != opts.Mode {
			continue
		}

		// Check validity requirement
		if opts.RequireValid && !profile.IsValid() {
			continue
		}

		resolved := s.resolveProfile(profile, opts)
		if resolved.Error == nil {
			return resolved
		}

		// Save the first error
		if result.Error == nil {
			result.Error = resolved.Error
		}
	}

	// No profile resolved
	if result.Error == nil {
		result.Error = fmt.Errorf("no valid profile found for provider %s", opts.Provider)
	}

	return result
}

// resolveProfile attempts to resolve credentials for a single profile.
func (s *vaultStore) resolveProfile(profile *AuthProfile, _ ResolutionOptions) *ProfileResolutionResult {
	result := &ProfileResolutionResult{
		Profile:  profile,
		Provider: profile.Provider,
	}

	switch profile.Mode {
	case ModeAPIKey:
		key, err := profile.GetAPIKey(s.vault.Get)
		if err != nil {
			result.Error = fmt.Errorf("failed to get API key for profile %s: %w", profile.ID, err)
			profile.MarkError(err)
			return result
		}
		result.Credential = key

	case ModeToken:
		token, err := profile.GetToken(s.vault.Get)
		if err != nil {
			result.Error = fmt.Errorf("failed to get token for profile %s: %w", profile.ID, err)
			profile.MarkError(err)
			return result
		}
		result.Credential = token

	case ModeOAuth:
		if profile.OAuth == nil {
			result.Error = fmt.Errorf("OAuth credentials not set for profile %s", profile.ID)
			profile.MarkError(result.Error)
			return result
		}

		// Check if we need to refresh
		if profile.OAuth.IsExpired(5 * time.Minute) {
			if s.oauthMgr == nil {
				result.Error = fmt.Errorf("OAuth manager not available for profile %s", profile.ID)
				profile.MarkError(result.Error)
				return result
			}

			// Try to get a valid token (this will refresh if needed)
			oauthCred, err := s.oauthMgr.GetValidToken(profile.Provider)
			if err != nil {
				result.Error = fmt.Errorf("failed to refresh OAuth token for profile %s: %w", profile.ID, err)
				profile.MarkError(err)
				return result
			}

			// Update the profile with refreshed credentials
			profile.OAuth.FromOAuthCredential(oauthCred, profile.Name)
			profile.ClearError()

			// Save the updated profile
			if err := s.Save(profile); err != nil {
				s.logger.Warn("failed to save refreshed profile", "id", profile.ID, "error", err)
			}
		}

		result.Credential = profile.OAuth.AccessToken
		result.Email = profile.OAuth.Email

	default:
		result.Error = fmt.Errorf("unknown auth mode %s for profile %s", profile.Mode, profile.ID)
		profile.MarkError(result.Error)
		return result
	}

	profile.ClearError()
	return result
}

// MarkUsed updates the last used timestamp for a profile.
func (s *vaultStore) MarkUsed(id ProfileID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if profile, ok := s.profiles.Get(id); ok {
		profile.MarkUsed()
		// Don't save here to avoid overhead - save on next explicit Save() call
	}
}

// MarkError records an error for a profile.
func (s *vaultStore) MarkError(id ProfileID, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if profile, ok := s.profiles.Get(id); ok {
		profile.MarkError(err)
		// Don't save here to avoid overhead
	}
}

// SyncWithOAuth syncs OAuth profiles with the OAuth manager.
func (s *vaultStore) SyncWithOAuth() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.oauthMgr == nil {
		return nil
	}

	for _, profile := range s.profiles.Profiles {
		if profile.Mode != ModeOAuth || profile.OAuth == nil {
			continue
		}

		// Check if the OAuth manager has newer credentials
		oauthCred, err := s.oauthMgr.GetValidToken(profile.Provider)
		if err != nil {
			// OAuth manager doesn't have this provider
			continue
		}

		// Check if the OAuth manager has fresher credentials
		if oauthCred.ExpiresAt.After(profile.OAuth.ExpiresAt) {
			profile.OAuth.FromOAuthCredential(oauthCred, profile.Name)
			profile.ClearError()
			s.logger.Info("synced OAuth credentials from manager",
				"profile", profile.ID,
				"expires", oauthCred.ExpiresAt)
		}
	}

	return s.save()
}

// ImportFromOAuthManager imports all OAuth credentials from the OAuth manager as profiles.
func (s *vaultStore) ImportFromOAuthManager() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.oauthMgr == nil {
		return fmt.Errorf("OAuth manager not available")
	}

	// This would require a ListProviders method on OAuthManager
	// For now, this is a placeholder for future implementation
	return nil
}

// GetStoreStats returns statistics about the profile store.
func (s *vaultStore) GetStoreStats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := map[string]any{
		"total_profiles": len(s.profiles.Profiles),
		"by_provider":    make(map[string]int),
		"by_mode":        make(map[string]int),
		"valid":          0,
		"expired":        0,
		"disabled":       0,
	}

	byProvider := stats["by_provider"].(map[string]int)
	byMode := stats["by_mode"].(map[string]int)

	for _, profile := range s.profiles.Profiles {
		byProvider[profile.Provider]++
		byMode[string(profile.Mode)]++

		if !profile.Enabled {
			stats["disabled"] = stats["disabled"].(int) + 1
		} else if profile.IsValid() {
			stats["valid"] = stats["valid"].(int) + 1
		} else if profile.IsExpired() {
			stats["expired"] = stats["expired"].(int) + 1
		}
	}

	return stats
}
