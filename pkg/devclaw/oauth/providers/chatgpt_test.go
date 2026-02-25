package providers

import (
	"strings"
	"testing"
)

func TestNewChatGPTProvider(t *testing.T) {
	p := NewChatGPTProvider()

	if p.Name() != "chatgpt" {
		t.Errorf("Name() = %s, want chatgpt", p.Name())
	}

	if p.Label() == "" {
		t.Error("Label() should not be empty")
	}

	if !p.SupportsPKCE() {
		t.Error("ChatGPT should support PKCE")
	}

	if p.SupportsDeviceCode() {
		t.Error("ChatGPT should not support device code")
	}

	if !p.IsExperimental() {
		t.Error("ChatGPT should be marked as experimental")
	}
}

func TestChatGPTProviderAuthURL(t *testing.T) {
	p := NewChatGPTProvider(
		WithChatGPTClientID("test-client-id"),
		WithChatGPTRedirectPort(1455),
	)

	authURL := p.AuthURL("test-state", "test-challenge")

	if !strings.Contains(authURL, "auth.openai.com") {
		t.Errorf("AuthURL should contain auth.openai.com, got %s", authURL)
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
}

func TestChatGPTProviderRedirectPort(t *testing.T) {
	p := NewChatGPTProvider(
		WithChatGPTRedirectPort(1455),
	)

	if p.RedirectPort() != 1455 {
		t.Errorf("RedirectPort() = %d, want 1455", p.RedirectPort())
	}
}

func TestChatGPTProviderAPIBase(t *testing.T) {
	p := NewChatGPTProvider()

	if !strings.Contains(p.APIBase(), "chatgpt.com") {
		t.Errorf("APIBase() should contain chatgpt.com, got %s", p.APIBase())
	}
}

func TestChatGPTExperimentalWarning(t *testing.T) {
	if !strings.Contains(ExperimentalWarning, "EXPERIMENTAL") {
		t.Error("ExperimentalWarning should contain EXPERIMENTAL")
	}

	if !strings.Contains(ExperimentalWarning, "may stop working") {
		t.Error("ExperimentalWarning should warn about stability")
	}
}
