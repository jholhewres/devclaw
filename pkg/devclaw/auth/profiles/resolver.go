package profiles

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/oauth"
)

// Resolver handles profile resolution with caching and refresh logic.
type Resolver struct {
	mu       sync.RWMutex
	store    ProfileManager
	oauthMgr OAuthManager
	logger   *slog.Logger

	// Cache of resolved credentials
	cache map[string]*cacheEntry

	// Default provider priorities
	defaultPreference ProfilePreference
}

// cacheEntry represents a cached credential.
type cacheEntry struct {
	credential string
	email      string
	expiresAt  time.Time
	profileID  ProfileID
}

// IsExpired returns true if the cache entry is expired.
func (c *cacheEntry) IsExpired() bool {
	return time.Now().After(c.expiresAt)
}

// ResolverConfig contains configuration for the resolver.
type ResolverConfig struct {
	Store            ProfileManager
	OAuthManager     OAuthManager
	Logger           *slog.Logger
	DefaultPreference ProfilePreference
}

// NewResolver creates a new profile resolver.
func NewResolver(cfg ResolverConfig) *Resolver {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.DefaultPreference == "" {
		cfg.DefaultPreference = PreferValid
	}

	return &Resolver{
		store:             cfg.Store,
		oauthMgr:          cfg.OAuthManager,
		logger:            cfg.Logger.With("component", "profile-resolver"),
		cache:             make(map[string]*cacheEntry),
		defaultPreference: cfg.DefaultPreference,
	}
}

// Resolve resolves a credential for a provider, with caching and automatic fallback.
func (r *Resolver) Resolve(ctx context.Context, provider string, preferredProfile string) (*ProfileResolutionResult, error) {
	cacheKey := fmt.Sprintf("%s:%s", provider, preferredProfile)

	// Check cache first
	r.mu.RLock()
	if entry, ok := r.cache[cacheKey]; ok && !entry.IsExpired() {
		r.mu.RUnlock()
		r.logger.Debug("using cached credential", "provider", provider, "profile", preferredProfile)

		if profile, ok := r.store.Get(entry.profileID); ok {
			return &ProfileResolutionResult{
				Profile:    profile,
				Credential: entry.credential,
				Provider:   provider,
				Email:      entry.email,
			}, nil
		}
	}
	r.mu.RUnlock()

	// Resolve from store
	opts := ResolutionOptions{
		Provider:         provider,
		PreferredProfile: preferredProfile,
		Preference:       r.defaultPreference,
		RequireValid:     true,
	}

	result := r.store.Resolve(opts)
	if result.Error != nil {
		return result, result.Error
	}

	// Update cache
	if result.Profile != nil {
		r.cacheCredential(cacheKey, result)
		r.store.MarkUsed(result.Profile.ID)
	}

	return result, nil
}

// ResolveWithMode resolves a credential with a specific auth mode requirement.
func (r *Resolver) ResolveWithMode(ctx context.Context, provider string, mode AuthMode) (*ProfileResolutionResult, error) {
	opts := ResolutionOptions{
		Provider:     provider,
		Preference:   r.defaultPreference,
		RequireValid: true,
		Mode:         mode,
	}

	result := r.store.Resolve(opts)
	if result.Error != nil {
		return result, result.Error
	}

	return result, nil
}

// ResolveAPIKey resolves an API key for a provider.
func (r *Resolver) ResolveAPIKey(ctx context.Context, provider string) (string, *AuthProfile, error) {
	result, err := r.ResolveWithMode(ctx, provider, ModeAPIKey)
	if err != nil {
		return "", nil, err
	}
	return result.Credential, result.Profile, nil
}

// ResolveOAuth resolves an OAuth credential for a provider, with automatic refresh.
func (r *Resolver) ResolveOAuth(ctx context.Context, provider string) (*oauth.OAuthCredential, *AuthProfile, error) {
	result, err := r.ResolveWithMode(ctx, provider, ModeOAuth)
	if err != nil {
		return nil, nil, err
	}

	if result.Profile == nil || result.Profile.OAuth == nil {
		return nil, nil, fmt.Errorf("no OAuth profile found for provider %s", provider)
	}

	return result.Profile.OAuth.ToOAuthCredential(), result.Profile, nil
}

// ResolveAny resolves any valid credential for a provider (tries OAuth first, then API key, then token).
func (r *Resolver) ResolveAny(ctx context.Context, provider string) (*ProfileResolutionResult, error) {
	// Try OAuth first (usually preferred)
	result, err := r.ResolveWithMode(ctx, provider, ModeOAuth)
	if err == nil {
		return result, nil
	}

	// Try API key
	result, err = r.ResolveWithMode(ctx, provider, ModeAPIKey)
	if err == nil {
		return result, nil
	}

	// Try token
	result, err = r.ResolveWithMode(ctx, provider, ModeToken)
	if err == nil {
		return result, nil
	}

	// No valid credential found
	return r.store.Resolve(ResolutionOptions{
		Provider:     provider,
		Preference:   r.defaultPreference,
		RequireValid: false,
	}), fmt.Errorf("no valid credential found for provider %s", provider)
}

// InvalidateCache invalidates the cache for a specific provider or all if empty.
func (r *Resolver) InvalidateCache(provider string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if provider == "" {
		r.cache = make(map[string]*cacheEntry)
		r.logger.Debug("invalidated entire cache")
		return
	}

	for key := range r.cache {
		if len(key) > len(provider) && key[:len(provider)] == provider {
			delete(r.cache, key)
		}
	}
	r.logger.Debug("invalidated cache for provider", "provider", provider)
}

// cacheCredential caches a resolved credential.
func (r *Resolver) cacheCredential(key string, result *ProfileResolutionResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Determine cache expiration based on auth mode
	ttl := 5 * time.Minute // default TTL
	if result.Profile != nil {
		switch result.Profile.Mode {
		case ModeAPIKey:
			ttl = 1 * time.Hour // API keys don't expire
		case ModeToken:
			if result.Profile.Token != nil && result.Profile.Token.ExpiresAt != nil {
				ttl = time.Until(*result.Profile.Token.ExpiresAt) - 1*time.Minute
			}
		case ModeOAuth:
			if result.Profile.OAuth != nil {
				ttl = time.Until(result.Profile.OAuth.ExpiresAt) - 1*time.Minute
			}
		}
	}

	// Minimum TTL
	if ttl < 30*time.Second {
		ttl = 30 * time.Second
	}

	r.cache[key] = &cacheEntry{
		credential: result.Credential,
		email:      result.Email,
		expiresAt:  time.Now().Add(ttl),
		profileID:  result.Profile.ID,
	}
}

// GetCacheStats returns statistics about the resolver cache.
func (r *Resolver) GetCacheStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	valid := 0
	expired := 0

	for _, entry := range r.cache {
		if entry.IsExpired() {
			expired++
		} else {
			valid++
		}
	}

	return map[string]interface{}{
		"total_entries": len(r.cache),
		"valid":         valid,
		"expired":       expired,
	}
}

// MultiResolver resolves credentials for multiple providers simultaneously.
type MultiResolver struct {
	resolver *Resolver
}

// NewMultiResolver creates a new multi-provider resolver.
func NewMultiResolver(resolver *Resolver) *MultiResolver {
	return &MultiResolver{resolver: resolver}
}

// ResolveAll resolves credentials for multiple providers in parallel.
func (m *MultiResolver) ResolveAll(ctx context.Context, providers []string) map[string]*ProfileResolutionResult {
	results := make(map[string]*ProfileResolutionResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, provider := range providers {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			result, err := m.resolver.ResolveAny(ctx, p)
			mu.Lock()
			if err != nil {
				results[p] = &ProfileResolutionResult{
					Provider: p,
					Error:    err,
				}
			} else {
				results[p] = result
			}
			mu.Unlock()
		}(provider)
	}

	wg.Wait()
	return results
}

// FallbackChain represents a chain of fallback providers.
type FallbackChain struct {
	providers []string
	resolver  *Resolver
}

// NewFallbackChain creates a new fallback chain.
func NewFallbackChain(resolver *Resolver, providers ...string) *FallbackChain {
	return &FallbackChain{
		providers: providers,
		resolver:  resolver,
	}
}

// Resolve attempts to resolve each provider in order until one succeeds.
func (f *FallbackChain) Resolve(ctx context.Context) (*ProfileResolutionResult, error) {
	var lastErr error

	for _, provider := range f.providers {
		result, err := f.resolver.ResolveAny(ctx, provider)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
	}

	return nil, fmt.Errorf("no providers configured")
}
