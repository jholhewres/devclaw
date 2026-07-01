// Package memory — migration_import.go implements the one-time, idempotent
// import of the legacy flat-markdown memory files (MEMORY.md + daily 20*.md)
// into the v2 SQLite chunks table with lifecycle metadata (US-002 + US-003).
//
// Pipeline per entry (curation):
//   1. parse via parseMemoryFile (already skips [stale], parses [expires:]/[meta:])
//   2. drop [Contradiction] entries (counted, never inserted)
//   3. exact-content-hash dedup (first wins; later dups counted + skipped)
//   4. quality-score; sub-threshold -> curation_status='low_signal'
//   5. classify kind / memory_type / scope from category (+ wing via classifier)
//   6. derive expires_at (entry TTL, else category default for event/operational)
//   7. redact credentials in-place before storage
//   8. embed (synchronously, via the store's existing embedder) + insert as a
//      discrete one-chunk file so recall returns individual memories
//
// Idempotency: a sentinel marker row (importMarkerFileID) is written in the
// files table on the first successful run. Re-running detects the marker and
// returns immediately. This is intentionally DECOUPLED from PRAGMA user_version
// (owned by MigrateMemoryV2): the column migration and the data import have
// independent failure modes, and the marker lets the import retry on the next
// boot if it failed, without re-running the column migration.
//
// Fail-open: the caller (assistant startup, fire-and-forget goroutine) logs any
// error and never blocks startup. A failed import leaves the marker UNSET so it
// retries next boot.
//
// The .md files are READ ONLY here — they are never edited or deleted.
package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// plausibleEventTime reports whether a parsed entry timestamp is a real event
// date worth storing as occurred_at. It rejects the zero value AND implausibly
// old years (e.g. a malformed/partial stamp that time.Parse coerces to year 1):
// such values must not be stamped — they'd land as "0001-01-01" garbage that
// never matches a real date window and pollutes the data. DevClaw memory only
// post-dates 2026, so any year before 2000 is a parse artifact.
func plausibleEventTime(t time.Time) bool {
	return !t.IsZero() && t.Year() >= 2000
}

// importMarkerFileID is the sentinel files row that records a completed legacy
// import. Its presence makes ImportLegacyMarkdown a no-op on subsequent runs.
const importMarkerFileID = "memory/imported/_marker"

// importedFileIDPrefix namespaces every imported memory so recall and future
// maintenance can distinguish migrated legacy facts from live indexed files.
const importedFileIDPrefix = "memory/imported/"

// savedFileIDPrefix namespaces memories written by the live save path (US-004
// cutover) after migration completes, distinguishing them from migrated facts
// (importedFileIDPrefix) and from indexed .md files.
const savedFileIDPrefix = "memory/saved/"

// eventTTL is the default retention applied to ephemeral categories
// (event / operational) that arrive without an explicit [expires:] tag.
const eventTTL = 30 * 24 * time.Hour

// contradictionMarker is the substring (case-insensitive) that flags an entry
// the dream system recorded as a detected-but-unresolved contradiction. These
// are bloat for long-term recall and are dropped during import.
const contradictionMarker = "[contradiction]"

// ImportStats summarizes one ImportLegacyMarkdown run.
type ImportStats struct {
	FilesScanned          int
	EntriesParsed         int
	ContradictionsDropped int
	DuplicatesDropped     int
	LowSignal             int
	Inserted              int
	// AlreadyImported is true when the run was a no-op because the marker
	// row already existed (idempotency short-circuit).
	AlreadyImported bool
}

// ImportLegacyMarkdown parses the legacy flat-markdown memory files under
// memoryDir, curates each entry, and writes the survivors into the v2 chunks
// table with lifecycle metadata. It is idempotent (guarded by importMarkerFileID)
// and fail-open (errors are returned for logging but the caller must not block
// startup on them).
//
// Embedding happens SYNCHRONOUSLY here, via the store's existing embedder, in
// the same per-chunk fashion as indexChunksLocked. Because the only caller runs
// this in a fire-and-forget startup goroutine (see assistant.go), the synchronous
// embedding does not block the agent from coming online. Each curated memory is
// stored as its own single-chunk file (file_id = importedFileIDPrefix+<hash>)
// so recall returns discrete memories rather than giant file-spanning chunks.
//
// On a successful first import (and only then) it runs VACUUM to reclaim the
// space freed by curation churn.
func (s *SQLiteStore) ImportLegacyMarkdown(ctx context.Context, memoryDir string, logger *slog.Logger) (ImportStats, error) {
	var stats ImportStats
	if s == nil || s.db == nil {
		return stats, fmt.Errorf("import legacy markdown: store not initialized")
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Idempotency gate: bail if the marker already exists.
	imported, err := s.legacyImportDone(ctx)
	if err != nil {
		return stats, fmt.Errorf("check import marker: %w", err)
	}
	if imported {
		stats.AlreadyImported = true
		return stats, nil
	}

	files, err := discoverLegacyFiles(memoryDir)
	if err != nil {
		return stats, fmt.Errorf("discover legacy files: %w", err)
	}
	stats.FilesScanned = len(files)

	// Parse + curate every entry across all files, deduping by exact content
	// hash globally (a fact repeated across daily files collapses to one).
	seen := make(map[string]struct{})
	now := time.Now().UTC()

	for _, path := range files {
		data, readErr := os.ReadFile(path) // READ ONLY — never written back.
		if readErr != nil {
			logger.Warn("import: read failed", "file", path, "error", readErr)
			continue
		}
		source := filepath.Base(path)
		entries := parseMemoryFile(string(data), source)
		stats.EntriesParsed += len(entries)

		for _, e := range entries {
			curated, action := s.curateEntry(e, now)
			switch action {
			case curateDropContradiction:
				stats.ContradictionsDropped++
				continue
			case curateDropEmpty:
				continue
			}

			key := importHash(strings.TrimSpace(curated.text))
			if _, dup := seen[key]; dup {
				stats.DuplicatesDropped++
				continue
			}
			seen[key] = struct{}{}

			if curated.curationStatus == CurationStatusLowSignal {
				stats.LowSignal++
			}

			if err := s.insertCuratedChunk(ctx, key, curated); err != nil {
				logger.Warn("import: insert failed", "file", source, "error", err)
				continue
			}
			stats.Inserted++
		}
	}

	// Write the marker only after the data is in. If anything above returned
	// early with an error, the marker stays unset and the import retries next boot.
	if err := s.markLegacyImportDone(ctx, stats); err != nil {
		return stats, fmt.Errorf("write import marker: %w", err)
	}

	// Reclaim space freed by dedup/curation churn. Best-effort: a reclaim
	// failure must not fail the (already committed) import.
	s.reclaimSpace(ctx, logger)

	logger.Info("legacy memory import complete",
		"files", stats.FilesScanned,
		"parsed", stats.EntriesParsed,
		"inserted", stats.Inserted,
		"contradictions_dropped", stats.ContradictionsDropped,
		"duplicates_dropped", stats.DuplicatesDropped,
		"low_signal", stats.LowSignal,
	)
	return stats, nil
}

// curateAction enumerates the early-exit outcomes of curateEntry.
type curateAction int

const (
	curateKeep curateAction = iota
	curateDropContradiction
	curateDropEmpty
)

// curatedEntry is the post-curation, ready-to-insert representation of a memory.
type curatedEntry struct {
	text           string
	kind           string
	memoryType     string
	scope          string
	wing           string
	importance     float64
	confidence     float64
	curationStatus string
	curationRule   string
	expiresAt      *time.Time
	supersedes     []string
	// occurredAt is the memory's ORIGINAL event timestamp (the [YYYY-MM-DD HH:MM]
	// parsed from the legacy .md line, or time.Now() for a live save). nil means
	// the entry carried no parseable timestamp; the insert then falls back to
	// created_at / NOW so the column is never spuriously empty.
	occurredAt *time.Time
}

// curateEntry applies the full curation policy to a single parsed Entry and
// returns the ready-to-insert form plus an action telling the caller whether to
// drop it (contradiction / empty) or keep it.
func (s *SQLiteStore) curateEntry(e Entry, now time.Time) (curatedEntry, curateAction) {
	content := strings.TrimSpace(e.Content)
	if content == "" {
		return curatedEntry{}, curateDropEmpty
	}

	// Drop detected-but-unresolved contradictions outright (bloat). Cheaper and
	// cleaner than inserting them low_signal; we count them in stats.
	if strings.Contains(strings.ToLower(content), contradictionMarker) {
		return curatedEntry{}, curateDropContradiction
	}

	// Redact credentials BEFORE anything else touches the text, so neither the
	// stored chunk, the dedup hash, nor the embedding ever sees the secret.
	if LooksLikeCredential(content) {
		content = RedactCredentials(content)
	}

	kind, memoryType, scope := classifyCategory(e.Category, e.MemoryType, e.ContextTier)

	// Wing: prefer an explicit classifier signal when the store has keywords
	// configured (mirrors RunLegacyClassificationPass). The classifier is a
	// no-op (returns "") when no keywords are set, which keeps OSS builds neutral.
	wing := ""
	if kws := s.legacyKeywords(); len(kws) > 0 {
		if r := ClassifyLegacyContent(content, kws); r.Wing != "" && r.Confidence >= ClassifierMinConfidence {
			wing = r.Wing
		}
	}

	hasScope := scope != "" && scope != "global" || wing != ""
	verdict := ClassifyQuality(content, e.Category, hasScope, e.Pinned)

	importance := e.Importance
	if importance == 0 {
		importance = verdict.Score
	}
	confidence := verdict.Score

	expiresAt := deriveExpiry(e, kind, memoryType, now)

	// Preserve the original event timestamp. A zero Timestamp means the legacy
	// line had no parseable [YYYY-MM-DD HH:MM]; default to now (the import
	// instant, same time.Time passed to curateEntry) so occurred_at is ALWAYS
	// bound as a real Go time.Time with a consistent timezone offset — never
	// falling through to SQLite's CURRENT_TIMESTAMP which produces a bare UTC
	// string and breaks string-comparison day-window filters on non-UTC hosts.
	// occurred_at is always a LOCAL instant: a parsed .md timestamp is already
	// local (ParseInLocation), and the no-timestamp fallback uses now.Local() so
	// it never gets a UTC (+00:00) offset that would mis-sort against the
	// time.Local date windows in US-003 string comparisons.
	occurredAtVal := now.Local()
	if plausibleEventTime(e.Timestamp) {
		occurredAtVal = e.Timestamp
	}
	occurredAt := &occurredAtVal

	return curatedEntry{
		text:           content,
		kind:           kind,
		memoryType:     memoryType,
		scope:          scope,
		wing:           wing,
		importance:     importance,
		confidence:     confidence,
		curationStatus: verdict.CurationStatus,
		curationRule:   verdict.CurationRule,
		expiresAt:      expiresAt,
		supersedes:     e.Supersedes,
		occurredAt:     occurredAt,
	}, curateKeep
}

// classifyCategory maps a legacy entry category (+ optional pre-set memory_type
// and context tier) to the v2 (kind, memory_type, scope) triple.
func classifyCategory(category, memoryType, contextTier string) (kind, mtype, scope string) {
	kind = strings.TrimSpace(strings.ToLower(category))
	if kind == "" {
		kind = "fact"
	}

	mtype = strings.TrimSpace(strings.ToLower(memoryType))
	if mtype == "" {
		switch kind {
		case "event":
			mtype = "episodic"
		case "operational", "task", "todo":
			mtype = "operational"
		default:
			// fact, decision, learning, preference, summary, plan, …
			mtype = "semantic"
		}
	}

	// DevClaw has no first-class project scope; tier (when present) is the
	// closest analogue carried by legacy entries. Default to global.
	scope = strings.TrimSpace(strings.ToLower(contextTier))
	if scope == "" {
		scope = "global"
	}
	return kind, mtype, scope
}

// deriveExpiry computes the chunk expires_at: an explicit entry TTL wins; else
// ephemeral categories (event / operational) get a default short TTL; durable
// facts/decisions/preferences never expire (nil).
func deriveExpiry(e Entry, kind, memoryType string, now time.Time) *time.Time {
	if e.ExpiresAt != nil {
		return e.ExpiresAt
	}
	switch {
	case kind == "event" || memoryType == "episodic" || memoryType == "operational":
		exp := now.Add(eventTTL)
		return &exp
	default:
		return nil
	}
}

// insertCuratedChunk writes a single curated memory as its own one-chunk file
// (file_id = importedFileIDPrefix+key) with lifecycle metadata, computing the
// embedding synchronously via the store's embedder. This is a focused write
// path that ADDS columns the generic IndexChunks path does not populate; it
// deliberately does not touch the US-005 recall functions.
func (s *SQLiteStore) insertCuratedChunk(ctx context.Context, key string, c curatedEntry) error {
	return s.insertCuratedChunkWithPrefix(ctx, importedFileIDPrefix, key, c)
}

// insertCuratedChunkWithPrefix is the shared implementation behind both the
// import path (importedFileIDPrefix) and the live save path (savedFileIDPrefix).
func (s *SQLiteStore) insertCuratedChunkWithPrefix(ctx context.Context, prefix, key string, c curatedEntry) error {
	fileID := prefix + key
	chunkHash := importHash(c.text)

	// Dedup-on-save (US-006): skip if a non-deleted chunk with the same exact
	// content hash already exists, regardless of file_id. Prevents the live
	// write path and re-imports from re-inserting an already-stored memory.
	if dup, err := s.chunkHashExists(ctx, chunkHash); err != nil {
		return fmt.Errorf("dedup check: %w", err)
	} else if dup {
		return nil
	}

	// Compute the embedding (cache-aware, mirroring indexChunksLocked).
	var embVec []float32
	if s.embedder != nil && s.embedder.Name() != "none" {
		if cached := s.getEmbeddingCache(c.text); cached != nil {
			embVec = cached
		} else if embs, err := s.embedder.Embed(ctx, []string{c.text}); err == nil && len(embs) > 0 {
			embVec = embs[0]
			s.setEmbeddingCache(ctx, c.text, embVec)
		} else if err != nil {
			s.logger.Warn("import: embedding failed, storing without vector", "error", err)
		}
	}
	var embJSON sql.NullString
	if embVec != nil {
		if data, err := json.Marshal(embVec); err == nil {
			embJSON = sql.NullString{String: string(data), Valid: true}
		}
	}

	var supersedesJSON sql.NullString
	if len(c.supersedes) > 0 {
		if data, err := json.Marshal(c.supersedes); err == nil {
			supersedesJSON = sql.NullString{String: string(data), Valid: true}
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// One files row per imported memory. wing may be NULL (legacy-neutral).
	if c.wing != "" {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO files (file_id, hash, indexed_at, wing) VALUES (?, ?, CURRENT_TIMESTAMP, ?)
			ON CONFLICT(file_id) DO UPDATE SET hash = excluded.hash
		`, fileID, chunkHash, c.wing)
	} else {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO files (file_id, hash, indexed_at) VALUES (?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(file_id) DO UPDATE SET hash = excluded.hash
		`, fileID, chunkHash)
	}
	if err != nil {
		return fmt.Errorf("insert file: %w", err)
	}

	// occurred_at is always bound as a real Go time.Time (never nil): curateEntry
	// defaults to the import-time `now` for entries that carried no timestamp, so
	// the stored value always uses a consistent offset string regardless of the
	// host timezone. COALESCE is kept for safety but never reached.
	res, err := tx.ExecContext(ctx, `
		INSERT INTO chunks (
			file_id, chunk_idx, text, hash, embedding,
			expires_at, supersedes, curation_status, curation_rule,
			importance, confidence, memory_type, kind, scope, scorer_version,
			occurred_at
		) VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, COALESCE(?, CURRENT_TIMESTAMP))
		ON CONFLICT(file_id, chunk_idx) DO NOTHING
	`,
		fileID, c.text, chunkHash, embJSON,
		nullableTime(c.expiresAt), supersedesJSON,
		nullableString(c.curationStatus), nullableString(c.curationRule),
		c.importance, c.confidence, c.memoryType, c.kind, c.scope, QualityScorerVersion,
		nullableTime(c.occurredAt),
	)
	if err != nil {
		return fmt.Errorf("insert chunk: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Keep the in-memory vector cache consistent so the freshly imported memory
	// is recallable without a restart.
	if embVec != nil {
		if chunkID, idErr := res.LastInsertId(); idErr == nil {
			entry := vectorCacheEntry{
				chunkID:   chunkID,
				fileID:    fileID,
				text:      c.text,
				embedding: embVec,
			}
			if s.quantizeEnabled {
				q := QuantizeFloat32(embVec)
				entry.quantized = &q
				entry.embedding = nil
			}
			s.updateVectorCacheForFile(fileID, []vectorCacheEntry{entry})
		}
	}
	return nil
}

// chunkHashExists reports whether a live (non-deleted) chunk with the given
// exact content hash already exists. Used by the dedup-on-save path (US-006)
// so the live write redirect and re-imports never store a memory twice.
func (s *SQLiteStore) chunkHashExists(ctx context.Context, chunkHash string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(1) FROM chunks WHERE hash = ? AND deleted_at IS NULL", chunkHash).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// reclaimSpace reclaims disk space freed by dedup/curation churn. A bare VACUUM
// fails ("cannot VACUUM - SQL statements in progress" / "database is locked") in
// WAL mode when the bounded connection pool keeps idle connections open. To make
// the reclaim actually happen on real deployments, we open a DEDICATED single-use
// connection (MaxOpenConns=1) so no pooled idle connection blocks the VACUUM. The
// whole path is best-effort and never fatal: any failure is logged and swallowed.
func (s *SQLiteStore) reclaimSpace(ctx context.Context, logger *slog.Logger) {
	if s.dbPath == "" {
		// No on-disk path (shouldn't happen for real stores). Fall back to a
		// best-effort checkpoint+VACUUM on the shared pool.
		if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			logger.Warn("import: wal_checkpoint failed (non-fatal)", "error", err)
		}
		if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
			logger.Warn("import: VACUUM failed (non-fatal)", "error", err)
		}
		return
	}

	db, err := sql.Open("sqlite3", s.dbPath+"?_journal_mode=WAL&_busy_timeout=30000&_txlock=immediate")
	if err != nil {
		logger.Warn("import: open dedicated VACUUM connection failed (non-fatal)", "error", err)
		return
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Checkpoint the WAL into the main DB first so VACUUM can reclaim it fully.
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		logger.Warn("import: wal_checkpoint failed (non-fatal)", "error", err)
	}
	if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
		logger.Warn("import: VACUUM failed (non-fatal)", "error", err)
	}
}

// LegacyImportDone is the exported wrapper over legacyImportDone, used by the
// cutover gate (US-004) in the indexer and the live write path to decide
// whether the migration from flat-markdown to SQLite has completed.
func (s *SQLiteStore) LegacyImportDone(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	return s.legacyImportDone(ctx)
}

// SaveCuratedMemory writes a single new memory directly into the v2 chunks table
// as a curated chunk (US-004 write redirect). It reuses the exact per-entry
// curation + dedup + insert path that ImportLegacyMarkdown uses, so live saves
// after cutover get the same lifecycle metadata, embedding, credential redaction,
// and content-hash dedup as imported memories. Saved memories are namespaced
// under savedFileIDPrefix to distinguish live writes from migrated ones.
//
// It is a no-op-safe wrapper: dedup is enforced inside insertCuratedChunk, so a
// repeated save of identical content does not create a second chunk.
func (s *SQLiteStore) SaveCuratedMemory(ctx context.Context, content, category, source string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("save curated memory: store not initialized")
	}
	text := strings.TrimSpace(content)
	if text == "" {
		return nil
	}
	now := time.Now().UTC()
	// Split rich multi-fact content into atomic facts so a narrow query
	// ("qual o localizador") matches a short focused chunk instead of being
	// diluted in one long blob. Short/single-fact content yields one piece.
	for _, fact := range splitAtomicFacts(text) {
		curated, action := s.curateEntry(Entry{
			Content:  fact,
			Category: category,
			Source:   source,
			// occurred_at must be a LOCAL instant so it lines up with US-003 date
			// windows (built in time.Local) under string comparison; the UTC `now`
			// is still used for deriveExpiry below. Mixing zones here would mis-bucket
			// late-evening saves by a day on a non-UTC host.
			Timestamp: now.Local(),
		}, now)
		if action != curateKeep {
			continue
		}
		key := importHash(strings.TrimSpace(curated.text))
		if err := s.insertSavedChunk(ctx, key, curated); err != nil {
			return err
		}
	}
	return nil
}

// insertSavedChunk is insertCuratedChunk with the savedFileIDPrefix namespace.
// It shares the dedup + embedding + lifecycle insert path so live saves and
// imports stay consistent.
func (s *SQLiteStore) insertSavedChunk(ctx context.Context, key string, c curatedEntry) error {
	return s.insertCuratedChunkWithPrefix(ctx, savedFileIDPrefix, key, c)
}

// SupersedeByContent resolves a contradiction in SQLite by hard-excluding the
// losing memory (US-006). For every live chunk whose text matches loserContent
// exactly, it sets deleted_at=CURRENT_TIMESTAMP and supersedes=<winnerKey> so the
// chunkLifecycleGuard drops it from all recall paths, then evicts the affected
// files from the in-memory vector cache so the loser disappears immediately
// without a restart. Returns the number of distinct files superseded.
//
// It NEVER inserts a new summary chunk — the supersede is purely a soft-delete of
// the loser. winnerContent is recorded (hashed) in supersedes for lineage.
func (s *SQLiteStore) SupersedeByContent(ctx context.Context, loserContent, winnerContent string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("supersede: store not initialized")
	}
	loser := strings.TrimSpace(loserContent)
	if loser == "" {
		return 0, nil
	}
	winnerKey := importHash(strings.TrimSpace(winnerContent))
	supersedesJSON, _ := json.Marshal([]string{winnerKey})

	// H2: dream passes RAW FileStore content as the loser, but curated chunks
	// were credential-redacted + trimmed at import (see curateEntry). An exact
	// `text = ?` match therefore finds 0 rows for any redacted/normalized entry,
	// making supersede a silent no-op. Mirror the import's key derivation EXACTLY
	// (redact-if-credential, then TrimSpace, then importHash) so we can locate the
	// curated chunk by its deterministic file_id (importedFileIDPrefix+key), the
	// same way insertCuratedChunk keyed it. The exact-text match is kept as a
	// fallback for non-redacted entries and for raw/.md-resident chunks.
	curatedKey := importHash(curatedKeyForm(loser))
	curatedFileID := importedFileIDPrefix + curatedKey
	savedFileID := savedFileIDPrefix + curatedKey

	// Collect the affected file_ids before mutating so we can evict their
	// vector-cache entries afterward. Match either by curated file_id (redacted
	// path) or by exact loser text (fallback / non-redacted path).
	var fileIDs []string
	seen := make(map[string]struct{})
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT file_id FROM chunks
		WHERE deleted_at IS NULL AND (file_id = ? OR file_id = ? OR text = ?)
	`, curatedFileID, savedFileID, loser)
	if err != nil {
		return 0, fmt.Errorf("supersede select: %w", err)
	}
	for rows.Next() {
		var fid string
		if scanErr := rows.Scan(&fid); scanErr != nil {
			rows.Close()
			return 0, fmt.Errorf("supersede scan: %w", scanErr)
		}
		if _, dup := seen[fid]; dup {
			continue
		}
		seen[fid] = struct{}{}
		fileIDs = append(fileIDs, fid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("supersede rows: %w", err)
	}
	if len(fileIDs) == 0 {
		return 0, nil
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE chunks
		SET deleted_at = CURRENT_TIMESTAMP, supersedes = ?
		WHERE deleted_at IS NULL AND (file_id = ? OR file_id = ? OR text = ?)
	`, string(supersedesJSON), curatedFileID, savedFileID, loser); err != nil {
		return 0, fmt.Errorf("supersede update: %w", err)
	}

	// Drop the superseded chunks from the in-memory vector cache so they stop
	// surfacing in SearchVector immediately (the guard is applied at load time
	// only). BM25/LIKE paths are query-time guarded and need no eviction.
	for _, fid := range fileIDs {
		s.EvictFromVectorCache(fid)
	}
	return len(fileIDs), nil
}

// curatedKeyForm reproduces the exact text transform curateEntry applies before
// hashing/storing a memory: redact credentials (only when the content looks like
// one, matching curateEntry's guard), then TrimSpace. Keeping this in lockstep
// with curateEntry is what lets SupersedeByContent locate redacted curated
// chunks by their deterministic file_id.
func curatedKeyForm(content string) string {
	// Mirror curateEntry precisely: TrimSpace the raw content (line ~212),
	// redact only when it looks like a credential (line ~225), then TrimSpace
	// again to match the key derivation (importHash(TrimSpace(curated.text))).
	c := strings.TrimSpace(content)
	if LooksLikeCredential(c) {
		c = RedactCredentials(c)
	}
	return strings.TrimSpace(c)
}

// DeleteRawLegacyChunks removes the RAW chunk rows that IndexMemoryDir wrote for
// the legacy flat-markdown files (file_id = bare basename, e.g. "MEMORY.md",
// "2026-06-01.md"). These raw chunks carry NULL lifecycle columns and
// un-redacted text, so once the curated import has run they would remain
// recallable alongside (and contradict) the curated/redacted copies. C1 fix:
// after a successful first import, drop them so the curated namespace
// (importedFileIDPrefix) is the only recallable copy.
//
// It deletes ONLY the chunk rows keyed by bare basename and evicts those file_ids
// from the in-memory vector cache. The .md FILES on disk are never touched.
// Returns the number of chunk rows deleted.
func (s *SQLiteStore) DeleteRawLegacyChunks(ctx context.Context, memoryDir string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("delete raw legacy chunks: store not initialized")
	}

	// Post-cutover the ONLY legitimate chunks are curated ones (importedFileIDPrefix
	// and savedFileIDPrefix; the import marker lives under importedFileIDPrefix too).
	// EVERYTHING else in the chunks table is a RAW copy that IndexMemoryDir wrote
	// from a legacy .md file — regardless of the file_id form it used: a bare
	// basename ("MEMORY.md"), a path-prefixed id ("data/memory/MEMORY.md", which is
	// what production wrote), daily files, or the archive. Those carry NULL lifecycle
	// columns and un-redacted text, so they MUST go. Matching on the curated prefixes
	// (instead of enumerating basenames) is robust to whatever path form was indexed.
	// The memoryDir param is retained for signature stability / future use.
	_ = memoryDir
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT file_id FROM chunks WHERE file_id NOT LIKE ? AND file_id NOT LIKE ?`,
		importedFileIDPrefix+"%", savedFileIDPrefix+"%")
	if err != nil {
		return 0, fmt.Errorf("enumerate raw chunks: %w", err)
	}
	var rawIDs []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			rows.Close()
			return 0, fmt.Errorf("scan raw file_id: %w", scanErr)
		}
		rawIDs = append(rawIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate raw chunks: %w", err)
	}

	total := 0
	for _, id := range rawIDs {
		res, err := s.db.ExecContext(ctx, "DELETE FROM chunks WHERE file_id = ?", id)
		if err != nil {
			return total, fmt.Errorf("delete raw chunks for %q: %w", id, err)
		}
		if n, aerr := res.RowsAffected(); aerr == nil {
			total += int(n)
		}
		// Best-effort: drop the matching files row too.
		if _, err := s.db.ExecContext(ctx, "DELETE FROM files WHERE file_id = ?", id); err != nil {
			return total, fmt.Errorf("delete raw file row for %q: %w", id, err)
		}
		// Evict the raw file's vector-cache entries so they stop surfacing in
		// SearchVector immediately, without a restart.
		s.EvictFromVectorCache(id)
	}
	return total, nil
}

// legacyImportDone reports whether the import marker row exists.
func (s *SQLiteStore) legacyImportDone(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM files WHERE file_id = ?", importMarkerFileID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// markLegacyImportDone writes the sentinel marker row. The hash encodes the run
// stats for forensic visibility; it is never read back for correctness.
func (s *SQLiteStore) markLegacyImportDone(ctx context.Context, stats ImportStats) error {
	marker := fmt.Sprintf("imported=%d;parsed=%d;contradictions=%d;dups=%d;low=%d;at=%s",
		stats.Inserted, stats.EntriesParsed, stats.ContradictionsDropped,
		stats.DuplicatesDropped, stats.LowSignal, time.Now().UTC().Format(time.RFC3339))
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO files (file_id, hash, indexed_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(file_id) DO NOTHING
	`, importMarkerFileID, marker)
	return err
}

// legacyKeywords returns the wing→keyword map the legacy classifier uses. The
// store does not currently carry a hierarchy config field, so this returns nil
// (classifier is a no-op) unless a future story wires real keywords in. Kept as
// a seam so wing classification can be enabled without touching curateEntry.
func (s *SQLiteStore) legacyKeywords() map[string][]string {
	return nil
}

// discoverLegacyFiles returns the absolute paths of the legacy memory files to
// import: MEMORY.md plus every daily 20*.md, directly under memoryDir. A missing
// directory yields an empty slice (not an error) so a fresh install is a no-op.
func discoverLegacyFiles(memoryDir string) ([]string, error) {
	var out []string

	memFile := filepath.Join(memoryDir, MemoryFileName)
	if _, err := os.Stat(memFile); err == nil {
		out = append(out, memFile)
	}

	matches, err := filepath.Glob(filepath.Join(memoryDir, "20*.md"))
	if err != nil {
		return nil, err
	}
	out = append(out, matches...)
	return out, nil
}

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullableTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// importHash returns the hex SHA-256 of s, used for content-dedup keys and the
// per-memory file_id suffix. (The copilot package has a sha256Hex helper but it
// is not importable here without a cycle.)
func importHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
