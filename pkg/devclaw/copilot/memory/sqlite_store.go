// Package memory – sqlite_store.go implements a SQLite-backed memory store with
// FTS5 (BM25 ranking) and in-process vector search (cosine similarity).
// Embeddings are stored as JSON-encoded float32 arrays in the chunks table.
// This avoids the need for the sqlite-vec extension while still providing
// hybrid semantic + keyword search.
package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg"
	_ "github.com/mattn/go-sqlite3" // SQLite driver with FTS5 support.
)

// SQLiteStore provides persistent memory storage with hybrid search.
type SQLiteStore struct {
	db       *sql.DB
	embedder EmbeddingProvider
	logger   *slog.Logger

	// ftsAvailable indicates whether FTS5 is available for full-text search.
	// When false, search falls back to LIKE queries (slower but functional).
	ftsAvailable bool

	// vectorCache holds all chunk embeddings in memory for fast cosine search.
	// Populated on startup, then updated incrementally per-file on index operations.
	vectorCacheMu     sync.RWMutex
	vectorCacheByID   map[int64]vectorCacheEntry // chunkID → entry
	vectorCacheByFile map[string][]int64         // fileID → []chunkID

	// kg is the lazy-initialized Knowledge Graph reference, nil when KG is
	// disabled. Inert until someone calls HybridSearchEnriched.
	kg *kg.KG

	// quantizeEnabled enables uint8 quantization of embeddings (~4x memory reduction).
	quantizeEnabled bool

	// lastQueryEmbedding stores the most recent query embedding from SearchVector.
	// Reused by TopicChangeDetector to avoid extra embedding API calls.
	lastQueryEmbMu  sync.RWMutex
	lastQueryEmb    []float32
}

// vectorCacheEntry holds a chunk embedding for in-memory vector search.
type vectorCacheEntry struct {
	chunkID   int64
	fileID    string
	text      string
	embedding []float32           // float32 path (used when quantize=false)
	quantized *QuantizedEmbedding // uint8 path (used when quantize=true, ~4x less memory)
}

// SearchResult represents a single search hit with score.
//
// The Wing field (added in Sprint 2 Room 2.0c) carries the originating
// file's palace wing, if any. It is empty for legacy (wing IS NULL) files
// and for results coming from the wing-unaware code paths. The field is
// last in the struct to preserve JSON marshalling compatibility for any
// caller that decoded SearchResult before Sprint 2.
type SearchResult struct {
	FileID string
	Text   string
	Score  float64
	Wing   string
}

// NewSQLiteStore opens or creates a SQLite memory database with FTS5.
func NewSQLiteStore(dbPath string, embedder EmbeddingProvider, logger *slog.Logger) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	store := &SQLiteStore{
		db:                db,
		embedder:          embedder,
		logger:            logger,
		vectorCacheByID:   make(map[int64]vectorCacheEntry),
		vectorCacheByFile: make(map[string][]int64),
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	// Load vector cache into memory.
	if err := store.loadVectorCache(); err != nil {
		logger.Warn("failed to load vector cache", "error", err)
	}

	return store, nil
}

// SetQuantizeEnabled enables or disables uint8 embedding quantization.
// Call before loadVectorCache (typically right after construction).
func (s *SQLiteStore) SetQuantizeEnabled(enabled bool) {
	s.quantizeEnabled = enabled
}

// LastQueryEmbedding returns a copy of the most recent query embedding from
// SearchVector. Returns a defensive copy to prevent data races if the embedding
// provider reuses buffers. Thread-safe.
func (s *SQLiteStore) LastQueryEmbedding() []float32 {
	s.lastQueryEmbMu.RLock()
	defer s.lastQueryEmbMu.RUnlock()
	if s.lastQueryEmb == nil {
		return nil
	}
	cp := make([]float32, len(s.lastQueryEmb))
	copy(cp, s.lastQueryEmb)
	return cp
}

// initSchema creates the required tables and indices.
func (s *SQLiteStore) initSchema() error {
	// Core tables — always created.
	coreSchema := `
		CREATE TABLE IF NOT EXISTS files (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id   TEXT UNIQUE NOT NULL,
			hash      TEXT NOT NULL,
			indexed_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS chunks (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id    TEXT NOT NULL,
			chunk_idx  INTEGER NOT NULL,
			text       TEXT NOT NULL,
			hash       TEXT NOT NULL,
			embedding  TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(file_id, chunk_idx)
		);

		CREATE TABLE IF NOT EXISTS embedding_cache (
			text_hash TEXT NOT NULL,
			provider  TEXT NOT NULL,
			model     TEXT NOT NULL,
			embedding TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (text_hash, provider, model)
		);
	`

	if _, err := s.db.Exec(coreSchema); err != nil {
		return err
	}

	// FTS5 full-text search — optional. Some SQLite builds don't include FTS5.
	// When unavailable, memory search falls back to LIKE queries (slower but functional).
	ftsSchema := `
		CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
			text,
			content='chunks',
			content_rowid='id',
			tokenize='porter unicode61'
		);

		CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
			INSERT INTO chunks_fts(rowid, text) VALUES (new.id, new.text);
		END;

		CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
			INSERT INTO chunks_fts(chunks_fts, rowid, text) VALUES('delete', old.id, old.text);
		END;

		CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
			INSERT INTO chunks_fts(chunks_fts, rowid, text) VALUES('delete', old.id, old.text);
			INSERT INTO chunks_fts(rowid, text) VALUES (new.id, new.text);
		END;
	`
	if _, err := s.db.Exec(ftsSchema); err != nil {
		// FTS5 not available — mark it and continue. Search will use LIKE fallback.
		s.ftsAvailable = false
		slog.Warn("FTS5 not available, falling back to LIKE search", "error", err.Error())
	} else {
		s.ftsAvailable = true
	}

	// Sprint 1 (v1.18.0) palace-aware schema additions.
	// Additive and idempotent — safe for retrocompat (see sqlite_hierarchy.go).
	// Failures are logged but not fatal; the core memory subsystem remains
	// operational even if the hierarchy schema cannot be created.
	if err := InitHierarchySchema(s.db, s.logger); err != nil {
		slog.Warn("failed to initialize palace hierarchy schema", "error", err)
		// Non-fatal: the core memory system continues to work with
		// legacy (wing=NULL) behavior.
	}

	// Sprint 2 Room 2.2 — L1 essential_stories cache table.
	// Same non-fatal policy as InitHierarchySchema: a failed migration
	// downgrades L1 to "render uncached on every call" but never blocks
	// the rest of the memory subsystem.
	if err := MigrateEssentialStories(s.db, s.logger); err != nil {
		slog.Warn("failed to migrate essential_stories cache", "error", err)
	}

	// Sprint 3 Room 3.1 — KG bitemporal schema.
	if err := MigrateKgSchema(s.db, s.logger); err != nil {
		slog.Warn("failed to migrate kg schema", "error", err)
	} else {
		if k, err := kg.NewKG(s.db, s.logger); err == nil {
			s.kg = k
		}
	}

	return nil
}

// IndexChunks indexes a set of chunks for a file. Uses delta sync: only
// re-embeds chunks whose hash has changed.
func (s *SQLiteStore) IndexChunks(ctx context.Context, fileID string, chunks []Chunk, fileHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if file is already indexed with same hash.
	var existingHash string
	err = tx.QueryRow("SELECT hash FROM files WHERE file_id = ?", fileID).Scan(&existingHash)
	if err == nil && existingHash == fileHash {
		return nil // File unchanged.
	}

	// Upsert file record.
	_, err = tx.Exec(`
		INSERT INTO files (file_id, hash, indexed_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(file_id) DO UPDATE SET hash = excluded.hash, indexed_at = CURRENT_TIMESTAMP
	`, fileID, fileHash)
	if err != nil {
		return err
	}

	// Get existing chunk hashes to identify what changed.
	existingChunks := make(map[string]string) // chunk_hash → embedding (JSON)
	rows, err := tx.Query("SELECT hash, embedding FROM chunks WHERE file_id = ?", fileID)
	if err != nil {
		if rows != nil {
			rows.Close()
		}
		// Non-fatal: treat all chunks as new.
		s.logger.Debug("could not read existing chunks", "file", fileID, "error", err)
	} else {
		for rows.Next() {
			var h string
			var emb sql.NullString
			if err := rows.Scan(&h, &emb); err == nil {
				if emb.Valid {
					existingChunks[h] = emb.String
				} else {
					existingChunks[h] = ""
				}
			}
		}
		rows.Close()
	}

	// Delete old chunks for this file.
	_, _ = tx.Exec("DELETE FROM chunks WHERE file_id = ?", fileID)

	// Find chunks that need new embeddings.
	var textsToEmbed []string
	var embedIndices []int
	for i, chunk := range chunks {
		if _, ok := existingChunks[chunk.Hash]; !ok {
			textsToEmbed = append(textsToEmbed, chunk.Text)
			embedIndices = append(embedIndices, i)
		}
	}

	// Generate embeddings for new/changed chunks.
	var newEmbeddings [][]float32
	if len(textsToEmbed) > 0 && s.embedder.Name() != "none" {
		// Check embedding cache first.
		newEmbeddings = make([][]float32, len(textsToEmbed))
		var uncachedTexts []string
		var uncachedIndices []int

		for i, text := range textsToEmbed {
			cached := s.getEmbeddingCache(text)
			if cached != nil {
				newEmbeddings[i] = cached
			} else {
				uncachedTexts = append(uncachedTexts, text)
				uncachedIndices = append(uncachedIndices, i)
			}
		}

		// Embed uncached texts.
		if len(uncachedTexts) > 0 {
			embeddings, err := s.embedder.Embed(ctx, uncachedTexts)
			if err != nil {
				s.logger.Warn("embedding generation failed, indexing without vectors", "error", err)
			} else {
				for i, emb := range embeddings {
					idx := uncachedIndices[i]
					newEmbeddings[idx] = emb
					s.setEmbeddingCache(uncachedTexts[i], emb)
				}
			}
		}
	}

	// Insert chunks.
	stmt, err := tx.Prepare(`
		INSERT INTO chunks (file_id, chunk_idx, text, hash, embedding)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Build a map from chunk index → embedding index for O(1) lookup.
	chunkToEmbed := make(map[int]int, len(embedIndices))
	for j, idx := range embedIndices {
		chunkToEmbed[idx] = j
	}

	var newCacheEntries []vectorCacheEntry
	for i, chunk := range chunks {
		var embJSON sql.NullString
		var embVec []float32

		// Try to reuse existing embedding.
		if existing, ok := existingChunks[chunk.Hash]; ok && existing != "" {
			embJSON = sql.NullString{String: existing, Valid: true}
			_ = json.Unmarshal([]byte(existing), &embVec)
		} else if newEmbeddings != nil {
			// Look up the new embedding for this chunk.
			if j, ok := chunkToEmbed[i]; ok && j < len(newEmbeddings) && newEmbeddings[j] != nil {
				data, _ := json.Marshal(newEmbeddings[j])
				embJSON = sql.NullString{String: string(data), Valid: true}
				embVec = newEmbeddings[j]
			}
		}

		result, err := stmt.Exec(fileID, chunk.Index, chunk.Text, chunk.Hash, embJSON)
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}

		if embVec != nil {
			chunkID, _ := result.LastInsertId()
			entry := vectorCacheEntry{
				chunkID:   chunkID,
				fileID:    fileID,
				text:      chunk.Text,
				embedding: embVec,
			}
			if s.quantizeEnabled {
				q := QuantizeFloat32(embVec)
				entry.quantized = &q
				entry.embedding = nil // free float32 in cache
			}
			newCacheEntries = append(newCacheEntries, entry)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Update vector cache incrementally for this file only.
	s.updateVectorCacheForFile(fileID, newCacheEntries)
	return nil
}

// SearchBM25 performs a keyword search using FTS5 BM25 ranking.
// Falls back to LIKE-based search when FTS5 is not available.
// Applies query expansion to handle conversational queries.
func (s *SQLiteStore) SearchBM25(query string, maxResults int) ([]SearchResult, error) {
	searchStart := time.Now()
	if maxResults <= 0 {
		maxResults = 10
	}

	// If FTS5 is not available, use LIKE fallback with expanded keywords.
	if !s.ftsAvailable {
		results, err := s.searchLikeFallback(query, maxResults)
		s.logger.Info("search_bm25 completed",
			"elapsed_ms", time.Since(searchStart).Milliseconds(),
			"results", len(results),
			"fts5", false,
		)
		return results, err
	}

	// Try phrase search first.
	safeQuery := sanitizeFTS5Query(query)
	if safeQuery == "" {
		return nil, nil
	}

	results, err := s.ftsQuery(safeQuery, maxResults)
	if err == nil && len(results) >= maxResults/2 {
		return results, nil
	}

	// Expand query: extract keywords and search with OR.
	keywords := extractKeywords(query)
	if len(keywords) > 0 {
		expandedQuery := expandQueryForFTS(keywords)
		if expandedQuery != "" && expandedQuery != safeQuery {
			moreResults, err := s.ftsQuery(expandedQuery, maxResults)
			if err == nil {
				results = mergeSearchResults(results, moreResults, maxResults*2)
			}
		}
	}

	s.logger.Info("search_bm25 completed",
		"elapsed_ms", time.Since(searchStart).Milliseconds(),
		"results", len(results),
		"fts5", true,
	)
	return results, nil
}

// ftsQuery runs a single FTS5 query and returns ranked results.
func (s *SQLiteStore) ftsQuery(ftsQuery string, maxResults int) ([]SearchResult, error) {
	rows, err := s.db.Query(`
		SELECT c.file_id, c.text, rank
		FROM chunks_fts
		JOIN chunks c ON c.id = chunks_fts.rowid
		WHERE chunks_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, ftsQuery, maxResults*2)
	if err != nil {
		return s.searchLikeFallback(ftsQuery, maxResults)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var rank float64
		if err := rows.Scan(&r.FileID, &r.Text, &rank); err != nil {
			continue
		}
		r.Score = 1.0 / (1.0 + math.Abs(rank))
		results = append(results, r)
	}

	return results, nil
}

// searchLikeFallback performs a simple LIKE search when FTS5 is not available.
func (s *SQLiteStore) searchLikeFallback(query string, maxResults int) ([]SearchResult, error) {
	// Split query into words and search for each with LIKE.
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil, nil
	}

	// Build a query that matches any word.
	var conditions []string
	var args []any
	for _, w := range words {
		conditions = append(conditions, "LOWER(text) LIKE ?")
		args = append(args, "%"+w+"%")
	}
	args = append(args, maxResults*2)

	sqlQuery := fmt.Sprintf(`
		SELECT file_id, text FROM chunks
		WHERE %s
		LIMIT ?
	`, strings.Join(conditions, " OR "))

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("LIKE search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.FileID, &r.Text); err != nil {
			continue
		}
		// Score based on word match count.
		text := strings.ToLower(r.Text)
		matches := 0
		for _, w := range words {
			if strings.Contains(text, w) {
				matches++
			}
		}
		r.Score = float64(matches) / float64(len(words))
		results = append(results, r)
	}

	return results, nil
}

// searchBM25WithWing is the wing-aware variant of SearchBM25. It runs the
// same BM25 ranking but JOINs against the files table so each result
// carries its files.wing value (empty string for legacy NULL rows). Used
// only by HybridSearchWithOpts when QueryWing != "" — the wing-unaware
// path stays on SearchBM25 to guarantee byte-identical scores.
//
// Falls back to fileWingFallback() when FTS5 is unavailable so the wing
// metadata is still attached even on the LIKE-based code path.
func (s *SQLiteStore) searchBM25WithWing(query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	// FTS5 not available — fall back to LIKE search and attach wings.
	if !s.ftsAvailable {
		results, err := s.searchLikeFallback(query, maxResults)
		if err != nil {
			return nil, err
		}
		s.attachWings(results)
		return results, nil
	}

	// Phrase query first.
	safeQuery := sanitizeFTS5Query(query)
	if safeQuery == "" {
		return nil, nil
	}

	results, err := s.ftsQueryWithWing(safeQuery, maxResults)
	if err == nil && len(results) >= maxResults/2 {
		return results, nil
	}

	// Expanded query: extract keywords and search with OR.
	keywords := extractKeywords(query)
	if len(keywords) > 0 {
		expandedQuery := expandQueryForFTS(keywords)
		if expandedQuery != "" && expandedQuery != safeQuery {
			moreResults, err := s.ftsQueryWithWing(expandedQuery, maxResults)
			if err == nil {
				results = mergeSearchResults(results, moreResults, maxResults*2)
			}
		}
	}

	return results, nil
}

// ftsQueryWithWing runs a single FTS5 query that JOINs against the files
// table to attach files.wing to each row. Mirrors ftsQuery but with the
// extra column. Used only by the wing-aware code path.
func (s *SQLiteStore) ftsQueryWithWing(ftsQuery string, maxResults int) ([]SearchResult, error) {
	rows, err := s.db.Query(`
		SELECT c.file_id, c.text, COALESCE(f.wing, ''), rank
		FROM chunks_fts
		JOIN chunks c ON c.id = chunks_fts.rowid
		LEFT JOIN files f ON f.file_id = c.file_id
		WHERE chunks_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, ftsQuery, maxResults*2)
	if err != nil {
		// Fall back to the LIKE path and attach wings afterwards.
		results, err := s.searchLikeFallback(ftsQuery, maxResults)
		if err != nil {
			return nil, err
		}
		s.attachWings(results)
		return results, nil
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var (
			r    SearchResult
			rank float64
		)
		if err := rows.Scan(&r.FileID, &r.Text, &r.Wing, &rank); err != nil {
			continue
		}
		r.Score = 1.0 / (1.0 + math.Abs(rank))
		results = append(results, r)
	}

	return results, nil
}

// searchVectorWithWing is the wing-aware variant of SearchVector. It
// performs the same in-memory cosine search but then attaches files.wing
// to every result via a single batched lookup. Used only by
// HybridSearchWithOpts when QueryWing != "".
func (s *SQLiteStore) searchVectorWithWing(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	results, err := s.SearchVector(ctx, query, maxResults)
	if err != nil || len(results) == 0 {
		return results, err
	}
	s.attachWings(results)
	return results, nil
}

// attachWings populates the Wing field on each result by batch-querying
// the files table for the unique fileIDs in the slice. Files with
// wing IS NULL get the empty string. Errors are non-fatal: a lookup
// failure leaves Wing="" on every row, which the multiplier code then
// treats as a legacy candidate (always neutral).
func (s *SQLiteStore) attachWings(results []SearchResult) {
	if len(results) == 0 {
		return
	}
	uniqueIDs := make([]string, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for _, r := range results {
		if _, ok := seen[r.FileID]; ok {
			continue
		}
		seen[r.FileID] = struct{}{}
		uniqueIDs = append(uniqueIDs, r.FileID)
	}

	wings := s.getWingsByFileIDs(uniqueIDs)
	for i := range results {
		if w, ok := wings[results[i].FileID]; ok {
			results[i].Wing = w
		}
	}
}

// getWingsByFileIDs returns a map[fileID]wing for the supplied fileIDs.
// Files with wing IS NULL map to "". Unknown fileIDs are absent from the
// map. The query uses a parameterized IN list — fileIDs are expected to
// be application-controlled (not user-provided).
//
// On any SQL error this returns an empty map; callers should treat that
// as "no wing info available" which the wing multiplier handles safely.
func (s *SQLiteStore) getWingsByFileIDs(fileIDs []string) map[string]string {
	out := make(map[string]string, len(fileIDs))
	if len(fileIDs) == 0 {
		return out
	}

	placeholders := make([]string, len(fileIDs))
	args := make([]any, len(fileIDs))
	for i, id := range fileIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := "SELECT file_id, COALESCE(wing, '') FROM files WHERE file_id IN (" +
		strings.Join(placeholders, ",") + ")"

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var fid, wing string
		if err := rows.Scan(&fid, &wing); err != nil {
			continue
		}
		out[fid] = wing
	}
	return out
}

// SearchVector performs a vector similarity search using in-memory cosine similarity.
func (s *SQLiteStore) SearchVector(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	searchStart := time.Now()
	if s.embedder.Name() == "none" {
		return nil, nil
	}

	// Generate query embedding.
	embeddings, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return nil, nil
	}
	queryVec := embeddings[0]

	// Save query embedding for reuse by TopicChangeDetector.
	s.lastQueryEmbMu.Lock()
	s.lastQueryEmb = queryVec
	s.lastQueryEmbMu.Unlock()

	// Snapshot in-memory cache for lock-free iteration.
	s.vectorCacheMu.RLock()
	cache := make([]vectorCacheEntry, 0, len(s.vectorCacheByID))
	for _, entry := range s.vectorCacheByID {
		cache = append(cache, entry)
	}
	s.vectorCacheMu.RUnlock()

	if len(cache) == 0 {
		return nil, nil
	}

	type scored struct {
		entry vectorCacheEntry
		score float64
	}
	var candidates []scored

	// Precompute query norm once for all quantized comparisons.
	queryNorm := VectorNorm(queryVec)

	for _, entry := range cache {
		var sim float64
		if entry.quantized != nil {
			sim = entry.quantized.CosineSimilarity(queryVec, queryNorm)
		} else if len(entry.embedding) > 0 {
			sim = cosineSimilarity(queryVec, entry.embedding)
		} else {
			continue
		}
		if sim > 0 {
			candidates = append(candidates, scored{entry: entry, score: sim})
		}
	}

	// Sort by score descending.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if maxResults <= 0 {
		maxResults = 10
	}
	if len(candidates) > maxResults*2 {
		candidates = candidates[:maxResults*2]
	}

	var results []SearchResult
	for _, c := range candidates {
		results = append(results, SearchResult{
			FileID: c.entry.fileID,
			Text:   c.entry.text,
			Score:  c.score,
		})
	}

	s.logger.Info("search_vector completed",
		"elapsed_ms", time.Since(searchStart).Milliseconds(),
		"chunks_scanned", len(cache),
		"results", len(results),
		"quantized", s.quantizeEnabled,
	)
	return results, nil
}

// defaultWingBoostMatch is the score multiplier applied to candidates whose
// wing matches the query wing when the caller leaves WingBoostMatch unset.
// Mirrors HierarchyConfig.WingBoostMatch default.
const defaultWingBoostMatch = 1.3

// defaultWingBoostPenalty is the score multiplier applied to candidates
// whose wing differs from the query wing when WingBoostPenalty is unset.
// Mirrors HierarchyConfig.WingBoostPenalty default.
const defaultWingBoostPenalty = 0.4

// HybridSearchOptions configures a hybrid (vector + BM25) memory search.
//
// This struct replaces the six-positional-float HybridSearch signature with
// a single options bag, which is necessary because Sprint 2 Room 2.0c adds
// wing-aware fusion. Adding "yet another float" was already the wrong shape;
// the options struct lets callers add new tuning knobs without breaking
// existing call sites.
//
// Zero-value semantics for the numeric fields preserve the legacy defaults:
// MaxResults=0 → 6, MinScore=0 → 0.1, VectorWeight=0 → 0.7, BM25Weight=0 → 0.3.
// Wing fields fall back to 1.3 / 0.4 when zero (see WingBoostMatch godoc).
type HybridSearchOptions struct {
	// MaxResults caps the number of results returned. Defaults to 6 when 0.
	MaxResults int

	// MinScore drops candidates whose final fused score falls below this
	// threshold. Defaults to 0.1 when 0.
	MinScore float64

	// VectorWeight is the fusion weight for the vector branch. Defaults
	// to 0.7 when 0.
	VectorWeight float64

	// BM25Weight is the fusion weight for the keyword branch. Defaults
	// to 0.3 when 0.
	BM25Weight float64

	// QueryWing, when non-empty, biases the fusion score: candidates whose
	// files.wing equals QueryWing are multiplied by WingBoostMatch, candidates
	// with a different non-empty wing are multiplied by WingBoostPenalty,
	// and candidates with wing IS NULL remain at multiplier 1.0 (neutral).
	//
	// When QueryWing is empty, the boost logic is bypassed entirely and the
	// search returns byte-identical scores and ordering to the legacy
	// HybridSearch signature. This is the Sprint 2 retrocompat contract.
	QueryWing string

	// WingBoostMatch is the score multiplier applied when a candidate's
	// wing matches QueryWing. Defaults to 1.3 when zero. A caller can set
	// this to 1.0 explicitly to disable matching boost while still applying
	// WingBoostPenalty to non-matching files.
	WingBoostMatch float64

	// WingBoostPenalty is the score multiplier applied when a candidate's
	// wing is non-empty but differs from QueryWing. Defaults to 0.4 when
	// zero. Files with wing IS NULL are NEVER penalized regardless of
	// this value — that is the Sprint 1 retrocompat contract.
	WingBoostPenalty float64

	// KGFactsPerEntity caps the number of KG facts returned per detected
	// entity in HybridSearchEnriched. Defaults to 3 when zero.
	// Set to -1 for unlimited.
	KGFactsPerEntity int
}

// HybridSearch performs a combined vector + BM25 search with configurable
// weights. This signature is preserved verbatim for backward compatibility
// with v1.17.0 callers; it is now a thin wrapper over HybridSearchWithOpts
// that passes QueryWing="" to take the wing-unaware fast path.
//
// Calls through this entry point are byte-identical to the pre-Sprint-2
// implementation: no JOIN against files.wing, no multiplier, no per-result
// Wing population.
func (s *SQLiteStore) HybridSearch(ctx context.Context, query string, maxResults int, minScore float64, vectorWeight, bm25Weight float64) ([]SearchResult, error) {
	return s.HybridSearchWithOpts(ctx, query, HybridSearchOptions{
		MaxResults:   maxResults,
		MinScore:     minScore,
		VectorWeight: vectorWeight,
		BM25Weight:   bm25Weight,
	})
}

// HybridSearchWithOpts is the wing-aware hybrid search implementation
// introduced in Sprint 2 Room 2.0c. It runs vector + BM25 in parallel,
// fuses the rankings via the existing weighted-inverse-rank formula
// (weight * 1/(rank+1)) — NOT standard RRF k=60; that migration is
// deferred to ADR-010 — and then applies a wing multiplier to the fused
// score when opts.QueryWing is non-empty.
//
// Wing multiplier rules:
//
//   - candidate.Wing == opts.QueryWing → fused *= WingBoostMatch (1.3 default)
//   - candidate.Wing != "" and != opts.QueryWing → fused *= WingBoostPenalty (0.4 default)
//   - candidate.Wing == "" (legacy NULL) → fused *= 1.0 (NEVER penalized)
//
// The "wing IS NULL stays neutral" rule is a hard invariant: legacy
// memories from v1.17.0 must rank exactly the same regardless of whether
// the query carries a wing. Violating this would silently degrade results
// for every user who hasn't yet started classifying their memories.
//
// When opts.QueryWing == "", this function is byte-identical to the
// pre-Sprint-2 HybridSearch — no JOIN against files.wing, no multiplier
// arithmetic, no per-result Wing field population.
func (s *SQLiteStore) HybridSearchWithOpts(ctx context.Context, query string, opts HybridSearchOptions) ([]SearchResult, error) {
	searchStart := time.Now()
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 6
	}
	minScore := opts.MinScore
	if minScore <= 0 {
		minScore = 0.1
	}
	vectorWeight := opts.VectorWeight
	if vectorWeight <= 0 {
		vectorWeight = 0.7
	}
	bm25Weight := opts.BM25Weight
	if bm25Weight <= 0 {
		bm25Weight = 0.3
	}

	wingAware := opts.QueryWing != ""
	matchBoost := opts.WingBoostMatch
	if matchBoost == 0 {
		matchBoost = defaultWingBoostMatch
	}
	penaltyBoost := opts.WingBoostPenalty
	if penaltyBoost == 0 {
		penaltyBoost = defaultWingBoostPenalty
	}

	// Run both searches in parallel.
	type searchResult struct {
		results []SearchResult
		err     error
	}

	vecCh := make(chan searchResult, 1)
	bm25Ch := make(chan searchResult, 1)

	go func() {
		var (
			results []SearchResult
			err     error
		)
		if wingAware {
			results, err = s.searchVectorWithWing(ctx, query, maxResults*4)
		} else {
			results, err = s.SearchVector(ctx, query, maxResults*4)
		}
		vecCh <- searchResult{results, err}
	}()

	go func() {
		var (
			results []SearchResult
			err     error
		)
		if wingAware {
			results, err = s.searchBM25WithWing(query, maxResults*4)
		} else {
			results, err = s.SearchBM25(query, maxResults*4)
		}
		bm25Ch <- searchResult{results, err}
	}()

	vecResult := <-vecCh
	bm25Result := <-bm25Ch

	// Merge results using the weighted-inverse-rank fusion formula
	// (weight * 1/(rank+1)). This is DevClaw's existing fusion — see the
	// HybridSearchWithOpts godoc for the rationale on not migrating to
	// standard RRF k=60.
	//
	// Use a hash of the full text as merge key to avoid collisions between
	// different chunks from the same file that share a common prefix.
	scoreMap := make(map[string]*SearchResult) // key = sha256(fileID + text)

	mergeResults := func(results []SearchResult, weight float64) {
		for i, r := range results {
			key := hashText(r.FileID + "|" + r.Text)
			if existing, ok := scoreMap[key]; ok {
				existing.Score += weight * (1.0 / float64(i+1))
				// Wing should be stable across both branches because both
				// the FTS and vector paths read it from the same files row,
				// but defensively prefer a non-empty value if one branch
				// happened to miss it (shouldn't occur with the JOIN paths).
				if existing.Wing == "" && r.Wing != "" {
					existing.Wing = r.Wing
				} else if existing.Wing != "" && r.Wing != "" && existing.Wing != r.Wing {
					// Disagreement between FTS and vector paths — should be
					// impossible since both JOIN the same files row. Log so
					// we notice if a future refactor breaks the invariant.
					slog.Warn("hybrid search wing disagreement between fts and vector paths",
						"file_id", r.FileID,
						"existing_wing", existing.Wing,
						"new_wing", r.Wing,
					)
				}
			} else {
				scoreMap[key] = &SearchResult{
					FileID: r.FileID,
					Text:   r.Text,
					Score:  weight * (1.0 / float64(i+1)),
					Wing:   r.Wing,
				}
			}
		}
	}

	if vecResult.err == nil {
		mergeResults(vecResult.results, vectorWeight)
	}
	if bm25Result.err == nil {
		mergeResults(bm25Result.results, bm25Weight)
	}

	// Apply wing multiplier to the fused score, when wing-aware mode is on.
	// This is the only behavioral difference vs. the legacy HybridSearch
	// path — and it is gated entirely on wingAware so QueryWing="" callers
	// observe identical numerics.
	if wingAware {
		for _, r := range scoreMap {
			r.Score *= wingMultiplier(r.Wing, opts.QueryWing, matchBoost, penaltyBoost)
		}
	}

	// Collect and sort by combined score.
	var merged []SearchResult
	for _, r := range scoreMap {
		if r.Score >= minScore {
			merged = append(merged, *r)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	if len(merged) > maxResults {
		merged = merged[:maxResults]
	}

	s.logger.Info("hybrid_search completed",
		"elapsed_ms", time.Since(searchStart).Milliseconds(),
		"vector_results", len(vecResult.results),
		"bm25_results", len(bm25Result.results),
		"fused_results", len(merged),
		"wing", opts.QueryWing,
	)
	return merged, nil
}

// wingMultiplier returns the score multiplier for a candidate file given
// the candidate's wing and the query's wing. The rules are:
//
//   - candidateWing == "" (legacy NULL) → 1.0 (ALWAYS neutral, never penalized)
//   - queryWing == "" → 1.0 (caller should not call this in that case anyway)
//   - candidateWing == queryWing → matchBoost
//   - otherwise → penaltyBoost
//
// The candidateWing == "" rule is the Sprint 1 retrocompat contract and
// MUST NOT be removed. Tested by TestHybridSearchWingNullNeutral.
func wingMultiplier(candidateWing, queryWing string, matchBoost, penaltyBoost float64) float64 {
	if candidateWing == "" {
		return 1.0
	}
	if queryWing == "" {
		return 1.0
	}
	if candidateWing == queryWing {
		return matchBoost
	}
	return penaltyBoost
}

// TemporalDecayConfig configures exponential score decay based on memory age.
type TemporalDecayConfig struct {
	Enabled      bool
	HalfLifeDays float64
}

// ApplyTemporalDecay applies exponential decay to search results based on file age.
// Files matching the pattern "memory/YYYY-MM-DD.md" are decayed; evergreen files
// (MEMORY.md or non-dated) are not decayed.
func (s *SQLiteStore) ApplyTemporalDecay(results []SearchResult, cfg TemporalDecayConfig) []SearchResult {
	if !cfg.Enabled || len(results) == 0 {
		return results
	}

	halfLife := cfg.HalfLifeDays
	if halfLife <= 0 {
		halfLife = 30
	}
	lambda := math.Log(2) / halfLife
	now := time.Now()

	for i := range results {
		fileDate := extractDateFromFileID(results[i].FileID)
		if fileDate == nil {
			continue // Evergreen file, no decay
		}

		ageDays := now.Sub(*fileDate).Hours() / 24
		if ageDays < 0 {
			ageDays = 0
		}
		decayFactor := math.Exp(-lambda * ageDays)
		results[i].Score *= decayFactor
	}

	return results
}

// extractDateFromFileID extracts a date from file IDs like "memory/2026-02-25.md"
// or "2026-02-25.md". Returns nil for evergreen files (MEMORY.md or non-dated).
func extractDateFromFileID(fileID string) *time.Time {
	// Evergreen files don't decay
	if strings.Contains(fileID, MemoryFileName) {
		return nil
	}

	// Extract base filename
	base := fileID
	if idx := strings.LastIndex(fileID, "/"); idx >= 0 {
		base = fileID[idx+1:]
	}

	// Remove extension
	if idx := strings.LastIndex(base, "."); idx > 0 {
		base = base[:idx]
	}

	// Try to parse as date (YYYY-MM-DD)
	t, err := time.Parse("2006-01-02", base)
	if err != nil {
		return nil
	}
	return &t
}

// MMRConfig configures Maximal Marginal Relevance for search diversification.
type MMRConfig struct {
	Enabled bool
	Lambda  float64
}

// ApplyMMR applies Maximal Marginal Relevance re-ranking to diversify results.
// Lambda controls the balance: 0 = max diversity, 1 = max relevance.
func (s *SQLiteStore) ApplyMMR(results []SearchResult, cfg MMRConfig, maxResults int) []SearchResult {
	if !cfg.Enabled || len(results) <= maxResults {
		return results
	}

	lambda := cfg.Lambda
	if lambda <= 0 {
		lambda = 0.7
	}
	if lambda > 1 {
		lambda = 1
	}

	selected := make([]SearchResult, 0, maxResults)
	remaining := make([]SearchResult, len(results))
	copy(remaining, results)

	// First: highest relevance
	selected = append(selected, remaining[0])
	remaining = remaining[1:]

	// Pre-tokenize for Jaccard similarity
	tokenCache := make(map[string]map[string]bool)
	tokenize := func(text string) map[string]bool {
		if cached, ok := tokenCache[text]; ok {
			return cached
		}
		tokens := make(map[string]bool)
		for _, word := range strings.Fields(strings.ToLower(text)) {
			if len(word) > 2 {
				tokens[word] = true
			}
		}
		tokenCache[text] = tokens
		return tokens
	}

	for len(selected) < maxResults && len(remaining) > 0 {
		bestIdx := 0
		bestScore := -1.0

		for i, candidate := range remaining {
			// MMR = lambda * relevance - (1-lambda) * max_similarity_to_selected
			maxSim := 0.0
			candidateTokens := tokenize(candidate.Text)
			for _, sel := range selected {
				sim := jaccardSimilarity(candidateTokens, tokenize(sel.Text))
				if sim > maxSim {
					maxSim = sim
				}
			}

			mmrScore := lambda*candidate.Score - (1-lambda)*maxSim
			if mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = i
			}
		}

		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

// jaccardSimilarity computes Jaccard similarity between two token sets.
func jaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	intersection := 0
	for token := range a {
		if b[token] {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// HybridSearchWithOptions performs hybrid search with optional temporal
// decay and MMR. This signature is preserved verbatim for backward
// compatibility with v1.17.0 callers; it delegates to HybridSearchWithOpts
// with QueryWing="" to take the wing-unaware fast path.
//
// Sprint 2 callers that need wing-aware fusion should use
// HybridSearchWithOptsAndPostFilters or HybridSearchWithOpts directly.
func (s *SQLiteStore) HybridSearchWithOptions(ctx context.Context, query string, maxResults int, minScore float64, vectorWeight, bm25Weight float64, decayCfg TemporalDecayConfig, mmrCfg MMRConfig) ([]SearchResult, error) {
	return s.HybridSearchWithOptsAndPostFilters(ctx, query, HybridSearchOptions{
		MaxResults:   maxResults,
		MinScore:     minScore,
		VectorWeight: vectorWeight,
		BM25Weight:   bm25Weight,
	}, decayCfg, mmrCfg)
}

// HybridSearchWithOptsAndPostFilters runs HybridSearchWithOpts and then
// applies the temporal-decay and MMR post-filters. It is the wing-aware
// equivalent of HybridSearchWithOptions and is the entry point used by
// memory_search and prompt_layers when wing routing is active.
//
// The retrocompat contract is the same as HybridSearchWithOpts: passing
// opts.QueryWing == "" produces results that are byte-identical to the
// legacy HybridSearchWithOptions for the same numeric inputs.
func (s *SQLiteStore) HybridSearchWithOptsAndPostFilters(ctx context.Context, query string, opts HybridSearchOptions, decayCfg TemporalDecayConfig, mmrCfg MMRConfig) ([]SearchResult, error) {
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 6
	}
	// Pre-fetch 2x the requested count so post-filters have headroom.
	innerOpts := opts
	innerOpts.MaxResults = maxResults * 2

	results, err := s.HybridSearchWithOpts(ctx, query, innerOpts)
	if err != nil {
		return nil, err
	}

	// Apply temporal decay
	results = s.ApplyTemporalDecay(results, decayCfg)

	// Re-sort after decay
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply MMR for diversification
	results = s.ApplyMMR(results, mmrCfg, maxResults)

	return results, nil
}

// getEmbeddingCache looks up a cached embedding by text hash.
func (s *SQLiteStore) getEmbeddingCache(text string) []float32 {
	hash := hashText(text)
	var embJSON string
	err := s.db.QueryRow(`
		SELECT embedding FROM embedding_cache
		WHERE text_hash = ? AND provider = ? AND model = ?
	`, hash, s.embedder.Name(), s.embedder.Model()).Scan(&embJSON)
	if err != nil {
		return nil
	}
	var emb []float32
	if err := json.Unmarshal([]byte(embJSON), &emb); err != nil {
		return nil
	}
	return emb
}

// setEmbeddingCache stores an embedding in the cache.
func (s *SQLiteStore) setEmbeddingCache(text string, embedding []float32) {
	hash := hashText(text)
	data, err := json.Marshal(embedding)
	if err != nil {
		return
	}
	_, _ = s.db.Exec(`
		INSERT INTO embedding_cache (text_hash, provider, model, embedding, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(text_hash, provider, model) DO UPDATE SET
			embedding = excluded.embedding, updated_at = CURRENT_TIMESTAMP
	`, hash, s.embedder.Name(), s.embedder.Model(), string(data))
}

// loadVectorCache loads all chunk embeddings into memory for fast search.
// Called once on startup. Subsequent index operations use updateVectorCacheForFile.
func (s *SQLiteStore) loadVectorCache() error {
	rows, err := s.db.Query("SELECT id, file_id, text, embedding FROM chunks WHERE embedding IS NOT NULL")
	if err != nil {
		return err
	}
	defer rows.Close()

	byID := make(map[int64]vectorCacheEntry)
	byFile := make(map[string][]int64)
	for rows.Next() {
		var e vectorCacheEntry
		var embJSON string
		if err := rows.Scan(&e.chunkID, &e.fileID, &e.text, &embJSON); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(embJSON), &e.embedding); err != nil {
			continue
		}
		if s.quantizeEnabled && len(e.embedding) > 0 {
			q := QuantizeFloat32(e.embedding)
			e.quantized = &q
			e.embedding = nil // free float32 memory (~4x savings)
		}
		byID[e.chunkID] = e
		byFile[e.fileID] = append(byFile[e.fileID], e.chunkID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("scan vector cache rows: %w", err)
	}

	s.vectorCacheMu.Lock()
	s.vectorCacheByID = byID
	s.vectorCacheByFile = byFile
	s.vectorCacheMu.Unlock()

	s.logger.Debug("vector cache loaded", "chunks", len(byID))
	return nil
}

// updateVectorCacheForFile replaces all cached entries for a single file.
// This avoids a full table scan on every IndexChunks call.
func (s *SQLiteStore) updateVectorCacheForFile(fileID string, newEntries []vectorCacheEntry) {
	s.vectorCacheMu.Lock()
	defer s.vectorCacheMu.Unlock()

	// Remove old entries for this fileID.
	if oldIDs, ok := s.vectorCacheByFile[fileID]; ok {
		for _, id := range oldIDs {
			delete(s.vectorCacheByID, id)
		}
		delete(s.vectorCacheByFile, fileID)
	}

	// Add new entries.
	newIDs := make([]int64, 0, len(newEntries))
	for _, e := range newEntries {
		s.vectorCacheByID[e.chunkID] = e
		newIDs = append(newIDs, e.chunkID)
	}
	if len(newIDs) > 0 {
		s.vectorCacheByFile[fileID] = newIDs
	}
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// ChunkCount returns the total number of indexed chunks.
func (s *SQLiteStore) ChunkCount() int {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&count)
	return count
}

// FileCount returns the total number of indexed files.
func (s *SQLiteStore) FileCount() int {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&count)
	return count
}

// PruneEmbeddingCache removes old cache entries exceeding maxEntries.
func (s *SQLiteStore) PruneEmbeddingCache(maxEntries int) {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	_, _ = s.db.Exec(`
		DELETE FROM embedding_cache WHERE rowid IN (
			SELECT rowid FROM embedding_cache
			ORDER BY updated_at DESC
			LIMIT -1 OFFSET ?
		)
	`, maxEntries)
}

// ---------- Math Helpers ----------

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// hashText computes the SHA-256 hex hash of a text for cache keying.
func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// sanitizeFTS5Query escapes FTS5 special characters and wraps the query in
// double quotes so it is treated as a phrase literal. This prevents accidental
// FTS5 syntax injection from user input.
func sanitizeFTS5Query(query string) string {
	// Remove characters that are FTS5 operators.
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '"', '(', ')', '*', '^', ':', '{', '}':
			return ' '
		default:
			return r
		}
	}, query)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return ""
	}
	// Wrap in double quotes for phrase matching.
	return `"` + cleaned + `"`
}

// IndexMemoryDir indexes all .md files in the memory directory and MEMORY.md.
func (s *SQLiteStore) IndexMemoryDir(ctx context.Context, memDir string, chunkCfg ChunkConfig) error {
	start := time.Now()

	fileChunks, err := IndexDirectory(memDir, chunkCfg)
	if err != nil {
		return fmt.Errorf("index directory: %w", err)
	}

	for fileID, chunks := range fileChunks {
		fHash := ""
		for _, c := range chunks {
			fHash += c.Hash
		}
		if err := s.IndexChunks(ctx, fileID, chunks, fHash); err != nil {
			s.logger.Warn("failed to index file", "file", fileID, "error", err)
		}
	}

	s.logger.Info("memory index complete",
		"files", len(fileChunks),
		"chunks", s.ChunkCount(),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return nil
}

// TranscriptEntry is a conversation entry for transcript indexing.
type TranscriptEntry struct {
	Role    string // "user" or "assistant"
	Content string
}

// IndexTranscript indexes conversation transcript entries as searchable chunks.
// Each entry is stored as a chunk with file_id "session:<sessionID>".
// Uses content hashing to avoid re-indexing identical content.
func (s *SQLiteStore) IndexTranscript(ctx context.Context, sessionID string, entries []TranscriptEntry) error {
	if len(entries) == 0 {
		return nil
	}

	fileID := "session:" + sessionID
	var chunks []Chunk
	for i, e := range entries {
		text := fmt.Sprintf("[%s] %s", e.Role, e.Content)
		if len(text) > 2000 {
			text = text[:2000]
		}
		h := sha256.Sum256([]byte(text))
		chunks = append(chunks, Chunk{
			Index: i,
			Text:  text,
			Hash:  hex.EncodeToString(h[:]),
		})
	}

	var allHashes string
	for _, c := range chunks {
		allHashes += c.Hash
	}
	fh := sha256.Sum256([]byte(allHashes))
	fileHash := hex.EncodeToString(fh[:])

	return s.IndexChunks(ctx, fileID, chunks, fileHash)
}
