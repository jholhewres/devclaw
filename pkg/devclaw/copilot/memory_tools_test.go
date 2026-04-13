// Package copilot — memory_tools_test.go tests wing assignment in handleMemorySave.
//
// Sprint 2 Room 2.0b: after a file is saved and indexed, the palace wing
// column on the files row must be populated correctly based on:
//   - args["wing"] supplied by the LLM (explicit override)
//   - ContextRouter resolution from the session delivery target
//   - Legacy NULL behavior when neither is available or valid
//
// Test strategy: because handleMemorySave's auto-indexing path depends on
// the full SQLite embedding pipeline (which may lack FTS5 in CI), tests that
// verify files.wing call IndexChunks directly to seed the file row, then
// assert on the wing column via AssignWingToFile's conditional UPDATE
// semantics. Tests for the full save path use cfg.Index.Auto=false so
// indexing is skipped while the file-write and wing-assignment paths
// are still exercised.
package copilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

// testMemToolsSetup returns a fresh FileStore, SQLiteStore, and MemoryConfig
// for each test. Auto-indexing is DISABLED by default so tests don't wait on
// the embedding pipeline. Tests that need a file row in `files` call
// seedFileRow() after setup.
func testMemToolsSetup(t *testing.T) (*memory.FileStore, *memory.SQLiteStore, MemoryConfig) {
	t.Helper()

	dir := t.TempDir()

	// FileStore writes MEMORY.md under dir/memory/
	memDir := filepath.Join(dir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatalf("mkdir memDir: %v", err)
	}
	store, err := memory.NewFileStore(memDir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	// SQLiteStore at dir/memory.db
	dbPath := filepath.Join(dir, "memory.db")
	sqliteStore, err := memory.NewSQLiteStore(dbPath, &memToolsTestEmbedder{}, slog.Default())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { sqliteStore.Close() })

	// cfg.Path is the MEMORY.md path. filepath.Dir(cfg.Path)/memory = memDir.
	// Auto-indexing OFF: tests seed file rows manually for speed.
	cfg := MemoryConfig{
		Path: filepath.Join(dir, "MEMORY.md"),
		Index: IndexConfig{
			Auto:           false,
			ChunkMaxTokens: 500,
		},
	}
	cfg.Hierarchy = DefaultHierarchyConfig()

	return store, sqliteStore, cfg
}

// seedFileRow inserts a file row for fileID into the SQLite store's files table
// so that AssignWingToFile has a row to UPDATE. Uses IndexChunks with a dummy
// chunk — this is the same path the production code uses after IndexMemoryDir.
func seedFileRow(t *testing.T, store *memory.SQLiteStore, fileID, content string) {
	t.Helper()
	h := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(h[:])
	chunk := memory.Chunk{Index: 0, Text: content, Hash: hash}
	if err := store.IndexChunks(context.Background(), fileID, []memory.Chunk{chunk}, hash); err != nil {
		t.Fatalf("seedFileRow(%s): %v", fileID, err)
	}
}

// memToolsTestEmbedder is a deterministic no-op embedder for tests.
type memToolsTestEmbedder struct{}

func (e *memToolsTestEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		v := float32(len(text))
		out[i] = []float32{v * 0.1, v * 0.2, v * 0.3, v * 0.4}
	}
	return out, nil
}
func (e *memToolsTestEmbedder) Dimensions() int { return 4 }
func (e *memToolsTestEmbedder) Name() string    { return "test" }
func (e *memToolsTestEmbedder) Model() string   { return "test-model" }

// fileWingEquals returns true when the wing registered under `expected` has
// MemoryCount > 0 — meaning at least one files row has wing=expected.
// Reliable for tests where exactly one file row exists (MEMORY.md).
func fileWingEquals(t *testing.T, store *memory.SQLiteStore, expected string) bool {
	t.Helper()
	wings, err := store.ListWings()
	if err != nil {
		t.Fatalf("ListWings: %v", err)
	}
	for _, w := range wings {
		if w.Name == expected && w.MemoryCount > 0 {
			return true
		}
	}
	return false
}

// fileWingIsNull returns true when the files table has at least one row with
// wing IS NULL (legacy file). In tests with exactly one file row this is the
// correct check for "wing was not assigned".
func fileWingIsNull(t *testing.T, store *memory.SQLiteStore) bool {
	t.Helper()
	n, err := store.TotalLegacyFiles()
	if err != nil {
		t.Fatalf("TotalLegacyFiles: %v", err)
	}
	return n > 0
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests: handleMemorySave wing assignment via args["wing"]
// ─────────────────────────────────────────────────────────────────────────────

// TestHandleMemorySaveAssignsWingFromArg verifies that args["wing"]="alpha"
// causes files.wing='alpha' after the save.
// The file row is seeded via IndexChunks before the save so AssignWingToFile
// has a row to UPDATE (in production, IndexMemoryDir creates this row).
func TestHandleMemorySaveAssignsWingFromArg(t *testing.T) {
	store, sqliteStore, cfg := testMemToolsSetup(t)
	ctx := context.Background()

	// Pre-register the wing and seed the file row (simulates post-index state).
	if err := sqliteStore.UpsertWing("alpha", "Alpha", ""); err != nil {
		t.Fatalf("UpsertWing: %v", err)
	}
	seedFileRow(t, sqliteStore, "MEMORY.md", "alpha test content")

	result, err := handleMemorySave(ctx, store, sqliteStore, cfg, nil, map[string]any{
		"content": "alpha test memory for wing assignment",
		"wing":    "alpha",
	})
	if err != nil {
		t.Fatalf("handleMemorySave: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if !fileWingEquals(t, sqliteStore, "alpha") {
		t.Error("expected files.wing='alpha' after save with explicit wing arg")
	}
}

// TestHandleMemorySaveEmptyWingLeavesNull verifies that args["wing"]=""
// leaves files.wing as NULL.
func TestHandleMemorySaveEmptyWingLeavesNull(t *testing.T) {
	store, sqliteStore, cfg := testMemToolsSetup(t)
	ctx := context.Background()

	// Seed a file row so there IS a row to check.
	seedFileRow(t, sqliteStore, "MEMORY.md", "gamma content no wing")

	_, err := handleMemorySave(ctx, store, sqliteStore, cfg, nil, map[string]any{
		"content": "gamma test memory — no wing",
		"wing":    "",
	})
	if err != nil {
		t.Fatalf("handleMemorySave: %v", err)
	}

	if !fileWingIsNull(t, sqliteStore) {
		t.Error("expected files.wing IS NULL when wing arg is empty string")
	}
}

// TestHandleMemorySaveNormalizesWing verifies that "  ALPHA-WING  " normalizes
// to "alpha-wing" before being stored in files.wing.
func TestHandleMemorySaveNormalizesWing(t *testing.T) {
	store, sqliteStore, cfg := testMemToolsSetup(t)
	ctx := context.Background()

	if err := sqliteStore.UpsertWing("alpha-wing", "Alpha Wing", ""); err != nil {
		t.Fatalf("UpsertWing: %v", err)
	}
	seedFileRow(t, sqliteStore, "MEMORY.md", "beta normalization content")

	_, err := handleMemorySave(ctx, store, sqliteStore, cfg, nil, map[string]any{
		"content": "beta memory for normalization test",
		"wing":    "  ALPHA-WING  ",
	})
	if err != nil {
		t.Fatalf("handleMemorySave: %v", err)
	}

	if !fileWingEquals(t, sqliteStore, "alpha-wing") {
		t.Error("expected files.wing='alpha-wing' after normalizing '  ALPHA-WING  '")
	}
}

// TestHandleMemorySaveInvalidWingLeavesNull verifies that args["wing"]="__system"
// (reserved prefix) is rejected by NormalizeWing and files.wing stays NULL.
func TestHandleMemorySaveInvalidWingLeavesNull(t *testing.T) {
	store, sqliteStore, cfg := testMemToolsSetup(t)
	ctx := context.Background()

	seedFileRow(t, sqliteStore, "MEMORY.md", "gamma reserved wing content")

	_, err := handleMemorySave(ctx, store, sqliteStore, cfg, nil, map[string]any{
		"content": "gamma memory with reserved wing name",
		"wing":    "__system",
	})
	if err != nil {
		t.Fatalf("handleMemorySave: %v", err)
	}

	if !fileWingIsNull(t, sqliteStore) {
		t.Error("expected files.wing IS NULL when wing arg starts with reserved prefix '__'")
	}
}

// TestHandleMemorySaveNoSQLiteNoOp verifies that sqliteStore=nil causes
// wing assignment to be skipped gracefully — no panic, no error, file is saved.
func TestHandleMemorySaveNoSQLiteNoOp(t *testing.T) {
	store, _, cfg := testMemToolsSetup(t)
	ctx := context.Background()

	result, err := handleMemorySave(ctx, store, nil, cfg, nil, map[string]any{
		"content": "alpha memory with nil sqlite store",
		"wing":    "alpha",
	})
	if err != nil {
		t.Fatalf("handleMemorySave with nil sqliteStore: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result even with nil sqliteStore")
	}

	// Verify the file was written to disk.
	memDir := filepath.Join(filepath.Dir(cfg.Path), "memory")
	data, err := os.ReadFile(filepath.Join(memDir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("MEMORY.md not written: %v", err)
	}
	if len(data) == 0 {
		t.Error("MEMORY.md is empty after save")
	}
}

// TestHandleMemorySaveHierarchyDisabledLeavesNull verifies that when
// cfg.Hierarchy.Enabled=false, files.wing stays NULL regardless of args["wing"].
func TestHandleMemorySaveHierarchyDisabledLeavesNull(t *testing.T) {
	store, sqliteStore, cfg := testMemToolsSetup(t)
	cfg.Hierarchy.Enabled = false
	ctx := context.Background()

	seedFileRow(t, sqliteStore, "MEMORY.md", "gamma hierarchy disabled content")

	_, err := handleMemorySave(ctx, store, sqliteStore, cfg, nil, map[string]any{
		"content": "gamma memory with hierarchy disabled",
		"wing":    "alpha",
	})
	if err != nil {
		t.Fatalf("handleMemorySave: %v", err)
	}

	if !fileWingIsNull(t, sqliteStore) {
		t.Error("expected files.wing IS NULL when Hierarchy.Enabled=false")
	}
}

// TestHandleMemorySaveRouterFallback verifies that when no explicit wing arg
// is provided but a ContextRouter is configured with a mapped (channel,chatID),
// the router resolves the wing from the delivery target in ctx.
func TestHandleMemorySaveRouterFallback(t *testing.T) {
	store, sqliteStore, cfg := testMemToolsSetup(t)

	// Map (telegram, "111") → "beta" in the store.
	if err := sqliteStore.SetChannelWing("telegram", "111", "beta", "manual", 1.0); err != nil {
		t.Fatalf("SetChannelWing: %v", err)
	}
	if err := sqliteStore.UpsertWing("beta", "Beta", ""); err != nil {
		t.Fatalf("UpsertWing: %v", err)
	}
	seedFileRow(t, sqliteStore, "MEMORY.md", "beta router fallback content")

	router := NewContextRouter(sqliteStore, slog.Default(), cfg.Hierarchy)

	// Simulate a telegram session via DeliveryTarget in context.
	ctx := ContextWithDelivery(context.Background(), "telegram", "111")

	// No explicit wing in args — router should resolve from delivery target.
	_, err := handleMemorySave(ctx, store, sqliteStore, cfg, router, map[string]any{
		"content": "beta session memory from telegram",
	})
	if err != nil {
		t.Fatalf("handleMemorySave: %v", err)
	}

	if !fileWingEquals(t, sqliteStore, "beta") {
		t.Error("expected files.wing='beta' from router resolution via delivery target")
	}
}

// TestHandleMemorySaveClassifierRaceIsSafe verifies the race-safety invariant:
// memory_save sets wing="alpha"; a subsequent RunLegacyClassificationPass
// skips the already-classified file (WHERE wing IS NULL is the race barrier).
func TestHandleMemorySaveClassifierRaceIsSafe(t *testing.T) {
	store, sqliteStore, cfg := testMemToolsSetup(t)
	ctx := context.Background()

	for _, w := range []string{"alpha", "beta"} {
		if err := sqliteStore.UpsertWing(w, w, ""); err != nil {
			t.Fatalf("UpsertWing(%s): %v", w, err)
		}
	}
	seedFileRow(t, sqliteStore, "MEMORY.md", "alpha sprint retro standup content")

	// memory_save sets wing="alpha" first.
	_, err := handleMemorySave(ctx, store, sqliteStore, cfg, nil, map[string]any{
		"content": "alpha sprint retro and standup",
		"wing":    "alpha",
	})
	if err != nil {
		t.Fatalf("handleMemorySave: %v", err)
	}

	if !fileWingEquals(t, sqliteStore, "alpha") {
		t.Fatal("precondition: expected files.wing='alpha' after save")
	}

	// Now run classifier with keywords that would match "beta" (overlapping).
	// The classifier should skip MEMORY.md since wing IS NOT NULL.
	classCfg := memory.LegacyClassificationConfig{
		Keywords: map[string][]string{
			"alpha": {"sprint", "standup"},
			"beta":  {"sprint", "standup"},
		},
		BatchSize: 100,
		DryRun:    false,
	}
	stats, err := sqliteStore.RunLegacyClassificationPass(ctx, classCfg)
	if err != nil {
		t.Fatalf("RunLegacyClassificationPass: %v", err)
	}

	// MEMORY.md has wing='alpha' so it must not be re-classified.
	if stats.Classified > 0 {
		t.Errorf("classifier should skip already-winged file, got Classified=%d", stats.Classified)
	}

	// Wing must still be "alpha", not "beta".
	if !fileWingEquals(t, sqliteStore, "alpha") {
		t.Error("classifier overwrote wing='alpha' — WHERE wing IS NULL race barrier failed")
	}
	if fileWingEquals(t, sqliteStore, "beta") {
		t.Error("unexpected wing='beta' after classifier pass on already-winged file")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// NormalizeWing contract (pure, no I/O)
// ─────────────────────────────────────────────────────────────────────────────

func TestNormalizeWingContract(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		expected string
	}{
		{"alpha", "alpha"},
		{"  ALPHA  ", "alpha"},
		{"alpha-wing", "alpha-wing"},
		{"  ALPHA-WING  ", "alpha-wing"},
		{"", ""},
		{"__system", ""},
		{"__reserved", ""},
		{"🎉🎉", ""},
	}
	for _, tc := range cases {
		got := memory.NormalizeWing(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeWing(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ---------- Credential Redaction Tests ----------

func TestRedactCredentials_Password(t *testing.T) {
	cases := []struct {
		input    string
		contains string
		absent   string
	}{
		{"senha: example123!", "[REDACTED — use vault]", "example123!"},
		{"password: hunter2", "[REDACTED — use vault]", "hunter2"},
		{"Password: SuperSecret123", "[REDACTED — use vault]", "SuperSecret123"},
		{"no credential here", "no credential here", "REDACTED"},
	}
	for _, tc := range cases {
		got := RedactCredentials(tc.input)
		if !strings.Contains(got, tc.contains) {
			t.Errorf("RedactCredentials(%q) = %q, want to contain %q", tc.input, got, tc.contains)
		}
		if tc.absent != "" && strings.Contains(got, tc.absent) {
			t.Errorf("RedactCredentials(%q) = %q, should NOT contain %q", tc.input, got, tc.absent)
		}
	}
}

func TestRedactCredentials_APIKey(t *testing.T) {
	cases := []struct {
		input    string
		contains string
		absent   string
	}{
		{"api_key: sk-abc123def456", "[REDACTED — use vault]", "sk-abc123def456"},
		{"secret_key: mysecret", "[REDACTED — use vault]", "mysecret"},
		{"access_token: tok_123456", "[REDACTED — use vault]", "tok_123456"},
		{"ghp_ABCDEFghijklmnopqrstuvwxyz1234567890", "[REDACTED — use vault]", "ghp_ABCDEF"},
		{"sk-abcdefghijklmnopqrstuvwxyz12345678", "[REDACTED — use vault]", "sk-abcdef"},
	}
	for _, tc := range cases {
		got := RedactCredentials(tc.input)
		if !strings.Contains(got, tc.contains) {
			t.Errorf("RedactCredentials(%q) = %q, want to contain %q", tc.input, got, tc.contains)
		}
		if tc.absent != "" && strings.Contains(got, tc.absent) {
			t.Errorf("RedactCredentials(%q) = %q, should NOT contain %q", tc.input, got, tc.absent)
		}
	}
}

func TestLooksLikeCredential(t *testing.T) {
	positives := []string{
		"senha: minhasenha123",
		"password: hunter2",
		"api_key: abc123",
		"ghp_ABCDEFghijklmnopqrstuvwxyz1234567890",
		"sk-abcdefghijklmnopqrstuvwxyz1234567",
		"bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
	}
	for _, p := range positives {
		if !LooksLikeCredential(p) {
			t.Errorf("LooksLikeCredential(%q) = false, want true", p)
		}
	}

	negatives := []string{
		"gostei do treino de hoje",
		"a reunião é às 15h",
		"prefere café sem açúcar",
	}
	for _, n := range negatives {
		if LooksLikeCredential(n) {
			t.Errorf("LooksLikeCredential(%q) = true, want false", n)
		}
	}
}

// ---------- Deduplication Tests ----------

func TestMemorySave_ExactDuplicate(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	cfg := MemoryConfig{}
	content := "user prefers dark mode"

	// First save succeeds.
	res1, err := handleMemorySave(context.Background(), store, nil, cfg, nil, map[string]any{
		"content": content,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res1.(string), "Saved to memory") {
		t.Errorf("first save should succeed, got: %v", res1)
	}

	// Second save of identical content should be skipped.
	res2, err := handleMemorySave(context.Background(), store, nil, cfg, nil, map[string]any{
		"content": content,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res2.(string), "duplicate skipped") {
		t.Errorf("duplicate save should be skipped, got: %v", res2)
	}
}

func TestMemorySave_FuzzyDuplicate(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	cfg := MemoryConfig{}

	// Save original.
	_, err = handleMemorySave(context.Background(), store, nil, cfg, nil, map[string]any{
		"content": "treino A-D às 6h da manhã segunda quarta sexta",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Near-duplicate (very similar tokens).
	res, err := handleMemorySave(context.Background(), store, nil, cfg, nil, map[string]any{
		"content": "treino A-D às 6h manhã segunda quarta sexta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.(string), "Similar memory already exists") {
		t.Errorf("near-duplicate should be detected, got: %v", res)
	}
}

func TestMemorySave_DifferentContentSaves(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	cfg := MemoryConfig{}

	_, err = handleMemorySave(context.Background(), store, nil, cfg, nil, map[string]any{
		"content": "user prefers dark mode",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Different content should save fine.
	res, err := handleMemorySave(context.Background(), store, nil, cfg, nil, map[string]any{
		"content": "user lives in Maceió",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.(string), "Saved to memory") {
		t.Errorf("different content should save, got: %v", res)
	}
}

func TestJaccardSimilarity(t *testing.T) {
	cases := []struct {
		a, b string
		min  float64
		max  float64
	}{
		{"hello world", "hello world", 1.0, 1.0},
		{"hello world", "goodbye moon", 0.0, 0.01},
		{"treino A-D 6h manhã", "treino A-D às 6h da manhã", 0.5, 0.9},
		{"", "", 1.0, 1.0},
		{"hello", "", 0.0, 0.0},
	}
	for _, tc := range cases {
		a := tokenize(tc.a)
		b := tokenize(tc.b)
		sim := jaccardSimilarity(a, b)
		if sim < tc.min || sim > tc.max {
			t.Errorf("jaccard(%q, %q) = %.3f, want [%.2f, %.2f]", tc.a, tc.b, sim, tc.min, tc.max)
		}
	}
}
