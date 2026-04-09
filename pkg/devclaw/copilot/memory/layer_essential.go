// Package memory — layer_essential.go implements the Sprint 2 L1
// EssentialLayer. It renders per-wing "essential stories": deterministic
// Markdown-ish summaries of the most recently touched rooms and the lead
// sentences of their top files in a wing. Output is cached in the
// essential_stories SQLite table with a TTL-based freshness check so that
// the generation cost is amortised across calls and survives restarts.
//
// Zero LLM calls. Template-only. This implements ADR-005 Option A
// ("template first"). A future sprint may swap to LLM-based generation
// behind a flag; until then, Render() is fully deterministic given a
// stable database snapshot.
//
// Until Room 2.4 wires the layer into the prompt stack, EssentialLayer is
// dead code at runtime — Render() is safe to call from tests but no
// production caller exists yet. The cache still gets populated by tests
// and (eventually) the dream cycle (Sprint 3+), so the schema lives in
// the DB from day one.
package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Layer-local defaults. The layer accepts a config so callers can override
// these; a zero-value EssentialLayerConfig falls through to the constants.
const (
	// defaultL1ByteBudget is the byte-budget approximation of the 400-token
	// budget from HierarchyConfig.L1Budget (1 token ≈ 4 bytes).
	defaultL1ByteBudget = 1600

	// defaultL1StaleAfter is the default TTL for a cached story. Matches
	// HierarchyConfig.EssentialStoryStaleAfter.
	defaultL1StaleAfter = 6 * time.Hour

	// defaultL1RoomsPerWing is the maximum number of rooms walked per wing.
	defaultL1RoomsPerWing = 4

	// defaultL1LeadSentencesPerRoom is how many top files per room feed the
	// bullet list. Each file contributes exactly one lead sentence.
	defaultL1LeadSentencesPerRoom = 3

	// defaultL1LeadSentenceMaxBytes is the hard cap for a single lead
	// sentence before the budgeting step takes over. 200 bytes is enough
	// for ~35 words which comfortably fits any one-line summary.
	defaultL1LeadSentenceMaxBytes = 200

	// legacyWingLabel is the heading suffix used for wing="" (legacy
	// wing IS NULL memories). Kept as a structural marker rather than a
	// domain assumption.
	legacyWingLabel = "(legacy)"
)

// EssentialLayerConfig is the subset of HierarchyConfig fields the layer
// needs. Passed in explicitly so the layer doesn't depend on the copilot
// package (avoiding an import cycle).
type EssentialLayerConfig struct {
	// ByteBudget caps the rendered story's byte length. <=0 uses
	// defaultL1ByteBudget.
	ByteBudget int

	// StaleAfter is the TTL before a cached story is regenerated. <=0
	// uses defaultL1StaleAfter.
	StaleAfter time.Duration

	// RoomsPerWing is the maximum number of rooms walked per wing. <=0
	// uses defaultL1RoomsPerWing.
	RoomsPerWing int

	// LeadSentencesPerRoom is the number of top files per room that
	// contribute a lead sentence. <=0 uses
	// defaultL1LeadSentencesPerRoom.
	LeadSentencesPerRoom int
}

// DefaultEssentialLayerConfig returns sensible defaults matching the
// Sprint 2 Room 2.2 spec (400-token budget, 6h TTL, 4 rooms × 3 files).
func DefaultEssentialLayerConfig() EssentialLayerConfig {
	return EssentialLayerConfig{
		ByteBudget:           defaultL1ByteBudget,
		StaleAfter:           defaultL1StaleAfter,
		RoomsPerWing:         defaultL1RoomsPerWing,
		LeadSentencesPerRoom: defaultL1LeadSentencesPerRoom,
	}
}

// withDefaults fills in any zero-valued fields of c with the package
// defaults. Used by the constructor to shield callers from having to
// supply every field.
func (c EssentialLayerConfig) withDefaults() EssentialLayerConfig {
	if c.ByteBudget <= 0 {
		c.ByteBudget = defaultL1ByteBudget
	}
	if c.StaleAfter <= 0 {
		c.StaleAfter = defaultL1StaleAfter
	}
	if c.RoomsPerWing <= 0 {
		c.RoomsPerWing = defaultL1RoomsPerWing
	}
	if c.LeadSentencesPerRoom <= 0 {
		c.LeadSentencesPerRoom = defaultL1LeadSentencesPerRoom
	}
	return c
}

// EssentialLayer generates and caches per-wing essential stories from the
// user's memory database. Stories are deterministic templates composed of
// room summaries and lead sentences from top memories, truncated to a byte
// budget. No LLM calls.
//
// Until Room 2.4 wires this layer into the prompt stack, EssentialLayer is
// dead code at runtime — Render() is safe to call from tests but no
// production caller exists yet.
type EssentialLayer struct {
	store  *SQLiteStore
	cfg    EssentialLayerConfig
	logger *slog.Logger

	// mu serializes regeneration across wings. A finer-grained per-wing
	// lock is tempting but adds complexity for no practical gain: the
	// Render hot path is a single indexed SELECT, and regeneration runs
	// at most once per wing per StaleAfter window.
	mu sync.Mutex
}

// NewEssentialLayer constructs a layer bound to the given store. store
// must NOT be nil — the layer is useless without a database. If logger is
// nil, slog.Default() is used. The passed cfg has its zero fields
// replaced by package defaults via withDefaults().
func NewEssentialLayer(store *SQLiteStore, cfg EssentialLayerConfig, logger *slog.Logger) *EssentialLayer {
	if logger == nil {
		logger = slog.Default()
	}
	return &EssentialLayer{
		store:  store,
		cfg:    cfg.withDefaults(),
		logger: logger,
	}
}

// Render returns the cached or freshly-generated essential story for the
// given wing. Wing="" is valid and returns the legacy-NULL wing's story
// (which may be empty).
//
// On SQL errors the layer logs at WARN and returns an empty string —
// never an error — so the prompt stack can always call Render() without
// worrying about hard failures.
//
// Concurrency: per-wing regeneration is serialized with the layer mutex.
// A cache hit path bypasses the mutex entirely.
func (l *EssentialLayer) Render(ctx context.Context, wing string) string {
	if l == nil || l.store == nil {
		return ""
	}
	wingKey := l.normalizeWingKey(wing)

	// Fast path: cache hit and fresh.
	if story, fresh, ok := l.readCache(ctx, wingKey); ok && fresh {
		return story
	}

	// Slow path: regenerate under the lock.
	l.mu.Lock()
	defer l.mu.Unlock()

	// Re-check after acquiring the mutex (double-checked cache pattern).
	// This is what keeps concurrent callers to a single generation per
	// wing when 10 goroutines hit Render() simultaneously.
	if story, fresh, ok := l.readCache(ctx, wingKey); ok && fresh {
		return story
	}

	story, err := l.generateLocked(ctx, wingKey)
	if err != nil {
		l.logger.Warn("essential layer: generate failed",
			"wing", wingKey, "err", err)
		return ""
	}
	return story
}

// Generate forces an immediate regeneration of the story for a wing,
// bypassing cache freshness checks. Used by tests and (eventually) by the
// dream cycle when it decides a wing's story is materially out of date.
//
// Unlike Render, Generate returns an error on SQL failure so the caller
// can react — tests rely on this to catch regressions early.
func (l *EssentialLayer) Generate(ctx context.Context, wing string) (string, error) {
	if l == nil || l.store == nil {
		return "", fmt.Errorf("essential layer: not initialized")
	}
	wingKey := l.normalizeWingKey(wing)

	l.mu.Lock()
	defer l.mu.Unlock()
	return l.generateLocked(ctx, wingKey)
}

// Invalidate marks the cached story for a wing as stale, forcing the next
// Render() call to regenerate. Pass wing="*" to invalidate all cached
// stories. Returns the count of rows affected.
func (l *EssentialLayer) Invalidate(ctx context.Context, wing string) (int64, error) {
	if l == nil || l.store == nil {
		return 0, fmt.Errorf("essential layer: not initialized")
	}
	if wing == "*" {
		res, err := l.store.db.ExecContext(ctx, `DELETE FROM essential_stories`)
		if err != nil {
			return 0, fmt.Errorf("invalidate all: %w", err)
		}
		return res.RowsAffected()
	}
	wingKey := l.normalizeWingKey(wing)
	res, err := l.store.db.ExecContext(ctx,
		`DELETE FROM essential_stories WHERE wing = ?`, wingKey)
	if err != nil {
		return 0, fmt.Errorf("invalidate %q: %w", wingKey, err)
	}
	return res.RowsAffected()
}

// normalizeWingKey converts the public wing parameter into the DB key.
// Empty string (legacy NULL wing) maps to the literal "" sentinel used
// as PRIMARY KEY. Reserved-prefix wings normalize to "" per the public
// NormalizeWing contract, which is equivalent to legacy for cache
// purposes — the caller still sees the legacy story in that case.
func (l *EssentialLayer) normalizeWingKey(wing string) string {
	if wing == "" {
		return ""
	}
	return NormalizeWing(wing)
}

// readCache performs a single SELECT against essential_stories for the
// given wing key. Returns (story, fresh, ok):
//
//   - ok=true when a row was found (regardless of freshness).
//   - fresh=true when the row is within the StaleAfter TTL.
//   - story is the cached bytes.
//
// SQL failures are logged and reported as ok=false so the caller falls
// through to regeneration.
func (l *EssentialLayer) readCache(ctx context.Context, wingKey string) (string, bool, bool) {
	var (
		story       string
		generatedAt int64
	)
	err := l.store.db.QueryRowContext(ctx,
		`SELECT story, generated_at FROM essential_stories WHERE wing = ?`,
		wingKey,
	).Scan(&story, &generatedAt)
	if err == sql.ErrNoRows {
		return "", false, false
	}
	if err != nil {
		l.logger.Warn("essential layer: cache read failed",
			"wing", wingKey, "err", err)
		return "", false, false
	}
	fresh := time.Since(time.Unix(generatedAt, 0)) < l.cfg.StaleAfter
	return story, fresh, true
}

// generateLocked renders a story for the wing and upserts it into the
// cache. Caller must hold l.mu.
func (l *EssentialLayer) generateLocked(ctx context.Context, wingKey string) (string, error) {
	story, sourceRooms, sourceFiles, err := l.renderTemplate(ctx, wingKey)
	if err != nil {
		return "", err
	}
	story = truncateAtBoundary(story, l.cfg.ByteBudget)

	now := time.Now().Unix()
	_, err = l.store.db.ExecContext(ctx, `
		INSERT INTO essential_stories
			(wing, story, generated_at, source_files, source_rooms, bytes, schema_version)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(wing) DO UPDATE SET
			story          = excluded.story,
			generated_at   = excluded.generated_at,
			source_files   = excluded.source_files,
			source_rooms   = excluded.source_rooms,
			bytes          = excluded.bytes,
			schema_version = excluded.schema_version
	`, wingKey, story, now, sourceFiles, sourceRooms, len(story), essentialStoriesSchemaVersion)
	if err != nil {
		return "", fmt.Errorf("upsert essential story: %w", err)
	}
	return story, nil
}

// renderTemplate walks the wing's top rooms and their top files, then
// assembles the Markdown-ish story. Returns the untruncated story plus
// counts of the rooms and files actually consumed — these feed the
// telemetry columns on the cache row.
//
// For wing="" (legacy) the function bypasses the rooms table entirely and
// works directly off files where wing IS NULL. This matches ADR-006
// "legacy cidadão" which treats wing IS NULL as a first-class bucket.
func (l *EssentialLayer) renderTemplate(ctx context.Context, wingKey string) (string, int, int, error) {
	var b strings.Builder

	// Heading: the wing label for non-legacy, (legacy) for wing="".
	if wingKey == "" {
		b.WriteString("## Wing: ")
		b.WriteString(legacyWingLabel)
		b.WriteString("\n\n")
	} else {
		b.WriteString("## Wing: ")
		b.WriteString(wingKey)
		b.WriteString("\n\n")
	}

	// Build the room set. Legacy wing uses a synthetic "(legacy)" single
	// room so the rest of the pipeline stays uniform.
	type roomSlot struct {
		name string
	}
	var slots []roomSlot

	if wingKey == "" {
		slots = []roomSlot{{name: ""}}
	} else {
		rooms, err := l.store.ListRoomsByRecency(ctx, wingKey, l.cfg.RoomsPerWing)
		if err != nil {
			return "", 0, 0, fmt.Errorf("list rooms by recency: %w", err)
		}
		slots = make([]roomSlot, 0, len(rooms))
		for _, r := range rooms {
			slots = append(slots, roomSlot{name: r.Name})
		}
	}

	usedRooms := 0
	usedFiles := 0

	for _, slot := range slots {
		files, err := l.store.TopFilesInRoom(ctx, wingKey, slot.name, l.cfg.LeadSentencesPerRoom)
		if err != nil {
			return "", 0, 0, fmt.Errorf("top files in room %q: %w", slot.name, err)
		}

		// Only materialize the heading if we actually have content for
		// this room — empty rooms would otherwise clutter the output and
		// waste budget.
		leadLines := make([]string, 0, len(files))
		for _, f := range files {
			lead := extractLeadSentence(f.Text)
			if lead == "" {
				continue
			}
			leadLines = append(leadLines, lead)
		}
		if len(leadLines) == 0 {
			continue
		}

		// Room heading.
		roomLabel := slot.name
		if roomLabel == "" {
			roomLabel = legacyWingLabel
		}
		b.WriteString("### ")
		b.WriteString(roomLabel)
		b.WriteString("\n")
		for _, ll := range leadLines {
			b.WriteString("- ")
			b.WriteString(ll)
			b.WriteString("\n")
			usedFiles++
		}
		b.WriteString("\n")
		usedRooms++
	}

	return b.String(), usedRooms, usedFiles, nil
}

// extractLeadSentence pulls the first sentence from a text blob. A
// "sentence" ends at the first ". " (period + space) or newline, whichever
// comes first. If neither marker is found within defaultL1LeadSentenceMaxBytes
// the prefix is hard-capped at that length.
//
// The returned string is trimmed of surrounding whitespace and never
// contains an embedded newline — bullet lines stay on a single row.
func extractLeadSentence(text string) string {
	if text == "" {
		return ""
	}
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}

	// Hard cap the search window so pathological inputs don't make us
	// walk a megabyte of text. Walk the cut point back to a UTF-8 rune
	// start so we never slice in the middle of a multi-byte sequence —
	// otherwise the downstream strings.Index / TrimSpace / ReplaceAll
	// operations could emit a malformed prefix.
	windowLen := defaultL1LeadSentenceMaxBytes
	if len(s) <= windowLen {
		windowLen = len(s)
	} else {
		for windowLen > 0 && !utf8.RuneStart(s[windowLen]) {
			windowLen--
		}
	}
	window := s[:windowLen]

	// Pick the earliest of: ". ", "\n", or end-of-window.
	end := len(window)
	if idx := strings.Index(window, ". "); idx >= 0 && idx < end {
		end = idx + 1 // include the period
	}
	if idx := strings.Index(window, "\n"); idx >= 0 && idx < end {
		end = idx
	}

	lead := strings.TrimSpace(window[:end])
	// Collapse any residual internal newlines or tabs into single spaces
	// to guarantee a single-line bullet.
	lead = strings.ReplaceAll(lead, "\n", " ")
	lead = strings.ReplaceAll(lead, "\t", " ")
	for strings.Contains(lead, "  ") {
		lead = strings.ReplaceAll(lead, "  ", " ")
	}
	return lead
}
