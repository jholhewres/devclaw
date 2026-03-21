package copilot

import (
	"strings"
	"testing"
)

func TestFindMaxSkillsFit(t *testing.T) {
	tests := []struct {
		name     string
		entries  []string
		maxChars int
		want     int
	}{
		{
			name:     "empty entries",
			entries:  nil,
			maxChars: 100,
			want:     0,
		},
		{
			name:     "all fit",
			entries:  []string{"abc", "def", "ghi"},
			maxChars: 100,
			want:     3,
		},
		{
			name:     "exact fit",
			entries:  []string{"abc", "def"},
			maxChars: 6,
			want:     2,
		},
		{
			name:     "only first fits",
			entries:  []string{"abc", "def"},
			maxChars: 4,
			want:     1,
		},
		{
			name:     "none fit",
			entries:  []string{"abcdef"},
			maxChars: 3,
			want:     0,
		},
		{
			name:     "zero budget",
			entries:  []string{"a"},
			maxChars: 0,
			want:     0,
		},
		{
			name:     "negative budget",
			entries:  []string{"a", "b"},
			maxChars: -10,
			want:     0,
		},
		{
			name:     "large set partial fit",
			entries:  []string{"12345", "12345", "12345", "12345"},
			maxChars: 12,
			want:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findMaxSkillsFit(tt.entries, tt.maxChars)
			if got != tt.want {
				t.Errorf("findMaxSkillsFit() = %d, want %d", got, tt.want)
			}
		})
	}
}

// panicBuilder always panics when called.
type panicBuilder struct{}

func (p panicBuilder) BuildMemorySection(_ *Session, _ string) string {
	panic("intentional panic in builder")
}

// safeBuilder returns a fixed section.
type safeBuilder struct{ section string }

func (s safeBuilder) BuildMemorySection(_ *Session, _ string) string {
	return s.section
}

func TestMemoryPromptSectionBuilder_PanicRecovery(t *testing.T) {
	composer := NewPromptComposer(&Config{})
	composer.RegisterMemorySectionBuilder(safeBuilder{section: "## Before\n\nfirst"})
	composer.RegisterMemorySectionBuilder(panicBuilder{})
	composer.RegisterMemorySectionBuilder(safeBuilder{section: "## After\n\nsecond"})

	session := &Session{
		ID:      "test-session",
		Channel: "test",
		ChatID:  "1",
	}

	// buildMemoryLayer should not panic even with a panicking builder.
	result := composer.buildMemoryLayer(session, "test input")

	// The two safe builders should have contributed their sections.
	if result == "" {
		t.Fatal("expected non-empty result from safe builders")
	}
	if !strings.Contains(result, "first") {
		t.Error("expected result to contain output from first safe builder")
	}
	if !strings.Contains(result, "second") {
		t.Error("expected result to contain output from second safe builder")
	}
}

func TestMemoryPromptSectionBuilder_NoDuplicateOnReregister(t *testing.T) {
	composer := NewPromptComposer(&Config{})
	b := safeBuilder{section: "## Test\n\ndata"}
	composer.RegisterMemorySectionBuilder(b)
	composer.RegisterMemorySectionBuilder(b)

	// Both are appended (no dedup — documented behavior).
	composer.memorySectionBuildersMu.RLock()
	count := len(composer.memorySectionBuilders)
	composer.memorySectionBuildersMu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 builders registered, got %d", count)
	}
}
