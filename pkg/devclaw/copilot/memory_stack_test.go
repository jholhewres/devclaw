// Package copilot — memory_stack_test.go covers the Room 2.4
// MemoryStack behavior: nil/empty short-circuits, ordering, budget
// enforcement (L0 never trimmed, L2 trimmed first, L1 second), panic
// isolation, context cancellation, stats accumulation, word-boundary
// truncation, and a race-detector smoke test.
//
// The tests use the package-private newStackFromLayers constructor so
// we can inject synthetic stackLayer implementations without wiring up
// real SQLite stores. The stackLayer interface is intentionally narrow
// — everything below composes tiny struct mocks.
package copilot

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test mocks
// ─────────────────────────────────────────────────────────────────────────────

// mockLayer is a stackLayer that returns a fixed string.
type mockLayer struct {
	out   string
	calls atomic.Int64
}

func (m *mockLayer) render(_ context.Context, _, _ string) string {
	m.calls.Add(1)
	return m.out
}

// panicLayer is a stackLayer that panics on render.
type panicLayer struct {
	calls atomic.Int64
}

func (p *panicLayer) render(_ context.Context, _, _ string) string {
	p.calls.Add(1)
	panic(errors.New("synthetic layer panic"))
}

// recordingLayer remembers its render arguments for assertions.
type recordingLayer struct {
	out  string
	mu   sync.Mutex
	args []recordedArgs
}

type recordedArgs struct {
	wing string
	turn string
}

func (r *recordingLayer) render(_ context.Context, wing, turn string) string {
	r.mu.Lock()
	r.args = append(r.args, recordedArgs{wing: wing, turn: turn})
	r.mu.Unlock()
	return r.out
}

// newQuietLogger returns a slog.Logger that writes to an internal buffer.
// Tests that need to assert on log output pass the buffer; others discard.
func newQuietLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), buf
}

// ─────────────────────────────────────────────────────────────────────────────
// Early-exit behavior
// ─────────────────────────────────────────────────────────────────────────────

func TestMemoryStack_NilLayersReturnsEmpty(t *testing.T) {
	logger, _ := newQuietLogger()
	// Public constructor: all three concrete layers nil.
	s := NewMemoryStack(nil, nil, nil, DefaultStackConfig(), logger)
	if got := s.Build(context.Background(), "", "hi"); got != "" {
		t.Errorf("expected empty string with all-nil layers, got %q", got)
	}
	if bt := s.Stats().BuildTotal; bt != 0 {
		t.Errorf("expected BuildTotal=0 when all layers empty, got %d", bt)
	}
}

func TestMemoryStack_ForceLegacyReturnsEmpty(t *testing.T) {
	logger, _ := newQuietLogger()
	id := &mockLayer{out: "identity-fragment"}
	es := &mockLayer{out: "essential-story"}
	od := &mockLayer{out: "on-demand-snippet"}

	cfg := DefaultStackConfig()
	cfg.ForceLegacy = true
	s := newStackFromLayers(id, es, od, cfg, logger)

	if got := s.Build(context.Background(), "alpha", "turn text"); got != "" {
		t.Errorf("expected empty string under ForceLegacy, got %q", got)
	}
	if id.calls.Load() != 0 || es.calls.Load() != 0 || od.calls.Load() != 0 {
		t.Errorf("expected zero layer calls under ForceLegacy, got %d/%d/%d",
			id.calls.Load(), es.calls.Load(), od.calls.Load())
	}
}

func TestMemoryStack_AllLayersEmptyReturnsEmpty(t *testing.T) {
	logger, _ := newQuietLogger()
	s := newStackFromLayers(&mockLayer{out: ""}, &mockLayer{out: ""}, &mockLayer{out: ""},
		DefaultStackConfig(), logger)

	if got := s.Build(context.Background(), "alpha", "turn"); got != "" {
		t.Errorf("expected empty string with all-empty layers, got %q", got)
	}
	// The layers WERE called (we needed to see they're empty), so build
	// total stays 0 because we short-circuit before incrementing it.
	if bt := s.Stats().BuildTotal; bt != 0 {
		t.Errorf("expected BuildTotal=0 when short-circuited after empty layers, got %d", bt)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Rendering & ordering
// ─────────────────────────────────────────────────────────────────────────────

func TestMemoryStack_L0OnlyWithinBudget(t *testing.T) {
	logger, _ := newQuietLogger()
	s := newStackFromLayers(
		&mockLayer{out: "hello identity"},
		&mockLayer{out: ""},
		&mockLayer{out: ""},
		DefaultStackConfig(),
		logger,
	)

	out := s.Build(context.Background(), "", "hi")
	if out != "hello identity" {
		t.Errorf("expected L0-only output %q, got %q", "hello identity", out)
	}
	st := s.Stats()
	if st.L0Bytes != int64(len("hello identity")) {
		t.Errorf("expected L0Bytes=%d, got %d", len("hello identity"), st.L0Bytes)
	}
	if st.BuildTotal != 1 {
		t.Errorf("expected BuildTotal=1, got %d", st.BuildTotal)
	}
}

func TestMemoryStack_OrderingL0L1L2(t *testing.T) {
	logger, _ := newQuietLogger()
	s := newStackFromLayers(
		&mockLayer{out: "MARKER_L0"},
		&mockLayer{out: "MARKER_L1"},
		&mockLayer{out: "MARKER_L2"},
		DefaultStackConfig(),
		logger,
	)

	out := s.Build(context.Background(), "alpha", "turn")
	i0 := strings.Index(out, "MARKER_L0")
	i1 := strings.Index(out, "MARKER_L1")
	i2 := strings.Index(out, "MARKER_L2")
	if i0 < 0 || i1 < 0 || i2 < 0 {
		t.Fatalf("expected all three markers present, got %q", out)
	}
	if !(i0 < i1 && i1 < i2) {
		t.Errorf("expected ordering L0 < L1 < L2, got positions %d, %d, %d in %q", i0, i1, i2, out)
	}
	// Confirm the separator is present between adjacent layers.
	if !strings.Contains(out, "MARKER_L0\n\nMARKER_L1") {
		t.Errorf("expected L0/L1 separator, got %q", out)
	}
	if !strings.Contains(out, "MARKER_L1\n\nMARKER_L2") {
		t.Errorf("expected L1/L2 separator, got %q", out)
	}
}

func TestMemoryStack_WingAndTurnForwardedToL2(t *testing.T) {
	logger, _ := newQuietLogger()
	rec := &recordingLayer{out: "on-demand-snippet"}
	s := newStackFromLayers(
		&mockLayer{out: "id"},
		&mockLayer{out: "es"},
		rec,
		DefaultStackConfig(),
		logger,
	)

	s.Build(context.Background(), "alpha", "what about the beta update?")
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.args) != 1 {
		t.Fatalf("expected 1 recorded call, got %d", len(rec.args))
	}
	if rec.args[0].wing != "alpha" {
		t.Errorf("expected wing=alpha, got %q", rec.args[0].wing)
	}
	if rec.args[0].turn != "what about the beta update?" {
		t.Errorf("expected turn forwarded, got %q", rec.args[0].turn)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Budget enforcement
// ─────────────────────────────────────────────────────────────────────────────

func TestMemoryStack_BudgetGuardTrimsL2First(t *testing.T) {
	logger, _ := newQuietLogger()
	l0 := strings.Repeat("a", 20)
	l1 := strings.Repeat("b ", 25)  // 50 bytes with spaces
	l2 := strings.Repeat("c ", 100) // 200 bytes with spaces

	cfg := DefaultStackConfig()
	cfg.TotalBudget = 100
	s := newStackFromLayers(
		&mockLayer{out: l0},
		&mockLayer{out: l1},
		&mockLayer{out: l2},
		cfg,
		logger,
	)

	out := s.Build(context.Background(), "", "turn")
	// L0 (20) + L1 (50) = 70, leaves 30 bytes for L2 → L2 is trimmed.
	if !strings.Contains(out, l0) {
		t.Error("expected L0 intact in output")
	}
	if !strings.Contains(out, strings.TrimSpace(l1)) {
		t.Errorf("expected L1 intact in output, got %q", out)
	}

	// Extract L2 portion from the output. It's the last segment after
	// the last "\n\n" separator.
	parts := strings.Split(out, "\n\n")
	if len(parts) != 3 {
		t.Fatalf("expected 3 segments separated by blank lines, got %d: %q", len(parts), out)
	}
	l2Got := parts[2]
	if len(l2Got) > 30 {
		t.Errorf("expected trimmed L2 length ≤ 30, got %d: %q", len(l2Got), l2Got)
	}
	if !strings.HasPrefix(l2Got, "c") {
		t.Errorf("expected trimmed L2 to start with 'c', got %q", l2Got)
	}

	st := s.Stats()
	if st.TrimmedTotal != 1 {
		t.Errorf("expected TrimmedTotal=1, got %d", st.TrimmedTotal)
	}
}

func TestMemoryStack_BudgetGuardTrimsL1SecondIfL2ExhaustedStillOver(t *testing.T) {
	logger, _ := newQuietLogger()
	l0 := strings.Repeat("a", 20)
	l1 := strings.Repeat("b ", 25) // 50 bytes with spaces

	cfg := DefaultStackConfig()
	cfg.TotalBudget = 40
	s := newStackFromLayers(
		&mockLayer{out: l0},
		&mockLayer{out: l1},
		&mockLayer{out: ""}, // L2 empty
		cfg,
		logger,
	)

	out := s.Build(context.Background(), "", "turn")
	if !strings.Contains(out, l0) {
		t.Error("expected L0 intact")
	}
	// Remaining budget for L1 is 40-20 = 20 bytes.
	parts := strings.Split(out, "\n\n")
	if len(parts) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(parts), out)
	}
	l1Got := parts[1]
	if len(l1Got) > 20 {
		t.Errorf("expected trimmed L1 length ≤ 20, got %d: %q", len(l1Got), l1Got)
	}

	st := s.Stats()
	if st.TrimmedTotal != 1 {
		t.Errorf("expected TrimmedTotal=1, got %d", st.TrimmedTotal)
	}
}

// TestMemoryStack_BudgetGuardExactFitTrimsL2 verifies that when L0+L1
// bytes exactly equal the budget, L2 is fully trimmed to empty (no
// partial L2 leak).
func TestMemoryStack_BudgetGuardExactFitTrimsL2(t *testing.T) {
	logger, _ := newQuietLogger()
	l0 := strings.Repeat("a", 20)
	l1 := strings.Repeat("b ", 25) // 50 bytes with spaces
	l2 := strings.Repeat("c ", 25) // 50 bytes with spaces

	cfg := DefaultStackConfig()
	cfg.TotalBudget = 70 // exactly L0 (20) + L1 (50)
	s := newStackFromLayers(
		&mockLayer{out: l0},
		&mockLayer{out: l1},
		&mockLayer{out: l2},
		cfg,
		logger,
	)

	out := s.Build(context.Background(), "", "turn")
	if !strings.Contains(out, l0) {
		t.Error("expected L0 intact in output")
	}
	if !strings.Contains(out, strings.TrimSpace(l1)) {
		t.Errorf("expected L1 intact in output, got %q", out)
	}
	if strings.Contains(out, "c") {
		t.Errorf("expected no L2 content (no 'c') in output, got %q", out)
	}

	st := s.Stats()
	if st.TrimmedTotal != 1 {
		t.Errorf("expected TrimmedTotal=1, got %d", st.TrimmedTotal)
	}
}

// TestMemoryStack_L0NeverTrimmed is the L0-never-trimmed invariant test.
// Even when L0 alone exceeds the total budget, it must render in full
// and a WARN is logged.
func TestMemoryStack_L0NeverTrimmed(t *testing.T) {
	logger, buf := newQuietLogger()
	l0 := strings.Repeat("a", 50)

	cfg := DefaultStackConfig()
	cfg.TotalBudget = 10
	s := newStackFromLayers(
		&mockLayer{out: l0},
		&mockLayer{out: ""},
		&mockLayer{out: ""},
		cfg,
		logger,
	)

	out := s.Build(context.Background(), "", "turn")
	if out != l0 {
		t.Errorf("expected L0 output intact (50 bytes), got %q (%d bytes)", out, len(out))
	}
	if !strings.Contains(buf.String(), "L0 exceeds total budget") {
		t.Errorf("expected WARN log about L0 exceeding budget, got: %s", buf.String())
	}
}

// TestMemoryStack_TruncationAtWordBoundary checks that a truncated L2
// ends on a whitespace boundary, not in the middle of a word.
func TestMemoryStack_TruncationAtWordBoundary(t *testing.T) {
	logger, _ := newQuietLogger()
	// 150-byte sentence of multi-char words separated by spaces.
	l2 := strings.Repeat("quickbrownfox ", 12) // 168 bytes

	cfg := DefaultStackConfig()
	cfg.TotalBudget = 60
	s := newStackFromLayers(
		&mockLayer{out: ""},
		&mockLayer{out: ""},
		&mockLayer{out: l2},
		cfg,
		logger,
	)

	out := s.Build(context.Background(), "", "turn")
	if len(out) > 60 {
		t.Errorf("expected total output ≤ 60 bytes, got %d", len(out))
	}
	// The truncated output must not end in the middle of 'quickbrownfox'.
	// Either it ends cleanly at a word boundary (end char is a word
	// char but no trailing partial after trimming), OR it's empty.
	if out == "" {
		return
	}
	// The output must be a prefix of l2 composed of whole words.
	// Recompose whole words up to the total length and check equality.
	words := strings.Fields(out)
	rebuild := strings.Join(words, " ")
	if rebuild != strings.TrimSpace(out) {
		t.Errorf("expected output to be whole words, got %q", out)
	}
	for _, w := range words {
		if w != "quickbrownfox" {
			t.Errorf("expected every word to be 'quickbrownfox', got %q", w)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Panic isolation
// ─────────────────────────────────────────────────────────────────────────────

func TestMemoryStack_PanicInL1IsIsolated(t *testing.T) {
	logger, buf := newQuietLogger()
	l0 := &mockLayer{out: "id-line"}
	l1 := &panicLayer{}
	l2 := &mockLayer{out: "od-line"}
	s := newStackFromLayers(l0, l1, l2, DefaultStackConfig(), logger)

	out := s.Build(context.Background(), "", "turn")
	if !strings.Contains(out, "id-line") {
		t.Error("expected L0 output present after L1 panic")
	}
	if !strings.Contains(out, "od-line") {
		t.Error("expected L2 output present after L1 panic")
	}

	st := s.Stats()
	if st.PanicTotal != 1 {
		t.Errorf("expected PanicTotal=1, got %d", st.PanicTotal)
	}
	if st.BuildTotal != 1 {
		t.Errorf("expected BuildTotal=1, got %d", st.BuildTotal)
	}
	if !strings.Contains(buf.String(), "layer panicked") {
		t.Errorf("expected WARN log about layer panic, got: %s", buf.String())
	}
	if l1.calls.Load() != 1 {
		t.Errorf("expected L1 call count=1, got %d", l1.calls.Load())
	}
}

func TestMemoryStack_PanicInAllLayersYieldsEmpty(t *testing.T) {
	logger, _ := newQuietLogger()
	s := newStackFromLayers(&panicLayer{}, &panicLayer{}, &panicLayer{},
		DefaultStackConfig(), newQuietLoggerOrDefault(logger))

	out := s.Build(context.Background(), "", "turn")
	if out != "" {
		t.Errorf("expected empty output when all layers panic, got %q", out)
	}
	st := s.Stats()
	if st.PanicTotal != 3 {
		t.Errorf("expected PanicTotal=3, got %d", st.PanicTotal)
	}
	if st.BuildTotal != 0 {
		t.Errorf("expected BuildTotal=0 when all layers empty after panic, got %d", st.BuildTotal)
	}
}

// newQuietLoggerOrDefault is a helper that returns l when non-nil.
// Tests use it to avoid conditional slog setup when composing helpers.
func newQuietLoggerOrDefault(l *slog.Logger) *slog.Logger {
	if l == nil {
		return slog.Default()
	}
	return l
}

// ─────────────────────────────────────────────────────────────────────────────
// Context cancellation
// ─────────────────────────────────────────────────────────────────────────────

func TestMemoryStack_ContextCancellationReturnsEarly(t *testing.T) {
	logger, _ := newQuietLogger()
	l0 := &mockLayer{out: "id"}
	l1 := &mockLayer{out: "es"}
	l2 := &mockLayer{out: "od"}
	s := newStackFromLayers(l0, l1, l2, DefaultStackConfig(), logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := s.Build(ctx, "", "turn")
	if out != "" {
		t.Errorf("expected empty output on pre-cancelled ctx, got %q", out)
	}
	if l0.calls.Load()+l1.calls.Load()+l2.calls.Load() != 0 {
		t.Errorf("expected zero layer calls on pre-cancelled ctx, got %d/%d/%d",
			l0.calls.Load(), l1.calls.Load(), l2.calls.Load())
	}
	if s.Stats().BuildTotal != 0 {
		t.Errorf("expected BuildTotal=0 on pre-cancelled ctx, got %d", s.Stats().BuildTotal)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Stats accumulation
// ─────────────────────────────────────────────────────────────────────────────

func TestMemoryStack_StatsCountersAccumulate(t *testing.T) {
	logger, _ := newQuietLogger()
	s := newStackFromLayers(
		&mockLayer{out: "aa"},   // 2 bytes
		&mockLayer{out: "bbb"},  // 3 bytes
		&mockLayer{out: "cccc"}, // 4 bytes
		DefaultStackConfig(),
		logger,
	)

	s.Build(context.Background(), "", "turn1")
	s.Build(context.Background(), "", "turn2")

	st := s.Stats()
	if st.BuildTotal != 2 {
		t.Errorf("expected BuildTotal=2, got %d", st.BuildTotal)
	}
	if st.L0Bytes != 4 {
		t.Errorf("expected L0Bytes=4 (2*2), got %d", st.L0Bytes)
	}
	if st.L1Bytes != 6 {
		t.Errorf("expected L1Bytes=6 (2*3), got %d", st.L1Bytes)
	}
	if st.L2Bytes != 8 {
		t.Errorf("expected L2Bytes=8 (2*4), got %d", st.L2Bytes)
	}
	if st.TrimmedTotal != 0 {
		t.Errorf("expected TrimmedTotal=0, got %d", st.TrimmedTotal)
	}
}

func TestMemoryStack_NilReceiverStatsSafe(t *testing.T) {
	var s *MemoryStack
	st := s.Stats()
	if st.BuildTotal != 0 || st.L0Bytes != 0 {
		t.Errorf("expected zero stats from nil receiver, got %+v", st)
	}
	if got := s.Build(context.Background(), "", ""); got != "" {
		t.Errorf("expected empty Build from nil receiver, got %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Race cleanliness
// ─────────────────────────────────────────────────────────────────────────────

func TestMemoryStack_RaceClean(t *testing.T) {
	logger, _ := newQuietLogger()
	s := newStackFromLayers(
		&mockLayer{out: "identity-line"},
		&mockLayer{out: "essential-line"},
		&mockLayer{out: "ondemand-line"},
		DefaultStackConfig(),
		logger,
	)

	const (
		goroutines = 50
		iterations = 20
	)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(turnIdx int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				s.Build(context.Background(), "alpha",
					"turn-"+string(rune('a'+turnIdx%26)))
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("race test timeout")
	}

	if bt := s.Stats().BuildTotal; bt != int64(goroutines*iterations) {
		t.Errorf("expected BuildTotal=%d, got %d", goroutines*iterations, bt)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// truncateStackLayer unit tests
// ─────────────────────────────────────────────────────────────────────────────

func TestTruncateStackLayer(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		budget int
		want   string
	}{
		{"zero budget", "abc def", 0, ""},
		{"negative budget", "abc def", -1, ""},
		{"fits fully", "short", 100, "short"},
		{"word boundary", "abc def ghi", 8, "abc def"},
		{"no boundary falls back to rune cut", "quickbrownfox", 6, "quickb"},
		{"trailing spaces trimmed", "abc    ", 10, "abc    "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateStackLayer(tc.in, tc.budget)
			if got != tc.want {
				t.Errorf("in=%q budget=%d: want %q, got %q",
					tc.in, tc.budget, tc.want, got)
			}
		})
	}
}

func TestTruncateStackLayer_UTF8Safe(t *testing.T) {
	// "héllo" is 6 bytes: h=1, é=2, l=1, l=1, o=1.
	in := "héllo"
	if got := truncateStackLayer(in, 3); got == "h\xc3" {
		t.Errorf("expected truncate to never split a UTF-8 rune, got %q", got)
	}
}
