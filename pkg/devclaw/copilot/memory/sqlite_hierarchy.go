// Package memory — sqlite_hierarchy.go adds palace-aware schema to the
// SQLite memory store without disturbing the existing core schema.
//
// Sprint 1 (v1.18.0): additive schema changes per ADR-003 and Sprint 0.5
// doc-02-errata §1.3. All changes are:
//   - ADDITIVE: new columns on existing `files` table (nullable, default NULL)
//   - IDEMPOTENT: safe to run multiple times
//   - RETROCOMPAT: legacy files without wing metadata continue to work
//   - REVERSIBLE: DROP TABLE + ALTER TABLE DROP COLUMN (SQLite 3.35+)
//
// NO BACKFILL of existing rows is performed. This preserves Princípio Zero
// rule 4 ("Backfill nunca automático") and matches ADR-006.
//
// Schema additions:
//
//	files (existing table) gets:
//	   wing             TEXT   DEFAULT NULL   -- ADR-001
//	   hall             TEXT   DEFAULT NULL   -- ADR-001
//	   room             TEXT   DEFAULT NULL   -- ADR-002
//	   session_id       TEXT   DEFAULT NULL   -- ADR-007-v2
//	   access_count     INTEGER DEFAULT 0      -- ADR-005
//	   last_accessed_at DATETIME                -- ADR-005
//	   deleted_at       DATETIME                -- ADR-003 orphan handling
//
//	NEW TABLES:
//	   wings             -- wing registry with display name / metadata
//	   rooms             -- room registry per wing with source/confidence
//	   rooms_archive     -- overflow archive for ADR-002 hard cap
//	   channel_wing_map  -- (channel, external_id) → wing routing table
//
// This file exposes InitHierarchySchema() which is called from the main
// initSchema() method in sqlite_store.go via a single added line.
package memory

import (
	"database/sql"
	"fmt"
	"log/slog"
)

// InitHierarchySchema applies the Sprint 1 palace-aware schema additions
// to an already-initialized devclaw memory database.
//
// This function is idempotent: running it multiple times against the same
// database is safe. It checks for existing columns before ALTER TABLE and
// uses CREATE TABLE IF NOT EXISTS for new tables.
//
// The caller must have already initialized the core schema (files, chunks,
// embedding_cache). This function does NOT create those.
//
// Retrocompat: this function does not touch existing rows. All new columns
// default to NULL (or 0 for counters), preserving v1.17.0 behavior when the
// palace feature flag is off.
func InitHierarchySchema(db *sql.DB, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	// 1. Add columns to `files` table if missing.
	if err := addFileColumnsIfMissing(db, logger); err != nil {
		return fmt.Errorf("add hierarchy columns to files: %w", err)
	}

	// 2. Create palace registry tables.
	if err := createPalaceTables(db); err != nil {
		return fmt.Errorf("create palace tables: %w", err)
	}

	// 3. Create indexes.
	if err := createHierarchyIndexes(db); err != nil {
		return fmt.Errorf("create hierarchy indexes: %w", err)
	}

	logger.Debug("hierarchy schema initialized")
	return nil
}

// hierarchyFileColumns lists the columns Sprint 1 adds to the `files` table.
// Each entry is the ALTER TABLE fragment (name + type + default).
//
// The order matters for backward-compatible reads but SQLite does not enforce
// any particular order. We keep them grouped logically.
var hierarchyFileColumns = []struct {
	Name string
	DDL  string // "TYPE [DEFAULT ...]"
}{
	{"wing", "TEXT DEFAULT NULL"},
	{"hall", "TEXT DEFAULT NULL"},
	{"room", "TEXT DEFAULT NULL"},
	{"session_id", "TEXT DEFAULT NULL"},
	{"access_count", "INTEGER DEFAULT 0"},
	{"last_accessed_at", "DATETIME"},
	{"deleted_at", "DATETIME"},
}

// addFileColumnsIfMissing queries PRAGMA table_info(files) to find the
// existing columns, then issues ALTER TABLE ADD COLUMN for any hierarchy
// column not already present.
//
// SQLite does not support "ALTER TABLE ... ADD COLUMN IF NOT EXISTS" so we
// must introspect first. This is the idiomatic way.
func addFileColumnsIfMissing(db *sql.DB, logger *slog.Logger) error {
	existing, err := existingColumns(db, "files")
	if err != nil {
		return err
	}

	for _, col := range hierarchyFileColumns {
		if _, present := existing[col.Name]; present {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE files ADD COLUMN %s %s", col.Name, col.DDL)
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("add column %s: %w", col.Name, err)
		}
		logger.Info("hierarchy: added column to files table", "column", col.Name)
	}
	return nil
}

// existingColumns returns the set of column names for a table using
// PRAGMA table_info, which is the SQLite way to introspect schema.
func existingColumns(db *sql.DB, table string) (map[string]struct{}, error) {
	// #nosec G202 — table name is a hardcoded constant from this package,
	// not user input.
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()

	cols := make(map[string]struct{})
	for rows.Next() {
		var (
			cid      int
			name     string
			ctype    string
			notnull  int
			dfltVal  sql.NullString
			isPK     int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltVal, &isPK); err != nil {
			return nil, fmt.Errorf("scan pragma row: %w", err)
		}
		cols[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cols, nil
}

// createPalaceTables creates the four new tables introduced by Sprint 1.
// All use CREATE TABLE IF NOT EXISTS so this function is idempotent.
func createPalaceTables(db *sql.DB) error {
	schema := `
		-- Wing registry: named namespaces that organize memories into
		-- contextual buckets (work, personal, family, ...). Created lazily
		-- on first use. Users can delete unused wings via CLI.
		CREATE TABLE IF NOT EXISTS wings (
			name         TEXT PRIMARY KEY,
			display_name TEXT,
			description  TEXT,
			color        TEXT,
			is_default   INTEGER DEFAULT 0,
			is_suggested INTEGER DEFAULT 0,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		-- Room registry: fine-grained topical groupings inside a wing.
		-- Source tracks how the room was created (manual, auto extraction,
		-- inferred from content, or legacy backfill). Confidence reflects
		-- the extraction system's certainty for auto/inferred rooms.
		-- Reuse count feeds the ADR-002 Addendum A confidence promotion rule.
		CREATE TABLE IF NOT EXISTS rooms (
			wing         TEXT NOT NULL,
			name         TEXT NOT NULL,
			hall         TEXT,
			source       TEXT NOT NULL DEFAULT 'manual'
				CHECK(source IN ('manual', 'auto', 'inferred', 'legacy')),
			confidence   REAL NOT NULL DEFAULT 1.0,
			reuse_count  INTEGER NOT NULL DEFAULT 0,
			memory_count INTEGER NOT NULL DEFAULT 0,
			display_name TEXT,
			description  TEXT,
			last_activity DATETIME,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (wing, name)
		);

		-- Rooms archive: when a wing exceeds the auto-room hard cap
		-- (ADR-002 Addendum A), the least recently used auto-only rooms
		-- are moved here. Preserved for audit; not considered by search.
		CREATE TABLE IF NOT EXISTS rooms_archive (
			wing                  TEXT NOT NULL,
			name                  TEXT NOT NULL,
			source                TEXT NOT NULL,
			original_memory_count INTEGER,
			reason                TEXT,  -- 'cap_hit' | 'user_delete' | 'merge_target'
			archived_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (wing, name, archived_at)
		);

		-- Channel → wing routing: given a (channel, external_id) pair like
		-- ("telegram", "123456"), which wing should the context router place
		-- new memories in?  Confidence distinguishes manual mappings (1.0)
		-- from heuristic suggestions (<1.0). Source tracks how the mapping
		-- was established so we can surface low-confidence entries for
		-- user review.
		CREATE TABLE IF NOT EXISTS channel_wing_map (
			channel     TEXT NOT NULL,
			external_id TEXT NOT NULL,
			wing        TEXT NOT NULL,
			confidence  REAL NOT NULL DEFAULT 1.0,
			source      TEXT NOT NULL,  -- 'manual' | 'heuristic' | 'llm' | 'inherited'
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (channel, external_id)
		);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	return nil
}

// createHierarchyIndexes creates partial indexes that accelerate the
// palace-aware queries without penalizing legacy rows.
//
// All indexes are partial (WHERE ... IS NOT NULL) so legacy rows with
// wing=NULL do not bloat the index. This keeps cost proportional to actual
// palace usage.
func createHierarchyIndexes(db *sql.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_files_wing
			ON files(wing) WHERE wing IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_files_wing_room
			ON files(wing, room) WHERE wing IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_files_session_wing
			ON files(session_id, wing) WHERE session_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_files_last_accessed
			ON files(last_accessed_at) WHERE last_accessed_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_files_deleted
			ON files(deleted_at) WHERE deleted_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_rooms_source
			ON rooms(source, confidence)`,
		`CREATE INDEX IF NOT EXISTS idx_channel_wing_confidence
			ON channel_wing_map(wing, confidence)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}
	return nil
}

// SeedSuggestedWings populates the `wings` table with the default suggestion
// list (work, personal, family, finance, learning, health) using
// INSERT OR IGNORE so it is safe to call multiple times and never overwrites
// user customizations.
//
// This is called at startup only when the `wings` table is empty — so new
// users get a starter palette while existing databases are untouched.
//
// Nothing about this is enforcing: users are free to delete these or create
// completely different wings. They exist purely for discoverability.
func SeedSuggestedWings(db *sql.DB) error {
	// Skip seeding if the table already has any rows.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM wings").Scan(&count); err != nil {
		return fmt.Errorf("count wings: %w", err)
	}
	if count > 0 {
		return nil
	}

	stmt, err := db.Prepare(`
		INSERT OR IGNORE INTO wings (name, display_name, is_suggested)
		VALUES (?, ?, 1)
	`)
	if err != nil {
		return fmt.Errorf("prepare seed: %w", err)
	}
	defer stmt.Close()

	suggestions := []struct {
		Name, Display string
	}{
		{"work", "Work"},
		{"personal", "Personal"},
		{"family", "Family"},
		{"finance", "Finance"},
		{"learning", "Learning"},
		{"health", "Health"},
	}

	for _, s := range suggestions {
		if _, err := stmt.Exec(s.Name, s.Display); err != nil {
			return fmt.Errorf("insert suggested wing %q: %w", s.Name, err)
		}
	}
	return nil
}
