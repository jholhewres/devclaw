// Package copilot — memory_stack.go implements Sprint 2 Room 2.4's
// MemoryStack: the composer that assembles the layered memory system
// (L0 Identity + L1 Essential + L2 OnDemand) into a single prompt prefix
// that buildMemoryLayer prepends to the legacy L3 output.
//
// Design principles (do not revise without an ADR):
//
//  1. Stack-over-legacy: the stack NEVER rewrites buildMemoryLayer. It
//     renders a prefix that is prepended. When the stack is nil or all
//     layers return empty, buildMemoryLayer is byte-identical to v1.18.0.
//     This is the retrocompat gate — enforced by the golden fixture test
//     in prompt_layers_golden_test.go.
//
//  2. L0-never-trimmed: the Identity layer is the user's anchor. It is
//     ALWAYS rendered in full, even when it exceeds the total byte
//     budget. Rationale: a 50-byte identity blurb that the user curated
//     themselves is never the reason a prompt is over-budget, and losing
//     it silently breaks persona continuity across turns. If the caller
//     provides an unusually large identity file, the stack logs a WARN
//     but still includes it.
//
//  3. Trim priority L2 > L1 > L0 (trim L2 first, L1 second, L0 never).
//     L2 is ephemeral per-turn context; L1 is the per-wing story
//     summary; L0 is the user's persona anchor.
//
//  4. Panic isolation: a layer that panics is caught, logged, and its
//     contribution becomes the empty string. The remaining layers still
//     render and the prompt is still produced. No single bad layer can
//     prevent the assistant from answering.
//
//  5. Context cancellation short-circuits: a pre-cancelled context
//     returns "" without touching any layer. This keeps the hot path
//     responsive when the caller has already given up.
//
// The stack has zero dependencies on the legacy memory path. It reads
// the three Room 2.1/2.2/2.3 layers via interfaces so tests can inject
// mocks that panic, sleep, or return large payloads.
package copilot

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

func init() {
	// Wire L1 cache hit/miss callbacks into the memory package at program
	// start. The memory package cannot import copilot (import cycle), so it
	// exposes function-variable setters. This init runs once per process.
	memory.SetL1CacheHitFn(IncL1CacheHit)
	memory.SetL1CacheMissFn(IncL1CacheMiss)
}

// defaultStackBudget is the default cap on L0+L1+L2 combined bytes.
// 3600 bytes ≈ 900 tokens (1 token ≈ 4 bytes). Sized so the stack can
// hold a generous identity blurb (200 tokens) plus a full essential
// story (400 tokens) plus a full on-demand snippet (300 tokens).
const defaultStackBudget = 3600

// layerSeparator is inserted between non-empty layer outputs. Two
// newlines keep each layer on its own Markdown block when the prefix
// lands at the top of the prompt.
const layerSeparator = "\n\n"

// stackLayer is the minimal interface the MemoryStack needs from each
// composed layer. The concrete Room 2.1/2.2/2.3 types satisfy it by
// virtue of their existing Render methods (or a tiny adapter for L0
// which is context-free).
//
// The interface lives in this file (not in the memory package) to keep
// the memory package free of the copilot-side stack abstraction.
type stackLayer interface {
	// render returns the layer's contribution as a string, or "" when
	// the layer has no content. activeWing is the session's current
	// wing (empty = legacy). turn is the user's current message text.
	// The method is called inside a panic-isolating wrapper — it is
	// free to panic or return an empty string on error.
	render(ctx context.Context, activeWing, turn string) string
}

// identityAdapter wraps a *memory.IdentityLayer so it satisfies
// stackLayer. The underlying layer's Render method takes no arguments,
// so the adapter simply ignores ctx/activeWing/turn.
type identityAdapter struct {
	l *memory.IdentityLayer
}

func (a identityAdapter) render(_ context.Context, _, _ string) string {
	if a.l == nil {
		return ""
	}
	return a.l.Render()
}

// essentialAdapter wraps a *memory.EssentialLayer. It forwards ctx and
// activeWing; turn is ignored because the essential story is per-wing
// and is unaffected by the current turn's text.
type essentialAdapter struct {
	l *memory.EssentialLayer
}

func (a essentialAdapter) render(ctx context.Context, activeWing, _ string) string {
	if a.l == nil {
		return ""
	}
	return a.l.Render(ctx, activeWing)
}

// onDemandAdapter wraps a *memory.OnDemandLayer. It forwards ctx,
// activeWing, and turn — the L2 layer is the only one that reads the
// current turn text.
type onDemandAdapter struct {
	l *memory.OnDemandLayer
}

func (a onDemandAdapter) render(ctx context.Context, activeWing, turn string) string {
	if a.l == nil {
		return ""
	}
	return a.l.Render(ctx, activeWing, turn)
}

// StackConfig is the subset of HierarchyConfig the stack consumes.
// A zero-valued StackConfig uses defaults (see DefaultStackConfig).
type StackConfig struct {
	// TotalBudget caps the combined byte length of L0+L1+L2. A value
	// <= 0 uses defaultStackBudget (3600 bytes ≈ 900 tokens).
	TotalBudget int

	// ForceLegacy, when true, makes Build() short-circuit to "". The
	// caller then falls back to the legacy buildMemoryLayer output.
	// This is the escape hatch for v1.18.0 byte-identical behavior
	// under the stack — exposed in Room 2.5 via memory.stack.force_legacy.
	ForceLegacy bool
}

// DefaultStackConfig returns the sensible defaults: 3600-byte total
// budget, stack-active (not forced to legacy).
func DefaultStackConfig() StackConfig {
	return StackConfig{
		TotalBudget: defaultStackBudget,
		ForceLegacy: false,
	}
}

// withDefaults fills zero fields with package defaults.
func (c StackConfig) withDefaults() StackConfig {
	if c.TotalBudget <= 0 {
		c.TotalBudget = defaultStackBudget
	}
	return c
}

// StackStats is a point-in-time snapshot of the MemoryStack telemetry
// counters. Returned by (*MemoryStack).Stats.
type StackStats struct {
	// L0Bytes is the cumulative byte count contributed by the identity
	// layer across all Build calls.
	L0Bytes int64

	// L1Bytes is the cumulative byte count contributed by the essential
	// layer across all Build calls (after any truncation).
	L1Bytes int64

	// L2Bytes is the cumulative byte count contributed by the on-demand
	// layer across all Build calls (after any truncation).
	L2Bytes int64

	// TrimmedTotal counts Build calls where at least one layer was
	// truncated to fit the byte budget.
	TrimmedTotal int64

	// PanicTotal counts individual layer panics caught by the stack.
	// A single Build call can contribute up to three panics (one per
	// layer), though in practice a single bug rarely affects more than
	// one layer at a time.
	PanicTotal int64

	// BuildTotal counts Build calls that proceeded past the early-exit
	// checks (ForceLegacy, all-nil, context cancellation).
	BuildTotal int64
}

// MemoryStack composes the Sprint 2 layered memory system:
//
//	L0 — IdentityLayer   (anchored, never trimmed)
//	L1 — EssentialLayer  (per-wing story from template cache)
//	L2 — OnDemandLayer   (per-turn entity detection + retrieval)
//	L3 — the legacy buildMemoryLayer fallback (handled by the caller)
//
// The stack renders L0, L1, L2 into a single prefix string that
// prompt_layers.go's buildMemoryLayer prepends to the L3 output. When
// all three Sprint 2 layers return empty (or the stack is nil or
// ForceLegacy is on), the stack returns an empty string and
// buildMemoryLayer produces byte-identical output to v1.18.0. This is
// the retrocompat gate enforced by the golden fixture test.
//
// Thread safety: all methods are safe for concurrent use. Telemetry
// counters use sync/atomic.
type MemoryStack struct {
	identity  stackLayer // L0 — may render empty but must not be nil
	essential stackLayer // L1 — may render empty but must not be nil
	onDemand  stackLayer // L2 — may render empty but must not be nil

	cfg    StackConfig
	logger *slog.Logger

	// Telemetry counters. Read via Stats().
	l0Bytes      atomic.Int64
	l1Bytes      atomic.Int64
	l2Bytes      atomic.Int64
	trimmedTotal atomic.Int64
	panicTotal   atomic.Int64
	buildTotal   atomic.Int64
}

// NewMemoryStack constructs a stack. Any layer argument may be nil —
// nil layers are internally replaced with a no-op adapter that always
// returns the empty string. If all three concrete layers are nil, the
// stack's Build() method short-circuits to "" and the caller degrades
// to the legacy L3 path.
//
// If logger is nil, slog.Default() is used. The cfg is normalized via
// withDefaults() so callers can pass a zero StackConfig for defaults.
func NewMemoryStack(
	identity *memory.IdentityLayer,
	essential *memory.EssentialLayer,
	onDemand *memory.OnDemandLayer,
	cfg StackConfig,
	logger *slog.Logger,
) *MemoryStack {
	if logger == nil {
		logger = slog.Default()
	}
	return &MemoryStack{
		identity:  identityAdapter{l: identity},
		essential: essentialAdapter{l: essential},
		onDemand:  onDemandAdapter{l: onDemand},
		cfg:       cfg.withDefaults(),
		logger:    logger,
	}
}

// newStackFromLayers is a test-only constructor that accepts arbitrary
// stackLayer implementations. Used by memory_stack_test.go to inject
// panicking, empty, or oversized mock layers without touching the
// public constructor signature.
func newStackFromLayers(
	identity, essential, onDemand stackLayer,
	cfg StackConfig,
	logger *slog.Logger,
) *MemoryStack {
	if logger == nil {
		logger = slog.Default()
	}
	if identity == nil {
		identity = identityAdapter{l: nil}
	}
	if essential == nil {
		essential = essentialAdapter{l: nil}
	}
	if onDemand == nil {
		onDemand = onDemandAdapter{l: nil}
	}
	return &MemoryStack{
		identity:  identity,
		essential: essential,
		onDemand:  onDemand,
		cfg:       cfg.withDefaults(),
		logger:    logger,
	}
}

// Build renders the L0+L1+L2 prefix for the current turn. Returns the
// empty string when:
//
//   - the stack is nil;
//   - cfg.ForceLegacy is true;
//   - the context is already cancelled;
//   - all three layers render empty strings.
//
// On panic in any layer, the layer is skipped, an error is logged, and
// the remaining layers still execute. panicTotal is incremented. The
// prompt is always produced unless ctx is cancelled.
//
// activeWing is the session's current wing ("" = legacy / no wing).
// turn is the current user message text (passed to L2 for entity
// detection).
//
// The byte-budget algorithm is documented at the top of this file and
// enforced in place:
//
//  1. L0 is always rendered in full. If L0 alone exceeds the budget,
//     it is still included and a WARN is logged.
//  2. L1 is truncated first when L0 + L1 would exceed the budget.
//  3. L2 is trimmed first when the combined stack exceeds the budget
//     (i.e. L2 is the lowest priority: trim L2 to zero before trimming L1).
func (s *MemoryStack) Build(ctx context.Context, activeWing, turn string) string {
	if s == nil {
		return ""
	}
	if s.cfg.ForceLegacy {
		return ""
	}
	if ctx != nil && ctx.Err() != nil {
		return ""
	}

	// Render each layer under a panic guard. A panic becomes an empty
	// contribution and bumps panicTotal.
	l0 := s.renderSafe("L0", func() string {
		return s.identity.render(ctx, activeWing, turn)
	})
	l1 := s.renderSafe("L1", func() string {
		return s.essential.render(ctx, activeWing, turn)
	})
	l2 := s.renderSafe("L2", func() string {
		return s.onDemand.render(ctx, activeWing, turn)
	})

	// Early exit: if all three layers are empty, the stack contributes
	// nothing and the caller falls through to the legacy L3 path. This
	// is the retrocompat gate — verified by the golden fixture test.
	if l0 == "" && l1 == "" && l2 == "" {
		return ""
	}

	// Apply the byte-budget enforcement. L0 is never trimmed. L2 is
	// trimmed first; L1 is trimmed only after L2 has been fully
	// exhausted or is already empty.
	budget := s.cfg.TotalBudget
	bytesL0 := len(l0)
	trimmed := false

	// L0 is an anchor. Always include it in full, even if it alone is
	// over-budget. The contract is documented on the type.
	remaining := budget - bytesL0
	if remaining < 0 {
		s.logger.Warn("memory stack: L0 exceeds total budget, included anyway",
			"l0_bytes", bytesL0,
			"total_budget", budget,
		)
		remaining = 0
	}

	// L1 gets whatever is left after L0.
	bytesL1 := len(l1)
	if bytesL1 > remaining {
		// L1 does not fit. Truncate it to the available window.
		l1 = truncateStackLayer(l1, remaining)
		bytesL1 = len(l1)
		trimmed = true
	}
	remaining -= bytesL1

	// L2 gets whatever is left after L0 + L1.
	bytesL2 := len(l2)
	if bytesL2 > remaining {
		l2 = truncateStackLayer(l2, remaining)
		bytesL2 = len(l2)
		trimmed = true
	}

	// Accumulate telemetry.
	s.l0Bytes.Add(int64(bytesL0))
	s.l1Bytes.Add(int64(bytesL1))
	s.l2Bytes.Add(int64(bytesL2))
	if trimmed {
		s.trimmedTotal.Add(1)
	}
	s.buildTotal.Add(1)

	// Emit global layer-token counters (ADR-008 Phase C metrics).
	// Only called when at least one layer rendered content — the early-exit
	// above (all-empty) already returns before reaching here.
	IncLayerTokensL0(bytesL0)
	IncLayerTokensL1(bytesL1)
	IncLayerTokensL2(bytesL2)

	// Concatenate non-empty layers in strict order with the separator.
	parts := make([]string, 0, 3)
	if l0 != "" {
		parts = append(parts, l0)
	}
	if l1 != "" {
		parts = append(parts, l1)
	}
	if l2 != "" {
		parts = append(parts, l2)
	}
	return strings.Join(parts, layerSeparator)
}

// Stats returns a point-in-time snapshot of the stack's telemetry
// counters. Safe to call concurrently with Build.
func (s *MemoryStack) Stats() StackStats {
	if s == nil {
		return StackStats{}
	}
	return StackStats{
		L0Bytes:      s.l0Bytes.Load(),
		L1Bytes:      s.l1Bytes.Load(),
		L2Bytes:      s.l2Bytes.Load(),
		TrimmedTotal: s.trimmedTotal.Load(),
		PanicTotal:   s.panicTotal.Load(),
		BuildTotal:   s.buildTotal.Load(),
	}
}

// renderSafe invokes fn under a panic-catching defer. A panic becomes
// an empty string and bumps the panicTotal counter. The layer name is
// logged for diagnostics.
func (s *MemoryStack) renderSafe(name string, fn func() string) (result string) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Warn("memory stack: layer panicked",
				"layer", name,
				"panic", r,
			)
			s.panicTotal.Add(1)
			result = ""
		}
	}()
	return fn()
}

// truncateStackLayer cuts s to at most maxBytes, preferring a word
// boundary. When maxBytes <= 0 the result is "". The result is always
// trimmed of trailing whitespace so the layer separator renders cleanly.
//
// Multi-byte UTF-8 runes are never split. If the max falls inside a
// multi-byte sequence the cut walks back to the preceding rune start.
func truncateStackLayer(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}

	// Walk back from the byte limit to a valid UTF-8 boundary.
	cut := maxBytes
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}

	// Prefer a word boundary (space or newline) just before cut.
	boundary := cut
	for boundary > 0 {
		c := s[boundary-1]
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' {
			break
		}
		boundary--
	}

	if boundary > 0 {
		return strings.TrimRight(s[:boundary], " \t\r\n")
	}
	// No word boundary within the window — fall back to the rune cut.
	return strings.TrimRight(s[:cut], " \t\r\n")
}

// isRuneStart reports whether b is the first byte of a UTF-8 rune.
// Inlined rather than importing unicode/utf8 for one call.
func isRuneStart(b byte) bool {
	return b&0xC0 != 0x80
}
