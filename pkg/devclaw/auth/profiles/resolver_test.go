package profiles

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestResolverResolve(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	mockVault := NewMockVault()
	store, err := NewStore(StoreConfig{
		Vault:     mockVault,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	resolver := NewResolver(ResolverConfig{
		Store: store,
	})

	// Create profiles
	p1 := &AuthProfile{
		ID:       NewProfileID("openai", "primary"),
		Provider: "openai",
		Name:     "primary",
		Mode:     ModeAPIKey,
		Enabled:  true,
		Priority: 10,
		APIKey:   &APIKeyCredential{Key: "key-primary"},
	}
	p2 := &AuthProfile{
		ID:       NewProfileID("openai", "secondary"),
		Provider: "openai",
		Name:     "secondary",
		Mode:     ModeAPIKey,
		Enabled:  true,
		Priority: 5, // Lower priority
		APIKey:   &APIKeyCredential{Key: "key-secondary"},
	}

	for _, p := range []*AuthProfile{p1, p2} {
		if err := store.Save(p); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	// Test resolve for provider
	result, err := resolver.Resolve(context.Background(), "openai", "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if result.Credential == "" {
		t.Fatal("Expected credential, got empty string")
	}
	if result.Profile == nil {
		t.Fatal("Expected profile, got nil")
	}
	if result.Profile.ID != p1.ID {
		t.Errorf("Expected profile %v, got %v", p1.ID, result.Profile.ID)
	}
}

func TestResolverResolveByProfileID(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	mockVault := NewMockVault()
	store, err := NewStore(StoreConfig{
		Vault:     mockVault,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	resolver := NewResolver(ResolverConfig{
		Store: store,
	})

	future := time.Now().Add(time.Hour)

	// Create profiles
	p1 := &AuthProfile{
		ID:       NewProfileID("google-gmail", "work"),
		Provider: "google-gmail",
		Name:     "work",
		Mode:     ModeOAuth,
		Enabled:  true,
		OAuth: &OAuthCredential{
			AccessToken:  "token-work",
			RefreshToken: "refresh-work",
			Email:        "work@example.com",
			ExpiresAt:    future,
		},
	}
	p2 := &AuthProfile{
		ID:       NewProfileID("google-gmail", "personal"),
		Provider: "google-gmail",
		Name:     "personal",
		Mode:     ModeOAuth,
		Enabled:  true,
		OAuth: &OAuthCredential{
			AccessToken:  "token-personal",
			RefreshToken: "refresh-personal",
			Email:        "personal@gmail.com",
			ExpiresAt:    future,
		},
	}

	for _, p := range []*AuthProfile{p1, p2} {
		if err := store.Save(p); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	// Test resolve with specific profile name
	result, err := resolver.Resolve(context.Background(), "google-gmail", "personal")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if result.Profile.ID != p2.ID {
		t.Errorf("Expected profile %v, got %v", p2.ID, result.Profile.ID)
	}
	if result.Credential != "token-personal" {
		t.Errorf("Expected credential 'token-personal', got %v", result.Credential)
	}
}

func TestResolverResolveNoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	mockVault := NewMockVault()
	store, err := NewStore(StoreConfig{
		Vault:     mockVault,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	resolver := NewResolver(ResolverConfig{
		Store: store,
	})

	// Try to resolve non-existent provider
	_, err = resolver.Resolve(context.Background(), "nonexistent", "")
	if err == nil {
		t.Error("Expected error for non-existent provider, got nil")
	}
}

func TestResolverResolveDisabledProfile(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	mockVault := NewMockVault()
	store, err := NewStore(StoreConfig{
		Vault:     mockVault,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	resolver := NewResolver(ResolverConfig{
		Store: store,
	})

	// Create disabled profile
	disabled := &AuthProfile{
		ID:       NewProfileID("openai", "disabled"),
		Provider: "openai",
		Name:     "disabled",
		Mode:     ModeAPIKey,
		Enabled:  false, // Disabled
		APIKey:   &APIKeyCredential{Key: "key-disabled"},
	}
	if err := store.Save(disabled); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Try to resolve disabled profile by ID
	_, err = resolver.Resolve(context.Background(), "openai", string(disabled.ID))
	if err == nil {
		t.Error("Expected error for disabled profile, got nil")
	}
}

func TestResolverCaching(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	mockVault := NewMockVault()
	store, err := NewStore(StoreConfig{
		Vault:     mockVault,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	resolver := NewResolver(ResolverConfig{
		Store: store,
	})

	profile := &AuthProfile{
		ID:       NewProfileID("openai", "cached"),
		Provider: "openai",
		Name:     "cached",
		Mode:     ModeAPIKey,
		Enabled:  true,
		APIKey:   &APIKeyCredential{Key: "cached-key"},
	}
	if err := store.Save(profile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// First resolve - cache miss
	result1, err := resolver.Resolve(context.Background(), "openai", "")
	if err != nil {
		t.Fatalf("First Resolve failed: %v", err)
	}

	// Second resolve - should hit cache
	result2, err := resolver.Resolve(context.Background(), "openai", "")
	if err != nil {
		t.Fatalf("Second Resolve failed: %v", err)
	}

	// Both should return same result
	if result1.Profile.ID != result2.Profile.ID {
		t.Error("Cache not working - results differ")
	}

	// Cache should have entry
	if len(resolver.cache) != 1 {
		t.Errorf("Expected 1 cache entry, got %d", len(resolver.cache))
	}
}

func TestStoreResolve(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	mockVault := NewMockVault()
	store, err := NewStore(StoreConfig{
		Vault:     mockVault,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	profile := &AuthProfile{
		ID:       NewProfileID("openai", "default"),
		Provider: "openai",
		Name:     "default",
		Mode:     ModeAPIKey,
		Enabled:  true,
		Priority: 10,
		APIKey:   &APIKeyCredential{Key: "sk-abc123"},
	}
	if err := store.Save(profile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Test Resolve method
	opts := ResolutionOptions{
		Provider: "openai",
	}
	result := store.Resolve(opts)

	if result == nil {
		t.Fatal("Resolve returned nil")
	}
	if result.Credential != "sk-abc123" {
		t.Errorf("Expected credential 'sk-abc123', got %v", result.Credential)
	}
	if result.Profile == nil || result.Profile.ID != profile.ID {
		t.Error("Profile not returned correctly")
	}
}
