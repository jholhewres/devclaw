package profiles

import (
	"testing"
	"time"
)

func TestNewProfileID(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		profile  string
		want     ProfileID
	}{
		{
			name:     "simple provider and profile",
			provider: "google-gmail",
			profile:  "work",
			want:     ProfileID("google-gmail:work"),
		},
		{
			name:     "profile with special chars",
			provider: "openai",
			profile:  "my-profile_123",
			want:     ProfileID("openai:my-profile_123"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewProfileID(tt.provider, tt.profile)
			if got != tt.want {
				t.Errorf("NewProfileID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProfileIDProvider(t *testing.T) {
	tests := []struct {
		name string
		id   ProfileID
		want string
	}{
		{
			name: "valid ID",
			id:   ProfileID("google-gmail:work"),
			want: "google-gmail",
		},
		{
			name: "ID without colon",
			id:   ProfileID("invalidid"),
			want: "invalidid",
		},
		{
			name: "empty ID",
			id:   ProfileID(""),
			want: "",
		},
		{
			name: "multiple colons",
			id:   ProfileID("provider:namespace:name"),
			want: "provider:namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.id.Provider()
			if got != tt.want {
				t.Errorf("ProfileID.Provider() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProfileIDName(t *testing.T) {
	tests := []struct {
		name string
		id   ProfileID
		want string
	}{
		{
			name: "valid ID",
			id:   ProfileID("google-gmail:work"),
			want: "work",
		},
		{
			name: "ID without colon - returns default",
			id:   ProfileID("invalidid"),
			want: "default",
		},
		{
			name: "empty ID - returns default",
			id:   ProfileID(""),
			want: "default",
		},
		{
			name: "multiple colons",
			id:   ProfileID("provider:namespace:name"),
			want: "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.id.Name()
			if got != tt.want {
				t.Errorf("ProfileID.Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthProfileIsValid(t *testing.T) {
	tests := []struct {
		name    string
		profile *AuthProfile
		want    bool
	}{
		{
			name: "valid API key profile",
			profile: &AuthProfile{
				Mode:   ModeAPIKey,
				Enabled: true,
				APIKey: &APIKeyCredential{Key: "test-key-123"},
			},
			want: true,
		},
		{
			name: "valid token profile",
			profile: &AuthProfile{
				Mode:    ModeToken,
				Enabled: true,
				Token:   &TokenCredential{Token: "test-token-456"},
			},
			want: true,
		},
		{
			name: "valid OAuth profile",
			profile: &AuthProfile{
				Mode:    ModeOAuth,
				Enabled: true,
				OAuth: &OAuthCredential{
					AccessToken:  "access-token",
					RefreshToken: "refresh-token",
					ExpiresAt:    time.Now().Add(time.Hour),
				},
			},
			want: true,
		},
		{
			name: "invalid - disabled",
			profile: &AuthProfile{
				Mode:    ModeAPIKey,
				Enabled: false,
				APIKey:  &APIKeyCredential{Key: "key"},
			},
			want: false,
		},
		{
			name: "invalid - no credentials",
			profile: &AuthProfile{
				Mode:    ModeAPIKey,
				Enabled: true,
			},
			want: false,
		},
		{
			name: "invalid - empty API key",
			profile: &AuthProfile{
				Mode:    ModeAPIKey,
				Enabled: true,
				APIKey:  &APIKeyCredential{Key: ""},
			},
			want: false,
		},
		{
			name: "invalid - empty token",
			profile: &AuthProfile{
				Mode:    ModeToken,
				Enabled: true,
				Token:   &TokenCredential{Token: ""},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.IsValid(); got != tt.want {
				t.Errorf("AuthProfile.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthProfileIsExpired(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	tests := []struct {
		name    string
		profile *AuthProfile
		want    bool
	}{
		{
			name: "not expired - future expiry",
			profile: &AuthProfile{
				Mode: ModeOAuth,
				OAuth: &OAuthCredential{
					AccessToken:  "token",
					RefreshToken: "refresh",
					ExpiresAt:    future,
				},
			},
			want: false,
		},
		{
			name: "expired - past expiry",
			profile: &AuthProfile{
				Mode: ModeOAuth,
				OAuth: &OAuthCredential{
					AccessToken:  "token",
					RefreshToken: "refresh",
					ExpiresAt:    past,
				},
			},
			want: true,
		},
		{
			name: "not expired - no expiry (API key)",
			profile: &AuthProfile{
				Mode:   ModeAPIKey,
				APIKey: &APIKeyCredential{Key: "test-key"},
			},
			want: false,
		},
		{
			name: "not expired - no expiry (token)",
			profile: &AuthProfile{
				Mode:  ModeToken,
				Token: &TokenCredential{Token: "test-token"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.IsExpired(); got != tt.want {
				t.Errorf("AuthProfile.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthProfileGetAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		profile *AuthProfile
		want    string
		wantErr bool
	}{
		{
			name: "direct key",
			profile: &AuthProfile{
				Mode:   ModeAPIKey,
				APIKey: &APIKeyCredential{Key: "sk-test123"},
			},
			want:    "sk-test123",
			wantErr: false,
		},
		{
			name: "key from vault ref",
			profile: &AuthProfile{
				Mode:   ModeAPIKey,
				APIKey: &APIKeyCredential{KeyRef: "OPENAI_API_KEY"},
			},
			want:    "vault-value",
			wantErr: false,
		},
		{
			name: "wrong mode - OAuth",
			profile: &AuthProfile{
				Mode: ModeOAuth,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "no API key",
			profile: &AuthProfile{
				Mode:   ModeAPIKey,
				APIKey: &APIKeyCredential{},
			},
			want:    "",
			wantErr: true,
		},
	}

	vaultGetter := func(ref string) (string, error) {
		if ref == "OPENAI_API_KEY" {
			return "vault-value", nil
		}
		return "", nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.profile.GetAPIKey(vaultGetter)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAPIKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetAPIKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthProfileGetToken(t *testing.T) {
	tests := []struct {
		name    string
		profile *AuthProfile
		want    string
		wantErr bool
	}{
		{
			name: "direct token",
			profile: &AuthProfile{
				Mode:  ModeToken,
				Token: &TokenCredential{Token: "token123"},
			},
			want:    "token123",
			wantErr: false,
		},
		{
			name: "token from vault ref",
			profile: &AuthProfile{
				Mode:  ModeToken,
				Token: &TokenCredential{TokenRef: "BEARER_TOKEN"},
			},
			want:    "vault-token",
			wantErr: false,
		},
		{
			name: "wrong mode - API key",
			profile: &AuthProfile{
				Mode:   ModeAPIKey,
				APIKey: &APIKeyCredential{Key: "key"},
			},
			want:    "",
			wantErr: true,
		},
	}

	vaultGetter := func(ref string) (string, error) {
		if ref == "BEARER_TOKEN" {
			return "vault-token", nil
		}
		return "", nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.profile.GetToken(vaultGetter)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthProfileMarkUsed(t *testing.T) {
	profile := &AuthProfile{
		ID:       NewProfileID("openai", "test"),
		Provider: "openai",
		Name:     "test",
	}

	if profile.LastUsedAt != nil {
		t.Error("LastUsedAt should be nil initially")
	}

	profile.MarkUsed()

	if profile.LastUsedAt == nil {
		t.Error("LastUsedAt should be set after MarkUsed")
	}
}

func TestAuthProfileMarkError(t *testing.T) {
	profile := &AuthProfile{
		ID:       NewProfileID("openai", "test"),
		Provider: "openai",
		Name:     "test",
	}

	if profile.LastError != "" || profile.LastErrorAt != nil {
		t.Error("LastError and LastErrorAt should be empty initially")
	}

	testErr := &testError{msg: "connection failed"}
	profile.MarkError(testErr)

	if profile.LastError != "connection failed" {
		t.Errorf("LastError = %v, want 'connection failed'", profile.LastError)
	}
	if profile.LastErrorAt == nil {
		t.Error("LastErrorAt should be set after MarkError")
	}
}

func TestAuthProfileClearError(t *testing.T) {
	profile := &AuthProfile{
		ID:         NewProfileID("openai", "test"),
		Provider:   "openai",
		Name:       "test",
		LastError:  "some error",
		LastErrorAt: &time.Time{},
	}

	profile.ClearError()

	if profile.LastError != "" {
		t.Errorf("LastError should be empty after ClearError, got %v", profile.LastError)
	}
	if profile.LastErrorAt != nil {
		t.Error("LastErrorAt should be nil after ClearError")
	}
}

func TestOAuthCredentialIsExpired(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	tests := []struct {
		name      string
		cred      *OAuthCredential
		buffer    time.Duration
		isExpired bool
	}{
		{
			name: "not expired - future expiry",
			cred: &OAuthCredential{
				AccessToken: "token",
				ExpiresAt:   future,
			},
			buffer:    5 * time.Minute,
			isExpired: false,
		},
		{
			name: "expired - past expiry",
			cred: &OAuthCredential{
				AccessToken: "token",
				ExpiresAt:   past,
			},
			buffer:    0,
			isExpired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cred.IsExpired(tt.buffer)
			if got != tt.isExpired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.isExpired)
			}
		})
	}
}

func TestOAuthCredentialIsValid(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	tests := []struct {
		name  string
		cred  *OAuthCredential
		valid bool
	}{
		{
			name: "valid - has token and future expiry",
			cred: &OAuthCredential{
				AccessToken: "token",
				ExpiresAt:   future,
			},
			valid: true,
		},
		{
			name: "invalid - expired",
			cred: &OAuthCredential{
				AccessToken: "token",
				ExpiresAt:   past,
			},
			valid: false,
		},
		{
			name: "invalid - no token",
			cred: &OAuthCredential{
				AccessToken: "",
				ExpiresAt:   future,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cred.IsValid()
			if got != tt.valid {
				t.Errorf("IsValid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestProfileStoreGetAndSet(t *testing.T) {
	store := NewProfileStore()

	profile := &AuthProfile{
		ID:       NewProfileID("openai", "test"),
		Provider: "openai",
		Name:     "test",
		Mode:     ModeAPIKey,
		Enabled:  true,
		APIKey:   &APIKeyCredential{Key: "key123"},
	}

	// Set profile
	store.Set(profile)

	// Get profile
	got, ok := store.Get(profile.ID)
	if !ok {
		t.Fatal("Get returned false, expected true")
	}
	if got.ID != profile.ID {
		t.Errorf("ID mismatch: got %v, want %v", got.ID, profile.ID)
	}

	// Get non-existent
	_, ok = store.Get(NewProfileID("nonexistent", "default"))
	if ok {
		t.Error("Get returned true for non-existent profile")
	}
}

func TestProfileStoreDelete(t *testing.T) {
	store := NewProfileStore()

	profile := &AuthProfile{
		ID:       NewProfileID("openai", "test"),
		Provider: "openai",
		Name:     "test",
	}

	store.Set(profile)
	store.Delete(profile.ID)

	_, ok := store.Get(profile.ID)
	if ok {
		t.Error("Profile still exists after Delete")
	}
}

func TestProfileStoreList(t *testing.T) {
	store := NewProfileStore()

	profiles := []*AuthProfile{
		{ID: NewProfileID("openai", "default"), Provider: "openai"},
		{ID: NewProfileID("anthropic", "default"), Provider: "anthropic"},
		{ID: NewProfileID("google-gmail", "work"), Provider: "google-gmail"},
	}

	for _, p := range profiles {
		store.Set(p)
	}

	list := store.List()
	if len(list) != 3 {
		t.Errorf("List() returned %d profiles, want 3", len(list))
	}
}
