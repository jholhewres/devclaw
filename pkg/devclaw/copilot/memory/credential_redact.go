// Package memory — credential_redact.go provides credential detection and
// redaction used by the legacy import curation pipeline (US-002/US-003).
//
// This logic is intentionally a self-contained copy of the detector in
// pkg/devclaw/copilot/memory_tools.go: that file lives in the `copilot`
// package, which already imports this `memory` package, so reaching back into
// it would create an import cycle. Keeping the patterns identical here means
// the import path redacts exactly what the live save path redacts. If the two
// ever need to diverge, that is a deliberate decision, not an accident.
package memory

import (
	"regexp"
	"strings"
)

// credentialPatternsMem match explicit assignments of credentials. Mirrors
// credentialPatterns in memory_tools.go (copilot package).
var credentialPatternsMem = []string{
	`(?i)senha\s*[:=]\s*\S{4,}`,
	`(?i)password\s*[:=]\s*\S{4,}`,
	`(?i)api[_-]?key\s*[:=]\s*\S{4,}`,
	`(?i)secret[_-]?key\s*[:=]\s*\S{4,}`,
	`(?i)access[_-]?token\s*[:=]\s*\S{4,}`,
	`(?i)bearer\s+[a-zA-Z0-9\-_.]{20,}`,
	`(?i)token\s*[:=]\s*[a-zA-Z0-9\-_.]{20,}`,
	`-----BEGIN\s+(RSA|EC|OPENSSH|PGP|DSA)\s+PRIVATE\s+KEY-----`,
	`(?i)(aws|gcp|azure)[_-]?(secret|key|token)\s*[:=]\s*\S{4,}`,
	`(?i)(senha|password|api[_-]?key|secret[_-]?key|access[_-]?token|token)\s*[:=]\s*"[^"\r\n]{4,}"`,
	`(?i)(senha|password|api[_-]?key|secret[_-]?key|access[_-]?token|token)\s*[:=]\s*'[^'\r\n]{4,}'`,
	`ghp_[a-zA-Z0-9]{36}`,
	`github_pat_[a-zA-Z0-9_]{82}`,
	`gho_[a-zA-Z0-9]{36}`,
	`sk-[a-zA-Z0-9]{32,}`,
	`AIza[a-zA-Z0-9\-_]{35}`,
	`AKIA[0-9A-Z]{16}`,
	`xox[bpasr]-[a-zA-Z0-9\-]{10,}`,
	`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`,
}

// credentialStopwordFollowupsMem are articles/prepositions that indicate a
// "credential-like" label is informational, not an assignment.
var credentialStopwordFollowupsMem = map[string]bool{
	"do": true, "da": true, "de": true, "dos": true, "das": true,
	"para": true, "pelo": true, "pela": true, "no": true, "na": true,
	"for": true, "to": true, "of": true, "the": true, "is": true,
	"at": true, "in": true, "on": true, "a": true, "an": true,
}

var compiledCredentialPatternsMem []*regexp.Regexp

func init() {
	for _, p := range credentialPatternsMem {
		compiledCredentialPatternsMem = append(compiledCredentialPatternsMem, regexp.MustCompile(p))
	}
}

func isCredentialStopwordMatchMem(match string) bool {
	parts := strings.Fields(match)
	if len(parts) < 2 {
		return false
	}
	last := strings.ToLower(strings.TrimRight(parts[len(parts)-1], ".,;:!?"))
	return credentialStopwordFollowupsMem[last]
}

// credentialKeywordReMem flags text that talks about a secret. The
// natural-language redaction pass only fires on lines containing one of these
// keywords, so random alphanumerics elsewhere are never touched.
var credentialKeywordReMem = regexp.MustCompile(`(?i)\b(senha|passwd|password|pwd|secret|token|api[_-]?key|apikey|credential|bearer|access[_-]?key)\b`)

// secretishTokenReMem matches candidate secret tokens on a keyword line.
var secretishTokenReMem = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._/+=@-]{5,}`)

// looksSecretishMem reports whether a token has the shape of a secret value: a
// long opaque string (>=20), or a 6+ char mix of letters and digits (a typical
// password). Pure words (no digit) and pure short numbers are left alone, and
// the redaction marker is never re-redacted.
func looksSecretishMem(tok string) bool {
	if strings.Contains(tok, "REDACTED") {
		return false
	}
	if len(tok) >= 20 {
		return true
	}
	if len(tok) < 6 {
		return false
	}
	var hasDigit, hasAlpha bool
	for _, r := range tok {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasAlpha = true
		}
	}
	return hasDigit && hasAlpha
}

// hasNaturalLanguageSecretMem reports whether any keyword line carries a
// secret-shaped token (e.g. "password for the site is 081082se"), which the
// [:=] assignment patterns miss.
func hasNaturalLanguageSecretMem(content string) bool {
	if !credentialKeywordReMem.MatchString(content) {
		return false
	}
	for _, line := range strings.Split(content, "\n") {
		if !credentialKeywordReMem.MatchString(line) {
			continue
		}
		for _, tok := range secretishTokenReMem.FindAllString(line, -1) {
			if looksSecretishMem(tok) && !credentialKeywordReMem.MatchString(tok) {
				return true
			}
		}
	}
	return false
}

// redactSecretishInKeywordLinesMem redacts secret-shaped tokens on every line
// that mentions a credential keyword. This catches natural-language secrets the
// assignment patterns miss (the stopword guard also lets "password ... is X"
// through). The credential keyword word itself is preserved.
func redactSecretishInKeywordLinesMem(content string) string {
	if !credentialKeywordReMem.MatchString(content) {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !credentialKeywordReMem.MatchString(line) {
			continue
		}
		lines[i] = secretishTokenReMem.ReplaceAllStringFunc(line, func(tok string) string {
			if !looksSecretishMem(tok) || credentialKeywordReMem.MatchString(tok) {
				return tok
			}
			return "[REDACTED — use vault]"
		})
	}
	return strings.Join(lines, "\n")
}

// LooksLikeCredential reports whether content contains a password, API key,
// token, or other secret. Self-contained copy of the copilot detector.
func LooksLikeCredential(content string) bool {
	for _, re := range compiledCredentialPatternsMem {
		for _, match := range re.FindAllString(content, -1) {
			if !isCredentialStopwordMatchMem(match) {
				return true
			}
		}
	}
	return hasNaturalLanguageSecretMem(content)
}

// RedactCredentials replaces detected credential values with a redaction marker,
// preserving any label before the ':' delimiter. Self-contained copy of the
// copilot redactor.
func RedactCredentials(content string) string {
	for _, re := range compiledCredentialPatternsMem {
		content = re.ReplaceAllStringFunc(content, func(match string) string {
			if isCredentialStopwordMatchMem(match) {
				return match
			}
			if idx := strings.IndexByte(match, ':'); idx >= 0 {
				return match[:idx] + ": [REDACTED — use vault]"
			}
			return "[REDACTED — use vault]"
		})
	}
	return redactSecretishInKeywordLinesMem(content)
}
