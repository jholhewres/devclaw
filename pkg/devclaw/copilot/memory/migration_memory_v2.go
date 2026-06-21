package memory

import (
	"database/sql"
	"fmt"
	"log/slog"
)

// memoryV2SchemaVersion is the PRAGMA user_version value that gates the
// schema-v2 lifecycle migration. Once user_version >= this value the
// migration is a no-op.
//
// SCHEMA-VERSION REGISTRY (PRAGMA user_version is a single DB-level int shared
// across the whole memory.db — coordinate all future schema migrations here):
//
//	1 = (reserved / pre-v2 baseline)
//	2 = MigrateMemoryV2 — lifecycle metadata columns on chunks
//	3 = MigrateMemoryV2 — occurred_at column (original-event timestamp) + index
//	4 = BackfillOccurredAt — restamp occurred_at from .md (US-002 self-heal;
//	    owned by migration_backfill_occurred.go, NOT MigrateMemoryV2)
//
// A future migration MUST claim the next integer (5, 6, …) and gate on it the
// same way; do not reuse a value owned above.
const memoryV2SchemaVersion = 3

// memoryV2Column describes a single additive column on the chunks table.
type memoryV2Column struct {
	name string
	ddl  string
}

// memoryV2Columns are the lifecycle metadata columns added to the chunks
// table by schema v2. SQLite has no ADD COLUMN IF NOT EXISTS, so each is
// added only when missing (see MigrateMemoryV2).
var memoryV2Columns = []memoryV2Column{
	{name: "deleted_at", ddl: "ALTER TABLE chunks ADD COLUMN deleted_at DATETIME"},
	{name: "expires_at", ddl: "ALTER TABLE chunks ADD COLUMN expires_at DATETIME"},
	{name: "supersedes", ddl: "ALTER TABLE chunks ADD COLUMN supersedes TEXT"},
	{name: "curation_status", ddl: "ALTER TABLE chunks ADD COLUMN curation_status TEXT"},
	{name: "curation_rule", ddl: "ALTER TABLE chunks ADD COLUMN curation_rule TEXT"},
	{name: "importance", ddl: "ALTER TABLE chunks ADD COLUMN importance REAL"},
	{name: "confidence", ddl: "ALTER TABLE chunks ADD COLUMN confidence REAL"},
	{name: "memory_type", ddl: "ALTER TABLE chunks ADD COLUMN memory_type TEXT"},
	{name: "kind", ddl: "ALTER TABLE chunks ADD COLUMN kind TEXT"},
	{name: "scope", ddl: "ALTER TABLE chunks ADD COLUMN scope TEXT"},
	{name: "injected_count", ddl: "ALTER TABLE chunks ADD COLUMN injected_count INTEGER DEFAULT 0"},
	{name: "used_count", ddl: "ALTER TABLE chunks ADD COLUMN used_count INTEGER DEFAULT 0"},
	{name: "last_used_at", ddl: "ALTER TABLE chunks ADD COLUMN last_used_at DATETIME"},
	{name: "scorer_version", ddl: "ALTER TABLE chunks ADD COLUMN scorer_version INTEGER DEFAULT 0"},
	// v1.22.2 (schema v3): the memory's ORIGINAL event timestamp, preserved on
	// write so temporal recall ("what happened Thursday") can query the real
	// date rather than the import/save date carried by created_at.
	{name: "occurred_at", ddl: "ALTER TABLE chunks ADD COLUMN occurred_at DATETIME"},
}

// memoryV2Indexes back the read-side lifecycle filtering wired in later
// stories (recall / injection). Created IF NOT EXISTS so they are idempotent.
var memoryV2Indexes = []string{
	"CREATE INDEX IF NOT EXISTS idx_chunks_deleted_at ON chunks(deleted_at)",
	"CREATE INDEX IF NOT EXISTS idx_chunks_expires_at ON chunks(expires_at)",
	"CREATE INDEX IF NOT EXISTS idx_chunks_curation ON chunks(curation_status)",
	"CREATE INDEX IF NOT EXISTS idx_chunks_occurred_at ON chunks(occurred_at)",
}

// MigrateMemoryV2 adds the schema-v2 lifecycle metadata columns to the chunks
// table and creates the supporting indexes. It is idempotent and version-gated
// via PRAGMA user_version: once user_version >= 2 it returns immediately as a
// no-op. On the first real run it sets user_version = 2 and emits a single INFO
// log; subsequent runs stay silent.
//
// Follows the same non-fatal policy as MigrateKgSchema: the caller logs a
// warning but never blocks startup.
func MigrateMemoryV2(db *sql.DB, logger *slog.Logger) error {
	if db == nil {
		return fmt.Errorf("migrate memory v2: nil db")
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Version gate — skip entirely if already at v2 or beyond.
	var userVersion int
	if err := db.QueryRow("PRAGMA user_version").Scan(&userVersion); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if userVersion >= memoryV2SchemaVersion {
		return nil
	}

	// Read existing columns; SQLite lacks ADD COLUMN IF NOT EXISTS.
	existing, err := memoryV2ExistingColumns(db)
	if err != nil {
		return fmt.Errorf("inspect chunks columns: %w", err)
	}

	for _, col := range memoryV2Columns {
		if existing[col.name] {
			continue
		}
		if _, err := db.Exec(col.ddl); err != nil {
			return fmt.Errorf("add column %s: %w", col.name, err)
		}
	}

	for _, idx := range memoryV2Indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("create lifecycle index: %w", err)
		}
	}

	// SQLite PRAGMA statements do NOT accept ? placeholders — a parameterized
	// "PRAGMA user_version = ?" silently does nothing. Sprintf is intentional
	// and safe here: memoryV2SchemaVersion is a package const, never user input.
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", memoryV2SchemaVersion)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}

	logger.Info("memory schema v2: lifecycle columns added")
	return nil
}

// memoryV2ExistingColumns returns the set of column names currently present on
// the chunks table via PRAGMA table_info.
func memoryV2ExistingColumns(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(chunks)")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return existing, nil
}

// downMemoryV2 is the reversible counterpart of MigrateMemoryV2. SQLite cannot
// cheaply drop columns, so this only drops the lifecycle indexes and resets
// user_version to 0; the additive columns themselves are left in place (they
// are harmless when unused). It is intentionally unexported and never called
// from production code.
//
//nolint:unused // retained for symmetry with downKgSchema; wired only from manual tools.
func downMemoryV2(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("down memory v2: nil db")
	}
	drops := []string{
		"DROP INDEX IF EXISTS idx_chunks_deleted_at",
		"DROP INDEX IF EXISTS idx_chunks_expires_at",
		"DROP INDEX IF EXISTS idx_chunks_curation",
		"DROP INDEX IF EXISTS idx_chunks_occurred_at",
	}
	for _, stmt := range drops {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("drop lifecycle index: %w", err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 0"); err != nil {
		return fmt.Errorf("reset user_version: %w", err)
	}
	return nil
}
