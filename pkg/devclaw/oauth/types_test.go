package oauth

import (
	"testing"
	"time"
)

func TestOAuthCredential_IsExpired(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		cred      *OAuthCredential
		buffer    time.Duration
		wantExpired bool
	}{
		{
			name: "not expired",
			cred: &OAuthCredential{
				ExpiresAt: now.Add(1 * time.Hour),
			},
			buffer:    5 * time.Minute,
			wantExpired: false,
		},
		{
			name: "expired",
			cred: &OAuthCredential{
				ExpiresAt: now.Add(-1 * time.Hour),
			},
			buffer:    5 * time.Minute,
			wantExpired: true,
		},
		{
			name: "expiring within buffer",
			cred: &OAuthCredential{
				ExpiresAt: now.Add(2 * time.Minute),
			},
			buffer:    5 * time.Minute,
			wantExpired: true,
		},
		{
			name: "exactly at buffer edge",
			cred: &OAuthCredential{
				ExpiresAt: now.Add(5 * time.Minute),
			},
			buffer:    5 * time.Minute,
			wantExpired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cred.IsExpired(tt.buffer)
			if got != tt.wantExpired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.wantExpired)
			}
		})
	}
}

func TestOAuthCredential_IsValid(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		cred      *OAuthCredential
		wantValid bool
	}{
		{
			name: "valid credential",
			cred: &OAuthCredential{
				AccessToken: "test-token",
				ExpiresAt:   now.Add(1 * time.Hour),
			},
			wantValid: true,
		},
		{
			name: "no access token",
			cred: &OAuthCredential{
				AccessToken: "",
				ExpiresAt:   now.Add(1 * time.Hour),
			},
			wantValid: false,
		},
		{
			name: "expired token",
			cred: &OAuthCredential{
				AccessToken: "test-token",
				ExpiresAt:   now.Add(-1 * time.Hour),
			},
			wantValid: false,
		},
		{
			name: "expiring soon (within 5 min buffer)",
			cred: &OAuthCredential{
				AccessToken: "test-token",
				ExpiresAt:   now.Add(2 * time.Minute),
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cred.IsValid()
			if got != tt.wantValid {
				t.Errorf("IsValid() = %v, want %v", got, tt.wantValid)
			}
		})
	}
}

func TestOAuthCredential_GetAccessToken(t *testing.T) {
	cred := &OAuthCredential{
		AccessToken: "my-access-token",
	}

	if cred.GetAccessToken() != "my-access-token" {
		t.Errorf("GetAccessToken() = %s, want my-access-token", cred.GetAccessToken())
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if cfg.Providers == nil {
		t.Fatal("Providers map is nil")
	}

	// Check expected providers
	expectedProviders := []string{"gemini", "chatgpt", "qwen", "minimax"}
	for _, p := range expectedProviders {
		if _, ok := cfg.Providers[p]; !ok {
			t.Errorf("Expected provider %s not found in default config", p)
		}
	}
}
