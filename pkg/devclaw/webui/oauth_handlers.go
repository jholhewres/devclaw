package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	baseURL   string // Base URL for redirect (e.g., "http://example.com:47716")
	expiresAt time.Time
	result    chan oauthFlowResult
}

type oauthFlowResult struct {
	cred *oauth.OAuthCredential
	err  error
}

// HubConfig holds OAuth Hub configuration for Hub mode.
type HubConfig struct {
	Enabled bool
	HubURL  string
	APIKey  string
}

// OAuthHandlers manages OAuth-related HTTP handlers.
type OAuthHandlers struct {
	tokenManager *oauth.TokenManager
	logger       *slog.Logger

	flowsMu sync.RWMutex
	flows   map[string]*oauthFlow // state -> flow

	dataDir string

	// Hub mode: delegate to OAuth Hub for external service connections
	hubConfig *HubConfig

	// onSkillInstall writes a skill to disk and reloads the registry.
	onSkillInstall func(name, content string) error

	// onSkillBundleInstall writes multiple files for a skill bundle and reloads the registry.
	onSkillBundleInstall func(name string, files map[string]string) error

	// onSkillReferenceRemove removes a reference file from a skill and reloads the registry.
	onSkillReferenceRemove func(skillName, refPath string) error

	// onSaveSecret stores a secret in the vault (encrypted).
	onSaveSecret func(name, value string) error

	// onSaveHubURL persists the Hub URL to the config file.
	onSaveHubURL func(hubURL string) error
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

// SetHubConfig enables Hub mode for OAuth operations.
func (h *OAuthHandlers) SetHubConfig(cfg *HubConfig) {
	h.hubConfig = cfg
}

// SetSkillInstaller sets the callback for writing skills to disk and reloading the registry.
func (h *OAuthHandlers) SetSkillInstaller(fn func(name, content string) error) {
	h.onSkillInstall = fn
}

// SetSkillBundleInstaller sets the callback for writing multi-file skill bundles.
func (h *OAuthHandlers) SetSkillBundleInstaller(fn func(name string, files map[string]string) error) {
	h.onSkillBundleInstall = fn
}

// SetSkillReferenceRemover sets the callback for removing skill reference files.
func (h *OAuthHandlers) SetSkillReferenceRemover(fn func(skillName, refPath string) error) {
	h.onSkillReferenceRemove = fn
}

// SetSecretSaver sets the callback for storing secrets in the vault.
func (h *OAuthHandlers) SetSecretSaver(fn func(name, value string) error) {
	h.onSaveSecret = fn
}

// SetHubURLSaver sets the callback for persisting the Hub URL to config.
func (h *OAuthHandlers) SetHubURLSaver(fn func(hubURL string) error) {
	h.onSaveHubURL = fn
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


	// Hub mode routes
	mux.HandleFunc("/api/oauth/hub/start/", authMiddleware(h.handleHubStart))
	mux.HandleFunc("/api/oauth/hub/status/", authMiddleware(h.handleHubStatus))
	mux.HandleFunc("/api/oauth/hub/connections", authMiddleware(h.handleHubConnections))
	mux.HandleFunc("/api/oauth/hub/connections/", authMiddleware(h.handleHubConnectionAction))
	mux.HandleFunc("/api/oauth/hub/skills", authMiddleware(h.handleHubSkills))
	mux.HandleFunc("/api/oauth/hub/skills/install", authMiddleware(h.handleHubSkillInstall))
	mux.HandleFunc("/api/oauth/hub/skills/install-bundle", authMiddleware(h.handleHubInstallBundle))
	mux.HandleFunc("/api/oauth/hub/skills/install-reference", authMiddleware(h.handleHubInstallReference))
	mux.HandleFunc("/api/oauth/hub/skills/remove-reference", authMiddleware(h.handleHubRemoveReference))
	mux.HandleFunc("/api/oauth/hub/connect", authMiddleware(h.handleHubConnect))
	mux.HandleFunc("/api/oauth/hub/setup", authMiddleware(h.handleHubSetup))
	mux.HandleFunc("/api/oauth/hub/config", authMiddleware(h.handleHubConfigStatus))
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

	// Get base URL from request for callback
	baseURL := getBaseURLFromRequest(r)

	// Store flow for callback
	flow := &oauthFlow{
		state:     state,
		pkce:      pkce,
		provider:  provider,
		baseURL:   baseURL,
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


// getBaseURLFromRequest extracts the base URL from the request.
func getBaseURLFromRequest(r *http.Request) string {
	// Check for X-Forwarded-* headers (proxy/load balancer)
	scheme := "http"
	host := r.Host

	if r.URL.Scheme != "" {
		scheme = r.URL.Scheme
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}

	return fmt.Sprintf("%s://%s", scheme, host)
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

// --- Hub mode handlers ---

// startHubFlow starts an OAuth flow via the Hub for external service providers.
func (h *OAuthHandlers) startHubFlow(w http.ResponseWriter, r *http.Request, provider string) {
	// Call Hub to start connection
	hubResp, err := h.hubRequest("POST", "/api/v1/connect/start", map[string]any{
		"provider": provider,
	})
	if err != nil {
		writeOAuthError(w, http.StatusBadGateway, "hub connection failed: "+err.Error())
		return
	}
	defer hubResp.Body.Close()

	if hubResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(hubResp.Body)
		writeOAuthError(w, hubResp.StatusCode, "hub error: "+string(body))
		return
	}

	var session struct {
		SessionID  string `json:"session_id"`
		ConnectURL string `json:"connect_url"`
		ExpiresIn  int    `json:"expires_in"`
	}
	if err := json.NewDecoder(hubResp.Body).Decode(&session); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "invalid hub response")
		return
	}

	writeOAuthJSON(w, http.StatusOK, OAuthStartResponse{
		FlowType: "hub",
		AuthURL:  session.ConnectURL,
		Provider: provider,
	})
}

// handleHubStart starts a connection flow via the Hub (alternative endpoint).
func (h *OAuthHandlers) handleHubStart(w http.ResponseWriter, r *http.Request) {
	if h.hubConfig == nil || !h.hubConfig.Enabled {
		writeOAuthError(w, http.StatusBadRequest, "hub mode not enabled")
		return
	}

	provider := strings.TrimPrefix(r.URL.Path, "/api/oauth/hub/start/")
	if provider == "" {
		writeOAuthError(w, http.StatusBadRequest, "provider required")
		return
	}

	h.startHubFlow(w, r, provider)
}

// handleHubStatus checks the status of a Hub OAuth session.
func (h *OAuthHandlers) handleHubStatus(w http.ResponseWriter, r *http.Request) {
	if h.hubConfig == nil || !h.hubConfig.Enabled {
		writeOAuthError(w, http.StatusBadRequest, "hub mode not enabled")
		return
	}

	sessionID := strings.TrimPrefix(r.URL.Path, "/api/oauth/hub/status/")
	if sessionID == "" {
		writeOAuthError(w, http.StatusBadRequest, "session_id required")
		return
	}

	hubResp, err := h.hubRequest("GET", "/api/v1/connect/status/"+sessionID, nil)
	if err != nil {
		writeOAuthError(w, http.StatusBadGateway, "hub connection failed: "+err.Error())
		return
	}
	defer hubResp.Body.Close()

	body, _ := io.ReadAll(hubResp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(hubResp.StatusCode)
	w.Write(body)
}

// handleHubConnections lists connections from the Hub.
func (h *OAuthHandlers) handleHubConnections(w http.ResponseWriter, r *http.Request) {
	if h.hubConfig == nil || !h.hubConfig.Enabled {
		writeOAuthError(w, http.StatusBadRequest, "hub mode not enabled")
		return
	}

	hubResp, err := h.hubRequest("GET", "/api/v1/connections", nil)
	if err != nil {
		writeOAuthError(w, http.StatusBadGateway, "hub connection failed: "+err.Error())
		return
	}
	defer hubResp.Body.Close()

	body, _ := io.ReadAll(hubResp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(hubResp.StatusCode)
	w.Write(body)
}

// hubRequest makes an authenticated HTTP request to the Hub.
func (h *OAuthHandlers) hubRequest(method, path string, body any) (*http.Response, error) {
	if h.hubConfig == nil {
		return nil, fmt.Errorf("hub not configured")
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = strings.NewReader(string(data))
	}

	url := strings.TrimRight(h.hubConfig.HubURL, "/") + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+h.hubConfig.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

// --- New Hub endpoints (GatorHub UI) ---

// handleHubConnect starts an OAuth connection for a specific service via the Hub.
func (h *OAuthHandlers) handleHubConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.hubConfig == nil || !h.hubConfig.Enabled {
		writeOAuthError(w, http.StatusBadRequest, "hub mode not enabled")
		return
	}

	var body struct {
		Provider string   `json:"provider"`
		Service  string   `json:"service"`
		Scopes   []string `json:"scopes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Provider == "" {
		body.Provider = "google"
	}
	if body.Service == "" {
		writeOAuthError(w, http.StatusBadRequest, "service is required")
		return
	}

	hubResp, err := h.hubRequest("POST", "/api/v1/connect/start", map[string]any{
		"provider": body.Provider,
		"service":  body.Service,
		"scopes":   body.Scopes,
	})
	if err != nil {
		writeOAuthError(w, http.StatusBadGateway, "hub connection failed: "+err.Error())
		return
	}
	defer hubResp.Body.Close()

	respBody, _ := io.ReadAll(hubResp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(hubResp.StatusCode)
	w.Write(respBody)
}

// handleHubSkills lists available skills from the Hub with connection status.
func (h *OAuthHandlers) handleHubSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOAuthError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.hubConfig == nil || !h.hubConfig.Enabled {
		writeOAuthError(w, http.StatusBadRequest, "hub mode not enabled")
		return
	}

	hubResp, err := h.hubRequest("GET", "/api/v1/skills", nil)
	if err != nil {
		writeOAuthError(w, http.StatusBadGateway, "hub connection failed: "+err.Error())
		return
	}
	defer hubResp.Body.Close()

	respBody, _ := io.ReadAll(hubResp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(hubResp.StatusCode)
	w.Write(respBody)
}

// handleHubSkillInstall downloads a skill from the Hub and installs it locally.
func (h *OAuthHandlers) handleHubSkillInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.hubConfig == nil || !h.hubConfig.Enabled {
		writeOAuthError(w, http.StatusBadRequest, "hub mode not enabled")
		return
	}

	var body struct {
		SkillID string `json:"skill_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SkillID == "" {
		writeOAuthError(w, http.StatusBadRequest, "skill_id is required")
		return
	}

	// Fetch skill content from Hub
	hubResp, err := h.hubRequest("GET", "/api/v1/skills/"+body.SkillID+"/content", nil)
	if err != nil {
		writeOAuthError(w, http.StatusBadGateway, "hub connection failed: "+err.Error())
		return
	}
	defer hubResp.Body.Close()

	if hubResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(hubResp.Body)
		writeOAuthError(w, hubResp.StatusCode, "hub error: "+string(respBody))
		return
	}

	var skillContent struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(hubResp.Body).Decode(&skillContent); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "invalid hub response")
		return
	}

	// Install the skill using the callback
	if h.onSkillInstall != nil {
		if err := h.onSkillInstall(skillContent.ID, skillContent.Content); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "failed to install skill: "+err.Error())
			return
		}
	} else {
		// Fallback: write directly to ./skills/
		dir := filepath.Join("./skills", skillContent.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "failed to create skill directory: "+err.Error())
			return
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillContent.Content), 0o644); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "failed to write skill file: "+err.Error())
			return
		}
	}

	h.logger.Info("hub skill installed", "skill_id", skillContent.ID)
	writeOAuthJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Skill " + skillContent.ID + " installed successfully.",
	})
}

// handleHubConnectionAction handles DELETE /api/oauth/hub/connections/{id} (disconnect).
func (h *OAuthHandlers) handleHubConnectionAction(w http.ResponseWriter, r *http.Request) {
	if h.hubConfig == nil || !h.hubConfig.Enabled {
		writeOAuthError(w, http.StatusBadRequest, "hub mode not enabled")
		return
	}

	connectionID := strings.TrimPrefix(r.URL.Path, "/api/oauth/hub/connections/")
	if connectionID == "" {
		writeOAuthError(w, http.StatusBadRequest, "connection_id required")
		return
	}

	if r.Method != http.MethodDelete {
		writeOAuthError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	hubResp, err := h.hubRequest("DELETE", "/api/v1/connections/"+connectionID, nil)
	if err != nil {
		writeOAuthError(w, http.StatusBadGateway, "hub connection failed: "+err.Error())
		return
	}
	defer hubResp.Body.Close()

	respBody, _ := io.ReadAll(hubResp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(hubResp.StatusCode)
	w.Write(respBody)
}

// handleHubSetup configures the Hub URL and API key.
func (h *OAuthHandlers) handleHubSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var body struct {
		HubURL string `json:"hub_url"`
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.HubURL == "" || body.APIKey == "" {
		writeOAuthError(w, http.StatusBadRequest, "hub_url and api_key are required")
		return
	}

	body.HubURL = strings.TrimRight(body.HubURL, "/")

	// Validate by calling Hub health endpoint
	testCfg := &HubConfig{
		Enabled: true,
		HubURL:  body.HubURL,
		APIKey:  body.APIKey,
	}
	oldCfg := h.hubConfig
	h.hubConfig = testCfg

	hubResp, err := h.hubRequest("GET", "/api/v1/health", nil)
	if err != nil {
		h.hubConfig = oldCfg
		writeOAuthError(w, http.StatusBadGateway, "cannot connect to Hub: "+err.Error())
		return
	}
	hubResp.Body.Close()

	if hubResp.StatusCode != http.StatusOK {
		h.hubConfig = oldCfg
		writeOAuthError(w, http.StatusBadGateway, "Hub health check failed")
		return
	}

	// Save API key to vault (encrypted) if vault callback is available
	if h.onSaveSecret != nil {
		if err := h.onSaveSecret("DEVCLAW_HUB_API_KEY", body.APIKey); err != nil {
			h.hubConfig = oldCfg
			writeOAuthError(w, http.StatusInternalServerError, "failed to save API key to vault: "+err.Error())
			return
		}
	}

	// Save Hub URL to config
	if h.onSaveHubURL != nil {
		if err := h.onSaveHubURL(body.HubURL); err != nil {
			h.logger.Warn("failed to save Hub URL to config", "error", err)
		}
	}

	// Save hub_url to local hub_config.json (current directory) as fallback
	cfgData, _ := json.MarshalIndent(map[string]string{
		"hub_url": body.HubURL,
	}, "", "  ")
	if err := os.WriteFile("hub_config.json", cfgData, 0o644); err != nil {
		h.logger.Warn("failed to save hub_config.json", "error", err)
	}

	// Inject env var for agent skill access
	os.Setenv("DEVCLAW_HUB_API_KEY", body.APIKey)
	h.logger.Info("Hub configured via web UI", "hub_url", body.HubURL)

	// Auto-install gator-hub skill on successful Hub setup
	go h.autoInstallGatorHub()

	writeOAuthJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Hub configured successfully.",
	})
}

// autoInstallGatorHub fetches and installs the gator-hub SKILL.md from the Hub.
func (h *OAuthHandlers) autoInstallGatorHub() {
	if h.onSkillInstall == nil {
		return
	}

	hubResp, err := h.hubRequest("GET", "/api/v1/skills/gator-hub/content", nil)
	if err != nil {
		h.logger.Error("failed to fetch gator-hub from Hub", "error", err)
		return
	}
	defer hubResp.Body.Close()

	if hubResp.StatusCode != http.StatusOK {
		h.logger.Error("failed to fetch gator-hub from Hub", "status", hubResp.StatusCode)
		return
	}

	var content struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(hubResp.Body).Decode(&content); err != nil {
		h.logger.Error("failed to decode gator-hub content", "error", err)
		return
	}

	if err := h.onSkillInstall(content.ID, content.Content); err != nil {
		h.logger.Error("failed to install gator-hub skill", "error", err)
		return
	}

	h.logger.Info("gator-hub skill auto-installed on Hub setup")
}

// handleHubConfigStatus returns the current Hub configuration status.
func (h *OAuthHandlers) handleHubConfigStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOAuthError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	configured := h.hubConfig != nil && h.hubConfig.Enabled
	hubURL := ""
	connected := false

	if configured {
		hubURL = h.hubConfig.HubURL
		// Quick health check
		hubResp, err := h.hubRequest("GET", "/api/v1/health", nil)
		if err == nil {
			hubResp.Body.Close()
			connected = hubResp.StatusCode == http.StatusOK
		}
	}

	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"configured": configured,
		"hub_url":    hubURL,
		"connected":  connected,
	})
}

// handleHubInstallBundle downloads the full gator-hub bundle (SKILL.md + references) and installs it.
func (h *OAuthHandlers) handleHubInstallBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.hubConfig == nil || !h.hubConfig.Enabled {
		writeOAuthError(w, http.StatusBadRequest, "hub mode not enabled")
		return
	}

	var body struct {
		SkillID string `json:"skill_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SkillID == "" {
		body.SkillID = "gator-hub"
	}

	hubResp, err := h.hubRequest("GET", "/api/v1/skills/"+body.SkillID+"/bundle", nil)
	if err != nil {
		writeOAuthError(w, http.StatusBadGateway, "hub connection failed: "+err.Error())
		return
	}
	defer hubResp.Body.Close()

	if hubResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(hubResp.Body)
		writeOAuthError(w, hubResp.StatusCode, "hub error: "+string(respBody))
		return
	}

	var bundle struct {
		ID      string            `json:"id"`
		Version string            `json:"version"`
		Files   map[string]string `json:"files"`
	}
	if err := json.NewDecoder(hubResp.Body).Decode(&bundle); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "invalid hub response")
		return
	}

	if h.onSkillBundleInstall != nil {
		if err := h.onSkillBundleInstall(bundle.ID, bundle.Files); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "failed to install bundle: "+err.Error())
			return
		}
	} else {
		// Fallback: write files directly
		for path, content := range bundle.Files {
			fullPath := filepath.Join("./skills", bundle.ID, path)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				writeOAuthError(w, http.StatusInternalServerError, "failed to create directory: "+err.Error())
				return
			}
			if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
				writeOAuthError(w, http.StatusInternalServerError, "failed to write file: "+err.Error())
				return
			}
		}
	}

	h.logger.Info("hub skill bundle installed", "skill_id", bundle.ID, "files", len(bundle.Files))
	writeOAuthJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Bundle " + bundle.ID + " installed successfully.",
	})
}

// handleHubInstallReference installs a single service reference into gator-hub/references/.
func (h *OAuthHandlers) handleHubInstallReference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.hubConfig == nil || !h.hubConfig.Enabled {
		writeOAuthError(w, http.StatusBadRequest, "hub mode not enabled")
		return
	}

	var body struct {
		Provider string `json:"provider"`
		Service  string `json:"service"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Service == "" {
		writeOAuthError(w, http.StatusBadRequest, "service is required")
		return
	}
	if body.Provider == "" {
		body.Provider = "google"
	}

	// Reference file name follows the pattern: {provider}-{service}.md
	refName := body.Provider + "-" + body.Service + ".md"
	refPath := "references/" + refName

	// Fetch the gator-hub bundle to extract this specific reference
	hubResp, err := h.hubRequest("GET", "/api/v1/skills/gator-hub/bundle", nil)
	if err != nil {
		writeOAuthError(w, http.StatusBadGateway, "hub connection failed: "+err.Error())
		return
	}
	defer hubResp.Body.Close()

	if hubResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(hubResp.Body)
		writeOAuthError(w, hubResp.StatusCode, "hub error: "+string(respBody))
		return
	}

	var bundle struct {
		ID    string            `json:"id"`
		Files map[string]string `json:"files"`
	}
	if err := json.NewDecoder(hubResp.Body).Decode(&bundle); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "invalid hub response")
		return
	}

	content, ok := bundle.Files[refPath]
	if !ok {
		writeOAuthError(w, http.StatusNotFound, "reference not found: "+refPath)
		return
	}

	// Install the reference file
	if h.onSkillBundleInstall != nil {
		files := map[string]string{refPath: content}
		if err := h.onSkillBundleInstall("gator-hub", files); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "failed to install reference: "+err.Error())
			return
		}
	} else {
		fullPath := filepath.Join("./skills", "gator-hub", refPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "failed to create directory: "+err.Error())
			return
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "failed to write file: "+err.Error())
			return
		}
	}

	h.logger.Info("hub service reference installed", "provider", body.Provider, "service", body.Service)
	writeOAuthJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Reference " + refName + " installed.",
	})
}

// handleHubRemoveReference removes a service reference from gator-hub/references/.
func (h *OAuthHandlers) handleHubRemoveReference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOAuthError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var body struct {
		Provider string `json:"provider"`
		Service  string `json:"service"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Service == "" {
		writeOAuthError(w, http.StatusBadRequest, "service is required")
		return
	}
	if body.Provider == "" {
		body.Provider = "google"
	}

	refName := body.Provider + "-" + body.Service + ".md"
	refPath := "references/" + refName

	if h.onSkillReferenceRemove != nil {
		if err := h.onSkillReferenceRemove("gator-hub", refPath); err != nil {
			writeOAuthError(w, http.StatusInternalServerError, "failed to remove reference: "+err.Error())
			return
		}
	} else {
		fullPath := filepath.Join("./skills", "gator-hub", refPath)
		os.Remove(fullPath)
	}

	h.logger.Info("hub service reference removed", "provider", body.Provider, "service", body.Service)
	writeOAuthJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Reference " + refName + " removed.",
	})
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
