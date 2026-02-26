package providers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/oauth"
)

const (
	// Gemini OAuth endpoints
	geminiAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	geminiTokenURL    = "https://oauth2.googleapis.com/token"
	geminiUserInfoURL = "https://www.googleapis.com/oauth2/v1/userinfo"
	geminiCodeAssist  = "https://cloudcode-pa.googleapis.com"

	// Gemini CLI credential extraction paths
	geminiCLIOAuthPath1 = "node_modules/@google/gemini-cli-core/dist/src/code_assist/oauth2.js"
	geminiCLIOAuthPath2 = "node_modules/@google/gemini-cli-core/dist/code_assist/oauth2.js"

	// Default redirect port
	geminiDefaultPort = 8085
)

// Gemini OAuth scopes
var geminiScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}

// GeminiProvider implements OAuth for Google Gemini/Code Assist.
type GeminiProvider struct {
	BaseProvider
	clientID      string
	clientSecret  string
	redirectPort  int
	httpClient    *http.Client
	logger        *slog.Logger
}

// GeminiOption configures the Gemini provider.
type GeminiOption func(*GeminiProvider)

// WithGeminiClientID sets a custom client ID.
func WithGeminiClientID(clientID string) GeminiOption {
	return func(p *GeminiProvider) {
		p.clientID = clientID
	}
}

// WithGeminiClientSecret sets a custom client secret.
func WithGeminiClientSecret(secret string) GeminiOption {
	return func(p *GeminiProvider) {
		p.clientSecret = secret
	}
}

// WithGeminiRedirectPort sets the redirect port.
func WithGeminiRedirectPort(port int) GeminiOption {
	return func(p *GeminiProvider) {
		p.redirectPort = port
	}
}

// WithGeminiLogger sets the logger.
func WithGeminiLogger(logger *slog.Logger) GeminiOption {
	return func(p *GeminiProvider) {
		p.logger = logger
	}
}

// NewGeminiProvider creates a new Gemini OAuth provider.
func NewGeminiProvider(opts ...GeminiOption) *GeminiProvider {
	p := &GeminiProvider{
		BaseProvider: BaseProvider{
			name:               "gemini",
			label:              "Google Gemini",
			authURL:            geminiAuthURL,
			tokenURL:           geminiTokenURL,
			scopes:             geminiScopes,
			supportsPKCE:       true,
			supportsDeviceCode: false,
		},
		redirectPort: geminiDefaultPort,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		logger:       slog.Default().With("provider", "gemini"),
	}

	for _, opt := range opts {
		opt(p)
	}

	// Try to extract client ID from Gemini CLI if not provided
	if p.clientID == "" {
		if cred, err := ExtractGeminiCLICredentials(); err == nil {
			p.clientID = cred.ClientID
			p.clientSecret = cred.ClientSecret
		}
	}

	return p
}

// GeminiCLICredentials contains extracted credentials from Gemini CLI.
type GeminiCLICredentials struct {
	ClientID     string
	ClientSecret string
}

// ExtractGeminiCLICredentials extracts OAuth credentials from an installed Gemini CLI.
func ExtractGeminiCLICredentials() (*GeminiCLICredentials, error) {
	geminiPath, err := findGeminiCLI()
	if err != nil {
		return nil, err
	}

	// Get the directory containing the Gemini CLI
	cliDir := filepath.Dir(filepath.Dir(geminiPath))

	// Search for oauth2.js
	searchPaths := []string{
		filepath.Join(cliDir, geminiCLIOAuthPath1),
		filepath.Join(cliDir, geminiCLIOAuthPath2),
	}

	var content []byte
	for _, path := range searchPaths {
		if data, err := os.ReadFile(path); err == nil {
			content = data
			break
		}
	}

	if content == nil {
		// Try recursive search
		if found := findFile(cliDir, "oauth2.js", 10); found != "" {
			content, _ = os.ReadFile(found)
		}
	}

	if content == nil {
		return nil, fmt.Errorf("could not find oauth2.js in Gemini CLI installation")
	}

	// Extract client ID (pattern: xxx-xxx.apps.googleusercontent.com)
	clientIDRegex := regexp.MustCompile(`(\d+-[a-z0-9]+\.apps\.googleusercontent\.com)`)
	clientIDMatch := clientIDRegex.FindSubmatch(content)
	if clientIDMatch == nil {
		return nil, fmt.Errorf("could not extract client ID from oauth2.js")
	}

	// Extract client secret (pattern: GOCSPX-xxx)
	clientSecretRegex := regexp.MustCompile(`(GOCSPX-[A-Za-z0-9_-]+)`)
	clientSecretMatch := clientSecretRegex.FindSubmatch(content)
	if clientSecretMatch == nil {
		return nil, fmt.Errorf("could not extract client secret from oauth2.js")
	}

	return &GeminiCLICredentials{
		ClientID:     string(clientIDMatch[1]),
		ClientSecret: string(clientSecretMatch[1]),
	}, nil
}

// findGeminiCLI finds the Gemini CLI executable.
func findGeminiCLI() (string, error) {
	// Check PATH
	path, err := exec.LookPath("gemini")
	if err == nil {
		return path, nil
	}

	// Check common installation locations
	homeDir, _ := os.UserHomeDir()
	possiblePaths := []string{
		filepath.Join(homeDir, ".npm-global", "bin", "gemini"),
		filepath.Join(homeDir, ".local", "bin", "gemini"),
		"/usr/local/bin/gemini",
		"/usr/bin/gemini",
	}

	if runtime.GOOS == "windows" {
		possiblePaths = append(possiblePaths,
			filepath.Join(homeDir, "AppData", "Roaming", "npm", "gemini.cmd"),
		)
	}

	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("gemini CLI not found in PATH")
}

// findFile recursively searches for a file.
func findFile(dir, name string, maxDepth int) string {
	if maxDepth <= 0 {
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			if found := findFile(path, name, maxDepth-1); found != "" {
				return found
			}
		} else if entry.Name() == name {
			return path
		}
	}

	return ""
}

// AuthURL returns the authorization URL for the OAuth flow.
func (p *GeminiProvider) AuthURL(state, challenge string) string {
	params := url.Values{
		"client_id":             {p.clientID},
		"response_type":         {"code"},
		"redirect_uri":          {p.redirectURI()},
		"scope":                 {strings.Join(p.scopes, " ")},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}

	return geminiAuthURL + "?" + params.Encode()
}

// redirectURI returns the OAuth redirect URI.
func (p *GeminiProvider) redirectURI() string {
	return fmt.Sprintf("http://localhost:%d/oauth/callback", p.redirectPort)
}

// RedirectPort returns the configured redirect port.
func (p *GeminiProvider) RedirectPort() int {
	return p.redirectPort
}

// ClientID returns the configured client ID.
func (p *GeminiProvider) ClientID() string {
	return p.clientID
}

// ExchangeCode exchanges an authorization code for tokens.
func (p *GeminiProvider) ExchangeCode(ctx context.Context, code, verifier string) (*oauth.OAuthCredential, error) {
	data := url.Values{
		"client_id":     {p.clientID},
		"code":          {code},
		"code_verifier": {verifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {p.redirectURI()},
	}

	if p.clientSecret != "" {
		data.Set("client_secret", p.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiTokenURL, strings.NewReader(data.Encode()))
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

	// Get user email
	email, _ := p.getUserEmail(ctx, tokenResp.AccessToken)

	// Discover/project ID
	projectID, _ := p.discoverProject(ctx, tokenResp.AccessToken)

	cred := &oauth.OAuthCredential{
		Provider:     p.name,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Email:        email,
		ClientID:     p.clientID,
	}

	if projectID != "" {
		cred.Metadata = map[string]string{"project_id": projectID}
	}

	return cred, nil
}

// RefreshToken refreshes an access token.
func (p *GeminiProvider) RefreshToken(ctx context.Context, refreshToken string) (*oauth.OAuthCredential, error) {
	data := url.Values{
		"client_id":     {p.clientID},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	if p.clientSecret != "" {
		data.Set("client_secret", p.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiTokenURL, strings.NewReader(data.Encode()))
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

// getUserEmail fetches the user's email from the userinfo endpoint.
func (p *GeminiProvider) getUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, geminiUserInfoURL, nil)
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

// discoverProject discovers or provisions a Google Cloud project for Code Assist.
func (p *GeminiProvider) discoverProject(ctx context.Context, accessToken string) (string, error) {
	envProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if envProject == "" {
		envProject = os.Getenv("GOOGLE_CLOUD_PROJECT_ID")
	}

	headers := map[string]string{
		"Authorization":    "Bearer " + accessToken,
		"Content-Type":     "application/json",
		"X-Goog-Api-Client": "devclaw-oauth",
	}

	loadBody := map[string]any{
		"cloudaicompanionProject": envProject,
		"metadata": map[string]any{
			"ideType":     "IDE_UNSPECIFIED",
			"platform":    "PLATFORM_UNSPECIFIED",
			"pluginType":  "GEMINI",
			"duetProject": envProject,
		},
	}

	bodyBytes, _ := json.Marshal(loadBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		geminiCodeAssist+"/v1internal:loadCodeAssist", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		CurrentTier               any `json:"currentTier"`
		CloudAICompanionProject   any `json:"cloudaicompanionProject"`
		AllowedTiers              []struct {
			ID        string `json:"id"`
			IsDefault bool   `json:"isDefault"`
		} `json:"allowedTiers"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	// If we have a current tier, try to get project
	if result.CurrentTier != nil {
		switch v := result.CloudAICompanionProject.(type) {
		case string:
			return v, nil
		case map[string]any:
			if id, ok := v["id"].(string); ok {
				return id, nil
			}
		}
		if envProject != "" {
			return envProject, nil
		}
	}

	// No current tier - would need to onboard
	// For now, just return env project or empty
	return envProject, nil
}

// base64URLEncode encodes bytes using base64url without padding.
func base64URLEncode(buf []byte) string {
	return base64.RawURLEncoding.EncodeToString(buf)
}

// sha256Sum computes SHA256 hash.
func sha256Sum(data string) []byte {
	h := sha256.Sum256([]byte(data))
	return h[:]
}
