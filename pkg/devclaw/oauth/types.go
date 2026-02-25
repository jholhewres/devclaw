// Package oauth provides OAuth 2.0 authentication for LLM providers.
// It supports PKCE flow (Google Gemini, ChatGPT) and Device Code flow (Qwen, MiniMax).
package oauth

import (
	"context"
	"time"
)

// OAuthCredential represents stored OAuth tokens for a provider.
type OAuthCredential struct {
	// Provider identifies which OAuth provider this credential belongs to
	// (e.g., "gemini", "chatgpt", "qwen", "minimax")
	Provider string `json:"provider"`

	// AccessToken is the OAuth access token used for API requests
	AccessToken string `json:"access_token"`

	// RefreshToken is used to obtain new access tokens
	RefreshToken string `json:"refresh_token"`

	// ExpiresAt is when the access token expires
	ExpiresAt time.Time `json:"expires_at"`

	// Email is the user's email associated with the OAuth account
	Email string `json:"email,omitempty"`

	// ClientID is the OAuth client ID used for this credential
	ClientID string `json:"client_id,omitempty"`

	// Metadata contains provider-specific data (e.g., project IDs, regions)
	Metadata map[string]string `json:"metadata,omitempty"`
}

// IsExpired returns true if the credential has expired or will expire within the buffer.
func (c *OAuthCredential) IsExpired(buffer time.Duration) bool {
	return time.Now().Add(buffer).After(c.ExpiresAt)
}

// IsValid returns true if the credential has a valid access token that hasn't expired.
func (c *OAuthCredential) IsValid() bool {
	return c.AccessToken != "" && !c.IsExpired(5*time.Minute)
}

// GetAccessToken returns the access token (implements interface for LLMClient).
func (c *OAuthCredential) GetAccessToken() string {
	return c.AccessToken
}

// PKCEPair contains the PKCE verifier and challenge for OAuth flows.
type PKCEPair struct {
	// Verifier is the random string used to verify the OAuth flow
	Verifier string `json:"verifier"`

	// Challenge is the SHA256 hash of the verifier, base64url encoded
	Challenge string `json:"challenge"`
}

// DeviceCodeResponse represents the response from a device code authorization request.
type DeviceCodeResponse struct {
	// DeviceCode is used to poll for the token
	DeviceCode string `json:"device_code"`

	// UserCode is displayed to the user for verification
	UserCode string `json:"user_code"`

	// VerificationURI is the URL the user visits to enter the code
	VerificationURI string `json:"verification_uri"`

	// VerificationURIComplete includes the user code in the URL (optional)
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`

	// ExpiresIn is the time in seconds until the device code expires
	ExpiresIn int `json:"expires_in"`

	// Interval is the polling interval in seconds
	Interval int `json:"interval"`
}

// TokenResponse represents the response from a token exchange request.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// OAuthProvider defines the interface for OAuth providers.
type OAuthProvider interface {
	// Name returns the provider identifier (e.g., "gemini", "chatgpt")
	Name() string

	// Label returns a human-readable provider name
	Label() string

	// AuthURL returns the authorization URL for the OAuth flow
	// state is a random string for CSRF protection
	// challenge is the PKCE code challenge (for PKCE flows)
	AuthURL(state, challenge string) string

	// TokenURL returns the token exchange endpoint
	TokenURL() string

	// ExchangeCode exchanges an authorization code for tokens
	// code is the authorization code from the callback
	// verifier is the PKCE code verifier
	ExchangeCode(ctx context.Context, code, verifier string) (*OAuthCredential, error)

	// RefreshToken obtains a new access token using a refresh token
	RefreshToken(ctx context.Context, refreshToken string) (*OAuthCredential, error)

	// Scopes returns the OAuth scopes required for this provider
	Scopes() []string

	// SupportsPKCE returns true if the provider supports PKCE flow
	SupportsPKCE() bool

	// SupportsDeviceCode returns true if the provider supports device code flow
	SupportsDeviceCode() bool
}

// DeviceCodeProvider extends OAuthProvider with device code flow methods.
type DeviceCodeProvider interface {
	OAuthProvider

	// StartDeviceFlow initiates a device code flow
	StartDeviceFlow(ctx context.Context) (*DeviceCodeResponse, error)

	// PollForToken polls the token endpoint until the user completes verification
	PollForToken(ctx context.Context, deviceCode string, interval time.Duration) (*OAuthCredential, error)
}

// OAuthFlowType indicates the type of OAuth flow to use.
type OAuthFlowType string

const (
	// FlowPKCE indicates a PKCE-based OAuth flow with local callback
	FlowPKCE OAuthFlowType = "pkce"

	// FlowDeviceCode indicates a device code flow for CLI/headless environments
	FlowDeviceCode OAuthFlowType = "device_code"
)

// ProviderConfig contains configuration for an OAuth provider.
type ProviderConfig struct {
	// Enabled indicates if this provider is available
	Enabled bool `yaml:"enabled" json:"enabled"`

	// ClientID is the OAuth client ID (optional, some providers have defaults)
	ClientID string `yaml:"client_id,omitempty" json:"client_id,omitempty"`

	// ClientSecret is the OAuth client secret (optional, rarely needed)
	ClientSecret string `yaml:"client_secret,omitempty" json:"client_secret,omitempty"`

	// RedirectPort is the local port for OAuth callbacks (PKCE flow)
	RedirectPort int `yaml:"redirect_port,omitempty" json:"redirect_port,omitempty"`

	// Region is the provider region (e.g., "global" or "cn" for MiniMax)
	Region string `yaml:"region,omitempty" json:"region,omitempty"`
}

// Config contains OAuth configuration for all providers.
type Config struct {
	// Providers maps provider names to their configurations
	Providers map[string]ProviderConfig `yaml:"providers" json:"providers"`
}

// DefaultConfig returns the default OAuth configuration.
func DefaultConfig() *Config {
	return &Config{
		Providers: map[string]ProviderConfig{
			"gemini": {
				Enabled:      false,
				RedirectPort: 8085,
			},
			"chatgpt": {
				Enabled:      false,
				RedirectPort: 1455,
			},
			"qwen": {
				Enabled: false,
			},
			"minimax": {
				Enabled: false,
				Region:  "global",
			},
		},
	}
}

// CallbackResult contains the result of an OAuth callback.
type CallbackResult struct {
	// Code is the authorization code (on success)
	Code string

	// State is the state parameter from the callback
	State string

	// Error contains the error message (on failure)
	Error string

	// ErrorDescription provides more details about the error
	ErrorDescription string
}

// LoginResult contains the result of an OAuth login attempt.
type LoginResult struct {
	// Credential is the OAuth credential (on success)
	Credential *OAuthCredential

	// Provider is the provider name
	Provider string

	// Error contains any error that occurred
	Error error

	// Warning contains non-fatal warnings (e.g., experimental feature)
	Warning string
}
