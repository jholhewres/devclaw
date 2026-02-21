// Package copilot – memory_indexer.go provides background memory indexing.
package copilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MemoryIndexer performs incremental indexing of memory files in the background.
type MemoryIndexer struct {
	interval   time.Duration
	memoryDir  string
	logger     *slog.Logger
	sqliteMem  SQLiteMemoryStore // Interface for SQLite memory operations

	// Hash tracking for incremental updates
	hashesMu sync.RWMutex
	hashes   map[string]string // filepath -> content hash

	// Stats
	indexedTotal  int64
	indexedLast   int64
	deletedTotal  int64
	lastIndexTime time.Time

	// Callbacks for indexing
	indexChunkFunc func(chunks []MemoryChunk) error
	deleteFileFunc func(filepath string) error

	ctx    context.Context
	cancel context.CancelFunc
}

// MemoryChunk represents a chunk of memory content for indexing.
type MemoryChunk struct {
	Filepath  string
	Content   string
	Hash      string
	CreatedAt time.Time
}

// SQLiteMemoryStore is an interface for SQLite memory operations.
type SQLiteMemoryStore interface {
	IndexChunks(chunks []MemoryChunk) error
	DeleteByFilepath(filepath string) error
	GetIndexedFiles() (map[string]string, error) // filepath -> hash
}

// MemoryIndexerConfig configures the memory indexer.
type MemoryIndexerConfig struct {
	Enabled   bool          `yaml:"enabled" json:"enabled"`
	Interval  time.Duration `yaml:"interval" json:"interval"`
	MemoryDir string        `yaml:"memory_dir" json:"memory_dir"`
}

// DefaultMemoryIndexerConfig returns default configuration.
func DefaultMemoryIndexerConfig() MemoryIndexerConfig {
	return MemoryIndexerConfig{
		Enabled:   true,
		Interval:  5 * time.Minute,
		MemoryDir: "",
	}
}

// NewMemoryIndexer creates a new memory indexer.
func NewMemoryIndexer(cfg MemoryIndexerConfig, logger *slog.Logger) *MemoryIndexer {
	if logger == nil {
		logger = slog.Default()
	}

	interval := cfg.Interval
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	return &MemoryIndexer{
		interval:  interval,
		memoryDir: cfg.MemoryDir,
		logger:    logger.With("component", "memory-indexer"),
		hashes:    make(map[string]string),
	}
}

// SetSQLiteStore sets the SQLite memory store for indexing.
func (m *MemoryIndexer) SetSQLiteStore(store SQLiteMemoryStore) {
	m.sqliteMem = store
}

// SetIndexChunkFunc sets the function for indexing chunks.
func (m *MemoryIndexer) SetIndexChunkFunc(fn func(chunks []MemoryChunk) error) {
	m.indexChunkFunc = fn
}

// SetDeleteFileFunc sets the function for deleting file from index.
func (m *MemoryIndexer) SetDeleteFileFunc(fn func(filepath string) error) {
	m.deleteFileFunc = fn
}

// SetMemoryDir sets the memory directory to index.
func (m *MemoryIndexer) SetMemoryDir(dir string) {
	m.memoryDir = dir
}

// Start begins periodic memory indexing.
func (m *MemoryIndexer) Start(ctx context.Context) error {
	if m.memoryDir == "" {
		m.logger.Debug("memory indexer disabled - no memory directory configured")
		return nil
	}

	// Check if memory directory exists
	if _, err := os.Stat(m.memoryDir); os.IsNotExist(err) {
		m.logger.Debug("memory indexer disabled - memory directory does not exist", "dir", m.memoryDir)
		return nil
	}

	if m.indexChunkFunc == nil {
		m.logger.Debug("memory indexer disabled - no index function configured")
		return nil
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	// Load existing hashes from SQLite on startup
	if m.sqliteMem != nil {
		existing, err := m.sqliteMem.GetIndexedFiles()
		if err != nil {
			m.logger.Warn("failed to load existing indexed files", "error", err)
		} else {
			m.hashesMu.Lock()
			for fp, hash := range existing {
				m.hashes[fp] = hash
			}
			m.hashesMu.Unlock()
			m.logger.Info("loaded existing indexed files", "count", len(existing))
		}
	}

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	m.logger.Info("memory indexer started",
		"interval", m.interval.String(),
		"memory_dir", m.memoryDir,
	)

	// Initial index
	m.indexAll()

	for {
		select {
		case <-ticker.C:
			m.indexAll()
		case <-m.ctx.Done():
			m.logger.Info("memory indexer stopped")
			return m.ctx.Err()
		}
	}
}

// Stop stops the memory indexer.
func (m *MemoryIndexer) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// indexAll performs a full incremental index.
func (m *MemoryIndexer) indexAll() {
	start := time.Now()
	m.logger.Debug("starting incremental memory index")

	// Track which files we've seen
	seen := make(map[string]bool)

	// Walk memory directory
	var indexed, deleted, errors int

	err := filepath.WalkDir(m.memoryDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Only process markdown files
		if filepath.Ext(path) != ".md" {
			return nil
		}

		// Mark as seen
		seen[path] = true

		// Check if file needs reindexing
		needsIndex, err := m.needsReindex(path)
		if err != nil {
			m.logger.Warn("failed to check file", "path", path, "error", err)
			errors++
			return nil
		}

		if !needsIndex {
			return nil
		}

		// Index the file
		if err := m.indexFile(path); err != nil {
			m.logger.Warn("failed to index file", "path", path, "error", err)
			errors++
			return nil
		}

		indexed++
		return nil
	})

	if err != nil {
		m.logger.Warn("memory index walk failed", "error", err)
	}

	// Check for deleted files
	// Collect files to delete first to avoid holding lock during deletion
	m.hashesMu.RLock()
	var toDelete []string
	for fp := range m.hashes {
		if !seen[fp] {
			toDelete = append(toDelete, fp)
		}
	}
	m.hashesMu.RUnlock()

	// Delete files from index
	for _, fp := range toDelete {
		if err := m.deleteFromIndex(fp); err != nil {
			m.logger.Warn("failed to delete from index", "path", fp, "error", err)
			errors++
		} else {
			deleted++
		}
	}

	// Update stats
	m.indexedLast = int64(indexed)
	m.indexedTotal += int64(indexed)
	m.deletedTotal += int64(deleted)
	m.lastIndexTime = time.Now()

	duration := time.Since(start)
	m.logger.Info("memory index complete",
		"indexed", indexed,
		"deleted", deleted,
		"errors", errors,
		"duration", duration.String(),
	)
}

// needsReindex checks if a file needs to be reindexed based on content hash.
func (m *MemoryIndexer) needsReindex(path string) (bool, error) {
	// Read file content
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	// Compute hash
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	// Check against stored hash
	m.hashesMu.RLock()
	storedHash, exists := m.hashes[path]
	m.hashesMu.RUnlock()

	if !exists || storedHash != hashStr {
		return true, nil
	}

	return false, nil
}

// indexFile indexes a single file.
func (m *MemoryIndexer) indexFile(path string) error {
	// Read file content
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Compute hash
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	// Create chunk
	chunk := MemoryChunk{
		Filepath:  path,
		Content:   string(content),
		Hash:      hashStr,
		CreatedAt: time.Now(),
	}

	// Index via callback
	if m.indexChunkFunc != nil {
		if err := m.indexChunkFunc([]MemoryChunk{chunk}); err != nil {
			return err
		}
	} else if m.sqliteMem != nil {
		if err := m.sqliteMem.IndexChunks([]MemoryChunk{chunk}); err != nil {
			return err
		}
	}

	// Update stored hash
	m.hashesMu.Lock()
	m.hashes[path] = hashStr
	m.hashesMu.Unlock()

	return nil
}

// deleteFromIndex removes a file from the index.
func (m *MemoryIndexer) deleteFromIndex(path string) error {
	// Delete via callback
	if m.deleteFileFunc != nil {
		if err := m.deleteFileFunc(path); err != nil {
			return err
		}
	} else if m.sqliteMem != nil {
		if err := m.sqliteMem.DeleteByFilepath(path); err != nil {
			return err
		}
	}

	// Remove stored hash
	m.hashesMu.Lock()
	delete(m.hashes, path)
	m.hashesMu.Unlock()

	return nil
}

// IndexNow triggers an immediate index (useful for manual triggers).
func (m *MemoryIndexer) IndexNow() {
	m.indexAll()
}

// Stats returns current indexer statistics.
func (m *MemoryIndexer) Stats() (indexedTotal, indexedLast, deletedTotal int64, lastIndexTime time.Time) {
	return m.indexedTotal, m.indexedLast, m.deletedTotal, m.lastIndexTime
}

// ForceReindex clears all stored hashes and triggers a full reindex.
func (m *MemoryIndexer) ForceReindex() {
	m.hashesMu.Lock()
	m.hashes = make(map[string]string)
	m.hashesMu.Unlock()

	m.logger.Info("forcing full memory reindex")
	m.indexAll()
}
