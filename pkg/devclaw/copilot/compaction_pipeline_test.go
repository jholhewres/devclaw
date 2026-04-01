package copilot

import (
	"strings"
	"testing"
	"time"
)

func TestCompactLevelString(t *testing.T) {
	tests := []struct {
		level CompactLevel
		want  string
	}{
		{CompactNone, "none"},
		{CompactCollapse, "collapse"},
		{CompactMicro, "micro-compact"},
		{CompactAuto, "auto-compact"},
		{CompactMemory, "memory-compact"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("CompactLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestCompactionPipelineEvaluate(t *testing.T) {
	p := NewCompactionPipeline(DefaultCompactThresholds())
	window := 100000

	tests := []struct {
		name   string
		tokens int
		want   CompactLevel
	}{
		{"no action at 50%", 50000, CompactNone},
		{"no action at 69%", 69000, CompactNone},
		{"collapse at 70%", 70000, CompactCollapse},
		{"collapse at 79%", 79000, CompactCollapse},
		{"micro at 80%", 80000, CompactMicro},
		{"micro at 92%", 92000, CompactMicro},
		{"auto at 93%", 93000, CompactAuto},
		{"auto at 96%", 96000, CompactAuto},
		{"memory at 97%", 97000, CompactMemory},
		{"memory at 100%", 100000, CompactMemory},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pressure := p.Evaluate(tt.tokens, window)
			if pressure.RecommendedLevel != tt.want {
				t.Errorf("Evaluate(%d, %d) = %s, want %s",
					tt.tokens, window, pressure.RecommendedLevel, tt.want)
			}
		})
	}
}

func TestCompactionPipelineContextPressure(t *testing.T) {
	p := NewCompactionPipeline(DefaultCompactThresholds())

	pressure := p.Evaluate(80000, 100000)

	if pressure.TokenCount != 80000 {
		t.Errorf("TokenCount = %d, want 80000", pressure.TokenCount)
	}
	if pressure.ContextWindow != 100000 {
		t.Errorf("ContextWindow = %d, want 100000", pressure.ContextWindow)
	}
	if pressure.Ratio != 0.8 {
		t.Errorf("Ratio = %f, want 0.8", pressure.Ratio)
	}
	if pressure.TokensUntilBlocking <= 0 {
		t.Errorf("TokensUntilBlocking should be positive, got %d", pressure.TokensUntilBlocking)
	}
}

func TestCircuitBreakerAllow(t *testing.T) {
	cb := NewCompactionCircuitBreaker(3, 100*time.Millisecond)

	t.Run("allows initially", func(t *testing.T) {
		if !cb.Allow() {
			t.Error("expected Allow() = true initially")
		}
	})

	t.Run("allows after 2 failures", func(t *testing.T) {
		cb.RecordFailure()
		cb.RecordFailure()
		if !cb.Allow() {
			t.Error("expected Allow() = true after 2 failures (max=3)")
		}
	})

	t.Run("blocks after 3 failures", func(t *testing.T) {
		cb.RecordFailure()
		if cb.Allow() {
			t.Error("expected Allow() = false after 3 failures")
		}
	})

	t.Run("resets after cooldown", func(t *testing.T) {
		time.Sleep(120 * time.Millisecond)
		if !cb.Allow() {
			t.Error("expected Allow() = true after cooldown")
		}
	})

	t.Run("resets on success", func(t *testing.T) {
		cb.RecordFailure()
		cb.RecordFailure()
		cb.RecordFailure()
		if cb.Allow() {
			t.Error("expected blocked")
		}
		// Wait for cooldown, then succeed
		time.Sleep(120 * time.Millisecond)
		cb.RecordSuccess()
		cb.RecordFailure()
		if !cb.Allow() {
			t.Error("expected Allow() = true after success reset + 1 failure")
		}
	})
}

func TestShouldCompact(t *testing.T) {
	p := NewCompactionPipeline(DefaultCompactThresholds())

	t.Run("none never compacts", func(t *testing.T) {
		if p.ShouldCompact(CompactNone) {
			t.Error("expected ShouldCompact(None) = false")
		}
	})

	t.Run("cheap levels allowed on first application", func(t *testing.T) {
		if !p.ShouldCompact(CompactCollapse) {
			t.Error("expected ShouldCompact(Collapse) = true on first call")
		}
		if !p.ShouldCompact(CompactMicro) {
			t.Error("expected ShouldCompact(Micro) = true on first call")
		}
	})

	t.Run("cheap levels suppressed on repeat", func(t *testing.T) {
		p.SetLastLevel(CompactCollapse)
		if p.ShouldCompact(CompactCollapse) {
			t.Error("expected ShouldCompact(Collapse) = false when same as lastLevel")
		}
		// But a higher level should still be allowed.
		if !p.ShouldCompact(CompactMicro) {
			t.Error("expected ShouldCompact(Micro) = true even after Collapse was last")
		}
		p.SetLastLevel(CompactNone) // Reset for next tests.
	})

	t.Run("expensive levels check breaker", func(t *testing.T) {
		if !p.ShouldCompact(CompactAuto) {
			t.Error("expected ShouldCompact(Auto) = true initially")
		}
		// Trip the breaker
		p.RecordFailure()
		p.RecordFailure()
		p.RecordFailure()
		if p.ShouldCompact(CompactAuto) {
			t.Error("expected ShouldCompact(Auto) = false after breaker trips")
		}
		// Cheap levels still work
		if !p.ShouldCompact(CompactCollapse) {
			t.Error("expected ShouldCompact(Collapse) = true even with tripped breaker")
		}
	})
}

func makeChatMessage(role, content string) chatMessage {
	return chatMessage{Role: role, Content: content}
}

func makeToolMessage(name, content string) chatMessage {
	return chatMessage{Role: "tool", Content: content, ToolCallID: "tc_" + name}
}

func TestMicroCompact(t *testing.T) {
	// Create messages: some old tool results + recent messages
	longContent := strings.Repeat("x", 500)
	messages := []chatMessage{
		makeChatMessage("user", "hello"),
		makeToolMessage("grep", longContent),       // old, large → cleared
		makeToolMessage("read_file", longContent),   // old, large → cleared
		makeToolMessage("bash", longContent),        // old, large → cleared
		makeToolMessage("write_file", longContent),  // old, large → cleared
		makeChatMessage("assistant", "working..."),
		makeChatMessage("user", "continue"),
		makeToolMessage("grep", longContent),        // recent, protected
		makeChatMessage("assistant", "done"),
	}

	result, cleared := MicroCompact(messages, 4)

	// Should clear all old large tool results (indices 1-4) but NOT recent (7)
	if cleared != 4 {
		t.Errorf("expected 4 cleared, got %d", cleared)
	}

	// Check cleared messages have placeholder
	for _, idx := range []int{1, 2, 3, 4} {
		content, ok := result[idx].Content.(string)
		if !ok {
			t.Errorf("result[%d].Content is not string", idx)
			continue
		}
		if !strings.HasPrefix(content, "[Old tool result cleared") {
			t.Errorf("result[%d] not cleared: %s", idx, content[:50])
		}
	}

	// Recent grep should NOT be cleared
	content7, _ := result[7].Content.(string)
	if strings.HasPrefix(content7, "[Old") {
		t.Error("recent tool result should NOT be cleared (protected)")
	}
}

func TestMicroCompactShortContent(t *testing.T) {
	messages := []chatMessage{
		makeToolMessage("grep", "short"),        // < 200 chars, should not be cleared
		makeToolMessage("grep", strings.Repeat("x", 300)), // > 200 chars, should be cleared
		makeChatMessage("user", "recent"),
	}

	_, cleared := MicroCompact(messages, 1)
	if cleared != 1 {
		t.Errorf("expected 1 cleared (only the long one), got %d", cleared)
	}
}

func TestCollapseToolResults(t *testing.T) {
	longContent := strings.Repeat("line\n", 2000) // ~10000 chars
	messages := []chatMessage{
		makeToolMessage("grep", longContent),
		makeToolMessage("bash", "short output"),
		makeChatMessage("assistant", "response"),
	}

	collapsed := CollapseToolResults(messages, 1000)

	if collapsed != 1 {
		t.Errorf("expected 1 collapsed, got %d", collapsed)
	}

	content, _ := messages[0].Content.(string)
	if len(content) > 1200 { // Allow some overhead for separator text
		t.Errorf("collapsed content too long: %d chars (max ~1000)", len(content))
	}
	if !strings.Contains(content, "chars omitted") {
		t.Error("collapsed content should contain 'chars omitted' separator")
	}

	// Short content should be unchanged
	content1, _ := messages[1].Content.(string)
	if content1 != "short output" {
		t.Error("short tool result should be unchanged")
	}
}

func TestMicroCompactByRatio(t *testing.T) {
	longContent := strings.Repeat("data\n", 2000) // ~10000 chars
	messages := []chatMessage{
		makeToolMessage("grep", longContent),        // Very old (pos 0)
		makeToolMessage("bash", longContent),         // Old (pos 1)
		makeChatMessage("user", "question"),           // Middle
		makeToolMessage("grep", longContent),         // Middle-recent
		makeChatMessage("assistant", "answer"),         // Recent (protected)
		makeChatMessage("user", "more"),                // Recent
		makeToolMessage("grep", "short"),              // Recent
		makeChatMessage("assistant", "final"),          // Most recent (protected)
	}

	cfg := ContextPruningConfig{
		SoftTrimRatio:      0.3,
		HardClearRatio:     0.5,
		SoftTrimMaxChars:   1000,
		ProtectRecentTurns: 2,
	}

	result, modified := MicroCompactByRatio(messages, 0.85, cfg)
	_ = result

	if modified == 0 {
		t.Error("expected some modifications at 85% context usage")
	}
}
