package copilot

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const maxToolCallIDLen = 40

// ToolCallIDMode determines the sanitization rules for tool call IDs.
type ToolCallIDMode int

const (
	// ToolCallIDStrict keeps only [a-zA-Z0-9], max 40 chars.
	// Used for OpenAI, Anthropic, OpenRouter, and most providers.
	ToolCallIDStrict ToolCallIDMode = iota

	// ToolCallIDStrict9 keeps only [a-zA-Z0-9], exactly 9 chars.
	// Used for Mistral which requires short fixed-length IDs.
	ToolCallIDStrict9
)

// SanitizeToolCallID cleans a tool call ID to be safe for the target provider.
func SanitizeToolCallID(id string, mode ToolCallIDMode) string {
	if id == "" {
		return fallbackID(mode)
	}

	cleaned := stripNonAlphanumeric(id)
	if cleaned == "" {
		return fallbackID(mode)
	}

	switch mode {
	case ToolCallIDStrict9:
		if len(cleaned) >= 9 {
			return cleaned[:9]
		}
		return shortHash(id)
	default:
		if len(cleaned) > maxToolCallIDLen {
			return cleaned[:maxToolCallIDLen]
		}
		return cleaned
	}
}

// ProviderToolCallIDMode returns the appropriate sanitization mode for a provider.
func ProviderToolCallIDMode(provider string) ToolCallIDMode {
	p := strings.ToLower(provider)
	if p == "mistral" || strings.HasPrefix(p, "mistral") {
		return ToolCallIDStrict9
	}
	return ToolCallIDStrict
}

func stripNonAlphanumeric(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:9]
}

func fallbackID(mode ToolCallIDMode) string {
	if mode == ToolCallIDStrict9 {
		return "defaultid"
	}
	return "sanitizedtoolid"
}
