package memory

import (
	"context"
	"fmt"
	"log/slog"
)

// recurateBatchSize bounds how many sub-version chunks are pulled and rescored
// per round trip. Re-scoring is pure text scoring (no embeddings), so this is
// only a memory/transaction-size guard, not a latency knob.
const recurateBatchSize = 500

// RecurateLowSignal re-scores every chunk whose scorer_version is below the
// current QualityScorerVersion using the recalibrated quality scorer, then
// updates curation_status / curation_rule / scorer_version accordingly.
//
// This is the boot-time self-heal for the v1.22.1 recalibration: chunks that
// the old (miscalibrated) scorer demoted to curation_status='low_signal' are
// re-judged, and genuine facts/decisions/events are promoted back to recallable
// (curation_status cleared) while real noise stays low_signal. Nothing is ever
// deleted.
//
// Properties:
//   - Version-gated: only chunks with scorer_version < QualityScorerVersion are
//     touched; each rescored chunk is stamped with the current version, so a
//     second run rescans nothing (idempotent — returns rescored=0).
//   - Fast: pure text scoring, no embeddings, batched — safe to run
//     synchronously at startup.
//   - Fail-open: any error is returned to the caller (which logs a warning and
//     never blocks startup); it never panics.
//
// Returns the number of chunks rescored on this invocation.
func (s *SQLiteStore) RecurateLowSignal(ctx context.Context, logger *slog.Logger) (rescored int, err error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("recurate low_signal: nil store")
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Detect whether the chunks table carries a "wing" column (added by the
	// hierarchy migration). It contributes to the hasScope signal alongside
	// scope. There is no "pinned" column on chunks, so pinned is always false.
	cols, err := memoryV2ExistingColumns(s.db)
	if err != nil {
		return 0, fmt.Errorf("inspect chunks columns: %w", err)
	}
	hasWingCol := cols["wing"]

	wingSelect := "'' AS wing"
	if hasWingCol {
		wingSelect = "COALESCE(c.wing, '') AS wing"
	}

	selectSQL := fmt.Sprintf(`
		SELECT c.id, c.text, COALESCE(c.kind, ''), COALESCE(c.scope, ''), %s
		FROM chunks c
		WHERE c.scorer_version IS NULL OR c.scorer_version < ?
		LIMIT ?
	`, wingSelect)

	for {
		select {
		case <-ctx.Done():
			return rescored, ctx.Err()
		default:
		}

		rows, qErr := s.db.QueryContext(ctx, selectSQL, QualityScorerVersion, recurateBatchSize)
		if qErr != nil {
			return rescored, fmt.Errorf("select sub-version chunks: %w", qErr)
		}

		type pending struct {
			id     int64
			status string // "" when it now passes
			rule   string
		}
		var batch []pending
		for rows.Next() {
			var (
				id                int64
				text, kind, scope string
				wing              string
			)
			if scanErr := rows.Scan(&id, &text, &kind, &scope, &wing); scanErr != nil {
				rows.Close()
				return rescored, fmt.Errorf("scan chunk for recuration: %w", scanErr)
			}
			hasScope := scope != "" || wing != ""
			v := ClassifyQuality(text, kind, hasScope, false)
			batch = append(batch, pending{id: id, status: v.CurationStatus, rule: v.CurationRule})
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return rescored, fmt.Errorf("iterate chunks for recuration: %w", rowsErr)
		}
		rows.Close()

		if len(batch) == 0 {
			break
		}

		tx, txErr := s.db.BeginTx(ctx, nil)
		if txErr != nil {
			return rescored, fmt.Errorf("begin recuration tx: %w", txErr)
		}
		stmt, prepErr := tx.PrepareContext(ctx, `
			UPDATE chunks
			SET curation_status = ?, curation_rule = ?, scorer_version = ?
			WHERE id = ?
		`)
		if prepErr != nil {
			_ = tx.Rollback()
			return rescored, fmt.Errorf("prepare recuration update: %w", prepErr)
		}

		for _, p := range batch {
			var status, rule any
			if p.status == "" {
				// Clear curation when the chunk now passes so recall returns it.
				status = nil
				rule = nil
			} else {
				status = p.status
				rule = p.rule
			}
			if _, execErr := stmt.ExecContext(ctx, status, rule, QualityScorerVersion, p.id); execErr != nil {
				stmt.Close()
				_ = tx.Rollback()
				return rescored, fmt.Errorf("update chunk %d: %w", p.id, execErr)
			}
		}
		stmt.Close()
		if commitErr := tx.Commit(); commitErr != nil {
			return rescored, fmt.Errorf("commit recuration batch: %w", commitErr)
		}

		rescored += len(batch)

		// A short batch means we drained the remaining sub-version rows.
		if len(batch) < recurateBatchSize {
			break
		}
	}

	if rescored > 0 {
		logger.Info("memory recuration: rescored chunks with recalibrated scorer",
			"rescored", rescored,
			"scorer_version", QualityScorerVersion,
		)
	}
	return rescored, nil
}
