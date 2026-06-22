// Package memory — reembed.go re-embeds the whole corpus when the active
// embedding model changes. Stored vectors live in a model-specific space, so a
// model swap (e.g. English → multilingual MiniLM) requires recomputing every
// chunk's embedding. Self-healing: triggered on boot, idempotent, fail-open.
package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const embeddingModelMetaKey = "embedding_model"

func (s *SQLiteStore) ensureMetaTable() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS memory_meta (key TEXT PRIMARY KEY, value TEXT)`)
	return err
}

func (s *SQLiteStore) getMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM memory_meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *SQLiteStore) setMeta(key, val string) error {
	_, err := s.db.Exec(
		`INSERT INTO memory_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, val)
	return err
}

// EnsureEmbeddingModel re-embeds all chunks when the active embedding model
// differs from the one recorded in memory_meta, then records the new model.
// No-op when unchanged. Skips entirely for a null/unavailable embedder so a
// missing ONNX runtime never wipes existing vectors. Returns whether a re-embed
// ran and how many chunks were updated.
func (s *SQLiteStore) EnsureEmbeddingModel(ctx context.Context) (changed bool, updated int, err error) {
	if s.embedder == nil || s.embedder.Dimensions() == 0 {
		return false, 0, nil
	}
	if err := s.ensureMetaTable(); err != nil {
		return false, 0, fmt.Errorf("meta table: %w", err)
	}
	current := s.embedder.Name() + ":" + s.embedder.Model()
	stored, err := s.getMeta(embeddingModelMetaKey)
	if err != nil {
		return false, 0, fmt.Errorf("read marker: %w", err)
	}
	if stored == current {
		return false, 0, nil
	}
	updated, err = s.ReembedAll(ctx)
	if err != nil {
		return false, updated, err
	}
	if err := s.setMeta(embeddingModelMetaKey, current); err != nil {
		return true, updated, fmt.Errorf("write marker: %w", err)
	}
	return true, updated, nil
}

// ReembedAll recomputes and stores the embedding for every live chunk using the
// current embedder, refreshes the in-memory vector cache, and prunes stale
// embedding_cache rows from other models.
func (s *SQLiteStore) ReembedAll(ctx context.Context) (int, error) {
	rows, err := s.db.Query(`SELECT id, text FROM chunks WHERE deleted_at IS NULL`)
	if err != nil {
		return 0, err
	}
	type rec struct {
		id   int64
		text string
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.text); err != nil {
			continue
		}
		recs = append(recs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	const batchSize = 64
	updated := 0
	for i := 0; i < len(recs); i += batchSize {
		end := min(i+batchSize, len(recs))
		texts := make([]string, end-i)
		for j := i; j < end; j++ {
			texts[j-i] = recs[j].text
		}
		vecs, err := s.embedder.Embed(ctx, texts)
		if err != nil {
			return updated, fmt.Errorf("reembed batch at %d: %w", i, err)
		}
		err = sqliteExecWithRetry(ctx, func(c context.Context) error {
			tx, err := s.db.BeginTx(c, nil)
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback() }()
			for j, v := range vecs {
				data, err := json.Marshal(v)
				if err != nil {
					return err
				}
				if _, err := tx.ExecContext(c,
					`UPDATE chunks SET embedding = ? WHERE id = ?`,
					string(data), recs[i+j].id); err != nil {
					return err
				}
			}
			return tx.Commit()
		}, DefaultRetryOpts())
		if err != nil {
			return updated, err
		}
		updated += len(vecs)
		if updated%512 == 0 {
			s.logger.Info("re-embedding progress", "done", updated, "total", len(recs))
		}
	}

	if err := s.loadVectorCache(); err != nil {
		return updated, fmt.Errorf("reload vector cache: %w", err)
	}
	_, _ = s.db.Exec(`DELETE FROM embedding_cache WHERE model != ?`, s.embedder.Model())
	return updated, nil
}
