package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	pair, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error = %v", err)
	}

	if pair.Verifier == "" {
		t.Error("Verifier is empty")
	}

	if pair.Challenge == "" {
		t.Error("Challenge is empty")
	}

	// Verifier should be 64 characters (32 bytes hex-encoded with base64url)
	if len(pair.Verifier) < 43 { // Minimum length per RFC 7636
		t.Errorf("Verifier too short: got %d characters", len(pair.Verifier))
	}
}

func TestPKCEChallengeVerification(t *testing.T) {
	pair, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error = %v", err)
	}

	// Compute expected challenge
	h := sha256.Sum256([]byte(pair.Verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])

	if pair.Challenge != expected {
		t.Errorf("Challenge mismatch: got %s, want %s", pair.Challenge, expected)
	}
}

func TestPKCEUniqueness(t *testing.T) {
	pair1, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error = %v", err)
	}

	pair2, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error = %v", err)
	}

	if pair1.Verifier == pair2.Verifier {
		t.Error("Verifiers should be unique")
	}

	if pair1.Challenge == pair2.Challenge {
		t.Error("Challenges should be unique")
	}
}

func TestVerifyChallenge(t *testing.T) {
	pair, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error = %v", err)
	}

	if !VerifyChallenge(pair.Verifier, pair.Challenge) {
		t.Error("VerifyChallenge() returned false for valid pair")
	}

	// Wrong verifier should fail
	if VerifyChallenge("wrong-verifier", pair.Challenge) {
		t.Error("VerifyChallenge() returned true for wrong verifier")
	}

	// Wrong challenge should fail
	if VerifyChallenge(pair.Verifier, "wrong-challenge") {
		t.Error("VerifyChallenge() returned true for wrong challenge")
	}
}

func TestBase64URLEncode(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte{0x00}, "AA"},
		{[]byte{0xFF}, "_w"},
		{[]byte{0x00, 0x01, 0x02}, "AAEC"},
	}

	for _, tt := range tests {
		got := base64URLEncode(tt.input)
		if got != tt.expected {
			t.Errorf("base64URLEncode(%v) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}
