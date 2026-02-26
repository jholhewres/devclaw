package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/oauth"
	"github.com/jholhewres/devclaw/pkg/devclaw/oauth/providers"
)

// OAuthAPI provides OAuth operations for the web UI.
type OAuthAPI interface {
	// TokenManager returns the OAuth token manager
	GetTokenManager() *oauth.TokenManager
}

// oauthFlow tracks active OAuth flows for callback handling.
type oauthFlow struct {
	state     string
	pkce      *oauth.PKCEPair
	provider  string
	expiresAt time.Time
	result    chan oauthFlowResult
}

type oauthFlowResult struct {
	cred *oauth.OAuthCredential
	err  error
}

// OAuthHandlers manages OAuth-related HTTP handlers.
type OAuthHandlers struct {
	tokenManager *oauth.TokenManager
	logger       *slog.Logger

	flowsMu sync.RWMutex
	flows   map[string]*oauthFlow // state -> flow

	dataDir string
}

// NewOAuthHandlers creates new OAuth handlers.
func NewOAuthHandlers(dataDir string, logger *slog.Logger) (*OAuthHandlers, error) {
	tm, err := oauth.NewTokenManager(dataDir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create token manager: %w", err)
	}

	// Register providers
	tm.RegisterProvider(providers.NewGeminiProvider())
	tm.RegisterProvider(providers.NewChatGPTProvider())
	tm.RegisterProvider(providers.NewQwenProvider())
	tm.RegisterProvider(providers.NewMiniMaxProvider())

	// Start auto-refresh
	tm.StartAutoRefresh()

	return &OAuthHandlers{
		tokenManager: tm,
		logger:       logger.With("component", "oauth-handlers"),
		flows:        make(map[string]*oauthFlow),
		dataDir:      dataDir,
	}, nil
}

// TokenManager returns the token manager.
func (h *OAuthHandlers) TokenManager() *oauth.TokenManager {
	return h.tokenManager
}

// Stop stops the OAuth handlers.
func (h *OAuthHandlers) Stop() {
	if h.tokenManager != nil {
		h.tokenManager.Stop()
	}
}

// RegisterRoutes registers OAuth routes on the mux.
func (h *OAuthHandlers) RegisterRoutes(mux *http.ServeMux, authMiddleware func(http.HandlerFunc) http.HandlerFunc) {
	// Public routes (for OAuth callbacks)
	mux.HandleFunc("/api/oauth/callback", h.handleOAuthCallback)

	// Protected routes
	mux.HandleFunc("/api/oauth/providers", authMiddleware(h.handleListProviders))
	mux.HandleFunc("/api/oauth/status", authMiddleware(h.handleOAuthStatus))
	mux.HandleFunc("/api/oauth/start/", authMiddleware(h.handleOAuthStart))
	mux.HandleFunc("/api/oauth/refresh/", authMiddleware(h.handleOAuthRefresh))
	mux.HandleFunc("/api/oauth/logout/", authMiddleware(h.handleOAuthLogout))
}

// OAuthProviderInfo contains provider info for the UI.
type OAuthProviderInfo struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	FlowType    string `json:"flow_type"` // "pkce" or "device_code"
	Experimental bool   `json:"experimental,omitempty"`
}

// handleListProviders returns available OAuth providers.
func (h *OAuthHandlers) handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers := []OAuthProviderInfo{
		{ID: "gemini", Label: "Google Gemini", FlowType: "pkce"},
		{ID: "chatgpt", Label: "ChatGPT/Codex", FlowType: "pkce", Experimental: true},
		{ID: "qwen", Label: "Qwen Portal", FlowType: "device_code"},
		{ID: "minimax", Label: "MiniMax Portal", FlowType: "device_code"},
	}

	writeOAuthJSON(w, http.StatusOK, providers)
}

// handleOAuthStatus returns OAuth status for all providers.
func (h *OAuthHandlers) handleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	status := h.tokenManager.GetStatus()
	writeOAuthJSON(w, http.StatusOK, status)
}

// OAuthStartResponse is returned when starting an OAuth flow.
type OAuthStartResponse struct {
	FlowType    string `json:"flow_type"`
	AuthURL     string `json:"auth_url,omitempty"`     // For PKCE flow
	Provider    string `json:"provider"`
	UserCode    string `json:"user_code,omitempty"`    // For device code flow
	VerifyURL   string `json:"verify_url,omitempty"`   // For device code flow
	ExpiresIn   int    `json:"expires_in,omitempty"`   // For device code flow
	Experimental bool   `json:"experimental,omitempty"` // Warning flag
}

// handleOAuthStart starts an OAuth flow for a provider.
func (h *OAuthHandlers) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimPrefix(r.URL.Path, "/api/oauth/start/")
	if provider == "" {
		writeOAuthError(w, http.StatusBadRequest, "provider required")
		return
	}

	ctx := r.Context()

	switch provider {
	case "gemini":
		h.startPKCEFlow(ctx, w, r, provider, providers.NewGeminiProvider())
	case "chatgpt":
		h.startPKCEFlow(ctx, w, r, provider, providers.NewChatGPTProvider())
	case "qwen":
		h.startDeviceCodeFlow(ctx, w, r, provider, providers.NewQwenProvider())
	case "minimax":
		region := r.URL.Query().Get("region")
		if region == "" {
			region = "global"
		}
		h.startDeviceCodeFlow(ctx, w, r, provider,
			providers.NewMiniMaxProvider(providers.WithMiniMaxRegion(region)))
	default:
		writeOAuthError(w, http.StatusBadRequest, "unknown provider: "+provider)
	}
}

// PKCEProvider is the interface for PKCE-based OAuth providers.
type PKCEProvider interface {
	Name() string
	Label() string
	AuthURL(state, challenge string) string
	ExchangeCode(ctx context.Context, code, verifier string) (*oauth.OAuthCredential, error)
	RedirectPort() int
}

func (h *OAuthHandlers) startPKCEFlow(ctx context.Context, w http.ResponseWriter, r *http.Request, provider string, p PKCEProvider) {
	// Generate PKCE
	pkce, err := oauth.GeneratePKCE()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "failed to generate PKCE: "+err.Error())
		return
	}

	// Generate state
	state := generateState()

	// Store flow for callback
	flow := &oauthFlow{
		state:     state,
		pkce:      pkce,
		provider:  provider,
		expiresAt: time.Now().Add(10 * time.Minute),
		result:    make(chan oauthFlowResult, 1),
	}

	h.flowsMu.Lock()
	h.flows[state] = flow
	h.flowsMu.Unlock()

	// Cleanup old flows
	go h.cleanupExpiredFlows()

	// Build auth URL
	authURL := p.AuthURL(state, pkce.Challenge)

	// Response
	resp := OAuthStartResponse{
		FlowType: "pkce",
		AuthURL:  authURL,
		Provider: provider,
	}

	if provider == "chatgpt" {
		resp.Experimental = true
	}

	writeOAuthJSON(w, http.StatusOK, resp)
}

// DeviceCodeProvider is the interface for device code OAuth providers.
type DeviceCodeProvider interface {
	Name() string
	StartDeviceFlow(ctx context.Context) (*oauth.DeviceCodeResponse, error)
	PollForToken(ctx context.Context, deviceCode string, interval time.Duration) (*oauth.OAuthCredential, error)
}

func (h *OAuthHandlers) startDeviceCodeFlow(ctx context.Context, w http.ResponseWriter, r *http.Request, provider string, p DeviceCodeProvider) {
	// Start device code flow
	deviceResp, err := p.StartDeviceFlow(ctx)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "failed to start device code flow: "+err.Error())
		return
	}

	// Response
	resp := OAuthStartResponse{
		FlowType:  "device_code",
		Provider:  provider,
		UserCode:  deviceResp.UserCode,
		VerifyURL: deviceResp.VerificationURI,
		ExpiresIn: deviceResp.ExpiresIn,
	}

	writeOAuthJSON(w, http.StatusOK, resp)
}

// handleOAuthCallback handles OAuth callbacks.
func (h *OAuthHandlers) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Check for error
	if err := query.Get("error"); err != "" {
		writeOAuthError(w, http.StatusBadRequest, "OAuth error: "+err)
		return
	}

	// Get code and state
	code := query.Get("code")
	state := query.Get("state")

	if code == "" || state == "" {
		writeOAuthError(w, http.StatusBadRequest, "missing code or state")
		return
	}

	// Find flow
	h.flowsMu.RLock()
	flow, ok := h.flows[state]
	h.flowsMu.RUnlock()

	if !ok {
		writeOAuthError(w, http.StatusBadRequest, "invalid or expired state")
		return
	}

	// Exchange code for token
	var cred *oauth.OAuthCredential
	var err error

	ctx := r.Context()

	switch flow.provider {
	case "gemini":
		p := providers.NewGeminiProvider()
		cred, err = p.ExchangeCode(ctx, code, flow.pkce.Verifier)
	case "chatgpt":
		p := providers.NewChatGPTProvider()
		cred, err = p.ExchangeCode(ctx, code, flow.pkce.Verifier)
	default:
		err = fmt.Errorf("unknown provider: %s", flow.provider)
	}

	if err != nil {
		// Send error response
		select {
		case flow.result <- oauthFlowResult{err: err}:
		default:
		}

		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<html><body><h1>Authentication Failed</h1><p>%s</p></body></html>`, err.Error())
		return
	}

	// Save credential
	if err := h.tokenManager.SaveCredential(cred); err != nil {
		select {
		case flow.result <- oauthFlowResult{err: err}:
		default:
		}

		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `<html><body><h1>Error</h1><p>Failed to save credential: %s</p></body></html>`, err.Error())
		return
	}

	// Cleanup flow
	h.flowsMu.Lock()
	delete(h.flows, state)
	h.flowsMu.Unlock()

	// Send success result
	select {
	case flow.result <- oauthFlowResult{cred: cred}:
	default:
	}

	// Success page
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
	<title>Authentication Successful</title>
	<style>
		body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #f5f5f5; }
		.container { text-align: center; padding: 2rem; background: white; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
		h1 { color: #22c55e; margin-bottom: 0.5rem; }
		p { color: #666; }
	</style>
</head>
<body>
	<div class="container">
		<h1>✓ Authentication Successful</h1>
		<p>You can close this window and return to DevClaw.</p>
	</div>
	<script>
		if (window.opener) {
			window.opener.postMessage({ type: 'oauth-success', provider: '`+flow.provider+`' }, '*');
		}
	</script>
</body>
</html>`)
}

// handleOAuthRefresh manually refreshes a token.
func (h *OAuthHandlers) handleOAuthRefresh(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimPrefix(r.URL.Path, "/api/oauth/refresh/")
	if provider == "" {
		writeOAuthError(w, http.StatusBadRequest, "provider required")
		return
	}

	cred, err := h.tokenManager.Refresh(provider)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "failed to refresh token: "+err.Error())
		return
	}

	writeOAuthJSON(w, http.StatusOK, map[string]interface{}{
		"provider":   cred.Provider,
		"email":      cred.Email,
		"expires_at": cred.ExpiresAt,
	})
}

// handleOAuthLogout removes OAuth credentials.
func (h *OAuthHandlers) handleOAuthLogout(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimPrefix(r.URL.Path, "/api/oauth/logout/")
	if provider == "" {
		writeOAuthError(w, http.StatusBadRequest, "provider required")
		return
	}

	if err := h.tokenManager.DeleteCredential(provider); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "failed to logout: "+err.Error())
		return
	}

	writeOAuthJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// cleanupExpiredFlows removes expired OAuth flows.
func (h *OAuthHandlers) cleanupExpiredFlows() {
	h.flowsMu.Lock()
	defer h.flowsMu.Unlock()

	now := time.Now()
	for state, flow := range h.flows {
		if flow.expiresAt.Before(now) {
			delete(h.flows, state)
		}
	}
}

// Helper functions

func generateState() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// writeOAuthJSON writes JSON response (renamed to avoid conflict with server.go)
func writeOAuthJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeOAuthError(w http.ResponseWriter, status int, message string) {
	writeOAuthJSON(w, status, map[string]string{"error": message})
}

// openBrowser opens a URL in the default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Start()
}

// GetDataDir returns the default data directory.
func GetDataDir() (string, error) {
	return "./data", nil
}
