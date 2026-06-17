// Package copilot – mcp_oauth.go implements OAuth 2.1 (authorization code +
// PKCE) for remote MCP servers, with automatic endpoint discovery (RFC 8414 /
// RFC 9728) and Dynamic Client Registration (RFC 7591). The agent starts a
// flow, surfaces the consent URL through its channel, the provider redirects to
// the local callback, and tokens are stored in the encrypted vault (keyed per
// server) with transparent refresh. No manual client registration required.
package copilot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// mcpVaultKey is the vault key holding the OAuth token blob for a server.
func mcpVaultKey(server string) string { return "mcp_oauth_" + server }

// mcpOAuthToken is the per-server credential blob persisted in the vault. It
// carries everything needed to refresh without re-running discovery/DCR.
type mcpOAuthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Expiry       int64  `json:"expiry,omitempty"` // unix seconds; 0 = no expiry
	TokenURL     string `json:"token_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// asMetadata is the subset of RFC 8414 authorization-server metadata we use.
type asMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

type mcpAuthFlow struct {
	server       string
	verifier     string
	tokenURL     string
	clientID     string
	clientSecret string
	scope        string
}

// MCPOAuthManager runs OAuth flows and stores tokens for remote MCP servers.
type MCPOAuthManager struct {
	vault       *Vault
	httpc       *http.Client
	redirectURI string
	logger      *slog.Logger

	// onAuthorized, if set, is invoked with the server name once a flow
	// completes successfully (used to connect the server automatically).
	onAuthorized func(server string)

	mu      sync.Mutex
	pending map[string]*mcpAuthFlow // key: state
}

// NewMCPOAuthManager creates an OAuth manager. redirectURI must match what is
// registered with the provider and route to HandleCallback (the local callback
// endpoint, e.g. http://localhost:8085/oauth/mcp/callback).
func NewMCPOAuthManager(vault *Vault, redirectURI string, logger *slog.Logger) *MCPOAuthManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &MCPOAuthManager{
		vault:       vault,
		httpc:       &http.Client{Timeout: 30 * time.Second},
		redirectURI: redirectURI,
		logger:      logger.With("component", "mcp-oauth"),
		pending:     make(map[string]*mcpAuthFlow),
	}
}

// BeginAuthorization discovers the server's authorization endpoints, registers
// a client via DCR, and returns the consent URL the user must open. The PKCE
// verifier and client credentials are held until the callback arrives.
func (m *MCPOAuthManager) BeginAuthorization(ctx context.Context, srv ManagedMCPServerConfig) (string, error) {
	if srv.URL == "" {
		return "", fmt.Errorf("oauth requires an http/sse server with a url")
	}
	meta, err := m.discover(ctx, srv.URL)
	if err != nil {
		return "", fmt.Errorf("discover oauth metadata: %w", err)
	}
	clientID, clientSecret, err := m.registerClient(ctx, meta.RegistrationEndpoint)
	if err != nil {
		return "", fmt.Errorf("dynamic client registration: %w", err)
	}

	verifier, challenge, err := pkcePair()
	if err != nil {
		return "", err
	}
	state, err := randURLToken(24)
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	m.pending[state] = &mcpAuthFlow{
		server:       srv.Name,
		verifier:     verifier,
		tokenURL:     meta.TokenEndpoint,
		clientID:     clientID,
		clientSecret: clientSecret,
		scope:        srv.OAuthScope,
	}
	m.mu.Unlock()

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", m.redirectURI)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	if srv.OAuthScope != "" {
		q.Set("scope", srv.OAuthScope)
	}
	authURL := meta.AuthorizationEndpoint + "?" + q.Encode()
	m.logger.Info("oauth flow started", "server", srv.Name)
	return authURL, nil
}

// HandleCallback completes a flow: it exchanges the authorization code for
// tokens, persists them in the vault, and invokes onAuthorized. Returns the
// server name the flow belonged to.
func (m *MCPOAuthManager) HandleCallback(ctx context.Context, state, code string) (string, error) {
	m.mu.Lock()
	flow := m.pending[state]
	delete(m.pending, state)
	m.mu.Unlock()
	if flow == nil {
		return "", fmt.Errorf("unknown or expired state")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", m.redirectURI)
	form.Set("client_id", flow.clientID)
	form.Set("code_verifier", flow.verifier)
	if flow.clientSecret != "" {
		form.Set("client_secret", flow.clientSecret)
	}

	tok, err := m.postToken(ctx, flow.tokenURL, form)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	tok.TokenURL = flow.tokenURL
	tok.ClientID = flow.clientID
	tok.ClientSecret = flow.clientSecret
	if tok.Scope == "" {
		tok.Scope = flow.scope
	}
	if err := m.storeToken(flow.server, tok); err != nil {
		return "", err
	}
	m.logger.Info("oauth flow completed", "server", flow.server)
	if m.onAuthorized != nil {
		m.onAuthorized(flow.server)
	}
	return flow.server, nil
}

// HasToken reports whether a stored token exists for a server.
func (m *MCPOAuthManager) HasToken(server string) bool {
	if m.vault == nil {
		return false
	}
	v, err := m.vault.Get(mcpVaultKey(server))
	return err == nil && v != ""
}

// provider returns an authProvider bound to a server (used by the bridge).
func (m *MCPOAuthManager) provider(server string) authProvider {
	return &oauthAuthProvider{mgr: m, server: server}
}

// ---------------------------------------------------------------------------
// token storage + refresh
// ---------------------------------------------------------------------------

func (m *MCPOAuthManager) storeToken(server string, tok *mcpOAuthToken) error {
	if m.vault == nil {
		return fmt.Errorf("vault is not available; cannot persist oauth token")
	}
	data, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	return m.vault.Set(mcpVaultKey(server), string(data))
}

func (m *MCPOAuthManager) loadToken(server string) (*mcpOAuthToken, error) {
	if m.vault == nil {
		return nil, fmt.Errorf("vault is not available")
	}
	raw, err := m.vault.Get(mcpVaultKey(server))
	if err != nil || raw == "" {
		return nil, fmt.Errorf("no oauth token for %q (run mcp authorize)", server)
	}
	var tok mcpOAuthToken
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		return nil, fmt.Errorf("corrupt oauth token for %q: %w", server, err)
	}
	return &tok, nil
}

// validAccessToken returns a non-expired access token for a server, refreshing
// it transparently when needed.
func (m *MCPOAuthManager) validAccessToken(ctx context.Context, server string) (string, error) {
	tok, err := m.loadToken(server)
	if err != nil {
		return "", err
	}
	// Refresh if expiring within 60s and a refresh token is available.
	if tok.Expiry > 0 && time.Now().Unix() > tok.Expiry-60 {
		if tok.RefreshToken == "" {
			return "", fmt.Errorf("access token for %q expired and no refresh token (run mcp authorize)", server)
		}
		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", tok.RefreshToken)
		form.Set("client_id", tok.ClientID)
		if tok.ClientSecret != "" {
			form.Set("client_secret", tok.ClientSecret)
		}
		fresh, err := m.postToken(ctx, tok.TokenURL, form)
		if err != nil {
			return "", fmt.Errorf("refresh token for %q: %w", server, err)
		}
		// Carry over fields the refresh response may omit.
		fresh.TokenURL = tok.TokenURL
		fresh.ClientID = tok.ClientID
		fresh.ClientSecret = tok.ClientSecret
		if fresh.RefreshToken == "" {
			fresh.RefreshToken = tok.RefreshToken
		}
		if err := m.storeToken(server, fresh); err != nil {
			return "", err
		}
		tok = fresh
	}
	return tok.AccessToken, nil
}

// postToken posts a token-endpoint form and decodes the OAuth token response.
func (m *MCPOAuthManager) postToken(ctx context.Context, tokenURL string, form url.Values) (*mcpOAuthToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := m.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if raw.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}
	tok := &mcpOAuthToken{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		TokenType:    raw.TokenType,
		Scope:        raw.Scope,
	}
	if raw.ExpiresIn > 0 {
		tok.Expiry = time.Now().Unix() + raw.ExpiresIn
	}
	return tok, nil
}

// ---------------------------------------------------------------------------
// discovery (RFC 8414 / RFC 9728) + dynamic client registration (RFC 7591)
// ---------------------------------------------------------------------------

// discover resolves the authorization-server metadata for an MCP server URL.
// It tries the AS metadata at the origin first, then falls back to the
// protected-resource metadata which points at the real authorization server.
func (m *MCPOAuthManager) discover(ctx context.Context, serverURL string) (*asMetadata, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server url: %w", err)
	}
	origin := u.Scheme + "://" + u.Host

	// 1) Direct authorization-server metadata at the origin.
	if meta, err := m.fetchASMetadata(ctx, origin+"/.well-known/oauth-authorization-server"); err == nil {
		return meta, nil
	}

	// 2) Protected-resource metadata → authorization_servers[0] → AS metadata.
	prURL := origin + "/.well-known/oauth-protected-resource"
	var pr struct {
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := m.fetchJSON(ctx, prURL, &pr); err == nil && len(pr.AuthorizationServers) > 0 {
		as := strings.TrimRight(pr.AuthorizationServers[0], "/")
		if meta, err := m.fetchASMetadata(ctx, as+"/.well-known/oauth-authorization-server"); err == nil {
			return meta, nil
		}
		// Some providers expose OIDC discovery instead.
		if meta, err := m.fetchASMetadata(ctx, as+"/.well-known/openid-configuration"); err == nil {
			return meta, nil
		}
	}
	return nil, fmt.Errorf("no oauth metadata found at %s", origin)
}

func (m *MCPOAuthManager) fetchASMetadata(ctx context.Context, metaURL string) (*asMetadata, error) {
	var meta asMetadata
	if err := m.fetchJSON(ctx, metaURL, &meta); err != nil {
		return nil, err
	}
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		return nil, fmt.Errorf("metadata missing authorization/token endpoint")
	}
	return &meta, nil
}

func (m *MCPOAuthManager) fetchJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := m.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d for %s", resp.StatusCode, u)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// registerClient performs Dynamic Client Registration. When no registration
// endpoint is advertised it returns a public client with an empty id, which
// works for providers that accept unregistered public PKCE clients.
func (m *MCPOAuthManager) registerClient(ctx context.Context, registrationEndpoint string) (clientID, clientSecret string, err error) {
	if registrationEndpoint == "" {
		return "", "", fmt.Errorf("server does not advertise a registration endpoint")
	}
	reqBody, _ := json.Marshal(map[string]any{
		"client_name":                "devclaw",
		"redirect_uris":              []string{m.redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := m.httpc.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var reg struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(body, &reg); err != nil {
		return "", "", fmt.Errorf("parse registration response: %w", err)
	}
	if reg.ClientID == "" {
		return "", "", fmt.Errorf("registration response missing client_id")
	}
	return reg.ClientID, reg.ClientSecret, nil
}

// ---------------------------------------------------------------------------
// PKCE + random helpers
// ---------------------------------------------------------------------------

func pkcePair() (verifier, challenge string, err error) {
	verifier, err = randURLToken(48)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randURLToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ---------------------------------------------------------------------------
// authProvider (consumed by the HTTP transport)
// ---------------------------------------------------------------------------

type oauthAuthProvider struct {
	mgr    *MCPOAuthManager
	server string
}

func (p *oauthAuthProvider) AuthHeader(ctx context.Context) (string, error) {
	access, err := p.mgr.validAccessToken(ctx, p.server)
	if err != nil {
		return "", err
	}
	return "Bearer " + access, nil
}
