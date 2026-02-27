package profiles

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ProfileTools provides tool functions for managing authentication profiles.
type ProfileTools struct {
	store    ProfileManager
	resolver *Resolver
}

// NewProfileTools creates a new profile tools instance.
func NewProfileTools(store ProfileManager, resolver *Resolver) *ProfileTools {
	return &ProfileTools{
		store:    store,
		resolver: resolver,
	}
}

// ProfileAddArgs contains arguments for adding a profile.
type ProfileAddArgs struct {
	Provider string `json:"provider"`           // e.g., "google", "openai"
	Name     string `json:"name"`               // e.g., "work", "personal"
	Mode     string `json:"mode"`               // "api_key", "oauth", "token"
	APIKey   string `json:"api_key,omitempty"`  // For API key mode
	Token    string `json:"token,omitempty"`    // For token mode
	Priority int    `json:"priority,omitempty"` // Higher = tried first
}

// ProfileAddResult contains the result of adding a profile.
type ProfileAddResult struct {
	Success   bool   `json:"success"`
	ProfileID string `json:"profile_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Add adds a new authentication profile.
func (t *ProfileTools) Add(ctx context.Context, args ProfileAddArgs) (*ProfileAddResult, error) {
	// Validate provider
	providerMeta, ok := GetProvider(args.Provider)
	if !ok {
		return &ProfileAddResult{
			Success: false,
			Error:   fmt.Sprintf("unknown provider: %s", args.Provider),
		}, nil
	}

	// Validate mode
	mode := AuthMode(args.Mode)
	if mode == "" {
		mode = ModeAPIKey // default
	}

	validMode := false
	for _, m := range providerMeta.Modes {
		if m == mode {
			validMode = true
			break
		}
	}
	if !validMode {
		return &ProfileAddResult{
			Success: false,
			Error:   fmt.Sprintf("provider %s does not support mode: %s", args.Provider, mode),
		}, nil
	}

	// Set default name
	if args.Name == "" {
		args.Name = "default"
	}

	// Create profile
	profile := &AuthProfile{
		ID:        NewProfileID(args.Provider, args.Name),
		Provider:  args.Provider,
		Name:      args.Name,
		Mode:      mode,
		Priority:  args.Priority,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set credential based on mode
	switch mode {
	case ModeAPIKey:
		if args.APIKey == "" {
			return &ProfileAddResult{
				Success: false,
				Error:   "api_key is required for api_key mode",
			}, nil
		}
		profile.APIKey = &APIKeyCredential{
			Key: args.APIKey,
		}

	case ModeToken:
		if args.Token == "" {
			return &ProfileAddResult{
				Success: false,
				Error:   "token is required for token mode",
			}, nil
		}
		profile.Token = &TokenCredential{
			Token: args.Token,
		}

	case ModeOAuth:
		// OAuth profiles are created without credentials initially
		// Credentials are added after OAuth flow completes
		profile.OAuth = &OAuthCredential{
			Provider: args.Provider,
		}
	}

	// Save profile
	if err := t.store.Save(profile); err != nil {
		return &ProfileAddResult{
			Success: false,
			Error:   fmt.Sprintf("failed to save profile: %v", err),
		}, nil
	}

	return &ProfileAddResult{
		Success:   true,
		ProfileID: string(profile.ID),
	}, nil
}

// ProfileListArgs contains arguments for listing profiles.
type ProfileListArgs struct {
	Provider string `json:"provider,omitempty"` // Filter by provider (optional)
	Mode     string `json:"mode,omitempty"`     // Filter by mode (optional)
	Enabled  *bool  `json:"enabled,omitempty"`  // Filter by enabled status (optional)
}

// ProfileListItem represents a profile in the list.
type ProfileListItem struct {
	ID         string    `json:"id"`
	Provider   string    `json:"provider"`
	Name       string    `json:"name"`
	Mode       string    `json:"mode"`
	Enabled    bool      `json:"enabled"`
	Valid      bool      `json:"valid"`
	Expired    bool      `json:"expired,omitempty"`
	Email      string    `json:"email,omitempty"`
	Priority   int       `json:"priority,omitempty"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
	LastError  string    `json:"last_error,omitempty"`
}

// ProfileListResult contains the result of listing profiles.
type ProfileListResult struct {
	Profiles []ProfileListItem `json:"profiles"`
	Total    int               `json:"total"`
}

// List lists authentication profiles.
func (t *ProfileTools) List(ctx context.Context, args ProfileListArgs) (*ProfileListResult, error) {
	profiles := t.store.List()

	var items []ProfileListItem
	for _, p := range profiles {
		// Apply filters
		if args.Provider != "" && p.Provider != args.Provider {
			continue
		}
		if args.Mode != "" && string(p.Mode) != args.Mode {
			continue
		}
		if args.Enabled != nil && p.Enabled != *args.Enabled {
			continue
		}

		item := ProfileListItem{
			ID:       string(p.ID),
			Provider: p.Provider,
			Name:     p.Name,
			Mode:     string(p.Mode),
			Enabled:  p.Enabled,
			Valid:    p.IsValid(),
			Priority: p.Priority,
		}

		if p.IsExpired() {
			item.Expired = true
		}

		if p.OAuth != nil {
			item.Email = p.OAuth.Email
		}

		if p.LastUsedAt != nil {
			item.LastUsedAt = *p.LastUsedAt
		}

		if p.LastError != "" {
			item.LastError = p.LastError
		}

		items = append(items, item)
	}

	return &ProfileListResult{
		Profiles: items,
		Total:    len(items),
	}, nil
}

// ProfileDeleteArgs contains arguments for deleting a profile.
type ProfileDeleteArgs struct {
	ProfileID string `json:"profile_id"` // Format: "provider:name"
}

// ProfileDeleteResult contains the result of deleting a profile.
type ProfileDeleteResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// Delete deletes an authentication profile.
func (t *ProfileTools) Delete(ctx context.Context, args ProfileDeleteArgs) (*ProfileDeleteResult, error) {
	id := ProfileID(args.ProfileID)

	if err := t.store.Delete(id); err != nil {
		return &ProfileDeleteResult{
			Success: false,
			Error:   fmt.Sprintf("failed to delete profile: %v", err),
		}, nil
	}

	return &ProfileDeleteResult{
		Success: true,
	}, nil
}

// ProfileEnableArgs contains arguments for enabling/disabling a profile.
type ProfileEnableArgs struct {
	ProfileID string `json:"profile_id"`
	Enable    bool   `json:"enable"`
}

// ProfileEnableResult contains the result of enabling/disabling a profile.
type ProfileEnableResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// Enable enables or disables a profile.
func (t *ProfileTools) Enable(ctx context.Context, args ProfileEnableArgs) (*ProfileEnableResult, error) {
	id := ProfileID(args.ProfileID)

	profile, ok := t.store.Get(id)
	if !ok {
		return &ProfileEnableResult{
			Success: false,
			Error:   fmt.Sprintf("profile not found: %s", args.ProfileID),
		}, nil
	}

	profile.Enabled = args.Enable
	if err := t.store.Save(profile); err != nil {
		return &ProfileEnableResult{
			Success: false,
			Error:   fmt.Sprintf("failed to update profile: %v", err),
		}, nil
	}

	return &ProfileEnableResult{
		Success: true,
	}, nil
}

// ProfileTestArgs contains arguments for testing a profile.
type ProfileTestArgs struct {
	ProfileID string `json:"profile_id"`
}

// ProfileTestResult contains the result of testing a profile.
type ProfileTestResult struct {
	Success   bool          `json:"success"`
	Valid     bool          `json:"valid"`
	Expired   bool          `json:"expired,omitempty"`
	Email     string        `json:"email,omitempty"`
	Error     string        `json:"error,omitempty"`
	ExpiresIn time.Duration `json:"expires_in,omitempty"`
}

// Test tests if a profile has valid credentials.
func (t *ProfileTools) Test(ctx context.Context, args ProfileTestArgs) (*ProfileTestResult, error) {
	id := ProfileID(args.ProfileID)

	profile, ok := t.store.Get(id)
	if !ok {
		return &ProfileTestResult{
			Success: false,
			Error:   fmt.Sprintf("profile not found: %s", args.ProfileID),
		}, nil
	}

	result := &ProfileTestResult{
		Valid:   profile.IsValid(),
		Expired: profile.IsExpired(),
	}

	if profile.OAuth != nil {
		result.Email = profile.OAuth.Email
		if !profile.OAuth.ExpiresAt.IsZero() {
			result.ExpiresIn = time.Until(profile.OAuth.ExpiresAt)
		}
	}

	if profile.LastError != "" {
		result.Error = profile.LastError
	}

	result.Success = true
	return result, nil
}

// ProvidersListResult contains the result of listing available providers.
type ProvidersListResult struct {
	Providers []ProviderInfo `json:"providers"`
}

// ProviderInfo represents information about a provider.
type ProviderInfo struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Modes       []string `json:"modes"`
	EnvKey      string   `json:"env_key,omitempty"`
	Website     string   `json:"website,omitempty"`
}

// Providers lists available authentication providers.
func (t *ProfileTools) Providers(ctx context.Context) (*ProvidersListResult, error) {
	providers := ListProviders()

	var items []ProviderInfo
	for _, p := range providers {
		modes := make([]string, len(p.Modes))
		for i, m := range p.Modes {
			modes[i] = string(m)
		}

		items = append(items, ProviderInfo{
			Name:        p.Name,
			Label:       p.Label,
			Description: p.Description,
			Modes:       modes,
			EnvKey:      p.EnvKey,
			Website:     p.Website,
		})
	}

	return &ProvidersListResult{
		Providers: items,
	}, nil
}

// ProfileSetPriorityArgs contains arguments for setting profile priority.
type ProfileSetPriorityArgs struct {
	ProfileID string `json:"profile_id"`
	Priority  int    `json:"priority"`
}

// ProfileSetPriorityResult contains the result of setting priority.
type ProfileSetPriorityResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// SetPriority sets the priority of a profile (higher = tried first in fallback).
func (t *ProfileTools) SetPriority(ctx context.Context, args ProfileSetPriorityArgs) (*ProfileSetPriorityResult, error) {
	id := ProfileID(args.ProfileID)

	profile, ok := t.store.Get(id)
	if !ok {
		return &ProfileSetPriorityResult{
			Success: false,
			Error:   fmt.Sprintf("profile not found: %s", args.ProfileID),
		}, nil
	}

	profile.Priority = args.Priority
	if err := t.store.Save(profile); err != nil {
		return &ProfileSetPriorityResult{
			Success: false,
			Error:   fmt.Sprintf("failed to update profile: %v", err),
		}, nil
	}

	return &ProfileSetPriorityResult{
		Success: true,
	}, nil
}

// FormatProfileList formats a profile list for display.
func FormatProfileList(result *ProfileListResult) string {
	if result.Total == 0 {
		return "No authentication profiles configured."
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Authentication Profiles (%d total):", result.Total))
	lines = append(lines, "")

	// Group by provider
	byProvider := make(map[string][]ProfileListItem)
	for _, p := range result.Profiles {
		byProvider[p.Provider] = append(byProvider[p.Provider], p)
	}

	for provider, profiles := range byProvider {
		meta, _ := GetProvider(provider)
		label := provider
		if meta != nil {
			label = meta.Label
		}
		lines = append(lines, fmt.Sprintf("📦 %s:", label))

		for _, p := range profiles {
			status := "✓"
			if !p.Enabled {
				status = "⊘"
			} else if p.Expired {
				status = "⚠"
			} else if !p.Valid {
				status = "✗"
			}

			mode := p.Mode
			parts := []string{
				fmt.Sprintf("  %s %s", status, p.Name),
				fmt.Sprintf("(%s)", mode),
			}

			if p.Email != "" {
				parts = append(parts, fmt.Sprintf("- %s", p.Email))
			}

			if p.LastError != "" {
				parts = append(parts, fmt.Sprintf("[error: %s]", p.LastError))
			}

			if p.Priority > 0 {
				parts = append(parts, fmt.Sprintf("[priority:%d]", p.Priority))
			}

			lines = append(lines, strings.Join(parts, " "))
		}
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// ProfileImportArgs contains arguments for importing a profile.
type ProfileImportArgs struct {
	Provider string          `json:"provider"`
	Name     string          `json:"name"`
	Mode     string          `json:"mode"`
	Data     json.RawMessage `json:"data"` // Provider-specific credential data
}

// ProfileImportResult contains the result of importing a profile.
type ProfileImportResult struct {
	Success   bool   `json:"success"`
	ProfileID string `json:"profile_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Import imports a profile from external credential data.
func (t *ProfileTools) Import(ctx context.Context, args ProfileImportArgs) (*ProfileImportResult, error) {
	// Validate provider
	if _, ok := GetProvider(args.Provider); !ok {
		return &ProfileImportResult{
			Success: false,
			Error:   fmt.Sprintf("unknown provider: %s", args.Provider),
		}, nil
	}

	// Set default name
	if args.Name == "" {
		args.Name = "default"
	}

	// Create profile
	profile := &AuthProfile{
		ID:        NewProfileID(args.Provider, args.Name),
		Provider:  args.Provider,
		Name:      args.Name,
		Mode:      AuthMode(args.Mode),
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Parse credential data based on mode
	switch profile.Mode {
	case ModeAPIKey:
		var cred APIKeyCredential
		if err := json.Unmarshal(args.Data, &cred); err != nil {
			return &ProfileImportResult{
				Success: false,
				Error:   fmt.Sprintf("invalid API key data: %v", err),
			}, nil
		}
		profile.APIKey = &cred

	case ModeToken:
		var cred TokenCredential
		if err := json.Unmarshal(args.Data, &cred); err != nil {
			return &ProfileImportResult{
				Success: false,
				Error:   fmt.Sprintf("invalid token data: %v", err),
			}, nil
		}
		profile.Token = &cred

	case ModeOAuth:
		var cred OAuthCredential
		if err := json.Unmarshal(args.Data, &cred); err != nil {
			return &ProfileImportResult{
				Success: false,
				Error:   fmt.Sprintf("invalid OAuth data: %v", err),
			}, nil
		}
		profile.OAuth = &cred

	default:
		return &ProfileImportResult{
			Success: false,
			Error:   fmt.Sprintf("unsupported mode: %s", profile.Mode),
		}, nil
	}

	// Save profile
	if err := t.store.Save(profile); err != nil {
		return &ProfileImportResult{
			Success: false,
			Error:   fmt.Sprintf("failed to save profile: %v", err),
		}, nil
	}

	return &ProfileImportResult{
		Success:   true,
		ProfileID:   string(profile.ID),
	}, nil
}

// ProfileExportArgs contains arguments for exporting a profile.
type ProfileExportArgs struct {
	ProfileID string `json:"profile_id"`
}

// ProfileExportResult contains the result of exporting a profile.
type ProfileExportResult struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Mode    string          `json:"mode,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// Export exports a profile's credential data (for backup/migration).
func (t *ProfileTools) Export(ctx context.Context, args ProfileExportArgs) (*ProfileExportResult, error) {
	id := ProfileID(args.ProfileID)

	profile, ok := t.store.Get(id)
	if !ok {
		return &ProfileExportResult{
			Success: false,
			Error:   fmt.Sprintf("profile not found: %s", args.ProfileID),
		}, nil
	}

	var data []byte
	var err error

	switch profile.Mode {
	case ModeAPIKey:
		data, err = json.Marshal(profile.APIKey)
	case ModeToken:
		data, err = json.Marshal(profile.Token)
	case ModeOAuth:
		data, err = json.Marshal(profile.OAuth)
	default:
		return &ProfileExportResult{
			Success: false,
			Error:   fmt.Sprintf("unsupported mode: %s", profile.Mode),
		}, nil
	}

	if err != nil {
		return &ProfileExportResult{
			Success: false,
			Error:   fmt.Sprintf("failed to marshal data: %v", err),
		}, nil
	}

	return &ProfileExportResult{
		Success: true,
		Mode:    string(profile.Mode),
		Data:    data,
	}, nil
}
