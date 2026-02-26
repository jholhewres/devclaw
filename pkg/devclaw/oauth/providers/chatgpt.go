package providers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	// ChatGPT OAuth endpoints (NOT official - reverse engineered)
	// These may change or be blocked at any time
	chatgptAuthURL  = "https://auth.openai.com/authorize"
	chatgptTokenURL = "https://auth.openai.com/oauth/token"
	chatgptAPIBase  = "https://chatgpt.com/backend-api/codex"
	chatgptUserInfo = "https://chatgpt.com/backend-api/me"

	// Default client ID (mimics Codex CLI)
	chatgptDefaultClientID = "pdlDIXlNxI6jaac5kp1cbLqF6qN6l9fJ"

	// Default redirect port (same as Codex CLI)
	chatgptDefaultPort = 1455
)

// ChatGPT OAuth scopes
var chatgptScopes = []string{
	"openid",
	"profile",
	"email",
	"model.completion",
	"model.request",
	"conversations",
	"offlines",
}

// ExperimentalWarning is shown when using ChatGPT OAuth.
const ExperimentalWarning = `
⚠️  EXPERIMENTAL FEATURE
ChatGPT OAuth uses unofficial endpoints and may stop working at any time.
OpenAI may block this approach like Anthropic did with Claude in Feb 2026.
Use at your own risk - consider Gemini as a more stable alternative.
`

// ChatGPTProvider implements OAuth for ChatGPT/Codex (EXPERIMENTAL).
type ChatGPTProvider struct {
	BaseProvider
	clientID     string
	redirectPort int
	httpClient   *http.Client
	logger       *slog.Logger
}

// ChatGPTOption configures the ChatGPT provider.
type ChatGPTOption func(*ChatGPTProvider)

// WithChatGPTClientID sets a custom client ID.
func WithChatGPTClientID(clientID string) ChatGPTOption {
	return func(p *ChatGPTProvider) {
		p.clientID = clientID
	}
}

// WithChatGPTRedirectPort sets the redirect port.
func WithChatGPTRedirectPort(port int) ChatGPTOption {
	return func(p *ChatGPTProvider) {
		p.redirectPort = port
	}
}

// WithChatGPTLogger sets the logger.
func WithChatGPTLogger(logger *slog.Logger) ChatGPTOption {
	return func(p *ChatGPTProvider) {
		p.logger = logger
	}
}

// NewChatGPTProvider creates a new ChatGPT OAuth provider.
func NewChatGPTProvider(opts ...ChatGPTOption) *ChatGPTProvider {
	p := &ChatGPTProvider{
		BaseProvider: BaseProvider{
			name:               "chatgpt",
			label:              "ChatGPT/Codex (Experimental)",
			authURL:            chatgptAuthURL,
			tokenURL:           chatgptTokenURL,
			scopes:             chatgptScopes,
			supportsPKCE:       true,
			supportsDeviceCode: false,
		},
		clientID:     chatgptDefaultClientID,
		redirectPort: chatgptDefaultPort,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		logger:       slog.Default().With("provider", "chatgpt"),
	}

	for _, opt := range opts {
		opt(p)
	}

	// Log experimental warning
	p.logger.Warn("ChatGPT OAuth is experimental and may stop working at any time")

	return p
}

// IsExperimental returns true (ChatGPT OAuth is experimental).
func (p *ChatGPTProvider) IsExperimental() bool {
	return true
}

// AuthURL returns the authorization URL for the OAuth flow.
func (p *ChatGPTProvider) AuthURL(state, challenge string) string {
	params := url.Values{
		"client_id":             {p.clientID},
		"response_type":         {"code"},
		"redirect_uri":          {p.redirectURI()},
		"scope":                 {strings.Join(p.scopes, " ")},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"audience":              {"https://api.openai.com/v1"},
	}

	return chatgptAuthURL + "?" + params.Encode()
}

// redirectURI returns the OAuth redirect URI.
func (p *ChatGPTProvider) redirectURI() string {
	return fmt.Sprintf("http://localhost:%d/oauth/callback", p.redirectPort)
}

// RedirectPort returns the configured redirect port.
func (p *ChatGPTProvider) RedirectPort() int {
	return p.redirectPort
}

// ExchangeCode exchanges an authorization code for tokens.
func (p *ChatGPTProvider) ExchangeCode(ctx context.Context, code, verifier string) (*oauth.OAuthCredential, error) {
	data := url.Values{
		"client_id":     {p.clientID},
		"code":          {code},
		"code_verifier": {verifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {p.redirectURI()},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp oauth.TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Get user info
	email, _ := p.getUserInfo(ctx, tokenResp.AccessToken)

	return &oauth.OAuthCredential{
		Provider:     p.name,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Email:        email,
		ClientID:     p.clientID,
	}, nil
}

// RefreshToken refreshes an access token.
func (p *ChatGPTProvider) RefreshToken(ctx context.Context, refreshToken string) (*oauth.OAuthCredential, error) {
	data := url.Values{
		"client_id":     {p.clientID},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptTokenURL, strings.NewReader(data.Encode()))
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

// getUserInfo fetches the user's info.
func (p *ChatGPTProvider) getUserInfo(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chatgptUserInfo, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var userInfo struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return "", err
	}

	return userInfo.Email, nil
}

// ChatRequest represents a request to the ChatGPT Codex API.
type ChatRequest struct {
	Model       string           `json:"model"`
	Messages    []ChatMessage    `json:"messages"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

// ChatMessage represents a message in a chat request.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse represents a response from the ChatGPT Codex API.
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// BuildAPIRequest builds an HTTP request for the ChatGPT Codex API.
func (p *ChatGPTProvider) BuildAPIRequest(ctx context.Context, accessToken string, req *ChatRequest) (*http.Request, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		chatgptAPIBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")

	return httpReq, nil
}

// DoAPIRequest executes a request to the ChatGPT Codex API.
func (p *ChatGPTProvider) DoAPIRequest(ctx context.Context, accessToken string, req *ChatRequest) (*ChatResponse, error) {
	httpReq, err := p.BuildAPIRequest(ctx, accessToken, req)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &chatResp, nil
}

// APIBase returns the ChatGPT Codex API base URL.
func (p *ChatGPTProvider) APIBase() string {
	return chatgptAPIBase
}

// base64URLEncode encodes bytes using base64url without padding.
func base64URLEncodeChatGPT(buf []byte) string {
	return base64.RawURLEncoding.EncodeToString(buf)
}

// sha256SumChatGPT computes SHA256 hash.
func sha256SumChatGPT(data string) []byte {
	h := sha256.Sum256([]byte(data))
	return h[:]
}
