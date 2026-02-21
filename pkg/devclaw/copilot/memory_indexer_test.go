// Package copilot – memory_indexer_test.go tests the memory indexer.
package copilot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryIndexer_New(t *testing.T) {
	cfg := MemoryIndexerConfig{
		Enabled:  true,
		Interval: 5 * time.Minute,
	}
	mi := NewMemoryIndexer(cfg, nil)

	if mi == nil {
		t.Fatal("NewMemoryIndexer returned nil")
	}
	if mi.interval != 5*time.Minute {
		t.Errorf("Expected interval 5m, got %v", mi.interval)
	}
}

func TestMemoryIndexer_DefaultConfig(t *testing.T) {
	cfg := DefaultMemoryIndexerConfig()

	if !cfg.Enabled {
		t.Error("Default config should have Enabled = true")
	}
	if cfg.Interval != 5*time.Minute {
		t.Errorf("Default interval should be 5m, got %v", cfg.Interval)
	}
}

func TestMemoryIndexer_Start_NoDir(t *testing.T) {
	cfg := MemoryIndexerConfig{
		Enabled:   true,
		Interval:  1 * time.Second,
		MemoryDir: "",
	}
	mi := NewMemoryIndexer(cfg, nil)

	ctx := context.Background()
	err := mi.Start(ctx)

	// Should return nil immediately when no directory configured
	if err != nil {
		t.Errorf("Expected nil error for no directory, got %v", err)
	}
}

func TestMemoryIndexer_Start_NoCallback(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	cfg := MemoryIndexerConfig{
		Enabled:   true,
		Interval:  1 * time.Second,
		MemoryDir: tmpDir,
	}
	mi := NewMemoryIndexer(cfg, nil)
	// Don't set indexChunkFunc

	ctx := context.Background()
	err := mi.Start(ctx)

	// Should return nil immediately when no callback configured
	if err != nil {
		t.Errorf("Expected nil error for no callback, got %v", err)
	}
}

func TestMemoryIndexer_IndexFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.md")
	err := os.WriteFile(testFile, []byte("# Test\n\nContent here"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg := MemoryIndexerConfig{
		Enabled:   true,
		Interval:  1 * time.Second,
		MemoryDir: tmpDir,
	}
	mi := NewMemoryIndexer(cfg, nil)

	var indexedChunks []MemoryChunk
	mi.SetIndexChunkFunc(func(chunks []MemoryChunk) error {
		indexedChunks = append(indexedChunks, chunks...)
		return nil
	})

	// Index the file
	err = mi.indexFile(testFile)
	if err != nil {
		t.Fatalf("indexFile failed: %v", err)
	}

	if len(indexedChunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(indexedChunks))
	}

	chunk := indexedChunks[0]
	if chunk.Filepath != testFile {
		t.Errorf("Expected filepath %s, got %s", testFile, chunk.Filepath)
	}
	if chunk.Content != "# Test\n\nContent here" {
		t.Errorf("Unexpected content: %s", chunk.Content)
	}
	if chunk.Hash == "" {
		t.Error("Hash should not be empty")
	}

	// Check hash was stored
	mi.hashesMu.RLock()
	storedHash, exists := mi.hashes[testFile]
	mi.hashesMu.RUnlock()

	if !exists {
		t.Error("Hash should be stored")
	}
	if storedHash != chunk.Hash {
		t.Errorf("Stored hash mismatch")
	}
}

func TestMemoryIndexer_NeedsReindex(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.md")
	err := os.WriteFile(testFile, []byte("original content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg := MemoryIndexerConfig{
		Enabled:   true,
		Interval:  1 * time.Second,
		MemoryDir: tmpDir,
	}
	mi := NewMemoryIndexer(cfg, nil)

	// First check - should need index
	needsIndex, err := mi.needsReindex(testFile)
	if err != nil {
		t.Fatalf("needsReindex failed: %v", err)
	}
	if !needsIndex {
		t.Error("New file should need indexing")
	}

	// Index the file (stores hash)
	mi.SetIndexChunkFunc(func(chunks []MemoryChunk) error { return nil })
	mi.indexFile(testFile)

	// Second check - should NOT need index (same content)
	needsIndex, err = mi.needsReindex(testFile)
	if err != nil {
		t.Fatalf("needsReindex failed: %v", err)
	}
	if needsIndex {
		t.Error("Unchanged file should NOT need reindexing")
	}

	// Modify file
	err = os.WriteFile(testFile, []byte("modified content"), 0644)
	if err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Third check - should need index (changed content)
	needsIndex, err = mi.needsReindex(testFile)
	if err != nil {
		t.Fatalf("needsReindex failed: %v", err)
	}
	if !needsIndex {
		t.Error("Modified file should need reindexing")
	}
}

func TestMemoryIndexer_IndexAll(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	file1 := filepath.Join(tmpDir, "file1.md")
	file2 := filepath.Join(tmpDir, "file2.md")
	os.WriteFile(file1, []byte("content 1"), 0644)
	os.WriteFile(file2, []byte("content 2"), 0644)

	// Create non-markdown file (should be ignored)
	os.WriteFile(filepath.Join(tmpDir, "ignore.txt"), []byte("ignore"), 0644)

	cfg := MemoryIndexerConfig{
		Enabled:   true,
		Interval:  1 * time.Second,
		MemoryDir: tmpDir,
	}
	mi := NewMemoryIndexer(cfg, nil)

	var indexedFiles []string
	mi.SetIndexChunkFunc(func(chunks []MemoryChunk) error {
		for _, c := range chunks {
			indexedFiles = append(indexedFiles, c.Filepath)
		}
		return nil
	})

	// Index all
	mi.indexAll()

	// Should have indexed 2 markdown files
	if len(indexedFiles) != 2 {
		t.Errorf("Expected 2 files indexed, got %d", len(indexedFiles))
	}

	// Check stats
	total, last, _, _ := mi.Stats()
	if total != 2 {
		t.Errorf("Expected total 2, got %d", total)
	}
	if last != 2 {
		t.Errorf("Expected last 2, got %d", last)
	}
}

func TestMemoryIndexer_DeleteFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	file1 := filepath.Join(tmpDir, "file1.md")
	os.WriteFile(file1, []byte("content"), 0644)

	cfg := MemoryIndexerConfig{
		Enabled:   true,
		Interval:  1 * time.Second,
		MemoryDir: tmpDir,
	}
	mi := NewMemoryIndexer(cfg, nil)

	var deletedFiles []string
	mi.SetIndexChunkFunc(func(chunks []MemoryChunk) error { return nil })
	mi.SetDeleteFileFunc(func(filepath string) error {
		deletedFiles = append(deletedFiles, filepath)
		return nil
	})

	// Index initial file
	mi.indexAll()

	// Delete the file
	os.Remove(file1)

	// Re-index - should detect deletion
	mi.indexAll()

	// Should have deleted the file from index
	if len(deletedFiles) != 1 {
		t.Errorf("Expected 1 file deleted, got %d", len(deletedFiles))
	}
	if deletedFiles[0] != file1 {
		t.Errorf("Expected %s to be deleted, got %s", file1, deletedFiles[0])
	}

	// Check stats
	_, _, deleted, _ := mi.Stats()
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}
}

func TestMemoryIndexer_ForceReindex(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	file1 := filepath.Join(tmpDir, "file1.md")
	os.WriteFile(file1, []byte("content"), 0644)

	cfg := MemoryIndexerConfig{
		Enabled:   true,
		Interval:  1 * time.Second,
		MemoryDir: tmpDir,
	}
	mi := NewMemoryIndexer(cfg, nil)

	indexCount := 0
	mi.SetIndexChunkFunc(func(chunks []MemoryChunk) error {
		indexCount += len(chunks)
		return nil
	})

	// Initial index
	mi.indexAll()
	if indexCount != 1 {
		t.Errorf("Expected 1 index, got %d", indexCount)
	}

	// Second index (no changes, should skip)
	mi.indexAll()
	if indexCount != 1 {
		t.Errorf("Expected still 1 index (no changes), got %d", indexCount)
	}

	// Force reindex
	mi.ForceReindex()
	if indexCount != 2 {
		t.Errorf("Expected 2 indexes after force reindex, got %d", indexCount)
	}
}

func TestMemoryIndexer_Stop(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := MemoryIndexerConfig{
		Enabled:   true,
		Interval:  100 * time.Millisecond,
		MemoryDir: tmpDir,
	}
	mi := NewMemoryIndexer(cfg, nil)
	mi.SetIndexChunkFunc(func(chunks []MemoryChunk) error { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- mi.Start(ctx)
	}()

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop
	mi.Stop()
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for indexer to stop")
	}
}
