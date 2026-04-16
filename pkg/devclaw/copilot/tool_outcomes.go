// Package copilot – tool_outcomes.go implements a per-turn, thread-safe
// log of tool execution outcomes used for structural provenance checks.
//
// The memory layer uses this log to reject fact saves whose content echoes
// a tool call that failed earlier in the same turn AND for which no later
// successful call on the same subject has been observed. This replaces
// the keyword-based access-failure filter with a locale-agnostic,
// content-signal-free mechanism: all decisions derive from the tool
// error flag, ordering, and generic token overlap.
package copilot

import (
	"context"
	"strings"
	"sync"
	"time"
)

// ToolOutcome captures the structured result of a single tool execution.
type ToolOutcome struct {
	Name      string    // tool name
	Args      string    // raw tool args (trimmed)
	Error     bool      // true if the tool returned an error
	Content   string    // tool output (truncated)
	Timestamp time.Time // when the outcome was recorded
}

// ToolOutcomeLog is a bounded, thread-safe chronological log of tool
// outcomes for a single agent turn. Entries are appended in execution
// order; older entries drop off once the capacity is exceeded.
type ToolOutcomeLog struct {
	mu      sync.Mutex
	entries []ToolOutcome
	max     int
}

// NewToolOutcomeLog constructs an outcome log with the given capacity.
// A non-positive capacity defaults to 32 entries, which comfortably
// covers the 25-turn tool loop with multiple calls per turn.
func NewToolOutcomeLog(capacity int) *ToolOutcomeLog {
	if capacity <= 0 {
		capacity = 32
	}
	return &ToolOutcomeLog{max: capacity}
}

// Record appends an outcome to the log, trimming oldest entries beyond max.
func (l *ToolOutcomeLog) Record(o ToolOutcome) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, o)
	if len(l.entries) > l.max {
		// Drop oldest to keep the log bounded.
		l.entries = l.entries[len(l.entries)-l.max:]
	}
}

// Snapshot returns a copy of the current outcomes in chronological order.
func (l *ToolOutcomeLog) Snapshot() []ToolOutcome {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]ToolOutcome, len(l.entries))
	copy(out, l.entries)
	return out
}

// ctxKeyToolOutcomeLog is the context key for per-turn outcome logs.
type ctxKeyToolOutcomeLog struct{}

// ContextWithToolOutcomeLog attaches an outcome log to the context.
func ContextWithToolOutcomeLog(ctx context.Context, log *ToolOutcomeLog) context.Context {
	return context.WithValue(ctx, ctxKeyToolOutcomeLog{}, log)
}

// ToolOutcomeLogFromContext extracts the outcome log, or nil if not set.
func ToolOutcomeLogFromContext(ctx context.Context) *ToolOutcomeLog {
	if v, ok := ctx.Value(ctxKeyToolOutcomeLog{}).(*ToolOutcomeLog); ok {
		return v
	}
	return nil
}

// ── Provenance check helpers ──────────────────────────────────────────────

// ProvenanceReason returns a non-empty reason string if `content` should be
// rejected as a fact save because it echoes the subject of a tool call that
// failed earlier in this turn AND no later successful call on the same
// subject has been observed.
//
// The comparison uses generic token overlap (identifier-like tokens of
// length ≥ 3, with a small locale-agnostic stopword set). No tool-specific
// names, no error-phrase keywords, no PT/EN heuristics.
func ProvenanceReason(content string, outcomes []ToolOutcome) string {
	if content == "" || len(outcomes) == 0 {
		return ""
	}
	memTokens := identifierTokens(content)
	if len(memTokens) == 0 {
		return ""
	}

	for i, o := range outcomes {
		if !o.Error {
			continue
		}
		failedTokens := mergeSets(identifierTokens(o.Args), identifierTokens(o.Content))
		overlap := intersectSets(memTokens, failedTokens)
		if len(overlap) == 0 {
			continue
		}

		if hasLaterSuccessOverlapping(overlap, outcomes[i+1:]) {
			// The tool that failed was followed by a success mentioning the
			// same subject tokens — treat the situation as resolved.
			continue
		}
		return "content overlaps a failed " + o.Name + " call from this turn; verify with a successful call before saving a fact"
	}
	return ""
}

// hasLaterSuccessOverlapping reports whether any later successful outcome
// in the slice has identifier-token overlap with the given subject tokens.
func hasLaterSuccessOverlapping(subject map[string]struct{}, later []ToolOutcome) bool {
	for _, o := range later {
		if o.Error {
			continue
		}
		laterTokens := mergeSets(identifierTokens(o.Args), identifierTokens(o.Content))
		if len(intersectSets(subject, laterTokens)) > 0 {
			return true
		}
	}
	return false
}

// genericStopwords are locale-spanning natural-language tokens that carry
// no subject identity. Kept deliberately small so that domain-specific
// vocabulary (hostnames, paths, identifiers) is never treated as noise.
var genericStopwords = map[string]struct{}{
	// English
	"the": {}, "and": {}, "for": {}, "not": {}, "with": {}, "that": {},
	"this": {}, "from": {}, "into": {}, "over": {}, "are": {}, "was": {},
	"were": {}, "has": {}, "have": {}, "had": {}, "its": {}, "you": {},
	"your": {}, "but": {}, "any": {}, "all": {},
	// Portuguese
	"para": {}, "com": {}, "não": {}, "nao": {}, "sem": {}, "por": {},
	"que": {}, "uma": {}, "uns": {}, "seu": {}, "sua": {}, "dos": {},
	"das": {}, "ele": {}, "ela": {}, "isso": {}, "este": {}, "esse": {},
	// Structural
	"true": {}, "false": {}, "null": {}, "none": {},
}

// identifierTokens extracts lowercased identifier-like tokens from a string.
// A token is a contiguous run of letters, digits, dots, hyphens, underscores,
// or colons (so hostnames, paths, and CLI flags survive intact). Tokens
// shorter than 3 characters or present in genericStopwords are dropped.
func identifierTokens(s string) map[string]struct{} {
	tokens := make(map[string]struct{})
	if s == "" {
		return tokens
	}
	lower := strings.ToLower(s)
	start := -1
	emit := func(end int) {
		if start < 0 {
			return
		}
		raw := strings.Trim(lower[start:end], ".-_:")
		if len(raw) < 3 {
			start = -1
			return
		}
		if _, skip := genericStopwords[raw]; !skip {
			tokens[raw] = struct{}{}
		}
		start = -1
	}
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if isIdentChar(c) {
			if start < 0 {
				start = i
			}
			continue
		}
		emit(i)
	}
	emit(len(lower))
	return tokens
}

// isIdentChar defines which byte values form an identifier token. ASCII-only
// on purpose — this runs on byte indices and does not care about UTF-8
// multi-byte characters (they simply break tokens, which is acceptable).
func isIdentChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '.' || c == '-' || c == '_' || c == ':' || c == '/':
		return true
	}
	return false
}

func mergeSets(a, b map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		out[k] = struct{}{}
	}
	for k := range b {
		out[k] = struct{}{}
	}
	return out
}

func intersectSets(a, b map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{})
	for k := range a {
		if _, ok := b[k]; ok {
			out[k] = struct{}{}
		}
	}
	return out
}
