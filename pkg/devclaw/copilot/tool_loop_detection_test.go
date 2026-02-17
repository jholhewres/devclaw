package copilot

import (
	"log/slog"
	"testing"
)

func newTestDetector(cfg ToolLoopConfig) *ToolLoopDetector {
	return NewToolLoopDetector(cfg, slog.Default())
}

func TestToolLoopDetector_NoLoopBeforeThreshold(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        5,
		CriticalThreshold:       10,
		CircuitBreakerThreshold: 15,
	})

	args := map[string]any{"command": "ls -la"}

	for i := 0; i < 4; i++ {
		r := d.RecordAndCheck("bash", args)
		if r.Severity != LoopNone {
			t.Fatalf("expected LoopNone at iteration %d, got %d", i, r.Severity)
		}
	}
}

func TestToolLoopDetector_Warning(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	args := map[string]any{"command": "cat file.txt"}

	for i := 0; i < 2; i++ {
		d.RecordAndCheck("bash", args)
	}

	r := d.RecordAndCheck("bash", args)
	if r.Severity != LoopWarning {
		t.Errorf("expected LoopWarning, got %d", r.Severity)
	}
	if r.Pattern != "repeat" {
		t.Errorf("expected pattern 'repeat', got %q", r.Pattern)
	}
	if r.Streak != 3 {
		t.Errorf("expected streak 3, got %d", r.Streak)
	}
}

func TestToolLoopDetector_Critical(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	args := map[string]any{"command": "curl http://example.com"}

	for i := 0; i < 5; i++ {
		d.RecordAndCheck("bash", args)
	}

	r := d.RecordAndCheck("bash", args)
	if r.Severity != LoopCritical {
		t.Errorf("expected LoopCritical, got %d", r.Severity)
	}
}

func TestToolLoopDetector_CircuitBreaker(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	args := map[string]any{"command": "cat file.txt"}

	for i := 0; i < 9; i++ {
		d.RecordAndCheck("bash", args)
	}

	r := d.RecordAndCheck("bash", args)
	if r.Severity != LoopBreaker {
		t.Errorf("expected LoopBreaker, got %d", r.Severity)
	}
	if r.Streak != 10 {
		t.Errorf("expected streak 10, got %d", r.Streak)
	}
}

func TestToolLoopDetector_DifferentArgsNoLoop(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	for i := 0; i < 20; i++ {
		args := map[string]any{"command": "echo " + string(rune('a'+i))}
		r := d.RecordAndCheck("bash", args)
		if r.Severity != LoopNone {
			t.Fatalf("expected LoopNone at iteration %d with unique args, got %d", i, r.Severity)
		}
	}
}

func TestToolLoopDetector_PingPong(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	argsA := map[string]any{"command": "cat a.txt"}
	argsB := map[string]any{"command": "cat b.txt"}

	// Build A-B-A-B-A-B pattern (6 calls = 3 pairs).
	var lastResult LoopDetectionResult
	for i := 0; i < 6; i++ {
		if i%2 == 0 {
			lastResult = d.RecordAndCheck("bash", argsA)
		} else {
			lastResult = d.RecordAndCheck("bash", argsB)
		}
	}

	if lastResult.Severity < LoopWarning {
		t.Errorf("expected at least LoopWarning after ping-pong pattern, got %d (streak=%d, pattern=%s)",
			lastResult.Severity, lastResult.Streak, lastResult.Pattern)
	}
}

func TestToolLoopDetector_Reset(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	args := map[string]any{"command": "ls"}

	for i := 0; i < 2; i++ {
		d.RecordAndCheck("bash", args)
	}

	d.Reset()

	r := d.RecordAndCheck("bash", args)
	if r.Severity != LoopNone {
		t.Errorf("after Reset, expected LoopNone, got %d (streak=%d)", r.Severity, r.Streak)
	}
}

func TestToolLoopDetector_Disabled(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 false,
		WarningThreshold:        3,
		CriticalThreshold:       6,
		CircuitBreakerThreshold: 10,
	})

	args := map[string]any{"command": "ls"}

	for i := 0; i < 20; i++ {
		r := d.RecordAndCheck("bash", args)
		if r.Severity != LoopNone {
			t.Fatalf("disabled detector should always return LoopNone, got %d", r.Severity)
		}
	}
}

func TestToolLoopDetector_HistoryRingBuffer(t *testing.T) {
	t.Parallel()
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             5,
		WarningThreshold:        4,
		CriticalThreshold:       8,
		CircuitBreakerThreshold: 12,
	})

	// Fill with 5 unique calls.
	for i := 0; i < 5; i++ {
		d.RecordAndCheck("bash", map[string]any{"i": i})
	}

	// Now repeat 3 times — history only holds 5, so streak can't hit 4.
	args := map[string]any{"command": "repeat"}
	for i := 0; i < 3; i++ {
		r := d.RecordAndCheck("bash", args)
		if r.Severity != LoopNone {
			t.Fatalf("streak within history window should not trigger warning, got %d", r.Severity)
		}
	}
}

func TestToolLoopConfig_DefaultValues(t *testing.T) {
	t.Parallel()
	cfg := DefaultToolLoopConfig()

	if !cfg.Enabled {
		t.Error("default should be enabled")
	}
	if cfg.HistorySize != 30 {
		t.Errorf("expected HistorySize 30, got %d", cfg.HistorySize)
	}
	if cfg.WarningThreshold != 8 {
		t.Errorf("expected WarningThreshold 8, got %d", cfg.WarningThreshold)
	}
	if cfg.CriticalThreshold != 15 {
		t.Errorf("expected CriticalThreshold 15, got %d", cfg.CriticalThreshold)
	}
	if cfg.CircuitBreakerThreshold != 25 {
		t.Errorf("expected CircuitBreakerThreshold 25, got %d", cfg.CircuitBreakerThreshold)
	}
}

func TestNewToolLoopDetector_NormalizesThresholds(t *testing.T) {
	t.Parallel()

	// Inverted thresholds should be corrected.
	d := newTestDetector(ToolLoopConfig{
		Enabled:                 true,
		HistorySize:             30,
		WarningThreshold:        10,
		CriticalThreshold:       5,  // Less than warning.
		CircuitBreakerThreshold: 3,  // Less than critical.
	})

	if d.config.CriticalThreshold <= d.config.WarningThreshold {
		t.Errorf("CriticalThreshold (%d) should be > WarningThreshold (%d)",
			d.config.CriticalThreshold, d.config.WarningThreshold)
	}
	if d.config.CircuitBreakerThreshold <= d.config.CriticalThreshold {
		t.Errorf("CircuitBreakerThreshold (%d) should be > CriticalThreshold (%d)",
			d.config.CircuitBreakerThreshold, d.config.CriticalThreshold)
	}
}

func TestHashToolCall_Deterministic(t *testing.T) {
	t.Parallel()

	args := map[string]any{"command": "echo hello", "timeout": 30}
	h1 := hashToolCall("bash", args)
	h2 := hashToolCall("bash", args)

	if h1 != h2 {
		t.Errorf("hash should be deterministic: %q != %q", h1, h2)
	}

	h3 := hashToolCall("ssh", args)
	if h1 == h3 {
		t.Error("different tool names should produce different hashes")
	}
}

func TestHashToolCall_DifferentArgs(t *testing.T) {
	t.Parallel()

	h1 := hashToolCall("bash", map[string]any{"command": "ls"})
	h2 := hashToolCall("bash", map[string]any{"command": "pwd"})

	if h1 == h2 {
		t.Error("different args should produce different hashes")
	}
}
