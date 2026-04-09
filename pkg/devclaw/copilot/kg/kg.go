package kg

import (
	"database/sql"
	"fmt"
	"log/slog"
)

// KG is the entry point for Knowledge Graph operations. It holds a reference
// to the shared SQLite database and a structured logger. All KG tables live in
// the same database file as the memory store (ADR-003).
//
// Construct via NewKG after the KG schema has been migrated. The constructor
// does NOT run DDL — call the memory package's MigrateKgSchema for that.
type KG struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewKG creates a KG instance backed by the given database handle. The caller
// is responsible for ensuring the KG schema has been applied (typically via
// MigrateKgSchema in the memory package).
//
// Both db and logger must be non-nil. Passing nil for either returns an error.
// The logger requirement can be satisfied by passing slog.Default().
func NewKG(db *sql.DB, logger *slog.Logger) (*KG, error) {
	if db == nil {
		return nil, fmt.Errorf("kg: nil db")
	}
	if logger == nil {
		return nil, fmt.Errorf("kg: nil logger")
	}
	return &KG{db: db, logger: logger}, nil
}
