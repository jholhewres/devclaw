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

// ── Dream classifier phase tests ─────────────────────────────────────────────

// noopEmbedder satisfies memory.EmbeddingProvider without any model I/O.
type noopEmbedder struct{}

func (noopEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, 4)
	}
	return out, nil
}
func (noopEmbedder) Dimensions() int { return 4 }
func (noopEmbedder) Name() string    { return "noop" }
func (noopEmbedder) Model() string   { return "noop" }

// newDreamTestStore creates a *memory.SQLiteStore backed by a temp file.
// The store is closed via t.Cleanup.
func newDreamTestStore(t *testing.T) *memory.SQLiteStore {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "dream-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	f.Close()
	store, err := memory.NewSQLiteStore(f.Name(), noopEmbedder{}, slog.Default())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// dreamTestKeywords is a locale-neutral keyword map for classifier tests.
// Each wing has ≥3 keywords to satisfy ClassifierMinHits=3.
var dreamTestKeywords = map[string][]string{
	"alpha": {"foo", "bar", "qux", "corge"},
	"beta":  {"baz", "grault", "garply", "waldo"},
}

// seedDreamFiles inserts files+chunks into the store without going through
// the full indexer (which would require real embeddings). One chunk per file.
// Content must contain ≥3 matches for a single wing to satisfy ClassifierMinHits.
func seedDreamFiles(t *testing.T, store *memory.SQLiteStore, files map[string]string) {
	t.Helper()
	ctx := context.Background()
	for fileID, content := range files {
		chunks := []memory.Chunk{
			{FileID: fileID, Index: 0, Text: content, Hash: "h-" + fileID},
		}
		if err := store.IndexChunks(ctx, fileID, chunks, "fh-"+fileID); err != nil {
			t.Fatalf("IndexChunks %s: %v", fileID, err)
		}
	}
}

// alphaContent and betaContent have ≥3 keyword matches each to clear
// ClassifierMinHits=3 and ClassifierDominanceFactor=2.0.
const alphaContent = "foo bar qux corge alpha-only content with no beta signals here"
const betaContent = "baz grault garply waldo beta-only content with no alpha signals here"

// newDreamWithSQLite builds a DreamConsolidator backed by both a mock
// memory.Store (for the existing phases) and a real SQLiteStore (for the
// classifier phase).
func newDreamWithSQLite(t *testing.T, sqlStore *memory.SQLiteStore, hierCfg HierarchyConfig) *DreamConsolidator {
	t.Helper()
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	mock := &mockMemoryStore{
		entries: []memory.Entry{
			{Content: "alpha foo bar entry one", Category: "fact", Timestamp: time.Now()},
			{Content: "alpha foo bar entry two", Category: "fact", Timestamp: time.Now()},
			{Content: "beta baz qux entry three", Category: "fact", Timestamp: time.Now()},
			{Content: "gamma unrelated entry four", Category: "fact", Timestamp: time.Now()},
		},
	}
	d := NewDreamConsolidator(DefaultDreamConfig(), mock, tmpDir, logger).
		WithSQLiteStore(sqlStore).
		WithHierarchyConfig(hierCfg)
	return d
}

// classifiableNullCount runs a real (non-dry) classification pass and returns
// how many NULL-wing files the pass was able to classify. When the dream cycle
// already ran, this should return 0 (all classifiable files already labeled).
// When the dream cycle was skipped/disabled, this returns the number of files
// that still need classifying.
func classifiableNullCount(t *testing.T, store *memory.SQLiteStore) int {
	t.Helper()
	stats, err := store.RunLegacyClassificationPass(context.Background(), memory.LegacyClassificationConfig{
		BatchSize: 50,
		Keywords:  dreamTestKeywords,
	})
	if err != nil {
		t.Fatalf("classifiableNullCount pass: %v", err)
	}
	return stats.Classified
}

// TestDreamClassifierPhaseRunsWhenEnabled verifies that Run() invokes the
// legacy classifier and labels matched files when hierarchy is enabled.
// After the dream run, a follow-up classification pass should find 0
// remaining classifiable NULL-wing files.
func TestDreamClassifierPhaseRunsWhenEnabled(t *testing.T) {
	sqlStore := newDreamTestStore(t)
	seedDreamFiles(t, sqlStore, map[string]string{
		"file-a.md": alphaContent,
		"file-b.md": betaContent,
		"file-c.md": "this file has no matching signals at all",
	})

	hierCfg := HierarchyConfig{Enabled: true, LegacyKeywords: dreamTestKeywords}
	d := newDreamWithSQLite(t, sqlStore, hierCfg)

	result := d.Run(context.Background())
	if result.Error != nil {
		t.Fatalf("Run returned error: %v", result.Error)
	}

	// A follow-up pass on the same store should find 0 classifiable NULL files —
	// the dream cycle already handled them all.
	remaining := classifiableNullCount(t, sqlStore)
	if remaining != 0 {
		t.Errorf("expected 0 remaining classifiable files after dream run, got %d", remaining)
	}
}

// TestDreamClassifierPhaseIsIdempotent verifies that a second dream cycle
// classifies zero additional files (already labelled by the first cycle).
func TestDreamClassifierPhaseIsIdempotent(t *testing.T) {
	sqlStore := newDreamTestStore(t)
	seedDreamFiles(t, sqlStore, map[string]string{
		"file-a.md": alphaContent,
		"file-b.md": betaContent,
	})

	hierCfg := HierarchyConfig{Enabled: true, LegacyKeywords: dreamTestKeywords}
	d := newDreamWithSQLite(t, sqlStore, hierCfg)

	// First cycle — classifies both files.
	if r := d.Run(context.Background()); r.Error != nil {
		t.Fatalf("first Run error: %v", r.Error)
	}

	// Second cycle — wing IS NULL filter means no additional files found.
	if r := d.Run(context.Background()); r.Error != nil {
		t.Fatalf("second Run error: %v", r.Error)
	}

	// A follow-up manual pass should also find 0 remaining NULL files.
	remaining := classifiableNullCount(t, sqlStore)
	if remaining != 0 {
		t.Errorf("expected 0 classifiable files after two dream runs, got %d", remaining)
	}
}

// TestDreamClassifierPhaseDisabledByFlag verifies that when hierarchy is
// disabled, the classifier phase is skipped and classifiable files remain NULL.
func TestDreamClassifierPhaseDisabledByFlag(t *testing.T) {
	sqlStore := newDreamTestStore(t)
	seedDreamFiles(t, sqlStore, map[string]string{
		"file-a.md": alphaContent,
		"file-b.md": betaContent,
	})

	hierCfg := HierarchyConfig{Enabled: false, LegacyKeywords: dreamTestKeywords}
	d := newDreamWithSQLite(t, sqlStore, hierCfg)

	result := d.Run(context.Background())
	if result.Error != nil {
		t.Fatalf("Run error: %v", result.Error)
	}

	// With hierarchy disabled the dream phase skipped classification.
	// A follow-up pass should still find the classifiable files (still NULL).
	remaining := classifiableNullCount(t, sqlStore)
	if remaining == 0 {
		t.Error("expected classifiable NULL files to remain after disabled-flag run")
	}
}

// TestDreamClassifierPhaseEmptyKeywordsNoOp verifies that when hierarchy is
// enabled but LegacyKeywords is nil, the classifier is a no-op with no error.
func TestDreamClassifierPhaseEmptyKeywordsNoOp(t *testing.T) {
	sqlStore := newDreamTestStore(t)
	seedDreamFiles(t, sqlStore, map[string]string{
		"file-a.md": alphaContent,
	})

	hierCfg := HierarchyConfig{Enabled: true, LegacyKeywords: nil}
	d := newDreamWithSQLite(t, sqlStore, hierCfg)

	result := d.Run(context.Background())
	if result.Error != nil {
		t.Fatalf("Run error: %v", result.Error)
	}

	// With no keywords the pass is a no-op — file still has NULL wing.
	// Verify by running with real keywords; it should find 1 classifiable file.
	remaining := classifiableNullCount(t, sqlStore)
	if remaining == 0 {
		t.Error("expected file to still have NULL wing after no-keyword dream run")
	}
}

// TestDreamClassifierPhaseErrorIsolation verifies that when the classifier
// returns an error (closed store), the dream cycle still completes without
// panic, and result.Error is nil.
func TestDreamClassifierPhaseErrorIsolation(t *testing.T) {
	sqlStore := newDreamTestStore(t)
	seedDreamFiles(t, sqlStore, map[string]string{
		"file-a.md": alphaContent,
	})

	// Close the store before running so the classifier's DB query fails.
	sqlStore.Close()

	hierCfg := HierarchyConfig{Enabled: true, LegacyKeywords: dreamTestKeywords}

	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	mock := &mockMemoryStore{
		entries: []memory.Entry{
			{Content: "alpha foo bar entry one", Category: "fact", Timestamp: time.Now()},
			{Content: "alpha foo bar entry two", Category: "fact", Timestamp: time.Now()},
			{Content: "beta baz qux entry three", Category: "fact", Timestamp: time.Now()},
			{Content: "gamma unrelated entry four", Category: "fact", Timestamp: time.Now()},
		},
	}
	d := NewDreamConsolidator(DefaultDreamConfig(), mock, tmpDir, logger).
		WithSQLiteStore(sqlStore).
		WithHierarchyConfig(hierCfg)

	// Must not panic; dream cycle must complete and return no error.
	result := d.Run(context.Background())
	if result.Error != nil {
		t.Fatalf("dream cycle should succeed even when classifier errors, got: %v", result.Error)
	}
}
