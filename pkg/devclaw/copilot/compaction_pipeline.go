// Package copilot – compaction_pipeline.go implements a multi-strategy context
// compaction pipeline that proactively manages context window usage. Instead of
// waiting for overflow errors, the pipeline monitors context pressure and applies
// increasingly aggressive compaction strategies as usage grows.
//
// Levels (cheapest → most expensive):
//
//	Collapse (70%)     → Truncate oversized tool results in-place
//	MicroCompact (80%) → Clear old tool result contents with placeholder
//	AutoCompact (93%)  → LLM-based summarization of conversation history
//	MemoryCompact (97%)→ Extract memories + summarize (preserves learnings)
package copilot

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// CompactLevel represents the severity of compaction to apply.
type CompactLevel int

const (
	// CompactNone means no compaction needed.
	CompactNone CompactLevel = -1

	// CompactCollapse truncates oversized tool results in-place.
	// Cheapest operation — no LLM calls. Triggered at ~70% context usage.
	CompactCollapse CompactLevel = iota

	// CompactMicro clears old tool results with a placeholder.
	// Still cheap — no LLM calls. Triggered at ~80% context usage.
	CompactMicro

	// CompactAuto triggers full LLM-based summarization.
	// Expensive — requires LLM call. Triggered at ~93% context usage.
	CompactAuto

	// CompactMemory extracts memories before summarizing.
	// Most expensive — multiple LLM calls. Triggered at ~97% context usage.
	CompactMemory
)

// String returns a human-readable name for the compaction level.
func (l CompactLevel) String() string {
	switch l {
	case CompactNone:
		return "none"
	case CompactCollapse:
		return "collapse"
	case CompactMicro:
		return "micro-compact"
	case CompactAuto:
		return "auto-compact"
	case CompactMemory:
		return "memory-compact"
	default:
		return fmt.Sprintf("unknown(%d)", l)
	}
}

// CompactThresholds defines the context usage ratios that trigger each level.
type CompactThresholds struct {
	// CollapseAt triggers tool result truncation. Default: 0.70.
	CollapseAt float64 `yaml:"collapse_at"`

	// MicroCompactAt triggers old tool result clearing. Default: 0.80.
	MicroCompactAt float64 `yaml:"micro_compact_at"`

	// AutoCompactAt triggers LLM summarization. Default: 0.93.
	AutoCompactAt float64 `yaml:"auto_compact_at"`

	// MemoryCompactAt triggers memory extraction + summarization. Default: 0.97.
	MemoryCompactAt float64 `yaml:"memory_compact_at"`
}

// DefaultCompactThresholds returns sensible defaults.
func DefaultCompactThresholds() CompactThresholds {
	return CompactThresholds{
		CollapseAt:      0.70,
		MicroCompactAt:  0.80,
		AutoCompactAt:   0.93,
		MemoryCompactAt: 0.97,
	}
}

// ContextPressure represents the current context window usage and recommended action.
type ContextPressure struct {
	// TokenCount is the estimated token usage.
	TokenCount int

	// ContextWindow is the effective context window size.
	ContextWindow int

	// Ratio is TokenCount / ContextWindow (0.0 to 1.0+).
	Ratio float64

	// RecommendedLevel is the compaction level that should be applied.
	RecommendedLevel CompactLevel

	// TokensUntilBlocking is how many tokens remain before the blocking limit.
	TokensUntilBlocking int
}

// CompactionPipeline coordinates multi-level proactive compaction.
type CompactionPipeline struct {
	thresholds CompactThresholds
	breaker    *CompactionCircuitBreaker

	// lastLevel tracks the last compaction level applied to avoid
	// re-applying the same level repeatedly in the same turn.
	lastLevel CompactLevel
	mu        sync.Mutex
}

// NewCompactionPipeline creates a pipeline with the given thresholds.
func NewCompactionPipeline(thresholds CompactThresholds) *CompactionPipeline {
	return &CompactionPipeline{
		thresholds: thresholds,
		breaker:    NewCompactionCircuitBreaker(3, 5*time.Minute),
		lastLevel:  CompactNone,
	}
}

// Evaluate determines which compaction level should be applied based on
// the current token count and context window size.
func (p *CompactionPipeline) Evaluate(tokenCount, contextWindow int) ContextPressure {
	ratio := float64(tokenCount) / float64(contextWindow)

	var level CompactLevel
	switch {
	case ratio >= p.thresholds.MemoryCompactAt:
		level = CompactMemory
	case ratio >= p.thresholds.AutoCompactAt:
		level = CompactAuto
	case ratio >= p.thresholds.MicroCompactAt:
		level = CompactMicro
	case ratio >= p.thresholds.CollapseAt:
		level = CompactCollapse
	default:
		level = CompactNone
	}

	blockingTokens := contextWindow - tokenCount - int(float64(contextWindow)*0.03)
	if blockingTokens < 0 {
		blockingTokens = 0
	}

	return ContextPressure{
		TokenCount:          tokenCount,
		ContextWindow:       contextWindow,
		Ratio:               ratio,
		RecommendedLevel:    level,
		TokensUntilBlocking: blockingTokens,
	}
}

// ShouldCompact returns true if compaction should be attempted at the given level.
// Checks the circuit breaker and ensures we don't re-apply the same level.
func (p *CompactionPipeline) ShouldCompact(level CompactLevel) bool {
	if level == CompactNone {
		return false
	}
	// Cheap levels (Collapse, Micro) always allowed — no circuit breaker needed.
	if level <= CompactMicro {
		return true
	}
	// Expensive levels check circuit breaker.
	return p.breaker.Allow()
}

// RecordSuccess records a successful compaction for circuit breaker tracking.
func (p *CompactionPipeline) RecordSuccess() {
	p.breaker.RecordSuccess()
}

// RecordFailure records a failed compaction for circuit breaker tracking.
func (p *CompactionPipeline) RecordFailure() {
	p.breaker.RecordFailure()
}

// SetLastLevel records the last applied compaction level.
func (p *CompactionPipeline) SetLastLevel(level CompactLevel) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastLevel = level
}

// LastLevel returns the last applied compaction level.
func (p *CompactionPipeline) LastLevel() CompactLevel {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastLevel
}

// ── Circuit Breaker ──

// CompactionCircuitBreaker prevents compaction loops by tracking failures.
// After maxFailures consecutive failures, compaction is blocked until cooldown expires.
type CompactionCircuitBreaker struct {
	maxFailures int
	cooldown    time.Duration
	failures    int
	lastAttempt time.Time
	mu          sync.Mutex
}

// NewCompactionCircuitBreaker creates a new circuit breaker.
func NewCompactionCircuitBreaker(maxFailures int, cooldown time.Duration) *CompactionCircuitBreaker {
	return &CompactionCircuitBreaker{
		maxFailures: maxFailures,
		cooldown:    cooldown,
	}
}

// Allow returns true if compaction is allowed (not tripped).
func (cb *CompactionCircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.failures >= cb.maxFailures {
		if time.Since(cb.lastAttempt) < cb.cooldown {
			return false
		}
		// Cooldown expired — reset.
		cb.failures = 0
	}
	return true
}

// RecordFailure increments the failure counter.
func (cb *CompactionCircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastAttempt = time.Now()
}

// RecordSuccess resets the failure counter.
func (cb *CompactionCircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
}

// Failures returns the current failure count (for testing/logging).
func (cb *CompactionCircuitBreaker) Failures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failures
}

// ── MicroCompact ──

// MicroCompact replaces old tool result contents with a placeholder to free
// context tokens without requiring an LLM summarization call.
// Clears any tool result that is large enough (>200 chars) in the older part
// of the conversation. Short results (e.g. "file written successfully") are
// kept as they consume negligible tokens.
//
// keepLastN specifies how many recent messages to protect from clearing.
// Returns the modified messages and the number of results cleared.
func MicroCompact(messages []chatMessage, keepLastN int) ([]chatMessage, int) {
	if keepLastN <= 0 {
		keepLastN = 10
	}

	threshold := len(messages) - keepLastN
	if threshold <= 0 {
		return messages, 0 // Not enough messages to clear.
	}

	cleared := 0
	for i := 0; i < threshold; i++ {
		if messages[i].Role != "tool" {
			continue
		}

		// Only clear if content is substantial (> 200 chars).
		content, ok := messages[i].Content.(string)
		if !ok {
			continue
		}
		if len(content) < 200 {
			continue
		}

		// Replace with placeholder preserving a brief excerpt.
		excerpt := content
		if len(excerpt) > 80 {
			excerpt = excerpt[:80] + "..."
		}
		messages[i].Content = fmt.Sprintf("[Old tool result cleared: %s]", excerpt)
		cleared++
	}

	return messages, cleared
}

// CollapseToolResults truncates oversized tool results in-place without clearing them.
// This is the cheapest compaction level — just trims excessively long outputs.
// Returns the number of results collapsed.
func CollapseToolResults(messages []chatMessage, maxChars int) int {
	if maxChars <= 0 {
		maxChars = 4000
	}

	collapsed := 0
	for i := range messages {
		if messages[i].Role != "tool" {
			continue
		}
		content, ok := messages[i].Content.(string)
		if !ok || len(content) <= maxChars {
			continue
		}

		// Keep head + tail with a separator.
		headSize := maxChars * 2 / 3
		tailSize := maxChars / 3
		head := content[:headSize]
		tail := content[len(content)-tailSize:]

		omitted := len(content) - headSize - tailSize
		messages[i].Content = fmt.Sprintf("%s\n\n[... %d chars omitted ...]\n\n%s", head, omitted, tail)
		collapsed++
	}

	return collapsed
}

// MicroCompactByRatio applies micro-compaction using the context pruning config ratios.
// Tools older than the protect window are cleared based on the usage ratio:
//   - Above softTrimRatio: trim head+tail to SoftTrimMaxChars
//   - Above hardClearRatio: replace with placeholder entirely
func MicroCompactByRatio(messages []chatMessage, ratio float64, cfg ContextPruningConfig) ([]chatMessage, int) {
	if cfg.ProtectRecentTurns <= 0 {
		cfg.ProtectRecentTurns = 3
	}
	if cfg.SoftTrimMaxChars <= 0 {
		cfg.SoftTrimMaxChars = 4096
	}
	if cfg.SoftTrimRatio <= 0 {
		cfg.SoftTrimRatio = 0.3
	}
	if cfg.HardClearRatio <= 0 {
		cfg.HardClearRatio = 0.5
	}

	// Count assistant turns from the end to find the protect boundary.
	assistantTurns := 0
	protectFrom := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			assistantTurns++
			if assistantTurns >= cfg.ProtectRecentTurns {
				protectFrom = i
				break
			}
		}
	}

	modified := 0
	for i := 0; i < protectFrom; i++ {
		if messages[i].Role != "tool" {
			continue
		}
		content, ok := messages[i].Content.(string)
		if !ok || len(content) < 200 {
			continue
		}

		posRatio := float64(i) / float64(len(messages))
		age := 1.0 - posRatio // Older messages get higher age score.

		if age > cfg.HardClearRatio && ratio > cfg.SoftTrimRatio {
			// Hard clear: replace with placeholder.
			excerpt := content
			if len(excerpt) > 60 {
				excerpt = excerpt[:60] + "..."
			}
			messages[i].Content = fmt.Sprintf("[Cleared: %s]", excerpt)
			modified++
		} else if age > cfg.SoftTrimRatio && len(content) > cfg.SoftTrimMaxChars {
			// Soft trim: keep head + tail.
			headSize := cfg.SoftTrimMaxChars * 2 / 3
			tailSize := cfg.SoftTrimMaxChars / 3
			if headSize+tailSize < len(content) {
				head := content[:headSize]
				tail := content[len(content)-tailSize:]
				omitted := len(content) - headSize - tailSize
				messages[i].Content = fmt.Sprintf("%s\n[... %d chars omitted ...]\n%s",
					strings.TrimRight(head, "\n"), omitted, strings.TrimLeft(tail, "\n"))
				modified++
			}
		}
	}

	return messages, modified
}
