package providers

import (
	"strings"
	"testing"
)

func TestNewMiniMaxProvider(t *testing.T) {
	p := NewMiniMaxProvider()

	if p.Name() != "minimax" {
		t.Errorf("Name() = %s, want minimax", p.Name())
	}

	if p.Label() == "" {
		t.Error("Label() should not be empty")
	}

	if p.SupportsPKCE() {
		t.Error("MiniMax should not support PKCE")
	}

	if !p.SupportsDeviceCode() {
		t.Error("MiniMax should support device code")
	}
}

func TestMiniMaxProviderRegion(t *testing.T) {
	// Default region
	p := NewMiniMaxProvider()
	if p.Region() != "global" {
		t.Errorf("Region() = %s, want global", p.Region())
	}

	// CN region
	pCN := NewMiniMaxProvider(WithMiniMaxRegion("cn"))
	if pCN.Region() != "cn" {
		t.Errorf("Region() = %s, want cn", pCN.Region())
	}
}

func TestMiniMaxProviderAPIBase(t *testing.T) {
	// Global region
	pGlobal := NewMiniMaxProvider(WithMiniMaxRegion("global"))
	if !strings.Contains(pGlobal.APIBase(), "minimax.io") {
		t.Errorf("APIBase() for global should contain minimax.io, got %s", pGlobal.APIBase())
	}

	// CN region
	pCN := NewMiniMaxProvider(WithMiniMaxRegion("cn"))
	if !strings.Contains(pCN.APIBase(), "minimaxi.com") {
		t.Errorf("APIBase() for CN should contain minimaxi.com, got %s", pCN.APIBase())
	}
}

func TestMiniMaxProviderDeviceCodeURL(t *testing.T) {
	// Global region
	pGlobal := NewMiniMaxProvider(WithMiniMaxRegion("global"))
	if !strings.Contains(pGlobal.DeviceCodeURL(), "minimax.io") {
		t.Errorf("DeviceCodeURL() for global should contain minimax.io, got %s", pGlobal.DeviceCodeURL())
	}

	// CN region
	pCN := NewMiniMaxProvider(WithMiniMaxRegion("cn"))
	if !strings.Contains(pCN.DeviceCodeURL(), "minimaxi.com") {
		t.Errorf("DeviceCodeURL() for CN should contain minimaxi.com, got %s", pCN.DeviceCodeURL())
	}
}

func TestMiniMaxProviderScopes(t *testing.T) {
	p := NewMiniMaxProvider()
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
