// Package memory — migration_essential_stories.go installs the Sprint 2
// Room 2.2 schema for the L1 EssentialLayer: a cache table keyed by wing
// holding pre-rendered per-wing essential stories plus a lookup index on
// generation time.
//
// The migration follows Sprint 1 conventions:
//
//   - ADDITIVE: introduces only a new table and a new index; nothing
//     else in the existing schema is touched.
//   - IDEMPOTENT: uses CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT
//     EXISTS, safe to run on every startup.
//   - REVERSIBLE: a commented-out Down() helper documents the rollback
//     path for Sprint 6 cleanup (kept unused on purpose to avoid any
//     accidental invocation).
//   - RETROCOMPAT: does not touch `files`, `chunks`, or any pre-existing
//     palace table; a legacy v1.17.0 upgrade path is unaffected.
//
// The caller (sqlite_store.go:initSchema) treats errors as non-fatal and
// logs a warning, matching InitHierarchySchema.
package memory

import (
	"database/sql"
	"fmt"
	"log/slog"
)

// essentialStoriesSchemaVersion is the on-disk schema version stored in
// every row of the essential_stories table. Bump this when the row format
// changes in a backward-incompatible way and wire a read-time check here.
const essentialStoriesSchemaVersion = 1

// MigrateEssentialStories installs the essential_stories cache table plus
// its generated_at lookup index if they do not already exist. Idempotent.
//
// First-run detection: the function checks whether the table already
// exists via sqlite_master before running the DDL. On the very first run
// it emits a single INFO log line; subsequent runs stay silent. Errors
// while reading sqlite_master are not fatal — the function falls through
// to the CREATE IF NOT EXISTS which is always safe.
func MigrateEssentialStories(db *sql.DB, logger *slog.Logger) error {
	if db == nil {
		return fmt.Errorf("migrate essential_stories: nil db")
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Pre-check so we can log "created" only on the real first run.
	existed := true
	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='essential_stories'`,
	).Scan(&name)
	if err != nil {
		// sql.ErrNoRows is the normal first-run path; anything else is
		// swallowed — we still try to create below and surface the real
		// error from there.
		existed = false
	}

	schema := `
		CREATE TABLE IF NOT EXISTS essential_stories (
			wing             TEXT NOT NULL,
			story            TEXT NOT NULL,
			generated_at     INTEGER NOT NULL,
			source_files     INTEGER NOT NULL,
			source_rooms     INTEGER NOT NULL,
			bytes            INTEGER NOT NULL,
			schema_version   INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (wing)
		);
		CREATE INDEX IF NOT EXISTS idx_essential_stories_generated
			ON essential_stories(generated_at);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("create essential_stories: %w", err)
	}

	if !existed {
		logger.Info("essential_stories: cache table created",
			"schema_version", essentialStoriesSchemaVersion)
	}
	return nil
}

// downEssentialStories is the reversible counterpart of
// MigrateEssentialStories. It is intentionally unexported and never called
// from production code — it exists so Sprint 6 cleanup can drop the L1
// cache if the feature is retired. Kept as a helper (not a comment) so
// the compiler keeps it honest.
//
//nolint:unused // retained for Sprint 6 cleanup; wired only from manual tools.
func downEssentialStories(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("down essential_stories: nil db")
	}
	// Order matters: drop the index before the table so a partial rollback
	// cannot leave a dangling index entry.
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_essential_stories_generated`); err != nil {
		return fmt.Errorf("drop idx_essential_stories_generated: %w", err)
	}
	if _, err := db.Exec(`DROP TABLE IF EXISTS essential_stories`); err != nil {
		return fmt.Errorf("drop essential_stories: %w", err)
	}
	return nil
}
