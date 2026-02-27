// Package profiles provides multi-account authentication management for DevClaw.
// It allows multiple credentials per provider (e.g., google:work, google:personal)
// with automatic fallback and integration with the encrypted vault.
package profiles

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/oauth"
)

// AuthMode represents the authentication method for a profile.
type AuthMode string

const (
	// ModeAPIKey uses a static API key.
	ModeAPIKey AuthMode = "api_key"
	// ModeOAuth uses OAuth 2.0 with refresh tokens.
	ModeOAuth AuthMode = "oauth"
	// ModeToken uses a static bearer token (no refresh).
	ModeToken AuthMode = "token"
)

// ProfileID uniquely identifies an auth profile.
// Format: "{provider}:{name}" e.g., "google:work", "openai:default"
type ProfileID string

// String returns the string representation of the profile ID.
func (p ProfileID) String() string {
	return string(p)
}

// Provider returns the provider part of the profile ID.
func (p ProfileID) Provider() string {
	s := string(p)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return s[:i]
		}
	}
	return s
}

// Name returns the name part of the profile ID.
func (p ProfileID) Name() string {
	s := string(p)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return s[i+1:]
		}
	}
	return "default"
}

// NewProfileID creates a profile ID from provider and name.
func NewProfileID(provider, name string) ProfileID {
	if name == "" {
		name = "default"
	}
	return ProfileID(fmt.Sprintf("%s:%s", provider, name))
}

// APIKeyCredential represents a static API key authentication.
type APIKeyCredential struct {
	// Key is the API key value.
	Key string `json:"key,omitempty"`

	// KeyRef is a vault reference for the key (e.g., "vault://OPENAI_API_KEY").
	KeyRef string `json:"key_ref,omitempty"`
}

// TokenCredential represents a static bearer token authentication.
type TokenCredential struct {
	// Token is the bearer token value.
	Token string `json:"token,omitempty"`

	// TokenRef is a vault reference for the token.
	TokenRef string `json:"token_ref,omitempty"`

	// ExpiresAt is when the token expires (optional).
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// OAuthCredential wraps the oauth.OAuthCredential with profile-specific fields.
type OAuthCredential struct {
	// Provider identifies the OAuth provider.
	Provider string `json:"provider"`

	// ProfileName is the name of this profile (e.g., "work", "personal").
	ProfileName string `json:"profile_name"`

	// AccessToken is the OAuth access token.
	AccessToken string `json:"access_token"`

	// RefreshToken is used to obtain new access tokens.
	RefreshToken string `json:"refresh_token"`

	// ExpiresAt is when the access token expires.
	ExpiresAt time.Time `json:"expires_at"`

	// Email is the user's email associated with the OAuth account.
	Email string `json:"email,omitempty"`

	// ClientID is the OAuth client ID.
	ClientID string `json:"client_id,omitempty"`

	// Scopes are the OAuth scopes granted.
	Scopes []string `json:"scopes,omitempty"`

	// Metadata contains provider-specific data.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// IsExpired returns true if the credential has expired or will expire within the buffer.
func (c *OAuthCredential) IsExpired(buffer time.Duration) bool {
	return time.Now().Add(buffer).After(c.ExpiresAt)
}

// IsValid returns true if the credential has a valid access token.
func (c *OAuthCredential) IsValid() bool {
	return c.AccessToken != "" && !c.IsExpired(5*time.Minute)
}

// ToOAuthCredential converts to the oauth package format.
func (c *OAuthCredential) ToOAuthCredential() *oauth.OAuthCredential {
	return &oauth.OAuthCredential{
		Provider:     c.Provider,
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		ExpiresAt:    c.ExpiresAt,
		Email:        c.Email,
		ClientID:     c.ClientID,
		Metadata:     c.Metadata,
	}
}

// FromOAuthCredential populates from an oauth package credential.
func (c *OAuthCredential) FromOAuthCredential(oc *oauth.OAuthCredential, profileName string) {
	c.Provider = oc.Provider
	c.ProfileName = profileName
	c.AccessToken = oc.AccessToken
	c.RefreshToken = oc.RefreshToken
	c.ExpiresAt = oc.ExpiresAt
	c.Email = oc.Email
	c.ClientID = oc.ClientID
	c.Metadata = oc.Metadata
}

// AuthProfile represents a named authentication profile for a provider.
type AuthProfile struct {
	// ID is the unique profile identifier (provider:name).
	ID ProfileID `json:"id"`

	// Provider is the authentication provider (e.g., "google", "openai").
	Provider string `json:"provider"`

	// Name is the profile name (e.g., "default", "work", "personal").
	Name string `json:"name"`

	// Mode is the authentication method.
	Mode AuthMode `json:"mode"`

	// Priority determines fallback order (higher = tried first).
	Priority int `json:"priority,omitempty"`

	// Enabled indicates if this profile is active.
	Enabled bool `json:"enabled"`

	// CreatedAt is when the profile was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the profile was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// LastUsedAt is when the profile was last successfully used.
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`

	// LastError is the last error message if authentication failed.
	LastError string `json:"last_error,omitempty"`

	// LastErrorAt is when the last error occurred.
	LastErrorAt *time.Time `json:"last_error_at,omitempty"`

	// Credential data based on Mode.
	APIKey *APIKeyCredential `json:"api_key,omitempty"`
	OAuth  *OAuthCredential  `json:"oauth,omitempty"`
	Token  *TokenCredential  `json:"token,omitempty"`
}

// IsValid returns true if the profile is enabled and has valid credentials.
func (p *AuthProfile) IsValid() bool {
	if !p.Enabled {
		return false
	}

	switch p.Mode {
	case ModeAPIKey:
		return p.APIKey != nil && (p.APIKey.Key != "" || p.APIKey.KeyRef != "")
	case ModeOAuth:
		return p.OAuth != nil && p.OAuth.IsValid()
	case ModeToken:
		return p.Token != nil && p.Token.Token != ""
	default:
		return false
	}
}

// IsExpired returns true if the profile's credentials have expired.
func (p *AuthProfile) IsExpired() bool {
	switch p.Mode {
	case ModeOAuth:
		return p.OAuth != nil && p.OAuth.IsExpired(0)
	case ModeToken:
		return p.Token != nil && p.Token.ExpiresAt != nil && time.Now().After(*p.Token.ExpiresAt)
	default:
		return false
	}
}

// GetAPIKey returns the API key value, resolving vault references if needed.
func (p *AuthProfile) GetAPIKey(vaultGetter func(string) (string, error)) (string, error) {
	if p.Mode != ModeAPIKey || p.APIKey == nil {
		return "", fmt.Errorf("profile %s is not an API key profile", p.ID)
	}

	// Direct key takes precedence
	if p.APIKey.Key != "" {
		return p.APIKey.Key, nil
	}

	// Resolve from vault reference
	if p.APIKey.KeyRef != "" && vaultGetter != nil {
		return vaultGetter(p.APIKey.KeyRef)
	}

	return "", fmt.Errorf("no API key available for profile %s", p.ID)
}

// GetToken returns the token value, resolving vault references if needed.
func (p *AuthProfile) GetToken(vaultGetter func(string) (string, error)) (string, error) {
	if p.Mode != ModeToken || p.Token == nil {
		return "", fmt.Errorf("profile %s is not a token profile", p.ID)
	}

	// Direct token takes precedence
	if p.Token.Token != "" {
		return p.Token.Token, nil
	}

	// Resolve from vault reference
	if p.Token.TokenRef != "" && vaultGetter != nil {
		return vaultGetter(p.Token.TokenRef)
	}

	return "", fmt.Errorf("no token available for profile %s", p.ID)
}

// MarkUsed updates the LastUsedAt timestamp.
func (p *AuthProfile) MarkUsed() {
	now := time.Now()
	p.LastUsedAt = &now
}

// MarkError records an authentication error.
func (p *AuthProfile) MarkError(err error) {
	if err == nil {
		return
	}
	now := time.Now()
	p.LastError = err.Error()
	p.LastErrorAt = &now
}

// ClearError clears the last error.
func (p *AuthProfile) ClearError() {
	p.LastError = ""
	p.LastErrorAt = nil
}

// ProfileStore is the on-disk format for auth profiles.
type ProfileStore struct {
	Version  int                    `json:"version"`
	Profiles map[string]*AuthProfile `json:"profiles"` // key is "provider:name"
}

// NewProfileStore creates a new empty profile store.
func NewProfileStore() *ProfileStore {
	return &ProfileStore{
		Version:  1,
		Profiles: make(map[string]*AuthProfile),
	}
}

// Get retrieves a profile by ID.
func (s *ProfileStore) Get(id ProfileID) (*AuthProfile, bool) {
	p, ok := s.Profiles[string(id)]
	return p, ok
}

// Set stores a profile.
func (s *ProfileStore) Set(p *AuthProfile) {
	s.Profiles[string(p.ID)] = p
}

// Delete removes a profile.
func (s *ProfileStore) Delete(id ProfileID) {
	delete(s.Profiles, string(id))
}

// List returns all profiles.
func (s *ProfileStore) List() []*AuthProfile {
	profiles := make([]*AuthProfile, 0, len(s.Profiles))
	for _, p := range s.Profiles {
		profiles = append(profiles, p)
	}
	return profiles
}

// ListByProvider returns all profiles for a specific provider.
func (s *ProfileStore) ListByProvider(provider string) []*AuthProfile {
	var result []*AuthProfile
	for _, p := range s.Profiles {
		if p.Provider == provider {
			result = append(result, p)
		}
	}
	return result
}

// MarshalJSON returns a JSON representation with sorted keys for consistency.
func (s *ProfileStore) MarshalJSON() ([]byte, error) {
	// Use a map with ordered keys for consistent serialization
	type profileStoreJSON struct {
		Version  int                    `json:"version"`
		Profiles map[string]*AuthProfile `json:"profiles"`
	}

	aux := profileStoreJSON{
		Version:  s.Version,
		Profiles: s.Profiles,
	}

	return json.MarshalIndent(aux, "", "  ")
}

// ProfileResolutionResult contains the result of resolving a profile.
type ProfileResolutionResult struct {
	// Profile is the resolved profile (nil if not found).
	Profile *AuthProfile

	// Credential is the resolved credential string (API key, token, or OAuth access token).
	Credential string

	// Provider is the provider name.
	Provider string

	// Email is the associated email (for OAuth).
	Email string

	// Error if resolution failed.
	Error error

	// AttemptedProfiles lists profiles that were tried.
	AttemptedProfiles []ProfileID
}

// ProfilePreference indicates how to select between multiple profiles.
type ProfilePreference string

const (
	// PreferDefault uses the "default" profile or the highest priority.
	PreferDefault ProfilePreference = "default"
	// PreferValid prefers profiles with valid (non-expired) credentials.
	PreferValid ProfilePreference = "valid"
	// PreferRecent prefers the most recently used profile.
	PreferRecent ProfilePreference = "recent"
)

// ResolutionOptions configures profile resolution behavior.
type ResolutionOptions struct {
	// Provider is the required provider (e.g., "google", "openai").
	Provider string

	// PreferredProfile is a specific profile name to use (optional).
	PreferredProfile string

	// Preference indicates how to select between multiple profiles.
	Preference ProfilePreference

	// RequireValid requires the profile to have valid (non-expired) credentials.
	RequireValid bool

	// Mode restricts to a specific authentication mode (optional).
	Mode AuthMode
}

// ProfileManager defines the interface for managing auth profiles.
type ProfileManager interface {
	// Get retrieves a profile by ID.
	Get(id ProfileID) (*AuthProfile, bool)

	// GetByProvider retrieves profiles for a provider, sorted by preference.
	GetByProvider(provider string, preference ProfilePreference) []*AuthProfile

	// Save stores a profile.
	Save(profile *AuthProfile) error

	// Delete removes a profile.
	Delete(id ProfileID) error

	// List returns all profiles.
	List() []*AuthProfile

	// Resolve resolves a profile based on options, with automatic fallback.
	Resolve(opts ResolutionOptions) *ProfileResolutionResult

	// MarkUsed updates the last used timestamp for a profile.
	MarkUsed(id ProfileID)

	// MarkError records an error for a profile.
	MarkError(id ProfileID, err error)
}

// OAuthTokenRefresher defines the interface for refreshing OAuth tokens.
type OAuthTokenRefresher interface {
	// RefreshToken refreshes an OAuth token for a provider.
	RefreshToken(provider, refreshToken string) (*oauth.OAuthCredential, error)
}
