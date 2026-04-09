package memory

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg"
)

// MigrateKgSchema installs the Knowledge Graph bitemporal tables (kg_entities,
// kg_entity_aliases, kg_predicates, kg_triples) plus their indexes if they do
// not already exist. Idempotent — safe to run on every startup.
//
// First-run detection checks sqlite_master for kg_entities before executing
// the DDL. On the very first run it emits a single INFO log; subsequent runs
// stay silent. Errors reading sqlite_master are not fatal — the function falls
// through to the CREATE IF NOT EXISTS which is always safe.
//
// Follows the same non-fatal policy as InitHierarchySchema and
// MigrateEssentialStories: the caller logs a warning but never blocks startup.
func MigrateKgSchema(db *sql.DB, logger *slog.Logger) error {
	if db == nil {
		return fmt.Errorf("migrate kg schema: nil db")
	}
	if logger == nil {
		logger = slog.Default()
	}

	existed := true
	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='kg_entities'`,
	).Scan(&name)
	if err != nil {
		existed = false
	}

	if _, err := db.Exec(kg.KgSchema); err != nil {
		return fmt.Errorf("create kg schema: %w", err)
	}

	if !existed {
		logger.Info("kg schema: bitemporal tables created")
	}
	return nil
}

// downKgSchema is the reversible counterpart of MigrateKgSchema.
// It is intentionally unexported and never called from production code — it
// exists so a future cleanup can drop the KG tables if the feature is retired.
//
//nolint:unused // retained for future cleanup; wired only from manual tools.
func downKgSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("down kg schema: nil db")
	}
	if _, err := db.Exec(kg.KgDropSchema); err != nil {
		return fmt.Errorf("drop kg schema: %w", err)
	}
	return nil
}
