package copilot

import (
	"strings"
	"sync"
)

// silentTokens are tokens that must be completely suppressed from streaming output.
// Partial prefixes of these tokens must be buffered until we can confirm or discard.
var silentTokens = []string{
	TokenNoReply,     // "NO_REPLY"
	TokenHeartbeatOK, // "HEARTBEAT_OK"
}

const streamBufferMax = 20

// StreamSanitizer wraps a StreamCallback to prevent partial silent tokens
// from leaking into user-visible output. It buffers short prefixes that
// could be the beginning of a silent token and flushes them when safe.
type StreamSanitizer struct {
	mu       sync.Mutex
	inner    StreamCallback
	buf      strings.Builder
	flushed  bool
}

// NewStreamSanitizer wraps a StreamCallback with token buffering.
func NewStreamSanitizer(cb StreamCallback) *StreamSanitizer {
	return &StreamSanitizer{inner: cb}
}

// Write is the StreamCallback-compatible function that buffers and filters.
func (s *StreamSanitizer) Write(chunk string) {
	if s.inner == nil || chunk == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf.WriteString(chunk)
	buffered := s.buf.String()

	// If buffer exceeds max and still matches a prefix, it's not a silent token.
	if len(buffered) > streamBufferMax {
		s.flushLocked(buffered)
		return
	}

	// Check if the buffer IS a complete silent token.
	for _, token := range silentTokens {
		if strings.TrimSpace(buffered) == token {
			s.buf.Reset()
			return
		}
	}

	// Check if the buffer could be the START of a silent token.
	if isPrefixOfAnySilentToken(buffered) {
		return
	}

	// Buffer does not match any prefix — flush everything.
	s.flushLocked(buffered)
}

// Flush sends any remaining buffered content to the inner callback.
// Must be called when the stream ends.
func (s *StreamSanitizer) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buf.Len() == 0 {
		return
	}

	buffered := s.buf.String()

	// If the full buffer is a silent token, suppress it entirely.
	trimmed := strings.TrimSpace(buffered)
	for _, token := range silentTokens {
		if trimmed == token {
			s.buf.Reset()
			return
		}
	}

	s.flushLocked(buffered)
}

func (s *StreamSanitizer) flushLocked(text string) {
	s.buf.Reset()
	if text != "" {
		s.inner(text)
		s.flushed = true
	}
}

// Callback returns the StreamCallback function to use in place of the inner callback.
func (s *StreamSanitizer) Callback() StreamCallback {
	return s.Write
}

// isPrefixOfAnySilentToken returns true if `s` could be the beginning of a silent token.
func isPrefixOfAnySilentToken(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	for _, token := range silentTokens {
		if len(trimmed) <= len(token) && strings.HasPrefix(token, trimmed) {
			return true
		}
	}
	return false
}
