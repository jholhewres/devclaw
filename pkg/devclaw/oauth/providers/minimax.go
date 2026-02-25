package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/oauth"
)

// MiniMax region endpoints
const (
	// Global region
	minimaxDeviceCodeURLGlobal = "https://api.minimax.io/v1/oauth/device/code"
	minimaxTokenURLGlobal      = "https://api.minimax.io/v1/oauth/token"
	minimaxAPIBaseGlobal       = "https://api.minimax.io/v1"

	// China region
	minimaxDeviceCodeURLCN = "https://api.minimaxi.com/v1/oauth/device/code"
	minimaxTokenURLCN      = "https://api.minimaxi.com/v1/oauth/token"
	minimaxAPIBaseCN       = "https://api.minimaxi.com/v1"
)

// MiniMax OAuth scopes
var minimaxScopes = []string{
	"openid",
	"profile",
	"email",
	"model.completion",
}

// MiniMaxProvider implements OAuth for MiniMax Portal using device code flow.
type MiniMaxProvider struct {
	BaseProvider
	region     string // "global" or "cn"
	httpClient *http.Client
	logger     *slog.Logger
}

// MiniMaxOption configures the MiniMax provider.
type MiniMaxOption func(*MiniMaxProvider)

// WithMiniMaxRegion sets the region ("global" or "cn").
func WithMiniMaxRegion(region string) MiniMaxOption {
	return func(p *MiniMaxProvider) {
		p.region = region
	}
}

// WithMiniMaxLogger sets the logger.
func WithMiniMaxLogger(logger *slog.Logger) MiniMaxOption {
	return func(p *MiniMaxProvider) {
		p.logger = logger
	}
}

// NewMiniMaxProvider creates a new MiniMax OAuth provider.
func NewMiniMaxProvider(opts ...MiniMaxOption) *MiniMaxProvider {
	p := &MiniMaxProvider{
		BaseProvider: BaseProvider{
			name:               "minimax",
			label:              "MiniMax Portal",
			scopes:             minimaxScopes,
			supportsPKCE:       false,
			supportsDeviceCode: true,
		},
		region:     "global",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     slog.Default().With("provider", "minimax"),
	}

	for _, opt := range opts {
		opt(p)
	}

	// Set URLs based on region
	if p.region == "cn" {
		p.authURL = minimaxDeviceCodeURLCN
		p.tokenURL = minimaxTokenURLCN
	} else {
		p.authURL = minimaxDeviceCodeURLGlobal
		p.tokenURL = minimaxTokenURLGlobal
	}

	return p
}

// Region returns the configured region.
func (p *MiniMaxProvider) Region() string {
	return p.region
}

// DeviceCodeURL returns the device code URL for the region.
func (p *MiniMaxProvider) DeviceCodeURL() string {
	if p.region == "cn" {
		return minimaxDeviceCodeURLCN
	}
	return minimaxDeviceCodeURLGlobal
}

// TokenURL returns the token URL for the region.
func (p *MiniMaxProvider) TokenURL() string {
	if p.region == "cn" {
		return minimaxTokenURLCN
	}
	return minimaxTokenURLGlobal
}

// APIBase returns the API base URL for the region.
func (p *MiniMaxProvider) APIBase() string {
	if p.region == "cn" {
		return minimaxAPIBaseCN
	}
	return minimaxAPIBaseGlobal
}

// StartDeviceFlow initiates the device code flow.
func (p *MiniMaxProvider) StartDeviceFlow(ctx context.Context) (*oauth.DeviceCodeResponse, error) {
	data := url.Values{
		"scope": {strings.Join(p.scopes, " ")},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.DeviceCodeURL(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read device code response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result oauth.DeviceCodeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse device code response: %w", err)
	}

	p.logger.Info("device code flow started",
		"user_code", result.UserCode,
		"verification_uri", result.VerificationURI,
		"region", p.region,
	)

	return &result, nil
}

// PollForToken polls for token completion.
func (p *MiniMaxProvider) PollForToken(ctx context.Context, deviceCode string, interval time.Duration) (*oauth.OAuthCredential, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			cred, err := p.pollTokenOnce(ctx, deviceCode)
			if err != nil {
				if isMiniMaxPendingError(err) {
					continue
				}
				return nil, err
			}
			return cred, nil
		}
	}
}

func (p *MiniMaxProvider) pollTokenOnce(ctx context.Context, deviceCode string) (*oauth.OAuthCredential, error) {
	data := url.Values{
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	// Check for error
	var errResp struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return nil, &minimaxError{code: errResp.Error, description: errResp.ErrorDescription}
	}

	var tokenResp oauth.TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &oauth.OAuthCredential{
		Provider:     p.name,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Metadata:     map[string]string{"region": p.region},
	}, nil
}

// ExchangeCode is not supported for MiniMax (uses device code flow).
func (p *MiniMaxProvider) ExchangeCode(ctx context.Context, code, verifier string) (*oauth.OAuthCredential, error) {
	return nil, fmt.Errorf("MiniMax uses device code flow, not authorization code flow")
}

// RefreshToken refreshes an access token.
func (p *MiniMaxProvider) RefreshToken(ctx context.Context, refreshToken string) (*oauth.OAuthCredential, error) {
	data := url.Values{
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp oauth.TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	return &oauth.OAuthCredential{
		Provider:     p.name,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Metadata:     map[string]string{"region": p.region},
	}, nil
}

type minimaxError struct {
	code        string
	description string
}

func (e *minimaxError) Error() string {
	if e.description != "" {
		return fmt.Sprintf("%s: %s", e.code, e.description)
	}
	return e.code
}

func (e *minimaxError) IsPending() bool {
	return e.code == "authorization_pending" || e.code == "slow_down"
}

func isMiniMaxPendingError(err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*minimaxError); ok {
		return e.IsPending()
	}
	return false
}
