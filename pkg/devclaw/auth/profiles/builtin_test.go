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
			name:      "openai provider",
			provider:  "openai",
			wantOK:    true,
			wantLabel: "OpenAI",
		},
		{
			name:      "anthropic provider",
			provider:  "anthropic",
			wantOK:    true,
			wantLabel: "Anthropic",
		},
		{
			name:     "unknown provider",
			provider: "unknown",
			wantOK:   false,
		},
		{
			name:      "case insensitive",
			provider:  "OPENAI",
			wantOK:    true,
			wantLabel: "OpenAI",
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

	// Check that openai is included
	found := false
	for _, p := range providers {
		if p.Name == "openai" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListProviders should include openai provider")
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
		{"openai", true},
		{"anthropic", true},
		{"unknown", false},
		{"OPENAI", true}, // case insensitive
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
		{"anthropic", ModeOAuth, true},
		{"anthropic", ModeAPIKey, true},
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
			name:       "gemini provider",
			provider:   "gemini",
			service:    "",
			wantScopes: true,
			wantErr:    false,
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
		{"openai", 1},
		{"anthropic", 1},
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
				Name:        "openai", // Already exists
				Label:       "OpenAI",
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
	meta, ok := GetProvider("gemini")
	if !ok {
		t.Fatal("gemini provider not found")
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
