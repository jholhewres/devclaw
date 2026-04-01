package copilot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// mockMemoryStore implements memory.Store for testing.
type mockMemoryStore struct {
	entries []memory.Entry
}

func (m *mockMemoryStore) Save(e memory.Entry) error {
	m.entries = append(m.entries, e)
	return nil
}

func (m *mockMemoryStore) Search(_ string, _ int) ([]memory.Entry, error) {
	return m.entries, nil
}

func (m *mockMemoryStore) GetRecent(limit int) ([]memory.Entry, error) {
	if limit > len(m.entries) {
		limit = len(m.entries)
	}
	return m.entries[len(m.entries)-limit:], nil
}

func (m *mockMemoryStore) GetByDate(_ time.Time) ([]memory.Entry, error) {
	return m.entries, nil
}

func (m *mockMemoryStore) GetAll() ([]memory.Entry, error) {
	return m.entries, nil
}

func (m *mockMemoryStore) SaveDailyLog(_ time.Time, _ string) error {
	return nil
}

func TestDreamConsolidatorGates(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	store := &mockMemoryStore{}

	cfg := DreamConfig{
		Enabled:            true,
		MinHoursBetween:    6,
		MinSessionsBetween: 2,
		IdleMinutes:        10,
	}

	d := NewDreamConsolidator(cfg, store, tmpDir, logger)

	t.Run("no dream without enough sessions", func(t *testing.T) {
		d.RecordSession() // Only 1 session, need 2.
		if d.shouldDream() {
			t.Error("expected shouldDream=false with only 1 session")
		}
	})

	t.Run("dream allowed after enough sessions", func(t *testing.T) {
		d.RecordSession() // Now 2 sessions.
		if !d.shouldDream() {
			t.Error("expected shouldDream=true with 2 sessions and no prior dream")
		}
	})

	t.Run("no dream too soon after last dream", func(t *testing.T) {
		// Run a dream to set LastDreamAt.
		d.recordResult("success")
		d.RecordSession()
		d.RecordSession()
		if d.shouldDream() {
			t.Error("expected shouldDream=false immediately after a dream")
		}
	})

	t.Run("lock file prevents concurrent dreams", func(t *testing.T) {
		// Reset state to pass time and session gates.
		d.mu.Lock()
		d.state.LastDreamAt = time.Now().Add(-24 * time.Hour)
		d.state.SessionsSince = 5
		d.mu.Unlock()

		lockPath := filepath.Join(tmpDir, "dream.lock")
		os.WriteFile(lockPath, []byte("locked"), 0o600)
		defer os.Remove(lockPath)

		if d.shouldDream() {
			t.Error("expected shouldDream=false when lock file exists")
		}
	})
}

func TestDreamConsolidatorRun(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	store := &mockMemoryStore{
		entries: []memory.Entry{
			{Content: "The project uses PostgreSQL for the main database", Category: "fact", Timestamp: time.Now()},
			{Content: "The project uses PostgreSQL as primary storage", Category: "fact", Timestamp: time.Now()},
			{Content: "User prefers Go over Python for backend services", Category: "preference", Timestamp: time.Now()},
			{Content: "API rate limit is 100 requests per minute", Category: "fact", Timestamp: time.Now()},
		},
	}

	cfg := DefaultDreamConfig()
	d := NewDreamConsolidator(cfg, store, tmpDir, logger)

	result := d.Run(context.Background())

	if result.Error != nil {
		t.Fatalf("dream run failed: %v", result.Error)
	}
	if result.MemoriesAnalyzed != 4 {
		t.Errorf("expected 4 analyzed, got %d", result.MemoriesAnalyzed)
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestDreamConsolidatorTooFewMemories(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	store := &mockMemoryStore{
		entries: []memory.Entry{
			{Content: "Single memory", Category: "fact"},
		},
	}

	cfg := DefaultDreamConfig()
	d := NewDreamConsolidator(cfg, store, tmpDir, logger)

	result := d.Run(context.Background())

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.MemoriesAnalyzed != 1 {
		t.Errorf("expected 1 analyzed, got %d", result.MemoriesAnalyzed)
	}
}

func TestDreamState(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	store := &mockMemoryStore{}

	cfg := DefaultDreamConfig()
	d := NewDreamConsolidator(cfg, store, tmpDir, logger)

	d.RecordSession()
	d.RecordSession()

	state := d.State()
	if state.SessionsSince != 2 {
		t.Errorf("expected 2 sessions, got %d", state.SessionsSince)
	}

	// Verify persistence — create new instance from same dir.
	d2 := NewDreamConsolidator(cfg, store, tmpDir, logger)
	state2 := d2.State()
	if state2.SessionsSince != 2 {
		t.Errorf("expected persisted 2 sessions, got %d", state2.SessionsSince)
	}
}

func TestFindDuplicates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	store := &mockMemoryStore{}
	d := NewDreamConsolidator(DefaultDreamConfig(), store, t.TempDir(), logger)

	entries := []memory.Entry{
		{Content: "The project uses PostgreSQL for the main database storage layer"},
		{Content: "The project uses PostgreSQL for the main database and persistence"},
		{Content: "User prefers tabs over spaces in all code files"},
	}

	dups := d.findDuplicates(entries)
	if len(dups) != 1 {
		t.Errorf("expected 1 duplicate group, got %d", len(dups))
	}
	if len(dups) > 0 && dups[0].count != 2 {
		t.Errorf("expected count 2, got %d", dups[0].count)
	}
}

func TestFindContradictions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	store := &mockMemoryStore{}
	d := NewDreamConsolidator(DefaultDreamConfig(), store, t.TempDir(), logger)

	entries := []memory.Entry{
		{Content: "User prefers the project to use PostgreSQL database for storage"},
		{Content: "User prefers the project to not use PostgreSQL database for storage"},
	}

	contras := d.findContradictions(entries)
	if len(contras) != 1 {
		t.Errorf("expected 1 contradiction, got %d", len(contras))
	}
}

func TestNormalizeForComparison(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello world"},
		{"  Multiple   Spaces  ", " multiple spaces "},
		{"UPPERCASE", "uppercase"},
	}
	for _, tt := range tests {
		got := normalizeForComparison(tt.input)
		if got != tt.want {
			t.Errorf("normalizeForComparison(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
