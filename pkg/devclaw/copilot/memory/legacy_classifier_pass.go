// Package memory — legacy_classifier_pass.go runs the pattern-based
// legacy classifier over the database, updating files.wing for any
// legacy file that the classifier can confidently categorize.
//
// This is the DB-touching counterpart to legacy_classifier.go. The
// classifier itself is pure Go (strings + maps). This file handles:
//
//   - Iterating over files where wing IS NULL (oldest first)
//   - Reading their content from the chunks table
//   - Calling the classifier
//   - Writing files.wing = result.Wing when confidence is high enough
//   - Logging every classification for audit
//   - Bounding work via a batch size so a single pass does not hog the DB
//
// This function is DESIGNED TO BE CALLED FROM THE DREAM SYSTEM. Sprint 1
// exposes it as a public method; Sprint 2 will wire it into dream.go's
// cycle runner so it fires automatically during idle periods. The
// manual invocation path (e.g., CLI `devclaw dream run --classify-legacy`)
// is also valuable for users who want to trigger it explicitly.
//
// Safety rails (Princípio Zero):
//
//   - We NEVER update a file that already has wing != NULL. The user's
//     explicit wing is sacred.
//   - We NEVER call LLMs. This is pure pattern matching.
//   - A failed classification leaves wing = NULL (still first-class).
//   - The batch size is bounded by the caller — a runaway classifier
//     cannot eat the entire DB in one pass.
//   - Every mutation is logged with source='auto-legacy' in logs (NOT in
//     files.wing — that column only stores the wing name).
//   - No user-facing notification by default. The feature is silent.
package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// LegacyClassificationStats summarizes the outcome of one pass.
type LegacyClassificationStats struct {
	// Scanned is the number of legacy files the pass inspected.
	Scanned int

	// Classified is the number of files that received a new wing.
	Classified int

	// Skipped is Scanned - Classified (files the classifier could not decide).
	Skipped int

	// Errors counts files that failed to read/update due to SQL errors.
	Errors int

	// PerWing tracks how many files landed in each wing this pass.
	PerWing map[string]int
}

// LegacyClassificationConfig tunes the pass behavior.
type LegacyClassificationConfig struct {
	// BatchSize caps how many files are inspected per pass. Default 20.
	// Set to 0 to use the default. Negative values are treated as 0.
	BatchSize int

	// MinConfidence overrides ClassifierMinConfidence for this pass.
	// Set to 0 to use the default. Lower bound is 0.5 (hard minimum
	// enforced to prevent accidental misuse).
	MinConfidence float64

	// DryRun reports what WOULD be classified without writing anything
	// to the database. Useful for CLI preview and tests.
	DryRun bool
}

// DefaultLegacyClassificationBatchSize is the number of files processed
// per pass when no override is given. Chosen to keep a single pass under
// ~100ms even on slow disks with 500-chunk files.
const DefaultLegacyClassificationBatchSize = 20

// MinAllowedClassifierConfidence is the absolute lowest confidence the
// pass will accept, regardless of caller override. Prevents accidental
// label pollution from misconfigured callers.
const MinAllowedClassifierConfidence = 0.5

// RunLegacyClassificationPass scans for legacy files (wing IS NULL),
// reads their content, runs the pattern-based classifier, and updates
// files.wing for any that the classifier confidently labels.
//
// This is idempotent: running it multiple times will only label NEW
// files, because the query filters out files that already have a wing.
//
// Returns a summary of the pass. Individual file errors are logged via
// the store's logger and counted but do not abort the pass.
//
// Callers typically invoke this from:
//  1. The dream system's cycle runner (Sprint 2 integration)
//  2. CLI commands (future: `devclaw memory classify-legacy`)
//  3. Tests
func (s *SQLiteStore) RunLegacyClassificationPass(ctx context.Context, cfg LegacyClassificationConfig) (*LegacyClassificationStats, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("sqlite store is not initialized")
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultLegacyClassificationBatchSize
	}
	minConf := cfg.MinConfidence
	if minConf <= 0 {
		minConf = ClassifierMinConfidence
	}
	if minConf < MinAllowedClassifierConfidence {
		minConf = MinAllowedClassifierConfidence
	}

	stats := &LegacyClassificationStats{PerWing: make(map[string]int)}

	// Step 1: select legacy file IDs. We use a two-step approach rather
	// than a JOIN so that each file's content is read in its own small
	// query — this avoids locking chunks for too long.
	fileRows, err := s.db.QueryContext(ctx, `
		SELECT file_id
		FROM files
		WHERE wing IS NULL
		  AND (deleted_at IS NULL)
		ORDER BY indexed_at ASC
		LIMIT ?
	`, batchSize)
	if err != nil {
		return stats, fmt.Errorf("query legacy files: %w", err)
	}
	var fileIDs []string
	for fileRows.Next() {
		var id string
		if err := fileRows.Scan(&id); err != nil {
			fileRows.Close()
			return stats, fmt.Errorf("scan legacy file: %w", err)
		}
		fileIDs = append(fileIDs, id)
	}
	fileRows.Close()
	if err := fileRows.Err(); err != nil {
		return stats, fmt.Errorf("iterate legacy files: %w", err)
	}

	// Step 2: for each file, read its chunks, classify, update.
	for _, fileID := range fileIDs {
		// Honor context cancellation between files so a dream cycle shutdown
		// does not have to wait for the whole batch to finish (NIT-10).
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		stats.Scanned++

		content, readErr := s.readFileContent(ctx, fileID)
		if readErr != nil {
			stats.Errors++
			s.logger.Debug("legacy classifier: read failed",
				"file", fileID, "error", readErr)
			continue
		}
		if strings.TrimSpace(content) == "" {
			stats.Skipped++
			continue
		}

		result := ClassifyLegacyContent(content)
		if result.Wing == "" || result.Confidence < minConf {
			stats.Skipped++
			continue
		}

		if cfg.DryRun {
			stats.Classified++
			stats.PerWing[result.Wing]++
			s.logger.Info("legacy classifier: DRY RUN would label",
				"file", fileID,
				"wing", result.Wing,
				"confidence", result.Confidence,
				"top_hits", result.TopWingHits,
				"second_hits", result.SecondWingHits,
				"keywords", strings.Join(result.MatchedKeywords, ","),
			)
			continue
		}

		// Apply the label. We use a conditional UPDATE that only writes
		// if wing IS STILL NULL — this avoids racing against a concurrent
		// manual classification by the user via a bot command.
		res, err := s.db.ExecContext(ctx, `
			UPDATE files
			SET wing = ?
			WHERE file_id = ? AND wing IS NULL
		`, result.Wing, fileID)
		if err != nil {
			stats.Errors++
			s.logger.Warn("legacy classifier: update failed",
				"file", fileID, "wing", result.Wing, "error", err)
			continue
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			// Race lost — user or another pass labeled this file first.
			stats.Skipped++
			continue
		}

		stats.Classified++
		stats.PerWing[result.Wing]++

		// Make sure the wing exists in the registry so ListWings() can
		// surface it. Ignore errors — the files.wing update is the
		// source of truth either way.
		_ = s.UpsertWing(result.Wing, "", "")

		s.logger.Info("legacy classifier: labeled file",
			"file", fileID,
			"wing", result.Wing,
			"confidence", result.Confidence,
			"keywords", strings.Join(result.MatchedKeywords, ","),
		)
	}

	return stats, nil
}

// readFileContent loads the full text of a file by concatenating its
// chunks in order. This is used only by the classifier pass — it is
// NOT a general-purpose file reader, and callers should not rely on
// the exact formatting between chunks.
//
// The caller is responsible for bounding calls to readFileContent via
// the classifier's batch size, since concatenating all chunks for a
// very large file could allocate a lot of memory.
func (s *SQLiteStore) readFileContent(ctx context.Context, fileID string) (string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT text FROM chunks
		WHERE file_id = ?
		ORDER BY chunk_idx ASC
	`, fileID)
	if err != nil {
		return "", fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	var b strings.Builder
	for rows.Next() {
		var text sql.NullString
		if err := rows.Scan(&text); err != nil {
			return "", fmt.Errorf("scan chunk: %w", err)
		}
		if text.Valid {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(text.String)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Compile-time assertion that slog usage is available — keeps import
// stable even if no LOG calls happen on a given build.
var _ = slog.Default
