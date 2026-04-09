package kg

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
)

type KG struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewKG(db *sql.DB, logger *slog.Logger) (*KG, error) {
	if db == nil {
		return nil, fmt.Errorf("kg: nil db")
	}
	if logger == nil {
		return nil, fmt.Errorf("kg: nil logger")
	}
	return &KG{db: db, logger: logger}, nil
}

func (k *KG) EnsureEntity(ctx context.Context, name string) (int64, error) {
	var entityID int64
	err := k.db.QueryRowContext(ctx,
		"SELECT entity_id FROM kg_entities WHERE canonical_name = ?",
		name,
	).Scan(&entityID)
	if err == nil {
		return entityID, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("ensure entity select: %w", err)
	}

	aliasID, err := k.ResolveAlias(ctx, name)
	if err == nil {
		return aliasID, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("ensure entity alias: %w", err)
	}

	result, err := k.db.ExecContext(ctx,
		"INSERT INTO kg_entities (canonical_name) VALUES (?)",
		name,
	)
	if err != nil {
		return 0, fmt.Errorf("ensure entity insert: %w", err)
	}

	entityID, err = result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("ensure entity lastinsertid: %w", err)
	}

	if err := k.AutoRegisterAlias(ctx, entityID, name); err != nil {
		return 0, fmt.Errorf("ensure entity aliases: %w", err)
	}

	return entityID, nil
}

func (k *KG) EnsurePredicate(ctx context.Context, name string, isFunctional bool, description string) (int64, error) {
	var predicateID int64
	err := k.db.QueryRowContext(ctx,
		"SELECT predicate_id FROM kg_predicates WHERE name = ?",
		name,
	).Scan(&predicateID)
	if err == nil {
		return predicateID, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("ensure predicate select: %w", err)
	}

	var fnFlag int
	if isFunctional {
		fnFlag = 1
	}

	result, err := k.db.ExecContext(ctx,
		"INSERT INTO kg_predicates (name, is_functional, description) VALUES (?, ?, ?)",
		name, fnFlag, description,
	)
	if err != nil {
		return 0, fmt.Errorf("ensure predicate insert: %w", err)
	}

	predicateID, err = result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("ensure predicate lastinsertid: %w", err)
	}

	return predicateID, nil
}

type TripleOpts struct {
	ObjectEntityName string
	Confidence       float64
	SourceMemoryID   string
	Wing             string
	RawText          string
	ValidFrom        string
}

func (k *KG) AddTriple(ctx context.Context, subjectName, predicateName, objectText string, opts TripleOpts) (int64, error) {
	tx, err := k.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("add triple begin: %w", err)
	}
	defer tx.Rollback()

	subjectID, err := ensureEntityInTx(ctx, tx, subjectName)
	if err != nil {
		return 0, fmt.Errorf("add triple subject: %w", err)
	}

	var objectEntityID *int64
	if opts.ObjectEntityName != "" {
		oid, err := ensureEntityInTx(ctx, tx, opts.ObjectEntityName)
		if err != nil {
			return 0, fmt.Errorf("add triple object entity: %w", err)
		}
		objectEntityID = &oid
	}

	predicateID, err := ensurePredicateInTx(ctx, tx, predicateName)
	if err != nil {
		return 0, fmt.Errorf("add triple predicate: %w", err)
	}

	var isFunctional bool
	err = tx.QueryRowContext(ctx,
		"SELECT is_functional FROM kg_predicates WHERE predicate_id = ?",
		predicateID,
	).Scan(&isFunctional)
	if err != nil {
		return 0, fmt.Errorf("add triple check functional: %w", err)
	}

	if isFunctional {
		_, err = tx.ExecContext(ctx,
			"UPDATE kg_triples SET valid_until = CURRENT_TIMESTAMP WHERE subject_entity_id = ? AND predicate_id = ? AND valid_until IS NULL",
			subjectID, predicateID,
		)
		if err != nil {
			return 0, fmt.Errorf("add triple invalidate old: %w", err)
		}
	}

	confidence := opts.Confidence
	if confidence == 0 {
		confidence = 0.5
	}

	var wingArg any
	if opts.Wing == "" {
		wingArg = nil
	} else {
		wingArg = opts.Wing
	}

	var rawTextArg any
	if opts.RawText == "" {
		rawTextArg = nil
	} else {
		rawTextArg = opts.RawText
	}

	var objEntityIDArg any
	if objectEntityID != nil {
		objEntityIDArg = *objectEntityID
	} else {
		objEntityIDArg = nil
	}

	var result sql.Result
	if opts.ValidFrom != "" {
		result, err = tx.ExecContext(ctx,
			`INSERT INTO kg_triples (subject_entity_id, predicate_id, object_text, object_entity_id, confidence, source_memory_id, wing, raw_text, valid_from)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			subjectID, predicateID, objectText, objEntityIDArg, confidence, opts.SourceMemoryID, wingArg, rawTextArg, opts.ValidFrom,
		)
	} else {
		result, err = tx.ExecContext(ctx,
			`INSERT INTO kg_triples (subject_entity_id, predicate_id, object_text, object_entity_id, confidence, source_memory_id, wing, raw_text)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			subjectID, predicateID, objectText, objEntityIDArg, confidence, opts.SourceMemoryID, wingArg, rawTextArg,
		)
	}
	if err != nil {
		return 0, fmt.Errorf("add triple insert: %w", err)
	}

	tripleID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("add triple lastinsertid: %w", err)
	}

	if err := autoRegisterAliasInTx(ctx, tx, subjectID, subjectName); err != nil {
		return 0, fmt.Errorf("add triple subject alias: %w", err)
	}

	if opts.ObjectEntityName != "" && objectEntityID != nil {
		if err := autoRegisterAliasInTx(ctx, tx, *objectEntityID, opts.ObjectEntityName); err != nil {
			return 0, fmt.Errorf("add triple object alias: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("add triple commit: %w", err)
	}

	return tripleID, nil
}

func (k *KG) InvalidateTriple(ctx context.Context, tripleID int64) error {
	result, err := k.db.ExecContext(ctx,
		"UPDATE kg_triples SET valid_until = CURRENT_TIMESTAMP WHERE triple_id = ? AND valid_until IS NULL",
		tripleID,
	)
	if err != nil {
		return fmt.Errorf("invalidate triple: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("invalidate triple rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("invalidate triple: triple %d not found or already invalidated", tripleID)
	}
	return nil
}

func (k *KG) DeleteEntity(ctx context.Context, entityName string) error {
	entityID, err := k.EnsureEntity(ctx, entityName)
	if err != nil {
		return fmt.Errorf("delete entity resolve: %w", err)
	}

	tx, err := k.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete entity begin: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		"UPDATE kg_triples SET txn_until = CURRENT_TIMESTAMP WHERE subject_entity_id = ? AND txn_until IS NULL",
		entityID,
	)
	if err != nil {
		return fmt.Errorf("delete entity subject triples: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		"UPDATE kg_triples SET txn_until = CURRENT_TIMESTAMP WHERE object_entity_id = ? AND txn_until IS NULL",
		entityID,
	)
	if err != nil {
		return fmt.Errorf("delete entity object triples: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		"DELETE FROM kg_triples WHERE (subject_entity_id = ? OR object_entity_id = ?) AND txn_until IS NOT NULL",
		entityID, entityID,
	)
	if err != nil {
		return fmt.Errorf("delete entity remove triple rows: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		"DELETE FROM kg_entity_aliases WHERE entity_id = ?",
		entityID,
	)
	if err != nil {
		return fmt.Errorf("delete entity aliases: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		"DELETE FROM kg_entities WHERE entity_id = ?",
		entityID,
	)
	if err != nil {
		return fmt.Errorf("delete entity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete entity commit: %w", err)
	}

	return nil
}

func ensureEntityInTx(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	var entityID int64
	err := tx.QueryRowContext(ctx,
		"SELECT entity_id FROM kg_entities WHERE canonical_name = ?",
		name,
	).Scan(&entityID)
	if err == nil {
		return entityID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	normalized := normalizeAlias(name)
	err = tx.QueryRowContext(ctx,
		"SELECT entity_id FROM kg_entity_aliases WHERE alias_name = ?",
		normalized,
	).Scan(&entityID)
	if err == nil {
		return entityID, nil
	}

	result, err := tx.ExecContext(ctx,
		"INSERT INTO kg_entities (canonical_name) VALUES (?)",
		name,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func ensurePredicateInTx(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	var predicateID int64
	err := tx.QueryRowContext(ctx,
		"SELECT predicate_id FROM kg_predicates WHERE name = ?",
		name,
	).Scan(&predicateID)
	if err == nil {
		return predicateID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	result, err := tx.ExecContext(ctx,
		"INSERT INTO kg_predicates (name, is_functional, description) VALUES (?, 0, '')",
		name,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func autoRegisterAliasInTx(ctx context.Context, tx *sql.Tx, entityID int64, canonicalName string) error {
	normalized := normalizeAlias(canonicalName)

	_, err := tx.ExecContext(ctx,
		"INSERT OR IGNORE INTO kg_entity_aliases (entity_id, alias_name) VALUES (?, ?)",
		entityID, strings.TrimSpace(strings.ToLower(canonicalName)),
	)
	if err != nil {
		return err
	}

	lowered := strings.TrimSpace(strings.ToLower(canonicalName))
	if normalized != lowered {
		_, err = tx.ExecContext(ctx,
			"INSERT OR IGNORE INTO kg_entity_aliases (entity_id, alias_name) VALUES (?, ?)",
			entityID, normalized,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
