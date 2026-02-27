package profiles

import (
	"testing"
)

func TestGetProvider(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		wantOK     bool
		wantLabel  string
	}{
		{
			name:      "google provider",
			provider:  "google",
			wantOK:    true,
			wantLabel: "Google",
		},
		{
			name:      "google-gmail provider",
			provider:  "google-gmail",
			wantOK:    true,
			wantLabel: "Gmail",
		},
		{
			name:     "unknown provider",
			provider: "unknown",
			wantOK:   false,
		},
		{
			name:      "case insensitive",
			provider:  "GOOGLE",
			wantOK:    true,
			wantLabel: "Google",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, ok := GetProvider(tt.provider)
			if ok != tt.wantOK {
				t.Errorf("GetProvider() ok = %v, want %v", ok, tt.wantOK)
				return
			}
			if ok && meta.Label != tt.wantLabel {
				t.Errorf("GetProvider() Label = %q, want %q", meta.Label, tt.wantLabel)
			}
		})
	}
}

func TestListProviders(t *testing.T) {
	providers := ListProviders()

	if len(providers) == 0 {
		t.Error("ListProviders should return at least one provider")
	}

	// Check that google is included
	found := false
	for _, p := range providers {
		if p.Name == "google" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListProviders should include google provider")
	}
}

func TestListProvidersByMode(t *testing.T) {
	oauthProviders := ListProvidersByMode(ModeOAuth)

	if len(oauthProviders) == 0 {
		t.Error("ListProvidersByMode(ModeOAuth) should return at least one provider")
	}

	// Verify all returned providers support OAuth
	for _, p := range oauthProviders {
		supportsOAuth := false
		for _, m := range p.Modes {
			if m == ModeOAuth {
				supportsOAuth = true
				break
			}
		}
		if !supportsOAuth {
			t.Errorf("Provider %s does not support OAuth mode", p.Name)
		}
	}
}

func TestIsValidProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     bool
	}{
		{"google", true},
		{"openai", true},
		{"unknown", false},
		{"GOOGLE", true}, // case insensitive
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			if got := IsValidProvider(tt.provider); got != tt.want {
				t.Errorf("IsValidProvider(%q) = %v, want %v", tt.provider, got, tt.want)
			}
		})
	}
}

func TestSupportsMode(t *testing.T) {
	tests := []struct {
		provider string
		mode     AuthMode
		want     bool
	}{
		{"google", ModeOAuth, true},
		{"google", ModeAPIKey, false},
		{"openai", ModeAPIKey, true},
		{"openai", ModeOAuth, false},
		{"unknown", ModeAPIKey, false},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"_"+string(tt.mode), func(t *testing.T) {
			if got := SupportsMode(tt.provider, tt.mode); got != tt.want {
				t.Errorf("SupportsMode(%q, %q) = %v, want %v", tt.provider, tt.mode, got, tt.want)
			}
		})
	}
}

func TestGetOAuthScopes(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		service    string
		wantScopes bool
		wantErr    bool
	}{
		{
			name:       "google with gmail service",
			provider:   "google-gmail",
			service:    "",
			wantScopes: true,
			wantErr:    false,
		},
		{
			name:       "google with calendar service",
			provider:   "google-calendar",
			service:    "",
			wantScopes: true,
			wantErr:    false,
		},
		{
			name:       "unknown provider",
			provider:   "unknown",
			service:    "",
			wantScopes: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scopes, err := GetOAuthScopes(tt.provider, tt.service)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetOAuthScopes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(scopes) == 0 && tt.wantScopes {
				t.Error("GetOAuthScopes() returned empty scopes")
			}
		})
	}
}

func TestGetSuggestedProfileNames(t *testing.T) {
	tests := []struct {
		provider string
		wantMin  int
	}{
		{"google-gmail", 1},
		{"google-calendar", 1},
		{"openai", 1},
		{"unknown", 1}, // should return default
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			names := GetSuggestedProfileNames(tt.provider)
			if len(names) < tt.wantMin {
				t.Errorf("GetSuggestedProfileNames(%q) returned %d names, want at least %d",
					tt.provider, len(names), tt.wantMin)
			}
		})
	}
}

func TestRegisterCustomProvider(t *testing.T) {
	// Save original to restore after test
	originalLen := len(BuiltinProviders)

	tests := []struct {
		name      string
		metadata  *ProviderMetadata
		wantErr   bool
	}{
		{
			name: "valid new provider",
			metadata: &ProviderMetadata{
				Name:        "custom-provider",
				Label:       "Custom Provider",
				Description: "A custom provider for testing",
				Modes:       []AuthMode{ModeAPIKey},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			metadata: &ProviderMetadata{
				Name:        "",
				Label:       "Empty Name",
				Description: "Should fail",
				Modes:       []AuthMode{ModeAPIKey},
			},
			wantErr: true,
		},
		{
			name: "existing provider",
			metadata: &ProviderMetadata{
				Name:        "google", // Already exists
				Label:       "Google",
				Description: "Should fail",
				Modes:       []AuthMode{ModeAPIKey},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RegisterCustomProvider(tt.metadata)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterCustomProvider() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Clean up if successful
			if err == nil {
				delete(BuiltinProviders, tt.metadata.Name)
			}
		})
	}

	// Verify we cleaned up
	if len(BuiltinProviders) != originalLen {
		t.Errorf("BuiltinProviders length changed from %d to %d", originalLen, len(BuiltinProviders))
	}
}

func TestProviderMetadata(t *testing.T) {
	meta, ok := GetProvider("google-gmail")
	if !ok {
		t.Fatal("google-gmail provider not found")
	}

	// Test ParentProvider
	if meta.ParentProvider != "google" {
		t.Errorf("ParentProvider = %q, want %q", meta.ParentProvider, "google")
	}

	// Test OAuthEndpoint
	if meta.OAuthEndpoint.AuthURL == "" {
		t.Error("OAuthEndpoint.AuthURL should not be empty")
	}
	if meta.OAuthEndpoint.TokenURL == "" {
		t.Error("OAuthEndpoint.TokenURL should not be empty")
	}
	if len(meta.OAuthEndpoint.Scopes) == 0 {
		t.Error("OAuthEndpoint.Scopes should not be empty")
	}
}
