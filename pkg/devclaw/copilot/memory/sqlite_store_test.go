package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
)

// deterministicEmbedder returns fixed-dimension embeddings derived from text length.
// This makes tests deterministic without needing an external API.
type deterministicEmbedder struct{}

func (d *deterministicEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		// 4-dim embedding based on text chars for test determinism.
		v := float32(len(text))
		result[i] = []float32{v * 0.1, v * 0.2, v * 0.3, v * 0.4}
	}
	return result, nil
}

func (d *deterministicEmbedder) Dimensions() int { return 4 }
func (d *deterministicEmbedder) Name() string    { return "test" }
func (d *deterministicEmbedder) Model() string   { return "test-model" }

func hashChunk(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewSQLiteStore(tmpFile.Name(), &deterministicEmbedder{}, slog.Default())
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestIncrementalVectorCache(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Index file A with 3 chunks.
	chunksA := []Chunk{
		{Index: 0, Text: "alpha one", Hash: hashChunk("alpha one")},
		{Index: 1, Text: "alpha two", Hash: hashChunk("alpha two")},
		{Index: 2, Text: "alpha three", Hash: hashChunk("alpha three")},
	}
	if err := store.IndexChunks(ctx, "fileA", chunksA, "hashA1"); err != nil {
		t.Fatalf("index A: %v", err)
	}

	// Index file B with 2 chunks.
	chunksB := []Chunk{
		{Index: 0, Text: "beta one", Hash: hashChunk("beta one")},
		{Index: 1, Text: "beta two", Hash: hashChunk("beta two")},
	}
	if err := store.IndexChunks(ctx, "fileB", chunksB, "hashB1"); err != nil {
		t.Fatalf("index B: %v", err)
	}

	// Cache should have 5 entries (3 + 2).
	store.vectorCacheMu.RLock()
	count := len(store.vectorCacheByID)
	store.vectorCacheMu.RUnlock()
	if count != 5 {
		t.Errorf("cache count after A+B = %d, want 5", count)
	}

	// Re-index file A with 2 chunks (one changed).
	chunksA2 := []Chunk{
		{Index: 0, Text: "alpha one", Hash: hashChunk("alpha one")},
		{Index: 1, Text: "alpha updated", Hash: hashChunk("alpha updated")},
	}
	if err := store.IndexChunks(ctx, "fileA", chunksA2, "hashA2"); err != nil {
		t.Fatalf("re-index A: %v", err)
	}

	// Cache should have 4 entries (2 from A + 2 from B).
	store.vectorCacheMu.RLock()
	count = len(store.vectorCacheByID)
	fileAIDs := len(store.vectorCacheByFile["fileA"])
	fileBIDs := len(store.vectorCacheByFile["fileB"])
	store.vectorCacheMu.RUnlock()

	if count != 4 {
		t.Errorf("cache count after re-index = %d, want 4", count)
	}
	if fileAIDs != 2 {
		t.Errorf("fileA cache entries = %d, want 2", fileAIDs)
	}
	if fileBIDs != 2 {
		t.Errorf("fileB cache entries = %d, want 2", fileBIDs)
	}
}

func TestSearchVectorAfterIncrementalUpdate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Index file with initial content.
	chunks := []Chunk{
		{Index: 0, Text: "old content here", Hash: hashChunk("old content here")},
	}
	if err := store.IndexChunks(ctx, "file1", chunks, "hash1"); err != nil {
		t.Fatalf("index: %v", err)
	}

	// Re-index with new content.
	chunks2 := []Chunk{
		{Index: 0, Text: "new content here updated", Hash: hashChunk("new content here updated")},
	}
	if err := store.IndexChunks(ctx, "file1", chunks2, "hash2"); err != nil {
		t.Fatalf("re-index: %v", err)
	}

	// Search should find the new content.
	results, err := store.SearchVector(ctx, "new content here updated", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one search result")
	}

	// The top result should be the new content, not the old one.
	if results[0].Text != "new content here updated" {
		t.Errorf("top result = %q, want %q", results[0].Text, "new content here updated")
	}

	// Old content should not appear.
	for _, r := range results {
		if r.Text == "old content here" {
			t.Error("old content should not appear in results after re-index")
		}
	}
}

func TestSearchVectorOnFreshStore(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Search on a completely empty store should return nil, not panic.
	results, err := store.SearchVector(ctx, "anything", 10)
	if err != nil {
		t.Fatalf("search on fresh store: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on fresh store, got %d", len(results))
	}
}

func TestConcurrentIndexAndSearch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const numFiles = 10

	// Index all files first (SQLite serializes writes).
	for i := 0; i < numFiles; i++ {
		fileID := fmt.Sprintf("file-%d", i)
		text := fmt.Sprintf("content for file number %d with some extra text", i)
		chunks := []Chunk{
			{Index: 0, Text: text, Hash: hashChunk(text)},
		}
		if err := store.IndexChunks(ctx, fileID, chunks, fmt.Sprintf("hash-%d", i)); err != nil {
			t.Fatalf("index %s: %v", fileID, err)
		}
	}

	// Now do concurrent reads + one writer to test cache safety under -race.
	var wg sync.WaitGroup

	// Concurrent searching.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.SearchVector(ctx, "content for file", 5)
			if err != nil {
				t.Errorf("search: %v", err)
			}
		}()
	}

	// Concurrent re-index of one file while searches run.
	wg.Add(1)
	go func() {
		defer wg.Done()
		text := "updated content for concurrent test"
		chunks := []Chunk{
			{Index: 0, Text: text, Hash: hashChunk(text)},
		}
		if err := store.IndexChunks(ctx, "file-0", chunks, "hash-updated"); err != nil {
			t.Errorf("re-index: %v", err)
		}
	}()

	wg.Wait()

	// Verify cache consistency.
	store.vectorCacheMu.RLock()
	cacheCount := len(store.vectorCacheByID)
	fileCount := len(store.vectorCacheByFile)
	store.vectorCacheMu.RUnlock()

	if cacheCount != numFiles {
		t.Errorf("cache count = %d, want %d", cacheCount, numFiles)
	}
	if fileCount != numFiles {
		t.Errorf("file count in cache = %d, want %d", fileCount, numFiles)
	}
}
