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

const (
	// Qwen OAuth endpoints
	qwenDeviceCodeURL = "https://qwen.ai/api/v1/oauth/device/code"
	qwenTokenURL      = "https://qwen.ai/api/v1/oauth/token"
	qwenAPIBase       = "https://api.qwen.ai/v1"

	// Default client ID for Qwen Portal
	qwenDefaultClientID = "f0304373b74a44d2b584a3fb70ca9e56"
)

// Qwen OAuth scopes
var qwenScopes = []string{
	"openid",
	"profile",
	"email",
	"model.completion",
}

// QwenProvider implements OAuth for Qwen Portal using device code flow.
type QwenProvider struct {
	BaseProvider
	clientID   string
	httpClient *http.Client
	logger     *slog.Logger
}

// QwenOption configures the Qwen provider.
type QwenOption func(*QwenProvider)

// WithQwenClientID sets a custom client ID.
func WithQwenClientID(clientID string) QwenOption {
	return func(p *QwenProvider) {
		p.clientID = clientID
	}
}

// WithQwenLogger sets the logger.
func WithQwenLogger(logger *slog.Logger) QwenOption {
	return func(p *QwenProvider) {
		p.logger = logger
	}
}

// NewQwenProvider creates a new Qwen OAuth provider.
func NewQwenProvider(opts ...QwenOption) *QwenProvider {
	p := &QwenProvider{
		BaseProvider: BaseProvider{
			name:               "qwen",
			label:              "Qwen Portal",
			authURL:            qwenDeviceCodeURL,
			tokenURL:           qwenTokenURL,
			scopes:             qwenScopes,
			supportsPKCE:       false,
			supportsDeviceCode: true,
		},
		clientID:   qwenDefaultClientID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     slog.Default().With("provider", "qwen"),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// StartDeviceFlow initiates the device code flow.
func (p *QwenProvider) StartDeviceFlow(ctx context.Context) (*oauth.DeviceCodeResponse, error) {
	data := url.Values{
		"client_id": {p.clientID},
		"scope":     {strings.Join(p.scopes, " ")},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, qwenDeviceCodeURL, strings.NewReader(data.Encode()))
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
	)

	return &result, nil
}

// PollForToken polls for token completion.
func (p *QwenProvider) PollForToken(ctx context.Context, deviceCode string, interval time.Duration) (*oauth.OAuthCredential, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			cred, err := p.pollTokenOnce(ctx, deviceCode)
			if err != nil {
				if isQwenPendingError(err) {
					continue
				}
				return nil, err
			}
			return cred, nil
		}
	}
}

func (p *QwenProvider) pollTokenOnce(ctx context.Context, deviceCode string) (*oauth.OAuthCredential, error) {
	data := url.Values{
		"client_id":   {p.clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, qwenTokenURL, strings.NewReader(data.Encode()))
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
		return nil, &qwenError{code: errResp.Error, description: errResp.ErrorDescription}
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
		ClientID:     p.clientID,
	}, nil
}

// ExchangeCode is not supported for Qwen (uses device code flow).
func (p *QwenProvider) ExchangeCode(ctx context.Context, code, verifier string) (*oauth.OAuthCredential, error) {
	return nil, fmt.Errorf("Qwen uses device code flow, not authorization code flow")
}

// RefreshToken refreshes an access token.
func (p *QwenProvider) RefreshToken(ctx context.Context, refreshToken string) (*oauth.OAuthCredential, error) {
	data := url.Values{
		"client_id":     {p.clientID},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, qwenTokenURL, strings.NewReader(data.Encode()))
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
		ClientID:     p.clientID,
	}, nil
}

// APIBase returns the Qwen API base URL.
func (p *QwenProvider) APIBase() string {
	return qwenAPIBase
}

type qwenError struct {
	code        string
	description string
}

func (e *qwenError) Error() string {
	if e.description != "" {
		return fmt.Sprintf("%s: %s", e.code, e.description)
	}
	return e.code
}

func (e *qwenError) IsPending() bool {
	return e.code == "authorization_pending" || e.code == "slow_down"
}

func isQwenPendingError(err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*qwenError); ok {
		return e.IsPending()
	}
	return false
}
