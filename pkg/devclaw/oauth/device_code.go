package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DeviceCodeFlow handles OAuth device code flow for providers like Qwen and MiniMax.
type DeviceCodeFlow struct {
	httpClient *http.Client
	logger     *slog.Logger

	// DeviceCodeURL is the endpoint to request a device code
	DeviceCodeURL string

	// TokenURL is the endpoint to exchange device code for tokens
	TokenURL string

	// ClientID is the OAuth client ID
	ClientID string

	// Scopes are the OAuth scopes to request
	Scopes []string
}

// NewDeviceCodeFlow creates a new device code flow handler.
func NewDeviceCodeFlow(deviceCodeURL, tokenURL, clientID string, scopes []string, logger *slog.Logger) *DeviceCodeFlow {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeviceCodeFlow{
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		logger:        logger.With("component", "device-code-flow"),
		DeviceCodeURL: deviceCodeURL,
		TokenURL:      tokenURL,
		ClientID:      clientID,
		Scopes:        scopes,
	}
}

// Start initiates the device code flow and returns the device code response.
func (d *DeviceCodeFlow) Start(ctx context.Context) (*DeviceCodeResponse, error) {
	data := url.Values{
		"client_id": []string{d.ClientID},
		"scope":     []string{strings.Join(d.Scopes, " ")},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.DeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
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

	var result DeviceCodeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse device code response: %w", err)
	}

	d.logger.Info("device code flow started",
		"user_code", result.UserCode,
		"verification_uri", result.VerificationURI,
		"expires_in", result.ExpiresIn,
		"interval", result.Interval,
	)

	return &result, nil
}

// Poll polls the token endpoint until the user completes verification.
// It respects the polling interval and handles the "authorization_pending" response.
func (d *DeviceCodeFlow) Poll(ctx context.Context, deviceCode string, interval time.Duration) (*OAuthCredential, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	d.logger.Debug("starting device code polling", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			cred, err := d.pollOnce(ctx, deviceCode)
			if err != nil {
				// Check if it's a pending error (user hasn't completed yet)
				if isPendingError(err) {
					d.logger.Debug("authorization pending, continuing to poll")
					continue
				}
				return nil, err
			}
			return cred, nil
		}
	}
}

// pollOnce makes a single poll request to the token endpoint.
func (d *DeviceCodeFlow) pollOnce(ctx context.Context, deviceCode string) (*OAuthCredential, error) {
	data := url.Values{
		"client_id":   []string{d.ClientID},
		"device_code": []string{deviceCode},
		"grant_type":  []string{"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	// Check for error response
	var errResp struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return nil, &DeviceCodeError{
			Code:             errResp.Error,
			ErrorDescription: errResp.ErrorDescription,
		}
	}

	// Parse successful token response
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	d.logger.Info("device code flow completed successfully")

	return &OAuthCredential{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

// DeviceCodeError represents an error from the device code flow.
type DeviceCodeError struct {
	Code             string // "authorization_pending", "slow_down", etc.
	ErrorDescription string
}

func (e *DeviceCodeError) Error() string {
	if e.ErrorDescription != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.ErrorDescription)
	}
	return e.Code
}

// IsPending returns true if the error indicates authorization is still pending.
func (e *DeviceCodeError) IsPending() bool {
	return e.Code == "authorization_pending" || e.Code == "slow_down"
}

// isPendingError checks if an error is a pending device code error.
func isPendingError(err error) bool {
	if err == nil {
		return false
	}
	var dcErr *DeviceCodeError
	if ok := errors.As(err, &dcErr); ok {
		return dcErr.IsPending()
	}
	return false
}

// ExchangePKCE exchanges an authorization code for tokens using PKCE.
// This is used by providers that support PKCE flow (like Qwen Portal).
func (d *DeviceCodeFlow) ExchangePKCE(ctx context.Context, code, verifier, redirectURI string) (*OAuthCredential, error) {
	data := url.Values{
		"client_id":     []string{d.ClientID},
		"code":          []string{code},
		"code_verifier": []string{verifier},
		"grant_type":    []string{"authorization_code"},
		"redirect_uri":  []string{redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &OAuthCredential{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

// RefreshToken refreshes an access token using a refresh token.
func (d *DeviceCodeFlow) RefreshToken(ctx context.Context, refreshToken string) (*OAuthCredential, error) {
	data := url.Values{
		"client_id":     []string{d.ClientID},
		"refresh_token": []string{refreshToken},
		"grant_type":    []string{"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	return &OAuthCredential{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

// doJSONRequest performs a JSON POST request.
func doJSONRequest(ctx context.Context, client *http.Client, method, url string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
