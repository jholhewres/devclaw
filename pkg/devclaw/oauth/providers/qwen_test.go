package providers

import (
	"strings"
	"testing"
)

func TestNewQwenProvider(t *testing.T) {
	p := NewQwenProvider()

	if p.Name() != "qwen" {
		t.Errorf("Name() = %s, want qwen", p.Name())
	}

	if p.Label() == "" {
		t.Error("Label() should not be empty")
	}

	if p.SupportsPKCE() {
		t.Error("Qwen should not support PKCE")
	}

	if !p.SupportsDeviceCode() {
		t.Error("Qwen should support device code")
	}
}

func TestQwenProviderScopes(t *testing.T) {
	p := NewQwenProvider()
	scopes := p.Scopes()

	if len(scopes) == 0 {
		t.Error("Scopes() should not be empty")
	}

	hasOpenID := false
	for _, s := range scopes {
		if s == "openid" {
			hasOpenID = true
			break
		}
	}

	if !hasOpenID {
		t.Error("Scopes should include openid")
	}
}

func TestQwenProviderAPIBase(t *testing.T) {
	p := NewQwenProvider()

	if !strings.Contains(p.APIBase(), "qwen.ai") {
		t.Errorf("APIBase() should contain qwen.ai, got %s", p.APIBase())
	}
}
