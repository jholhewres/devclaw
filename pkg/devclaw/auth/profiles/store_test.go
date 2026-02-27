package profiles

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// MockVault implements a minimal vault interface for testing
type MockVault struct {
	data map[string]string
}

func NewMockVault() *MockVault {
	return &MockVault{data: make(map[string]string)}
}

func (m *MockVault) Set(key string, value string) error {
	m.data[key] = value
	return nil
}

func (m *MockVault) Get(key string) (string, error) {
	if val, ok := m.data[key]; ok {
		return val, nil
	}
	return "", nil // Return empty string instead of error
}

func (m *MockVault) Has(key string) bool {
	_, ok := m.data[key]
	return ok
}

func (m *MockVault) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func (m *MockVault) IsUnlocked() bool {
	return true
}

func TestStoreSaveAndGet(t *testing.T) {
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
		ID:       NewProfileID("openai", "test"),
		Provider: "openai",
		Name:     "test",
		Mode:     ModeAPIKey,
		Enabled:  true,
		Priority: 10,
		APIKey:   &APIKeyCredential{Key: "sk-test123"},
	}

	// Save profile
	if err := store.Save(profile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Get profile
	got, ok := store.Get(profile.ID)
	if !ok {
		t.Fatal("Get returned false, expected true")
	}
	if got.ID != profile.ID {
		t.Errorf("ID mismatch: got %v, want %v", got.ID, profile.ID)
	}
	if got.Provider != profile.Provider {
		t.Errorf("Provider mismatch: got %v, want %v", got.Provider, profile.Provider)
	}
}

func TestStoreDelete(t *testing.T) {
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
		ID:       NewProfileID("google-gmail", "work"),
		Provider: "google-gmail",
		Name:     "work",
		Mode:     ModeOAuth,
		Enabled:  true,
		OAuth: &OAuthCredential{
			AccessToken:  "access123",
			RefreshToken: "refresh456",
			Email:        "user@example.com",
		},
	}

	// Save then delete
	if err := store.Save(profile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Delete(profile.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deletion
	_, ok := store.Get(profile.ID)
	if ok {
		t.Error("Get returned true after deletion, expected false")
	}
}

func TestStoreList(t *testing.T) {
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

	// Create multiple profiles
	profiles := []*AuthProfile{
		{
			ID:       NewProfileID("openai", "default"),
			Provider: "openai",
			Name:     "default",
			Mode:     ModeAPIKey,
			Enabled:  true,
			APIKey:   &APIKeyCredential{Key: "key1"},
		},
		{
			ID:       NewProfileID("anthropic", "default"),
			Provider: "anthropic",
			Name:     "default",
			Mode:     ModeAPIKey,
			Enabled:  false, // disabled
			APIKey:   &APIKeyCredential{Key: "key2"},
		},
		{
			ID:       NewProfileID("google-gmail", "personal"),
			Provider: "google-gmail",
			Name:     "personal",
			Mode:     ModeOAuth,
			Enabled:  true,
			OAuth: &OAuthCredential{
				AccessToken: "token",
				Email:       "user@gmail.com",
			},
		},
	}

	for _, p := range profiles {
		if err := store.Save(p); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	// Test List (all)
	all := store.List()
	if len(all) != 3 {
		t.Errorf("List() returned %d profiles, want 3", len(all))
	}

	// Test GetByProvider
	byProvider := store.GetByProvider("google-gmail", PreferValid)
	if len(byProvider) != 1 {
		t.Errorf("GetByProvider returned %d profiles, want 1", len(byProvider))
	}
}

func TestStoreMarkUsed(t *testing.T) {
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
		ID:       NewProfileID("openai", "test"),
		Provider: "openai",
		Name:     "test",
		Mode:     ModeAPIKey,
		Enabled:  true,
		APIKey:   &APIKeyCredential{Key: "key"},
	}

	if err := store.Save(profile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Mark as used
	store.MarkUsed(profile.ID)

	// Verify LastUsedAt was set
	got, ok := store.Get(profile.ID)
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.LastUsedAt == nil {
		t.Error("LastUsedAt should be set after MarkUsed")
	}
}

func TestStoreMarkError(t *testing.T) {
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
		ID:       NewProfileID("openai", "test"),
		Provider: "openai",
		Name:     "test",
		Mode:     ModeAPIKey,
		Enabled:  true,
		APIKey:   &APIKeyCredential{Key: "key"},
	}

	if err := store.Save(profile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Mark error
	testErr := &testError{msg: "connection failed"}
	store.MarkError(profile.ID, testErr)

	// Verify error was recorded
	got, ok := store.Get(profile.ID)
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.LastError == "" {
		t.Error("LastError should be set after MarkError")
	}
	if got.LastErrorAt == nil {
		t.Error("LastErrorAt should be set after MarkError")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string { return e.msg }

func TestStorePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	mockVault := NewMockVault()

	// Create store and save profiles
	store1, err := NewStore(StoreConfig{
		Vault:     mockVault,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	profile := &AuthProfile{
		ID:        NewProfileID("openai", "persistent"),
		Provider:  "openai",
		Name:      "persistent",
		Mode:      ModeAPIKey,
		Enabled:   true,
		Priority:  42,
		CreatedAt: time.Now(),
		APIKey:    &APIKeyCredential{Key: "persistent-key"},
	}

	if err := store1.Save(profile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Create new store with same vault and cache path
	store2, err := NewStore(StoreConfig{
		Vault:     mockVault,
		CachePath: cachePath,
	})
	if err != nil {
		t.Fatalf("NewStore (second) failed: %v", err)
	}

	// Verify profile exists in new store
	got, ok := store2.Get(profile.ID)
	if !ok {
		t.Fatal("Profile not found after reloading store")
	}
	if got.ID != profile.ID {
		t.Errorf("ID mismatch: got %v, want %v", got.ID, profile.ID)
	}
}

func TestProfileMarshalUnmarshal(t *testing.T) {
	now := time.Now()
	profile := &AuthProfile{
		ID:        NewProfileID("google-gmail", "test"),
		Provider:  "google-gmail",
		Name:      "test",
		Mode:      ModeOAuth,
		Enabled:   true,
		Priority:  5,
		CreatedAt: now,
		UpdatedAt: now,
		OAuth: &OAuthCredential{
			AccessToken:  "access-token-123",
			RefreshToken: "refresh-token-456",
			Email:        "user@example.com",
			Scopes:       []string{"https://www.googleapis.com/auth/gmail.readonly"},
		},
	}

	// Marshal
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	var got AuthProfile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify fields
	if got.ID != profile.ID {
		t.Errorf("ID mismatch: got %v, want %v", got.ID, profile.ID)
	}
	if got.Provider != profile.Provider {
		t.Errorf("Provider mismatch: got %v, want %v", got.Provider, profile.Provider)
	}
	if got.Mode != profile.Mode {
		t.Errorf("Mode mismatch: got %v, want %v", got.Mode, profile.Mode)
	}
	if got.OAuth == nil {
		t.Fatal("OAuth credential is nil after unmarshal")
	}
	if got.OAuth.AccessToken != profile.OAuth.AccessToken {
		t.Errorf("AccessToken mismatch: got %v, want %v", got.OAuth.AccessToken, profile.OAuth.AccessToken)
	}
	if got.OAuth.Email != profile.OAuth.Email {
		t.Errorf("Email mismatch: got %v, want %v", got.OAuth.Email, profile.OAuth.Email)
	}
}
