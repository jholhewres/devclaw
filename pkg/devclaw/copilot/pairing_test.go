package copilot

import (
	"testing"
	"time"
)

func TestPairingToken_IsExpired(t *testing.T) {
	// Not expired - no expiration set.
	token := &PairingToken{}
	if token.IsExpired() {
		t.Error("token with no expiration should not be expired")
	}

	// Not expired - future expiration.
	future := time.Now().Add(24 * time.Hour)
	token = &PairingToken{ExpiresAt: &future}
	if token.IsExpired() {
		t.Error("token with future expiration should not be expired")
	}

	// Expired - past expiration.
	past := time.Now().Add(-1 * time.Hour)
	token = &PairingToken{ExpiresAt: &past}
	if !token.IsExpired() {
		t.Error("token with past expiration should be expired")
	}
}

func TestPairingToken_CanUse(t *testing.T) {
	tests := []struct {
		name      string
		token     *PairingToken
		expectUse bool
	}{
		{
			name:      "valid token",
			token:     &PairingToken{},
			expectUse: true,
		},
		{
			name:      "revoked token",
			token:     &PairingToken{Revoked: true},
			expectUse: false,
		},
		{
			name:      "expired token",
			token:     &PairingToken{ExpiresAt: ptrTime(time.Now().Add(-1 * time.Hour))},
			expectUse: false,
		},
		{
			name:      "exhausted uses",
			token:     &PairingToken{MaxUses: 5, UseCount: 5},
			expectUse: false,
		},
		{
			name:      "still has uses left",
			token:     &PairingToken{MaxUses: 5, UseCount: 4},
			expectUse: true,
		},
		{
			name:      "unlimited uses",
			token:     &PairingToken{MaxUses: 0, UseCount: 100},
			expectUse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.token.CanUse(); got != tt.expectUse {
				t.Errorf("CanUse() = %v, want %v", got, tt.expectUse)
			}
		})
	}
}

func TestGenerateSecureToken(t *testing.T) {
	// Generate multiple tokens and verify they're unique.
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := generateSecureToken(24)
		if err != nil {
			t.Fatalf("generateSecureToken failed: %v", err)
		}

		// Check length (24 bytes = 48 hex chars).
		if len(token) != 48 {
			t.Errorf("token length = %d, want 48", len(token))
		}

		// Check uniqueness.
		if tokens[token] {
			t.Errorf("duplicate token generated: %s", token)
		}
		tokens[token] = true
	}
}

func TestExtractTokenFromMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid 48-char token",
			input:    "a1b2c3d4e5f6789012345678901234567890abcdef123456",
			expected: "a1b2c3d4e5f6789012345678901234567890abcdef123456",
		},
		{
			name:     "valid token with uppercase (converted to lowercase)",
			input:    "A1B2C3D4E5F6789012345678901234567890ABCDEF123456",
			expected: "a1b2c3d4e5f6789012345678901234567890abcdef123456",
		},
		{
			name:     "token: prefix format",
			input:    "token: a1b2c3d4e5f6789012345678901234567890abcdef123456",
			expected: "a1b2c3d4e5f6789012345678901234567890abcdef123456",
		},
		{
			name:     "token: prefix with spaces",
			input:    "  token:   a1b2c3d4e5f6789012345678901234567890abcdef123456  ",
			expected: "a1b2c3d4e5f6789012345678901234567890abcdef123456",
		},
		{
			name:     "too short",
			input:    "a1b2c3d4e5f6",
			expected: "",
		},
		{
			name:     "invalid characters",
			input:    "ghijklmnopqrstuvwxyz1234567890abcdefghijklmnopqr",
			expected: "",
		},
		{
			name:     "regular message",
			input:    "Hello, how are you?",
			expected: "",
		},
		{
			name:     "command",
			input:    "/help",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTokenFromMessage(tt.input)
			if got != tt.expected {
				t.Errorf("ExtractTokenFromMessage(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsHexString(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"0123456789abcdef", true},
		{"ABCDEF", false}, // uppercase not allowed
		{"0123456789", true},
		{"ghijkl", false},
		{"abc123def456", true},
		{"", true}, // empty is valid hex
		{"abc xyz", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isHexString(tt.input); got != tt.expected {
				t.Errorf("isHexString(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTokenOptions_Defaults(t *testing.T) {
	opts := TokenOptions{}

	if opts.Role != "" {
		t.Errorf("default Role should be empty, got %s", opts.Role)
	}
	if opts.MaxUses != 0 {
		t.Errorf("default MaxUses should be 0, got %d", opts.MaxUses)
	}
	if opts.ExpiresIn != 0 {
		t.Errorf("default ExpiresIn should be 0, got %v", opts.ExpiresIn)
	}
	if opts.AutoApprove {
		t.Error("default AutoApprove should be false")
	}
}

func TestPairingRequest_Status(t *testing.T) {
	request := &PairingRequest{
		ID:     "test-id",
		Status: "pending",
	}

	if request.Status != "pending" {
		t.Errorf("expected status 'pending', got %s", request.Status)
	}

	request.Status = "approved"
	if request.Status != "approved" {
		t.Errorf("expected status 'approved', got %s", request.Status)
	}
}

func TestTokenRole_Constants(t *testing.T) {
	if TokenRoleUser != "user" {
		t.Errorf("TokenRoleUser = %s, want 'user'", TokenRoleUser)
	}
	if TokenRoleAdmin != "admin" {
		t.Errorf("TokenRoleAdmin = %s, want 'admin'", TokenRoleAdmin)
	}
}

// Helper function to create a time pointer.
func ptrTime(t time.Time) *time.Time {
	return &t
}
