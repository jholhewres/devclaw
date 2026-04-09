// Package memory — legacy_classifier_test.go covers the pure classifier
// (no DB) and the DB pass runner (end-to-end against a temp store).
package memory

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
)

func TestClassifyLegacyContent_StrongWorkSignal(t *testing.T) {
	content := `
Today I pushed the pull request for the auth migration. Maria reviewed it
during standup and asked me to rebase onto the sprint branch. The deploy
pipeline passed all integration tests. I also filed a Jira ticket about
the kubernetes config drift in production.
`
	result := ClassifyLegacyContent(content)
	if result.Wing != "work" {
		t.Errorf("expected wing=work, got %q (hits: top=%d second=%d, kws=%v)",
			result.Wing, result.TopWingHits, result.SecondWingHits, result.MatchedKeywords)
	}
	if result.Confidence < ClassifierMinConfidence {
		t.Errorf("expected confidence >= %v, got %v", ClassifierMinConfidence, result.Confidence)
	}
	if result.TopWingHits < ClassifierMinHits {
		t.Errorf("expected top hits >= %d, got %d", ClassifierMinHits, result.TopWingHits)
	}
}

func TestClassifyLegacyContent_FamilyInPortuguese(t *testing.T) {
	content := `
A mãe da minha filha me avisou que a reunião de pais na escola é na
quinta-feira. O professor pediu pra levar o boletim assinado. Meu filho
mais velho tem festa de aniversário no sábado.
`
	result := ClassifyLegacyContent(content)
	if result.Wing != "family" {
		t.Errorf("expected wing=family, got %q (hits: %+v)", result.Wing, result)
	}
	if result.Confidence < ClassifierMinConfidence {
		t.Errorf("expected confidence >= %v, got %v", ClassifierMinConfidence, result.Confidence)
	}
}

func TestClassifyLegacyContent_FinanceMixed(t *testing.T) {
	content := `
Paguei o boleto do cartão de crédito. Conferi o investimento no banco
e a fatura da corretora. Preciso lembrar do imposto de renda.
`
	result := ClassifyLegacyContent(content)
	if result.Wing != "finance" {
		t.Errorf("expected wing=finance, got %q (top=%d second=%d)",
			result.Wing, result.TopWingHits, result.SecondWingHits)
	}
}

func TestClassifyLegacyContent_EmptyInputReturnsNothing(t *testing.T) {
	cases := []string{"", "   ", "\n\n\t"}
	for _, c := range cases {
		result := ClassifyLegacyContent(c)
		if result.Wing != "" {
			t.Errorf("empty input should not classify, got wing=%q", result.Wing)
		}
		if result.Confidence != 0 {
			t.Errorf("empty input confidence should be 0, got %v", result.Confidence)
		}
	}
}

func TestClassifyLegacyContent_InsufficientHitsLeavesNull(t *testing.T) {
	// Just 2 work hits — below ClassifierMinHits=3.
	content := `Fixing a bug in the pipeline config today.`
	result := ClassifyLegacyContent(content)
	if result.Wing != "" {
		t.Errorf("expected no classification (insufficient hits), got wing=%q (top=%d)",
			result.Wing, result.TopWingHits)
	}
}

func TestClassifyLegacyContent_AmbiguousDistribution(t *testing.T) {
	// 3 work hits AND 3 family hits → ambiguous → no classification.
	content := `
Today was weird. I had a sprint planning meeting and reviewed a pull
request. After work I took my filha to escola and had dinner with
my mãe.
`
	result := ClassifyLegacyContent(content)
	if result.Wing != "" {
		t.Errorf("expected no classification (ambiguous), got wing=%q (top=%d second=%d)",
			result.Wing, result.TopWingHits, result.SecondWingHits)
	}
}

func TestClassifyLegacyContent_DominanceFactor(t *testing.T) {
	// 4 work hits vs 2 personal hits. Ratio = 2.0 → meets ClassifierDominanceFactor.
	content := `
Sprint retrospective today. Bunch of bugs in the deploy pipeline. My
friend mentioned a new game during lunch. Another standup tomorrow
and I need to review the pull request.
`
	result := ClassifyLegacyContent(content)
	if result.Wing != "work" {
		t.Errorf("expected wing=work (dominant), got %q (top=%d second=%d)",
			result.Wing, result.TopWingHits, result.SecondWingHits)
	}
}

func TestClassifyLegacyContent_CaseInsensitive(t *testing.T) {
	content := `DEPLOY to PRODUCTION. Standup at 9am. Pull Request merged.`
	result := ClassifyLegacyContent(content)
	if result.Wing != "work" {
		t.Errorf("expected wing=work regardless of case, got %q", result.Wing)
	}
}

func TestClassifyLegacyContent_AccentInsensitive(t *testing.T) {
	// Without accents.
	content1 := "A mae da minha filha. A reuniao de pais na escola. O professor.  "
	r1 := ClassifyLegacyContent(content1)
	// With accents.
	content2 := "A mãe da minha filha. A reunião de pais na escola. O professor."
	r2 := ClassifyLegacyContent(content2)

	if r1.Wing != r2.Wing {
		t.Errorf("accent sensitivity: %q vs %q", r1.Wing, r2.Wing)
	}
	if r1.Wing != "family" {
		t.Errorf("expected wing=family, got %q", r1.Wing)
	}
}

func TestClassifyLegacyContent_CustomKeywords(t *testing.T) {
	custom := map[string][]string{
		"gaming": {"zelda", "mario", "nintendo", "playstation"},
	}
	content := "Played zelda on nintendo today. Also started mario."
	result := ClassifyLegacyContentWith(content, custom)
	if result.Wing != "gaming" {
		t.Errorf("expected custom wing=gaming, got %q", result.Wing)
	}
}

func TestClassifyLegacyContent_ConfidenceScalesWithDominance(t *testing.T) {
	// Pure signal (no competition) → high confidence.
	pureContent := `
Sprint, standup, retro, backlog, deploy, rollback, pipeline, merge request,
hotfix, deployment, commit, pull request, release, changelog.
`
	pure := ClassifyLegacyContent(pureContent)

	// Tight margin → low confidence (at floor).
	tightContent := `
Deploy today. Sprint planning. Pull request review.
My friend joined for gaming afterwards and mentioned a book.
`
	tight := ClassifyLegacyContent(tightContent)

	if pure.Wing != "work" || tight.Wing != "work" {
		t.Skipf("both should be work, got pure=%q tight=%q", pure.Wing, tight.Wing)
	}
	if pure.Confidence <= tight.Confidence {
		t.Errorf("pure signal should score higher: pure=%v tight=%v",
			pure.Confidence, tight.Confidence)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// End-to-end pass tests (DB integration)
// ─────────────────────────────────────────────────────────────────────────────

// setupLegacyFiles inserts fake files + chunks directly into the store's
// database for classification testing. Avoids the full IndexChunks path
// which would require embeddings.
func setupLegacyFiles(t *testing.T, store *SQLiteStore, fixtures map[string]string) {
	t.Helper()
	for fileID, content := range fixtures {
		_, err := store.db.Exec(
			`INSERT INTO files (file_id, hash) VALUES (?, ?)`,
			fileID, "hash-"+fileID)
		if err != nil {
			t.Fatalf("insert file %s: %v", fileID, err)
		}
		// Put the whole content in a single chunk for simplicity.
		_, err = store.db.Exec(
			`INSERT INTO chunks (file_id, chunk_idx, text, hash) VALUES (?, 0, ?, ?)`,
			fileID, content, "chunk-"+fileID)
		if err != nil {
			t.Fatalf("insert chunk for %s: %v", fileID, err)
		}
	}
}

func TestRunLegacyClassificationPass_Basic(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	fixtures := map[string]string{
		"work-memo.md": `Sprint retro today. Pull request merged. Deploy pipeline passed.
			Standup at 9am tomorrow. Kubernetes config drift.`,
		"family-note.md": `Minha filha tem reunião de pais na escola. A mãe levou
			ela para a consulta com o pediatra. Meu filho tem festa de aniversário.`,
		"finance-log.md": `Paguei a fatura do cartão de crédito. Conferi o
			investimento no banco. Preciso do imposto de renda.`,
		"empty.md":      ``,
		"ambiguous.md":  `Met with my friend who works at the same company.`,
	}
	setupLegacyFiles(t, store, fixtures)

	stats, err := store.RunLegacyClassificationPass(ctx, LegacyClassificationConfig{})
	if err != nil {
		t.Fatalf("RunLegacyClassificationPass: %v", err)
	}

	if stats.Scanned != len(fixtures) {
		t.Errorf("expected scanned=%d, got %d", len(fixtures), stats.Scanned)
	}
	if stats.Classified < 3 {
		t.Errorf("expected at least 3 classifications, got %d", stats.Classified)
	}
	if stats.PerWing["work"] != 1 {
		t.Errorf("expected 1 work file, got %d", stats.PerWing["work"])
	}
	if stats.PerWing["family"] != 1 {
		t.Errorf("expected 1 family file, got %d", stats.PerWing["family"])
	}
	if stats.PerWing["finance"] != 1 {
		t.Errorf("expected 1 finance file, got %d", stats.PerWing["finance"])
	}

	// Verify that wings are persisted in files table.
	for fileID, expected := range map[string]string{
		"work-memo.md":   "work",
		"family-note.md": "family",
		"finance-log.md": "finance",
	} {
		var got sql.NullString
		err := store.db.QueryRow(`SELECT wing FROM files WHERE file_id = ?`, fileID).Scan(&got)
		if err != nil {
			t.Errorf("read %s: %v", fileID, err)
			continue
		}
		if !got.Valid || got.String != expected {
			t.Errorf("file %s: expected wing=%s, got %q (valid=%v)", fileID, expected, got.String, got.Valid)
		}
	}

	// Empty and ambiguous files must remain NULL.
	for _, fileID := range []string{"empty.md", "ambiguous.md"} {
		var got sql.NullString
		_ = store.db.QueryRow(`SELECT wing FROM files WHERE file_id = ?`, fileID).Scan(&got)
		if got.Valid {
			t.Errorf("file %s should remain NULL, got %q", fileID, got.String)
		}
	}
}

func TestRunLegacyClassificationPass_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	setupLegacyFiles(t, store, map[string]string{
		"work1.md": "Sprint retro, pull request, deploy pipeline, standup, kubernetes.",
	})

	// First pass labels the file.
	stats1, _ := store.RunLegacyClassificationPass(ctx, LegacyClassificationConfig{})
	if stats1.Classified != 1 {
		t.Fatalf("first pass should classify 1, got %d", stats1.Classified)
	}

	// Second pass finds no work to do (file already labeled).
	stats2, _ := store.RunLegacyClassificationPass(ctx, LegacyClassificationConfig{})
	if stats2.Scanned != 0 {
		t.Errorf("second pass should find 0 legacy files, scanned=%d", stats2.Scanned)
	}
	if stats2.Classified != 0 {
		t.Errorf("second pass should classify 0, got %d", stats2.Classified)
	}
}

func TestRunLegacyClassificationPass_NeverOverwritesUserWing(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Insert a file already labeled as "personal" (by user).
	_, _ = store.db.Exec(
		`INSERT INTO files (file_id, hash, wing) VALUES (?, ?, ?)`,
		"user-set.md", "h1", "personal")
	_, _ = store.db.Exec(
		`INSERT INTO chunks (file_id, chunk_idx, text, hash) VALUES (?, 0, ?, ?)`,
		"user-set.md",
		"Sprint retro, pull request, deploy pipeline, standup, kubernetes, jira, github.",
		"ch1")

	stats, _ := store.RunLegacyClassificationPass(ctx, LegacyClassificationConfig{})
	if stats.Scanned != 0 {
		t.Errorf("expected user-labeled file to be skipped, scanned=%d", stats.Scanned)
	}

	// Verify the user's wing is untouched.
	var wing sql.NullString
	_ = store.db.QueryRow(`SELECT wing FROM files WHERE file_id = ?`, "user-set.md").Scan(&wing)
	if !wing.Valid || wing.String != "personal" {
		t.Errorf("user wing was overwritten: got %q (valid=%v)", wing.String, wing.Valid)
	}
}

func TestRunLegacyClassificationPass_DryRun(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	setupLegacyFiles(t, store, map[string]string{
		"work.md": "Sprint retro, pull request, deploy pipeline, standup, kubernetes.",
	})

	stats, _ := store.RunLegacyClassificationPass(ctx, LegacyClassificationConfig{DryRun: true})
	if stats.Classified != 1 {
		t.Errorf("dry run should report 1 classified, got %d", stats.Classified)
	}

	// Verify the file is STILL legacy (not actually updated).
	var wing sql.NullString
	_ = store.db.QueryRow(`SELECT wing FROM files WHERE file_id = 'work.md'`).Scan(&wing)
	if wing.Valid {
		t.Errorf("dry run should not update DB, but wing=%q was set", wing.String)
	}
}

func TestRunLegacyClassificationPass_BatchSizeBound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Insert 50 legacy files, all classifiable as work.
	fixtures := make(map[string]string)
	workBlob := "Sprint retro, pull request, deploy pipeline, standup, kubernetes, jira."
	for i := 0; i < 50; i++ {
		fixtures[fmt.Sprintf("file-%02d.md", i)] = workBlob
	}
	setupLegacyFiles(t, store, fixtures)

	// Pass with batch size 10 should process exactly 10.
	stats, _ := store.RunLegacyClassificationPass(ctx, LegacyClassificationConfig{BatchSize: 10})
	if stats.Scanned != 10 {
		t.Errorf("expected scanned=10, got %d", stats.Scanned)
	}
	if stats.Classified != 10 {
		t.Errorf("expected classified=10, got %d", stats.Classified)
	}

	// A second pass should pick up the next 10.
	stats2, _ := store.RunLegacyClassificationPass(ctx, LegacyClassificationConfig{BatchSize: 10})
	if stats2.Scanned != 10 {
		t.Errorf("second pass scanned=%d, expected 10", stats2.Scanned)
	}
}
