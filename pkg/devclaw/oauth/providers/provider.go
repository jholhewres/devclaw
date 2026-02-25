// Package providers implements OAuth providers for various LLM services.
package providers

import (
	"context"
)

// BaseProvider contains common functionality for OAuth providers.
type BaseProvider struct {
	name        string
	label       string
	authURL     string
	tokenURL    string
	scopes      []string
	supportsPKCE       bool
	supportsDeviceCode bool
}

// Name returns the provider identifier.
func (p *BaseProvider) Name() string {
	return p.name
}

// Label returns the human-readable provider name.
func (p *BaseProvider) Label() string {
	return p.label
}

// AuthURL returns the authorization URL.
func (p *BaseProvider) AuthURL(state, challenge string) string {
	return p.authURL
}

// TokenURL returns the token exchange URL.
func (p *BaseProvider) TokenURL() string {
	return p.tokenURL
}

// Scopes returns the OAuth scopes.
func (p *BaseProvider) Scopes() []string {
	return p.scopes
}

// SupportsPKCE returns true if PKCE is supported.
func (p *BaseProvider) SupportsPKCE() bool {
	return p.supportsPKCE
}

// SupportsDeviceCode returns true if device code flow is supported.
func (p *BaseProvider) SupportsDeviceCode() bool {
	return p.supportsDeviceCode
}

// ExchangeCode must be implemented by each provider.
func (p *BaseProvider) ExchangeCode(ctx context.Context, code, verifier string) (credential any, err error) {
	return nil, nil
}

// RefreshToken must be implemented by each provider.
func (p *BaseProvider) RefreshToken(ctx context.Context, refreshToken string) (credential any, err error) {
	return nil, nil
}
