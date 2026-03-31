package copilot

import (
	"strings"
	"testing"
)

func TestParseExtractedMemories(t *testing.T) {
	t.Run("valid JSON array", func(t *testing.T) {
		input := `[{"type":"decision","content":"Use PostgreSQL for persistence","importance":4},{"type":"fact","content":"API rate limit is 100/min","importance":3}]`
		memories := parseExtractedMemories(input)
		if len(memories) != 2 {
			t.Fatalf("expected 2 memories, got %d", len(memories))
		}
		if memories[0].Type != "decision" {
			t.Errorf("expected type 'decision', got %q", memories[0].Type)
		}
		if memories[0].Content != "Use PostgreSQL for persistence" {
			t.Errorf("unexpected content: %q", memories[0].Content)
		}
		if memories[0].Importance != 4 {
			t.Errorf("expected importance 4, got %d", memories[0].Importance)
		}
	})

	t.Run("JSON in markdown code block", func(t *testing.T) {
		input := "```json\n[{\"type\":\"learning\",\"content\":\"Mutex needed for shared state\",\"importance\":3}]\n```"
		memories := parseExtractedMemories(input)
		if len(memories) != 1 {
			t.Fatalf("expected 1 memory, got %d", len(memories))
		}
		if memories[0].Type != "learning" {
			t.Errorf("expected type 'learning', got %q", memories[0].Type)
		}
	})

	t.Run("JSON with surrounding text", func(t *testing.T) {
		input := "Here are the extracted memories:\n[{\"type\":\"preference\",\"content\":\"User prefers short responses\",\"importance\":2}]\nThat's all."
		memories := parseExtractedMemories(input)
		if len(memories) != 1 {
			t.Fatalf("expected 1 memory, got %d", len(memories))
		}
	})

	t.Run("empty array", func(t *testing.T) {
		memories := parseExtractedMemories("[]")
		if len(memories) != 0 {
			t.Errorf("expected 0 memories, got %d", len(memories))
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		memories := parseExtractedMemories("not json at all")
		if memories != nil {
			t.Errorf("expected nil for invalid JSON, got %v", memories)
		}
	})

	t.Run("filters empty entries", func(t *testing.T) {
		input := `[{"type":"","content":"no type","importance":3},{"type":"fact","content":"","importance":3},{"type":"fact","content":"valid","importance":3}]`
		memories := parseExtractedMemories(input)
		if len(memories) != 1 {
			t.Fatalf("expected 1 valid memory, got %d", len(memories))
		}
		if memories[0].Content != "valid" {
			t.Errorf("unexpected content: %q", memories[0].Content)
		}
	})

	t.Run("clamps importance", func(t *testing.T) {
		input := `[{"type":"fact","content":"too low","importance":0},{"type":"fact","content":"too high","importance":10}]`
		memories := parseExtractedMemories(input)
		if len(memories) != 2 {
			t.Fatalf("expected 2 memories, got %d", len(memories))
		}
		if memories[0].Importance != 1 {
			t.Errorf("expected importance clamped to 1, got %d", memories[0].Importance)
		}
		if memories[1].Importance != 5 {
			t.Errorf("expected importance clamped to 5, got %d", memories[1].Importance)
		}
	})

	t.Run("normalizes type to lowercase", func(t *testing.T) {
		input := `[{"type":"DECISION","content":"test","importance":3}]`
		memories := parseExtractedMemories(input)
		if len(memories) != 1 {
			t.Fatalf("expected 1 memory, got %d", len(memories))
		}
		if memories[0].Type != "decision" {
			t.Errorf("expected lowercase type, got %q", memories[0].Type)
		}
	})
}

func TestMemoryTypeSummary(t *testing.T) {
	memories := []ExtractedMemory{
		{Type: "decision", Content: "a", Importance: 3},
		{Type: "decision", Content: "b", Importance: 4},
		{Type: "fact", Content: "c", Importance: 2},
		{Type: "learning", Content: "d", Importance: 5},
	}

	summary := memoryTypeSummary(memories)
	if !strings.Contains(summary, "2 decision") {
		t.Errorf("expected '2 decision' in summary, got %q", summary)
	}
	if !strings.Contains(summary, "1 fact") {
		t.Errorf("expected '1 fact' in summary, got %q", summary)
	}
	if !strings.Contains(summary, "1 learning") {
		t.Errorf("expected '1 learning' in summary, got %q", summary)
	}
}

func TestFormatMemoriesForStorage(t *testing.T) {
	memories := []ExtractedMemory{
		{Type: "decision", Content: "Use Go modules", Importance: 4},
		{Type: "fact", Content: "API has 100/min limit", Importance: 3},
	}

	result := FormatMemoriesForStorage(memories, "session-123")

	if !strings.Contains(result, "session-123") {
		t.Error("expected session ID in output")
	}
	if !strings.Contains(result, "[decision]") {
		t.Error("expected [decision] tag")
	}
	if !strings.Contains(result, "Use Go modules") {
		t.Error("expected memory content")
	}
	if !strings.Contains(result, "(!!!!)" ) {
		t.Error("expected importance markers (4 = !!!!)")
	}
}

func TestFormatMemoriesForStorageEmpty(t *testing.T) {
	result := FormatMemoriesForStorage(nil, "session-123")
	if result != "" {
		t.Errorf("expected empty string for nil memories, got %q", result)
	}
}

func TestIsSystemInjectedMessage(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"[System: Turn 5/25...]", true},
		{"HEARTBEAT", true},
		{"contains HEARTBEAT_OK in text", true},
		{"Hello, how can I help?", false},
		{"Please fix the bug", false},
	}
	for _, tt := range tests {
		if got := isSystemInjectedMessage(tt.input); got != tt.want {
			t.Errorf("isSystemInjectedMessage(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
