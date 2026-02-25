package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// refreshBuffer is the time before expiry to trigger a refresh
	refreshBuffer = 5 * time.Minute

	// tokensFileName is the name of the file where tokens are stored
	tokensFileName = "oauth_tokens.json"
)

// TokenStore is the on-disk format for OAuth tokens.
type TokenStore struct {
	Version   int                          `json:"version"`
	Providers map[string]*OAuthCredential  `json:"providers"`
}

// TokenManager manages OAuth tokens with automatic refresh.
type TokenManager struct {
	mu       sync.RWMutex
	store    *TokenStore
	filePath string
	providers map[string]OAuthProvider
	logger   *slog.Logger

	// refreshCancel is used to stop the background refresh goroutine
	refreshCtx    context.Context
	refreshCancel context.CancelFunc
	refreshWg     sync.WaitGroup
}

// NewTokenManager creates a new token manager.
func NewTokenManager(dataDir string, logger *slog.Logger) (*TokenManager, error) {
	if logger == nil {
		logger = slog.Default()
	}

	filePath := filepath.Join(dataDir, tokensFileName)
	tm := &TokenManager{
		filePath:  filePath,
		providers: make(map[string]OAuthProvider),
		logger:    logger.With("component", "token-manager"),
		store: &TokenStore{
			Version:   1,
			Providers: make(map[string]*OAuthCredential),
		},
	}

	// Load existing tokens
	if err := tm.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load tokens: %w", err)
	}

	// Set up background refresh context
	tm.refreshCtx, tm.refreshCancel = context.WithCancel(context.Background())

	return tm, nil
}

// RegisterProvider registers an OAuth provider for token refresh.
func (tm *TokenManager) RegisterProvider(provider OAuthProvider) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.providers[provider.Name()] = provider
}

// GetCredential returns the credential for a provider.
func (tm *TokenManager) GetCredential(provider string) (*OAuthCredential, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	cred, ok := tm.store.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("no credential found for provider %s", provider)
	}

	return cred, nil
}

// GetValidToken returns a valid access token, refreshing if necessary.
func (tm *TokenManager) GetValidToken(provider string) (*OAuthCredential, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	cred, ok := tm.store.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("no credential found for provider %s", provider)
	}

	// Check if token needs refresh
	if cred.IsExpired(refreshBuffer) {
		// Try to refresh
		newCred, err := tm.refreshCredential(provider, cred)
		if err != nil {
			return nil, fmt.Errorf("token expired and refresh failed: %w", err)
		}
		cred = newCred
	}

	return cred, nil
}

// GetValidTokenInterface returns a valid token as interface{} (for LLMClient compatibility).
func (tm *TokenManager) GetValidTokenInterface(provider string) (interface{}, error) {
	return tm.GetValidToken(provider)
}

// SaveCredential saves a credential for a provider.
func (tm *TokenManager) SaveCredential(cred *OAuthCredential) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if cred.Provider == "" {
		return fmt.Errorf("credential must have a provider")
	}

	tm.store.Providers[cred.Provider] = cred
	return tm.save()
}

// DeleteCredential removes a credential for a provider.
func (tm *TokenManager) DeleteCredential(provider string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	delete(tm.store.Providers, provider)
	return tm.save()
}

// ListProviders returns the list of providers with credentials.
func (tm *TokenManager) ListProviders() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	providers := make([]string, 0, len(tm.store.Providers))
	for p := range tm.store.Providers {
		providers = append(providers, p)
	}
	return providers
}

// GetStatus returns the status of all provider tokens.
func (tm *TokenManager) GetStatus() map[string]TokenStatus {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	status := make(map[string]TokenStatus)
	for provider, cred := range tm.store.Providers {
		s := TokenStatus{
			Provider: provider,
			Email:    cred.Email,
			HasToken: cred.AccessToken != "",
		}

		if cred.ExpiresAt.IsZero() {
			s.Status = "unknown"
		} else if cred.IsExpired(0) {
			s.Status = "expired"
		} else if cred.IsExpired(refreshBuffer) {
			s.Status = "expiring_soon"
		} else {
			s.Status = "valid"
			s.ExpiresIn = time.Until(cred.ExpiresAt).Round(time.Second)
		}

		status[provider] = s
	}
	return status
}

// TokenStatus represents the status of a provider's token.
type TokenStatus struct {
	Provider string        `json:"provider"`
	Status   string        `json:"status"` // valid, expiring_soon, expired, unknown
	Email    string        `json:"email,omitempty"`
	ExpiresIn time.Duration `json:"expires_in,omitempty"`
	HasToken bool          `json:"has_token"`
}

// StartAutoRefresh starts the background token refresh goroutine.
func (tm *TokenManager) StartAutoRefresh() {
	tm.refreshWg.Add(1)
	go tm.autoRefresh()
}

// Stop stops the background refresh goroutine.
func (tm *TokenManager) Stop() {
	tm.refreshCancel()
	tm.refreshWg.Wait()
}

// autoRefresh periodically checks and refreshes tokens.
func (tm *TokenManager) autoRefresh() {
	defer tm.refreshWg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-tm.refreshCtx.Done():
			return
		case <-ticker.C:
			tm.checkAndRefresh()
		}
	}
}

// checkAndRefresh checks all tokens and refreshes those expiring soon.
func (tm *TokenManager) checkAndRefresh() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for provider, cred := range tm.store.Providers {
		if cred.IsExpired(refreshBuffer) && cred.RefreshToken != "" {
			_, err := tm.refreshCredential(provider, cred)
			if err != nil {
				tm.logger.Warn("failed to refresh token",
					"provider", provider,
					"error", err,
				)
			} else {
				tm.logger.Info("token refreshed", "provider", provider)
			}
		}
	}
}

// refreshCredential refreshes a credential using the registered provider.
func (tm *TokenManager) refreshCredential(provider string, cred *OAuthCredential) (*OAuthCredential, error) {
	p, ok := tm.providers[provider]
	if !ok {
		return nil, fmt.Errorf("no provider registered for %s", provider)
	}

	ctx, cancel := context.WithTimeout(tm.refreshCtx, 30*time.Second)
	defer cancel()

	newCred, err := p.RefreshToken(ctx, cred.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Preserve provider name and email
	newCred.Provider = provider
	if newCred.Email == "" {
		newCred.Email = cred.Email
	}

	// Update store
	tm.store.Providers[provider] = newCred
	if err := tm.save(); err != nil {
		tm.logger.Warn("failed to save refreshed token", "error", err)
	}

	return newCred, nil
}

// load loads tokens from disk.
func (tm *TokenManager) load() error {
	data, err := os.ReadFile(tm.filePath)
	if err != nil {
		return err
	}

	var store TokenStore
	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("failed to parse tokens file: %w", err)
	}

	// Ensure providers map is initialized
	if store.Providers == nil {
		store.Providers = make(map[string]*OAuthCredential)
	}

	tm.store = &store
	return nil
}

// save saves tokens to disk.
func (tm *TokenManager) save() error {
	// Ensure directory exists
	dir := filepath.Dir(tm.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create tokens directory: %w", err)
	}

	data, err := json.MarshalIndent(tm.store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	// Write with secure permissions
	if err := os.WriteFile(tm.filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write tokens file: %w", err)
	}

	return nil
}

// Refresh forces a refresh of a provider's token.
func (tm *TokenManager) Refresh(provider string) (*OAuthCredential, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	cred, ok := tm.store.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("no credential found for provider %s", provider)
	}

	if cred.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available for provider %s", provider)
	}

	return tm.refreshCredential(provider, cred)
}

// NeedsLogin returns true if a provider needs to log in (no valid credential).
func (tm *TokenManager) NeedsLogin(provider string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	cred, ok := tm.store.Providers[provider]
	if !ok {
		return true
	}

	return !cred.IsValid()
}
