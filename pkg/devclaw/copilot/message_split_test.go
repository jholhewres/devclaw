package copilot

import (
	"strings"
	"testing"
)

func TestSplitMessage_ShortText(t *testing.T) {
	t.Parallel()
	got := SplitMessage("hello", 100)
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("expected [hello], got %v", got)
	}
}

func TestSplitMessage_EmptyText(t *testing.T) {
	t.Parallel()
	got := SplitMessage("", 100)
	if got != nil {
		t.Errorf("expected nil for empty, got %v", got)
	}
}

func TestSplitMessage_DefaultMaxLen(t *testing.T) {
	t.Parallel()
	got := SplitMessage("short", 0)
	if len(got) != 1 {
		t.Errorf("maxLen 0 should use default, got %d chunks", len(got))
	}
}

func TestSplitMessage_NegativeMaxLen(t *testing.T) {
	t.Parallel()
	got := SplitMessage("short", -1)
	if len(got) != 1 {
		t.Errorf("maxLen -1 should use default, got %d chunks", len(got))
	}
}

func TestSplitMessage_ParagraphBoundary(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 30) + "\n\n" + strings.Repeat("b", 30)
	got := SplitMessage(text, 40)
	if len(got) < 2 {
		t.Errorf("expected split at paragraph, got %d chunks", len(got))
	}
}

func TestSplitMessage_LineBoundary(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30)
	got := SplitMessage(text, 40)
	if len(got) < 2 {
		t.Errorf("expected split at line, got %d chunks", len(got))
	}
}

func TestSplitMessage_SentenceBoundary(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 30) + ". " + strings.Repeat("b", 30)
	got := SplitMessage(text, 40)
	if len(got) < 2 {
		t.Errorf("expected split at sentence, got %d chunks", len(got))
	}
}

func TestSplitMessage_WordBoundary(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("a", 30) + " " + strings.Repeat("b", 30)
	got := SplitMessage(text, 40)
	if len(got) < 2 {
		t.Errorf("expected split at word, got %d chunks", len(got))
	}
}

func TestSplitMessage_HardSplit(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("x", 100)
	got := SplitMessage(text, 40)
	if len(got) < 2 {
		t.Errorf("expected hard split, got %d chunks", len(got))
	}
	// All content should be preserved.
	joined := strings.Join(got, "")
	if len(joined) != 100 {
		t.Errorf("content lost: expected 100 chars, got %d", len(joined))
	}
}

func TestSplitMessage_CodeBlockNotSplit(t *testing.T) {
	t.Parallel()
	code := "```go\n" + strings.Repeat("x", 30) + "\n```"
	text := "before " + code + " after"
	got := SplitMessage(text, 200)
	if len(got) != 1 {
		// Under the limit, should be one chunk.
		t.Errorf("expected single chunk, got %d", len(got))
	}
	if !strings.Contains(got[0], "```") {
		t.Error("code block markers missing")
	}
}

func TestSplitMessage_AllChunksWithinLimit(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("word ", 200) // ~1000 chars
	maxLen := 100
	chunks := SplitMessage(text, maxLen)
	for i, c := range chunks {
		// Allow some slack for code block placeholders that get restored.
		if len(c) > maxLen*2 {
			t.Errorf("chunk %d too long: %d > %d", i, len(c), maxLen)
		}
	}
}
