// Package memory — entity_detector.go implements a lightweight, regex-based
// entity detector that resolves candidate tokens from turn text against the
// wings/rooms tables stored in the SQLite store.
//
// The detector is a shared primitive: Room 2.3 uses it inside OnDemandLayer;
// Sprint 3 will reuse it for Knowledge Graph entity extraction (Kind="kg_entity").
// No LLM calls. Zero external dependencies beyond the existing go-sqlite3 driver.
//
// Thread-safety: EntityDetector is safe for concurrent use. The internal
// snapshot is protected by a RWMutex; cache refresh is serialized with a
// compare-and-swap on the refreshing field to prevent stampedes.
package memory

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
)

// tokenRe matches a Unicode word token of at least 3 characters (base + 2
// more). It is compiled once at package init to avoid allocating a regexp on
// every call. The pattern intentionally allows hyphens and underscores inside
// a token so that names like "project-x" or "feature_y" are captured whole.
var tokenRe = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}_-]{2,}`)

// EntityCandidate is a token extracted from a turn that plausibly matches a
// stored entity (wing, room, or future KG entity).
type EntityCandidate struct {
	// Text is the raw matched token from the turn, preserving the user's
	// casing. Used for display and logging.
	Text string

	// Normalized is the lowercase + accent-stripped form used for SQL
	// lookups. Always a subset of StripAccents(strings.ToLower(Text)).
	Normalized string

	// Offset is the byte offset in the source turn where the match starts.
	// Used for highlighting in future UIs.
	Offset int
}

// EntityMatch is a candidate that successfully resolved to a stored entity
// after SQL lookup.
type EntityMatch struct {
	Candidate EntityCandidate

	// Kind is "wing", "room", or (future) "kg_entity".
	Kind string

	// Wing is the wing this entity belongs to. For kind="wing" this equals
	// Candidate.Normalized. For kind="room" this is the room's parent wing.
	// May be empty for legacy NULL-wing rooms.
	Wing string

	// Room is the room name when Kind=="room", empty otherwise.
	Room string
}

// EntityDetectorConfig configures an EntityDetector instance.
type EntityDetectorConfig struct {
	// CacheTTL is how long the in-process entity snapshot survives before a
	// Detect() call triggers a refresh. Default: 30 seconds.
	CacheTTL time.Duration

	// MaxTokens caps how many candidate tokens are extracted from a single
	// turn. Protects against adversarial long inputs. Default: 40.
	MaxTokens int

	// MinTokenLen is the minimum character length (rune count) for a
	// candidate token to be checked against the snapshot. Default: 3.
	MinTokenLen int
}

// DefaultEntityDetectorConfig returns sensible defaults.
func DefaultEntityDetectorConfig() EntityDetectorConfig {
	return EntityDetectorConfig{
		CacheTTL:    30 * time.Second,
		MaxTokens:   40,
		MinTokenLen: 3,
	}
}

func (c EntityDetectorConfig) withDefaults() EntityDetectorConfig {
	if c.CacheTTL <= 0 {
		c.CacheTTL = 30 * time.Second
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 40
	}
	if c.MinTokenLen <= 0 {
		c.MinTokenLen = 3
	}
	return c
}

// EntityDetector extracts candidate tokens from turn text and resolves them
// against the wings/rooms tables. Stateless except for an internal TTL-cached
// snapshot of known entity names (refreshed on demand).
//
// Thread-safe. Cache refresh is serialized so concurrent turns do not
// stampede the store.
type EntityDetector struct {
	store  *SQLiteStore
	cfg    EntityDetectorConfig
	logger *slog.Logger

	mu         sync.RWMutex
	snapshot   map[string]EntityMatch // key = normalized entity name
	loadedAt   time.Time
	refreshing bool
}

// NewEntityDetector constructs a detector bound to the given store. store
// must NOT be nil. If logger is nil, slog.Default() is used.
func NewEntityDetector(store *SQLiteStore, cfg EntityDetectorConfig, logger *slog.Logger) *EntityDetector {
	if logger == nil {
		logger = slog.Default()
	}
	return &EntityDetector{
		store:    store,
		cfg:      cfg.withDefaults(),
		logger:   logger,
		snapshot: make(map[string]EntityMatch),
	}
}

// Detect scans the turn text and returns resolved entity matches. The turn is
// expected to be untrusted user input — tokenization uses Unicode letter/digit
// classes, skips punctuation, and caps the candidate count. Latency target:
// < 2ms on a warm cache for turns up to 500 chars.
//
// Detection is deterministic for the same (turn, snapshot) pair. Results are
// ordered by offset ascending.
func (d *EntityDetector) Detect(ctx context.Context, turn string) ([]EntityMatch, error) {
	if err := d.ensureSnapshot(ctx); err != nil {
		// If ensureSnapshot failed and the snapshot is empty, return early.
		d.mu.RLock()
		empty := len(d.snapshot) == 0
		d.mu.RUnlock()
		if empty {
			return nil, err
		}
		// Non-empty old snapshot: proceed, swallow the error (non-fatal policy).
	}

	d.mu.RLock()
	snap := d.snapshot
	d.mu.RUnlock()

	if len(snap) == 0 {
		return nil, nil
	}

	locs := tokenRe.FindAllStringIndex(turn, -1)
	var matches []EntityMatch
	seen := make(map[string]struct{}) // dedupe by normalized key

	tokenCount := 0
	for _, loc := range locs {
		if tokenCount >= d.cfg.MaxTokens {
			break
		}
		raw := turn[loc[0]:loc[1]]
		if len([]rune(raw)) < d.cfg.MinTokenLen {
			continue
		}
		tokenCount++
		normalized := StripAccents(strings.ToLower(raw))
		if _, ok := seen[normalized]; ok {
			continue
		}
		if m, ok := snap[normalized]; ok {
			seen[normalized] = struct{}{}
			m.Candidate = EntityCandidate{
				Text:       raw,
				Normalized: normalized,
				Offset:     loc[0],
			}
			matches = append(matches, m)
		}
	}

	return matches, nil
}

// Refresh forces the internal snapshot to reload from the store. Called
// automatically when CacheTTL elapses; exposed for tests.
func (d *EntityDetector) Refresh(ctx context.Context) error {
	return d.loadSnapshot(ctx)
}

// ensureSnapshot checks whether the snapshot is stale and refreshes if
// needed. Only one goroutine performs the refresh; others return immediately
// with the (possibly stale) existing snapshot.
func (d *EntityDetector) ensureSnapshot(ctx context.Context) error {
	d.mu.RLock()
	stale := time.Since(d.loadedAt) >= d.cfg.CacheTTL
	refreshing := d.refreshing
	d.mu.RUnlock()

	if !stale {
		return nil
	}
	if refreshing {
		// Another goroutine is already refreshing; use the old snapshot.
		return nil
	}

	// Mark refreshing under write lock (double-checked).
	d.mu.Lock()
	if time.Since(d.loadedAt) < d.cfg.CacheTTL {
		d.mu.Unlock()
		return nil
	}
	if d.refreshing {
		d.mu.Unlock()
		return nil
	}
	d.refreshing = true
	d.mu.Unlock()

	err := d.loadSnapshot(ctx)

	d.mu.Lock()
	d.refreshing = false
	d.mu.Unlock()

	return err
}

// loadSnapshot queries the wings and rooms tables and builds a new snapshot
// map. Wings are processed first so that a wing named "foo" wins over a room
// named "foo" in a different wing when both normalize to the same key.
//
// The query runs inside a 2-second context timeout to prevent hangs on a
// locked or slow database. A refresh failure is non-fatal: the previous
// snapshot stays in place and an error is returned so the caller can log it.
func (d *EntityDetector) loadSnapshot(ctx context.Context) error {
	tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	rows, err := d.store.db.QueryContext(tctx, `
		SELECT 'wing', name, ''
		  FROM wings
		UNION ALL
		SELECT 'room', name, wing
		  FROM rooms
	`)
	if err != nil {
		d.logger.Debug("entity detector: snapshot query failed", "err", err)
		return err
	}
	defer rows.Close()

	newSnap := make(map[string]EntityMatch)
	for rows.Next() {
		var kind, name, wing string
		if err := rows.Scan(&kind, &name, &wing); err != nil {
			d.logger.Debug("entity detector: row scan failed", "err", err)
			continue
		}
		normalized := StripAccents(strings.ToLower(name))
		if normalized == "" {
			continue
		}
		// Wings win over rooms on key collision.
		if existing, ok := newSnap[normalized]; ok && existing.Kind == "wing" {
			continue
		}

		m := EntityMatch{
			Kind: kind,
			Wing: wing,
		}
		if kind == "wing" {
			m.Wing = normalized
		} else {
			m.Room = name
		}
		// Candidate is populated by Detect(); leave zero here.
		newSnap[normalized] = m
	}
	if err := rows.Err(); err != nil {
		d.logger.Debug("entity detector: rows iteration error", "err", err)
		// Partial snapshot is still usable; proceed with what we have.
	}

	d.mu.Lock()
	d.snapshot = newSnap
	d.loadedAt = time.Now()
	d.mu.Unlock()

	return nil
}

// snapshotAge returns the time elapsed since the last successful snapshot
// load. Exposed for tests that need to inspect cache freshness.
func (d *EntityDetector) snapshotAge() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.loadedAt.IsZero() {
		return d.cfg.CacheTTL + 1 // treat as stale
	}
	return time.Since(d.loadedAt)
}

// snapshotLoadedAt returns the time of the last snapshot load.
// Exposed for tests that need to inspect cache freshness.
func (d *EntityDetector) snapshotLoadedAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.loadedAt
}


