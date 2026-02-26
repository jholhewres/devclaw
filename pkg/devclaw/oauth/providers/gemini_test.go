package providers

import (
	"strings"
	"testing"
)

func TestNewGeminiProvider(t *testing.T) {
	p := NewGeminiProvider()

	if p.Name() != "gemini" {
		t.Errorf("Name() = %s, want gemini", p.Name())
	}

	if p.Label() == "" {
		t.Error("Label() should not be empty")
	}

	if !p.SupportsPKCE() {
		t.Error("Gemini should support PKCE")
	}

	if p.SupportsDeviceCode() {
		t.Error("Gemini should not support device code")
	}
}

func TestGeminiProviderAuthURL(t *testing.T) {
	p := NewGeminiProvider(
		WithGeminiClientID("test-client-id"),
		WithGeminiRedirectPort(8085),
	)

	authURL := p.AuthURL("test-state", "test-challenge")

	if !strings.Contains(authURL, "accounts.google.com") {
		t.Errorf("AuthURL should contain accounts.google.com, got %s", authURL)
	}

	if !strings.Contains(authURL, "client_id=test-client-id") {
		t.Errorf("AuthURL should contain client_id, got %s", authURL)
	}

	if !strings.Contains(authURL, "state=test-state") {
		t.Errorf("AuthURL should contain state, got %s", authURL)
	}

	if !strings.Contains(authURL, "code_challenge=test-challenge") {
		t.Errorf("AuthURL should contain code_challenge, got %s", authURL)
	}

	if !strings.Contains(authURL, "redirect_uri") {
		t.Errorf("AuthURL should contain redirect_uri, got %s", authURL)
	}
}

func TestGeminiProviderRedirectPort(t *testing.T) {
	p := NewGeminiProvider(
		WithGeminiRedirectPort(9999),
	)

	if p.RedirectPort() != 9999 {
		t.Errorf("RedirectPort() = %d, want 9999", p.RedirectPort())
	}
}

func TestGeminiProviderScopes(t *testing.T) {
	p := NewGeminiProvider()
	scopes := p.Scopes()

	if len(scopes) == 0 {
		t.Error("Scopes() should not be empty")
	}

	hasCloudPlatform := false
	for _, s := range scopes {
		if strings.Contains(s, "cloud-platform") {
			hasCloudPlatform = true
			break
		}
	}

	if !hasCloudPlatform {
		t.Error("Scopes should include cloud-platform")
	}
}
