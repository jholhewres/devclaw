package kg

import (
	"context"
	"database/sql"
	"fmt"
)

type Direction int

const (
	Out Direction = iota
	In
	Both
)

type Triple struct {
	TripleID       int64
	SubjectID      int64
	SubjectName    string
	PredicateID    int64
	PredicateName  string
	ObjectText     string
	ObjectEntityID *int64
	ObjectName     string
	ValidFrom      string
	ValidUntil     string
	TxnFrom        string
	TxnUntil       string
	Confidence     float64
	SourceMemoryID string
	Wing           string
	RawText        string
}

type TimelineOpts struct {
	Subject   string
	From      string
	Until     string
	Direction Direction
	Predicate string
	Limit     int
}

func (k *KG) Timeline(ctx context.Context, opts TimelineOpts) ([]Triple, error) {
	entityID, err := k.EnsureEntity(ctx, opts.Subject)
	if err != nil {
		return nil, fmt.Errorf("timeline: resolve subject: %w", err)
	}

	limit := 100
	if opts.Limit > 0 {
		limit = opts.Limit
	}

	directions := []string{}
	switch opts.Direction {
	case Out:
		directions = append(directions, "t.subject_entity_id = ?")
	case In:
		directions = append(directions, "t.object_entity_id = ?")
	case Both:
		directions = append(directions, "t.subject_entity_id = ?")
		directions = append(directions, "t.object_entity_id = ?")
	}

	var unionParts []string
	var allArgs []any

	for _, dirClause := range directions {
		q := `SELECT t.triple_id, t.subject_entity_id, se.canonical_name,
			t.predicate_id, p.name, t.object_text, t.object_entity_id,
			oe.canonical_name, t.valid_from, t.valid_until,
			t.txn_from, t.txn_until, t.confidence, t.source_memory_id,
			t.wing, t.raw_text
		FROM kg_triples t
		JOIN kg_entities se ON se.entity_id = t.subject_entity_id
		JOIN kg_predicates p ON p.predicate_id = t.predicate_id
		LEFT JOIN kg_entities oe ON oe.entity_id = t.object_entity_id
		WHERE ` + dirClause + `
		  AND (t.valid_until IS NULL OR t.valid_until >= ?)
		  AND t.valid_from <= ?
		  AND t.txn_until IS NULL`

		args := []any{entityID, opts.From, opts.Until}

		if opts.Predicate != "" {
			q += ` AND t.predicate_id = (SELECT predicate_id FROM kg_predicates WHERE name = ?)`
			args = append(args, opts.Predicate)
		}

		unionParts = append(unionParts, q)
		allArgs = append(allArgs, args...)
	}

	fullQuery := unionParts[0]
	for i := 1; i < len(unionParts); i++ {
		fullQuery += " UNION ALL " + unionParts[i]
	}
	fullQuery += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := k.db.QueryContext(ctx, fullQuery, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("timeline query: %w", err)
	}
	defer rows.Close()

	return scanTriples(rows)
}

func (k *KG) QueryEntity(ctx context.Context, entityName string, dir Direction) ([]Triple, error) {
	return k.Timeline(ctx, TimelineOpts{
		Subject:   entityName,
		Direction: dir,
		From:      "0001-01-01T00:00:00Z",
		Until:     "9999-12-31T23:59:59Z",
	})
}

func (k *KG) CurrentFacts(ctx context.Context, subjectName string) ([]Triple, error) {
	entityID, err := k.EnsureEntity(ctx, subjectName)
	if err != nil {
		return nil, fmt.Errorf("current facts: resolve entity: %w", err)
	}

	const query = `SELECT t.triple_id, t.subject_entity_id, se.canonical_name,
		t.predicate_id, p.name, t.object_text, t.object_entity_id,
		oe.canonical_name, t.valid_from, t.valid_until,
		t.txn_from, t.txn_until, t.confidence, t.source_memory_id,
		t.wing, t.raw_text
	FROM kg_triples t
	JOIN kg_entities se ON se.entity_id = t.subject_entity_id
	JOIN kg_predicates p ON p.predicate_id = t.predicate_id
	LEFT JOIN kg_entities oe ON oe.entity_id = t.object_entity_id
	WHERE t.subject_entity_id = ?
	  AND t.valid_until IS NULL
	  AND t.txn_until IS NULL`

	rows, err := k.db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("current facts query: %w", err)
	}
	defer rows.Close()

	return scanTriples(rows)
}

func scanTriples(rows *sql.Rows) ([]Triple, error) {
	var triples []Triple
	for rows.Next() {
		var tr Triple
		var validUntil, txnUntil, wing, rawText sql.NullString
		var objEntityID sql.NullInt64
		var objName sql.NullString

		err := rows.Scan(
			&tr.TripleID, &tr.SubjectID, &tr.SubjectName,
			&tr.PredicateID, &tr.PredicateName, &tr.ObjectText, &objEntityID,
			&objName, &tr.ValidFrom, &validUntil,
			&tr.TxnFrom, &txnUntil, &tr.Confidence, &tr.SourceMemoryID,
			&wing, &rawText,
		)
		if err != nil {
			return nil, fmt.Errorf("scan triple: %w", err)
		}

		if objEntityID.Valid {
			tr.ObjectEntityID = &objEntityID.Int64
		}
		if objName.Valid {
			tr.ObjectName = objName.String
		}
		if validUntil.Valid {
			tr.ValidUntil = validUntil.String
		}
		if txnUntil.Valid {
			tr.TxnUntil = txnUntil.String
		}
		if wing.Valid {
			tr.Wing = wing.String
		}
		if rawText.Valid {
			tr.RawText = rawText.String
		}

		triples = append(triples, tr)
	}
	return triples, rows.Err()
}
