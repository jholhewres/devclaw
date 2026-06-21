// Package memory — migration_backfill_occurred.go implements the v1.22.2 (US-002)
// boot-time self-heal that restamps the chunks.occurred_at column for memories
// that were imported BEFORE US-001 added occurred_at.
//
// Why this exists: US-001 added the occurred_at column (schema v3) and now
// preserves Entry.Timestamp on NEW imports. But EXISTING production stores had
// already run the legacy import, so every imported chunk was stamped with the
// migration date (created_at / NOW), not the memory's real original date. Those
// rows cannot be fixed by re-importing (the import is idempotent and won't run
// again), so this pass re-reads the untouched .md files, matches each parsed
// entry to its already-imported chunk by the SAME content-hash file_id the
// import used, and backfills occurred_at with the real original timestamp.
//
// Properties (mirrors RecurateLowSignal):
//   - Boot-triggered: wired in the import startup goroutine in assistant.go (after
//     ImportLegacyMarkdown, so imported chunks exist and occurred_at is present).
//   - Version-gated via PRAGMA user_version: claims schema version 4. Once
//     user_version >= 4 the pass is a no-op (and MigrateMemoryV2's gate at >= 3
//     still holds, since 4 >= 3). The version is bumped to 4 only AFTER a fully
//     successful pass; on error it stays unset so the next boot retries.
//   - Idempotent: a chunk already carrying the correct original date is skipped;
//     a second run updates 0 rows.
//   - Fail-open: any error is returned for the caller to log; startup never blocks.
//   - .md files are READ ONLY (os.ReadFile only — never written back).
package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// backfillOccurredAtVersion is the PRAGMA user_version this self-heal claims.
// It is one past memoryV2SchemaVersion (3, owned by MigrateMemoryV2). See the
// SCHEMA-VERSION REGISTRY in migration_memory_v2.go.
const backfillOccurredAtVersion = 4

// BackfillOccurredAt re-reads the legacy .md files under memoryDir and restamps
// chunks.occurred_at for already-imported memories whose occurred_at was set to
// the migration date (because their import predated US-001). It matches each
// parsed entry to its imported chunk by reproducing the import's keying EXACTLY
// (importHash(strings.TrimSpace(curateEntry(e, now).text)) → file_id =
// importedFileIDPrefix+key), then updates occurred_at to the entry's original
// [YYYY-MM-DD HH:MM] timestamp.
//
// It is version-gated (PRAGMA user_version >= 4 → no-op), idempotent (a chunk
// already stamped with the correct date is not re-updated), and fail-open. The
// .md files are read-only here. Returns the number of chunk rows updated.
func (s *SQLiteStore) BackfillOccurredAt(ctx context.Context, memoryDir string, logger *slog.Logger) (updated int, err error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("backfill occurred_at: nil store")
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Version gate — once we (or a later migration) advanced user_version to >= 4
	// this pass is permanently a no-op.
	var userVersion int
	if vErr := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVersion); vErr != nil {
		return 0, fmt.Errorf("read user_version: %w", vErr)
	}
	if userVersion >= backfillOccurredAtVersion {
		return 0, nil
	}

	files, err := discoverLegacyFiles(memoryDir)
	if err != nil {
		return 0, fmt.Errorf("discover legacy files: %w", err)
	}

	// Re-derive each entry's import key and its original timestamp, mirroring the
	// import pipeline (curateEntry → importHash(TrimSpace(curated.text))) so the
	// computed file_id matches the row insertCuratedChunk wrote. now is only used
	// by curateEntry for expiry/fallback derivation; it does not affect the key.
	// now is used by curateEntry only for deriveExpiry; that result is discarded —
	// the backfill only needs curated.text for the hash key. curateEntry never
	// drops an entry based on expiry, so a slightly stale now is safe.
	now := time.Now().UTC()
	for _, path := range files {
		data, readErr := os.ReadFile(path) // READ ONLY — never written back.
		if readErr != nil {
			logger.Warn("backfill: read failed", "file", path, "error", readErr)
			continue
		}
		source := filepath.Base(path)
		entries := parseMemoryFile(string(data), source)

		for _, e := range entries {
			// Only entries that carry a real original date can correct a chunk.
			// e.Timestamp is parsed by parseMemoryFile via ParseInLocation(time.Local),
			// so it is a real local instant — we restamp occurred_at with exactly the
			// same value (and location) the import would have stored, keeping the
			// two paths consistent (and consistent with US-003 local date windows).
			if e.Timestamp.IsZero() {
				continue
			}
			// `now` is passed to curateEntry only so deriveExpiry can compute an
			// expires_at — which the backfill DISCARDS. We rely solely on
			// curated.text to recompute the import's content-hash key, so the exact
			// value of `now` here cannot shift the key or mis-target a chunk.
			curated, action := s.curateEntry(e, now)
			if action != curateKeep {
				continue
			}
			key := importHash(strings.TrimSpace(curated.text))
			fileID := importedFileIDPrefix + key

			n, upErr := s.restampOccurredAt(ctx, fileID, e.Timestamp)
			if upErr != nil {
				return updated, fmt.Errorf("restamp %s: %w", fileID, upErr)
			}
			updated += n
		}
	}

	// Claim version 4 only after a fully successful pass. SQLite PRAGMA does not
	// accept ? placeholders; the value is a package const, never user input.
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", backfillOccurredAtVersion)); err != nil {
		return updated, fmt.Errorf("set user_version: %w", err)
	}

	if updated > 0 {
		logger.Info("memory backfill: restamped occurred_at from .md original dates",
			"updated", updated,
		)
	}
	return updated, nil
}

// restampOccurredAt updates the imported chunk identified by fileID so its
// occurred_at matches want, but ONLY when the current value differs (NULL or a
// different instant). The differ-check is done in Go after reading the stored
// value, which keeps the pass idempotent regardless of how SQLite serializes the
// DATETIME (a SQL `occurred_at != ?` comparison is brittle across storage
// formats). Returns 1 when a row was actually updated, else 0.
func (s *SQLiteStore) restampOccurredAt(ctx context.Context, fileID string, want time.Time) (int, error) {
	var current sql.NullTime
	err := s.db.QueryRowContext(ctx,
		"SELECT occurred_at FROM chunks WHERE file_id = ? AND deleted_at IS NULL", fileID).Scan(&current)
	if err == sql.ErrNoRows {
		// No imported chunk for this entry (e.g. it was dropped as a duplicate,
		// contradiction, or never imported). Nothing to restamp.
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read occurred_at: %w", err)
	}

	// Idempotency: skip when the stored date already equals the original date.
	if current.Valid && current.Time.Equal(want) {
		return 0, nil
	}

	res, err := s.db.ExecContext(ctx,
		"UPDATE chunks SET occurred_at = ? WHERE file_id = ? AND deleted_at IS NULL", want, fileID)
	if err != nil {
		return 0, fmt.Errorf("update occurred_at: %w", err)
	}
	if n, aErr := res.RowsAffected(); aErr == nil && n > 0 {
		return 1, nil
	}
	return 0, nil
}
