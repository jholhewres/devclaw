package profiles

import (
	"fmt"
	"strings"
)

// ClassifyFailure determines the failure reason from an error.
// Used to select the appropriate cooldown strategy.
func ClassifyFailure(err error) FailureReason {
	if err == nil {
		return FailureUnknown
	}
	return ClassifyFailureMessage(err.Error())
}

// ClassifyFailureMessage determines the failure reason from an error message string.
func ClassifyFailureMessage(msg string) FailureReason {
	lower := strings.ToLower(msg)

	switch {
	// Rate limiting (includes HTTP 429 and Anthropic 529 "overloaded")
	case containsAny(lower, "rate limit", "rate_limit", "too many requests"):
		return FailureRateLimit
	case containsHTTPStatus(msg, 429) || containsHTTPStatus(msg, 529):
		return FailureRateLimit

	// Billing / quota exhaustion
	case containsAny(lower, "billing", "quota exceeded", "quota_exceeded",
		"insufficient_quota", "payment required", "spending limit"):
		return FailureBilling
	case containsHTTPStatus(msg, 402):
		return FailureBilling

	// Permanent auth failures (won't self-heal)
	case containsAny(lower, "permission_error", "permission denied",
		"access denied", "forbidden", "account deactivated"):
		return FailureAuthPermanent
	case containsHTTPStatus(msg, 403):
		return FailureAuthPermanent

	// Session expired (token/session invalidated, needs refresh)
	case containsAny(lower, "session expired", "session_expired",
		"token expired", "session invalid", "refresh token",
		"token has been revoked", "token revoked", "invalid token"):
		return FailureSessionExpired

	// Transient auth failures (may be fixable by refreshing keys)
	case containsAny(lower, "invalid api key", "invalid_api_key",
		"unauthorized", "authentication failed"):
		return FailureAuth
	case containsHTTPStatus(msg, 401):
		return FailureAuth

	// Timeout
	case containsAny(lower, "timeout", "deadline exceeded",
		"context deadline", "context canceled"):
		return FailureTimeout

	// Model not found
	case containsAny(lower, "model not found", "model_not_found",
		"does not exist", "no such model", "unknown model"):
		return FailureModelNotFound

	// Format errors (bad request, schema issues)
	case containsAny(lower, "invalid request", "bad request", "malformed"):
		return FailureFormat
	case containsHTTPStatus(msg, 400):
		return FailureFormat

	default:
		return FailureUnknown
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func containsHTTPStatus(s string, code int) bool {
	codeStr := fmt.Sprintf("%d", code)
	return strings.Contains(s, "status "+codeStr) ||
		strings.Contains(s, "status: "+codeStr) ||
		strings.Contains(s, "HTTP "+codeStr) ||
		strings.Contains(s, "http "+codeStr)
}
