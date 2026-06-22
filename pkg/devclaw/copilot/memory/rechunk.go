// Package memory — rechunk.go splits already-stored long curated memories into
// atomic facts on boot. The embedding-model swap fixed PT-BR semantics, but a
// rich memory stored as one long chunk still dilutes retrieval for narrow
// queries. This self-heals existing data: marker-gated, idempotent, fail-open.
package memory

import (
	"context"
	"database/sql"
	"strings"
)

const (
	rechunkVersionKey = "rechunk_version"
	rechunkVersion    = "1"
	rechunkMinLen     = 200 // only chunks longer than this are candidates
)

// RechunkLongCuratedMemories splits curated chunks (saved/imported) whose text
// is long and multi-fact into atomic-fact chunks, then soft-deletes the
// original. No-op once the marker is recorded. Returns how many originals were
// split.
func (s *SQLiteStore) RechunkLongCuratedMemories(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if err := s.ensureMetaTable(); err != nil {
		return 0, err
	}
	if v, err := s.getMeta(rechunkVersionKey); err != nil {
		return 0, err
	} else if v == rechunkVersion {
		return 0, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.file_id, c.text, c.memory_type, c.kind, c.scope,
		       c.importance, c.confidence, c.curation_status, c.curation_rule,
		       c.occurred_at, c.expires_at, f.wing
		FROM chunks c
		LEFT JOIN files f ON f.file_id = c.file_id
		WHERE c.deleted_at IS NULL
		  AND (c.file_id LIKE 'memory/saved/%' OR c.file_id LIKE 'memory/imported/%')
		  AND length(c.text) > ?`, rechunkMinLen)
	if err != nil {
		return 0, err
	}

	type longChunk struct {
		id     int64
		prefix string
		ent    curatedEntry
	}
	var targets []longChunk
	for rows.Next() {
		var (
			id                                   int64
			fileID, text                         string
			memType, kind, scope, cStatus, cRule sql.NullString
			wing                                 sql.NullString
			importance, confidence               sql.NullFloat64
			occurredAt, expiresAt                sql.NullTime
		)
		if err := rows.Scan(&id, &fileID, &text, &memType, &kind, &scope,
			&importance, &confidence, &cStatus, &cRule, &occurredAt, &expiresAt, &wing); err != nil {
			continue
		}
		if len(splitAtomicFacts(text)) <= 1 {
			continue
		}
		prefix := savedFileIDPrefix
		if strings.HasPrefix(fileID, importedFileIDPrefix) {
			prefix = importedFileIDPrefix
		}
		ent := curatedEntry{
			memoryType:     memType.String,
			kind:           kind.String,
			scope:          scope.String,
			wing:           wing.String,
			curationStatus: cStatus.String,
			curationRule:   cRule.String,
		}
		if importance.Valid {
			ent.importance = importance.Float64
		}
		if confidence.Valid {
			ent.confidence = confidence.Float64
		}
		if occurredAt.Valid {
			t := occurredAt.Time
			ent.occurredAt = &t
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			ent.expiresAt = &t
		}
		targets = append(targets, longChunk{id: id, prefix: prefix, ent: ent})
		// keep the original text to split below (stored on ent.text temporarily)
		targets[len(targets)-1].ent.text = text
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	split := 0
	for _, tgt := range targets {
		facts := splitAtomicFacts(tgt.ent.text)
		if len(facts) <= 1 {
			continue
		}
		for _, fact := range facts {
			piece := tgt.ent
			piece.text = strings.TrimSpace(fact)
			if piece.text == "" {
				continue
			}
			key := importHash(piece.text)
			if err := s.insertCuratedChunkWithPrefix(ctx, tgt.prefix, key, piece); err != nil {
				return split, err
			}
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE chunks SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?`, tgt.id); err != nil {
			return split, err
		}
		split++
	}

	if err := s.loadVectorCache(); err != nil {
		return split, err
	}
	if err := s.setMeta(rechunkVersionKey, rechunkVersion); err != nil {
		return split, err
	}
	return split, nil
}
