// Package memory — layer_ondemand.go implements the Sprint 2 L2
// OnDemandLayer. It inspects the current user turn, detects entities
// (via EntityDetector) that match stored wings/rooms, retrieves the top-N
// memories for those entities from the active wing, and returns a
// prompt-ready Markdown snippet truncated to a byte budget.
//
// The layer runs on the hot path (every turn). Latency contract: p95 < 10ms
// on a warm cache. Context timeouts enforce this contract:
//   - Detector call: DetectorTimeoutMs (default 3ms)
//   - Search calls:  SearchTimeoutMs (default 8ms)
//   - Overall:       DetectorTimeoutMs + SearchTimeoutMs
//
// Until Room 2.4 wires this layer into the prompt stack, OnDemandLayer is
// dead code at runtime — Render() is safe to call from tests but no
// production caller exists yet.
//
// Cross-wing fallback: if the active wing yields no results but
// CrossWingEnabled is true, ONE additional search is run without a wing
// filter. This surfaces the single best globally-relevant result when the
// user mentions an entity that lives only in another wing (e.g., a topic
// from a secondary context).
package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Layer-local defaults.
const (
	// defaultL2ByteBudget ≈ 300 tokens (1 token ≈ 4 bytes).
	defaultL2ByteBudget = 1200

	// defaultL2MaxResults is the default cap on returned memories.
	defaultL2MaxResults = 5

	// defaultL2DetectorTimeoutMs is the hard latency guard for the entity
	// detector call. 3ms is sufficient for 500-char turns on warm cache.
	defaultL2DetectorTimeoutMs = 3

	// defaultL2SearchTimeoutMs is the hard latency guard for each store
	// search. 8ms leaves headroom for the overall 10ms p95 contract.
	defaultL2SearchTimeoutMs = 8
)

// OnDemandLayerConfig configures the OnDemandLayer hot-path behavior.
type OnDemandLayerConfig struct {
	// ByteBudget caps the total byte length of Render's output.
	// <=0 uses defaultL2ByteBudget (1200 bytes ≈ 300 tokens).
	ByteBudget int

	// MaxResults caps the number of distinct memory results returned.
	// <=0 uses defaultL2MaxResults (5).
	MaxResults int

	// CrossWingEnabled, when true, allows ONE cross-wing fallback result
	// when the active wing yields nothing. Default: true.
	CrossWingEnabled bool

	// DetectorTimeoutMs is the hard latency guard for the entity detector
	// call. <=0 uses defaultL2DetectorTimeoutMs (3ms).
	DetectorTimeoutMs int

	// SearchTimeoutMs is the hard latency guard for store search calls.
	// <=0 uses defaultL2SearchTimeoutMs (8ms).
	SearchTimeoutMs int
}

// DefaultOnDemandLayerConfig returns sensible defaults for hot-path use.
func DefaultOnDemandLayerConfig() OnDemandLayerConfig {
	return OnDemandLayerConfig{
		ByteBudget:        defaultL2ByteBudget,
		MaxResults:        defaultL2MaxResults,
		CrossWingEnabled:  true,
		DetectorTimeoutMs: defaultL2DetectorTimeoutMs,
		SearchTimeoutMs:   defaultL2SearchTimeoutMs,
	}
}

func (c OnDemandLayerConfig) withDefaults() OnDemandLayerConfig {
	if c.ByteBudget <= 0 {
		c.ByteBudget = defaultL2ByteBudget
	}
	if c.MaxResults <= 0 {
		c.MaxResults = defaultL2MaxResults
	}
	if c.DetectorTimeoutMs <= 0 {
		c.DetectorTimeoutMs = defaultL2DetectorTimeoutMs
	}
	if c.SearchTimeoutMs <= 0 {
		c.SearchTimeoutMs = defaultL2SearchTimeoutMs
	}
	return c
}

// OnDemandLayer retrieves per-turn contextual memories based on detected
// entities. Composes an EntityDetector with a wing-aware search over the
// SQLite store.
//
// Until Room 2.4 wires this into the prompt stack, the layer is dead code
// at runtime but fully functional for tests.
type OnDemandLayer struct {
	store    *SQLiteStore
	detector *EntityDetector
	cfg      OnDemandLayerConfig
	logger   *slog.Logger
}

// NewOnDemandLayer constructs a layer. detector may be nil — in that case the
// layer auto-constructs one from store and config defaults. Passing a shared
// detector allows callers to amortize the entity snapshot cache across
// multiple layers (Room 2.4 pattern).
//
// store must NOT be nil. If logger is nil, slog.Default() is used.
func NewOnDemandLayer(store *SQLiteStore, detector *EntityDetector, cfg OnDemandLayerConfig, logger *slog.Logger) *OnDemandLayer {
	if logger == nil {
		logger = slog.Default()
	}
	cfg = cfg.withDefaults()
	if detector == nil {
		detector = NewEntityDetector(store, DefaultEntityDetectorConfig(), logger)
	}
	return &OnDemandLayer{
		store:    store,
		detector: detector,
		cfg:      cfg,
		logger:   logger,
	}
}

// Render inspects the turn, detects entities, retrieves memories from the
// active wing first, optionally falls back to ONE cross-wing result when
// nothing is found in the active wing, and returns a prompt-ready string
// truncated to the byte budget.
//
// activeWing is the current session's wing ("" = legacy / no wing).
// turn is the current user message text.
//
// This runs on the hot path. Latency contract: p95 < 10ms warm.
// Errors are logged at DEBUG and result in an empty string.
func (l *OnDemandLayer) Render(ctx context.Context, activeWing string, turn string) string {
	if l == nil || l.store == nil {
		return ""
	}

	totalBudget := time.Duration(l.cfg.DetectorTimeoutMs+l.cfg.SearchTimeoutMs) * time.Millisecond
	outerCtx, outerCancel := context.WithTimeout(ctx, totalBudget)
	defer outerCancel()

	start := time.Now()

	// Step 1 — detect entities with a hard timeout.
	detectorCtx, detectorCancel := context.WithTimeout(outerCtx, time.Duration(l.cfg.DetectorTimeoutMs)*time.Millisecond)
	defer detectorCancel()

	entities, err := l.detector.Detect(detectorCtx, turn)
	if err != nil {
		l.logger.Debug("on-demand layer: detector failed", "err", err)
	}
	if detectorCtx.Err() != nil {
		l.logger.Debug("on-demand layer: detector timeout exceeded",
			"elapsed", time.Since(start))
		return ""
	}

	// Step 2 — fast path: no entities detected.
	if len(entities) == 0 {
		return ""
	}

	// Step 3 — search for each entity in the active wing.
	searchCtx, searchCancel := context.WithTimeout(outerCtx, time.Duration(l.cfg.SearchTimeoutMs)*time.Millisecond)
	defer searchCancel()

	merged := l.searchEntities(searchCtx, entities, activeWing)

	// Step 4 — cross-wing fallback: if we got nothing from the active wing
	// and cross-wing is enabled, run ONE more search without a wing filter.
	if len(merged) == 0 && l.cfg.CrossWingEnabled {
		merged = l.searchEntities(searchCtx, entities[:1], "")
		if len(merged) > 1 {
			merged = merged[:1]
		}
	}

	if searchCtx.Err() != nil {
		l.logger.Debug("on-demand layer: search timeout exceeded",
			"elapsed", time.Since(start))
		// Return whatever we have so far, or empty.
	}

	if len(merged) == 0 {
		return ""
	}

	// Step 5 — render as Markdown.
	output := l.renderMarkdown(merged, entities)

	// Step 6 — truncate to byte budget.
	output = truncateAtBoundary(output, l.cfg.ByteBudget)

	elapsed := time.Since(start)
	if elapsed > totalBudget {
		l.logger.Debug("on-demand layer: overall budget exceeded",
			"budget_ms", l.cfg.DetectorTimeoutMs+l.cfg.SearchTimeoutMs,
			"elapsed_ms", elapsed.Milliseconds())
	}

	return output
}

// searchEntities runs a HybridSearchWithOpts call per entity against the
// store, deduplicates results by FileID, and returns at most cfg.MaxResults
// entries ordered by score descending.
func (l *OnDemandLayer) searchEntities(ctx context.Context, entities []EntityMatch, wing string) []onDemandResult {
	type scored struct {
		r     onDemandResult
		score float64
	}

	seen := make(map[string]struct{}) // dedupe by FileID
	var all []scored

	for _, entity := range entities {
		if ctx.Err() != nil {
			break
		}

		queryText := entity.Candidate.Text
		if queryText == "" {
			queryText = entity.Room
		}
		if queryText == "" {
			queryText = entity.Wing
		}

		results, err := l.store.HybridSearchWithOpts(ctx, queryText, HybridSearchOptions{
			MaxResults: l.cfg.MaxResults,
			MinScore:   0,
			QueryWing:  wing,
		})
		if err != nil {
			l.logger.Debug("on-demand layer: search failed",
				"entity", queryText, "err", err)
			continue
		}

		for _, r := range results {
			if _, ok := seen[r.FileID]; ok {
				continue
			}
			// Post-filter: when searching within an active wing, only
			// include results that actually belong to that wing or are
			// legacy (wing == ""). HybridSearchWithOpts boosts/penalizes
			// but does not exclude cross-wing candidates, so we must
			// filter here.
			if wing != "" && r.Wing != "" && r.Wing != wing {
				continue
			}
			seen[r.FileID] = struct{}{}
			all = append(all, scored{
				r: onDemandResult{
					fileID: r.FileID,
					text:   r.Text,
					score:  r.Score,
					wing:   r.Wing,
					entity: entity,
				},
				score: r.Score,
			})
		}
	}

	// Sort by score descending.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].score > all[j-1].score; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}

	// Cap at MaxResults.
	if len(all) > l.cfg.MaxResults {
		all = all[:l.cfg.MaxResults]
	}

	out := make([]onDemandResult, 0, len(all))
	for _, s := range all {
		out = append(out, s.r)
	}
	return out
}

// onDemandResult is an internal type carrying a single search result plus the
// entity that triggered the lookup.
type onDemandResult struct {
	fileID string
	text   string
	score  float64
	wing   string
	entity EntityMatch
}

// renderMarkdown assembles a Markdown snippet from the merged results.
// Format:
//
//	## Related context
//	- **{entity}**: {lead sentence}
//	- ...
func (l *OnDemandLayer) renderMarkdown(results []onDemandResult, entities []EntityMatch) string {
	// Build a lookup from normalized entity key to display text.
	entityDisplay := make(map[string]string, len(entities))
	for _, e := range entities {
		key := e.Candidate.Normalized
		if key == "" {
			key = e.Wing
			if key == "" {
				key = e.Room
			}
		}
		display := e.Candidate.Text
		if display == "" {
			display = e.Room
		}
		if display == "" {
			display = e.Wing
		}
		entityDisplay[key] = display
	}

	var b strings.Builder
	b.WriteString("## Related context\n")

	for _, r := range results {
		lead := extractLeadSentence(r.text)
		if lead == "" {
			continue
		}

		// Pick the entity display name for this result.
		entityKey := r.entity.Candidate.Normalized
		if entityKey == "" {
			entityKey = r.entity.Wing
			if entityKey == "" {
				entityKey = r.entity.Room
			}
		}
		display := entityDisplay[entityKey]
		if display == "" {
			display = entityKey
		}

		b.WriteString(fmt.Sprintf("- **%s**: %s\n", display, lead))
	}

	return b.String()
}
