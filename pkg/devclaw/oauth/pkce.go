package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

const (
	// pkceVerifierLength is the length of the PKCE verifier in bytes (32-96 bytes, we use 32)
	pkceVerifierLength = 32
)

// GeneratePKCE creates a new PKCE verifier and challenge pair.
// The verifier is a cryptographically random string.
// The challenge is the SHA256 hash of the verifier, base64url encoded.
func GeneratePKCE() (*PKCEPair, error) {
	verifier, err := generateVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PKCE verifier: %w", err)
	}

	challenge := computeChallenge(verifier)

	return &PKCEPair{
		Verifier:  verifier,
		Challenge: challenge,
	}, nil
}

// generateVerifier creates a cryptographically random verifier string.
func generateVerifier() (string, error) {
	buf := make([]byte, pkceVerifierLength)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	return base64URLEncode(buf), nil
}

// computeChallenge computes the S256 code challenge from a verifier.
func computeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64URLEncode(h[:])
}

// base64URLEncode encodes bytes using base64url encoding without padding.
// This is the encoding required for PKCE as per RFC 7636.
func base64URLEncode(buf []byte) string {
	return base64.RawURLEncoding.EncodeToString(buf)
}

// VerifyChallenge verifies that a verifier produces the expected challenge.
// This is useful for testing and validation.
func VerifyChallenge(verifier, expectedChallenge string) bool {
	computedChallenge := computeChallenge(verifier)
	// Use constant-time comparison to prevent timing attacks
	return constantTimeEqual(computedChallenge, expectedChallenge)
}

// constantTimeEqual compares two strings in constant time.
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
