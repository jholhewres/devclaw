// text_util.go — shared text manipulation helpers.

package memory

import (
	"strings"
	"unicode/utf8"
)

// truncateAtBoundary truncates s to at most maxBytes, ending at a clean
// word boundary (space or newline) when possible. If the cut point falls
// inside a word, the truncation is moved back to the previous boundary.
// Multi-byte UTF-8 sequences are never split.
func truncateAtBoundary(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}

	// Walk back from the byte limit to find a valid UTF-8 rune boundary.
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}

	// Walk back further to find a word boundary (space or newline).
	boundary := cut
	for boundary > 0 {
		c := s[boundary-1]
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' {
			break
		}
		boundary--
	}

	// If we found a word boundary, prefer it. Otherwise use the rune boundary.
	if boundary > 0 {
		// Include the trailing whitespace character for clean output.
		return strings.TrimRight(s[:boundary], " \t\r\n")
	}
	return s[:cut]
}
