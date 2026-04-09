// Package copilot — memory_hierarchy_config.go holds the Sprint 1 (v1.18.0)
// palace-aware feature flag config and observability scaffolding.
//
// The config lives in its own file so config.go stays focused on core
// memory configuration. One line in config.go's MemoryConfig struct pulls
// HierarchyConfig in.
//
// Metric emission follows the devclaw convention of structured slog lines
// with a known prefix. A log aggregator (fluent-bit, vector, loki) can
// scrape these into Prometheus time series without DevClaw needing a
// direct Prometheus dependency.
//
// Metric naming convention (dot-separated, matching ADR-008):
//
//	memory.context_router.confidence      histogram bucket counter
//	memory.search.wing_filter_usage       counter labeled {on, off, fallback}
//	memory.hierarchy.enabled              gauge (0 or 1)
//	memory.layer_tokens                   histogram labeled {layer=L0|L1|L2}
//
// All metrics are emitted via slog at INFO level with a "metric" attribute
// equal to the metric name and a "value" attribute equal to the data point.
// Zero external dependencies.
package copilot

import (
	"log/slog"
	"sync/atomic"
)

// WingHeuristic matches channel or group name patterns to a wing.
// The binary ships zero defaults — all heuristics are user-provided via
// YAML config. This is intentional: hardcoded locale or domain-specific
// keywords would be biased for open-source deployments.
//
// Example YAML snippet under memory.hierarchy.heuristics:
//
//	heuristics:
//	  - wing: work
//	    match_channel_name: [work, job, office]
//	  - wing: family
//	    match_channel_name: [family, home, kids]
type WingHeuristic struct {
	// Wing is the canonical wing identifier to assign on match.
	Wing string `yaml:"wing"`
	// MatchChannelName contains lowercase substrings; if the hint (channel
	// name, group name, display name) contains any of them, this wing wins.
	MatchChannelName []string `yaml:"match_channel_name,omitempty"`
	// MatchGroupName is an alias for MatchChannelName for readability
	// when the hint comes from a messaging group name.
	MatchGroupName []string `yaml:"match_group_name,omitempty"`
}

// HierarchyConfig configures the palace-aware memory subsystem introduced
// in Sprint 1. It lives under MemoryConfig.Hierarchy and is opt-in:
// Enabled=false by default preserves v1.17.0 behavior byte-for-byte.
type HierarchyConfig struct {
	// Enabled turns palace-aware memory on or off at the subsystem level.
	//
	// When false:
	//   - The context router is instantiated but Resolve always returns
	//     SourceDisabled with an empty wing.
	//   - The memory_list_wings / memory_list_rooms / memory_get_taxonomy
	//     tools return a "feature disabled" error.
	//   - Search continues to use the v1.17.0 hybrid fusion without any
	//     wing boost. Byte-identical behavior guaranteed.
	//   - Schema additions remain in the DB (harmless: new columns are
	//     nullable, new tables are empty).
	//
	// When true:
	//   - Context router resolves (channel, chatID) to wings.
	//   - Palace tools are fully functional.
	//   - Wing boost is applied in hybrid search (Sprint 2).
	//   - L0/L1/L2 layered memory stack activates (Sprint 2).
	//
	// Default: false. Flip to default-on only after telemetry per ADR-008
	// gates are all green for 2+ weeks.
	Enabled bool `yaml:"enabled"`

	// DefaultWing is the fallback wing assigned when neither explicit
	// mapping nor heuristics match. Empty string (the default) means
	// "wing IS NULL" — the legacy first-class citizen behavior per ADR-006.
	//
	// Setting this to a non-empty value changes semantics: new memories
	// arriving through unmapped channels get assigned to DefaultWing
	// rather than staying as legacy. Use with caution — this effectively
	// "backfills by default" which violates Princípio Zero rule 4 unless
	// the user explicitly opts in.
	DefaultWing string `yaml:"default_wing"`

	// L1Budget is the token budget for Layer 1 (essential story).
	// Sprint 2 reads this. Sprint 1 stores it but does not consume it.
	// Default: 400 tokens.
	L1Budget int `yaml:"l1_budget_tokens"`

	// L2Budget is the token budget for Layer 2 (on-demand retrieval).
	// Sprint 2 reads this. Default: 300 tokens.
	L2Budget int `yaml:"l2_budget_tokens"`

	// WingBoostMatch is the multiplier applied to search scores when a
	// document's wing matches the query's wing. Sprint 2 reads this.
	// Default: 1.3. See doc-02-errata.md correction 3 — the value is
	// relative to DevClaw's weighted inverse rank fusion, NOT standard RRF k=60.
	WingBoostMatch float64 `yaml:"wing_boost_match,omitempty"`

	// WingBoostPenalty is the multiplier applied when a document's wing
	// differs from the query's wing. Default: 0.4 (-60% penalty).
	WingBoostPenalty float64 `yaml:"wing_boost_penalty,omitempty"`

	// AutoRoomCap is the maximum number of auto-created rooms per wing.
	// When exceeded, the least recently used auto-only rooms are archived.
	// Default: 30. See ADR-002 Addendum A.
	AutoRoomCap int `yaml:"auto_room_cap"`

	// AutoRoomDedupeDistance is the Levenshtein distance threshold for
	// insert-time room deduplication. If a candidate room has distance
	// <= this value from an existing auto-room in the same wing, the
	// existing room is reused instead of creating a new one. Default: 2.
	AutoRoomDedupeDistance int `yaml:"auto_room_dedupe_distance"`

	// IdentityPath is the filesystem path to the L0 identity markdown file.
	// Sprint 2 reads this. Empty string uses the default:
	// ~/.devclaw/identity.md. Sprint 1 stores it but does not consume it.
	IdentityPath string `yaml:"identity_path"`

	// Heuristics is the user-provided list of channel/group name patterns
	// used by the context router's heuristic tier. The binary ships zero
	// defaults — a fresh install classifies nothing (all memories land with
	// wing=NULL) until the user opts in via YAML. This is intentional:
	// hardcoded domain or locale keywords would be biased for an open-source
	// project with diverse deployments.
	//
	// The router iterates this slice in order; first match wins (confidence 0.7).
	// If nil or empty, the heuristic tier is a no-op and every unmapped
	// message falls through to DefaultWing or wing=NULL.
	Heuristics []WingHeuristic `yaml:"heuristics,omitempty"`

	// LegacyKeywords is the keyword-to-wing mapping used by the legacy
	// content classifier (RunLegacyClassificationPass). The binary ships
	// zero defaults — the classifier is a no-op unless the user provides
	// keywords here. This preserves locale and domain neutrality.
	//
	// Map key: wing identifier (e.g. "work", "family").
	// Map value: list of lowercase substrings that signal that wing.
	LegacyKeywords map[string][]string `yaml:"legacy_keywords,omitempty"`
}

// DefaultHierarchyConfig returns the defaults for HierarchyConfig.
//
// IMPORTANT — Sprint 1 amendment (2026-04-08): Enabled defaults to TRUE.
// This is a deliberate reversal of the original Sprint 0.5 "default off"
// stance. The rationale:
//
//  1. "wing IS NULL is a first-class citizen" (ADR-006) means legacy
//     memories continue to work transparently even when the feature is on.
//     The wing boost code path explicitly treats wing="" as NEUTRAL (no
//     boost, no penalty), so a v1.17.0 user's search results remain the
//     same ordering as before until they start classifying memories.
//
//  2. Feature flags that default off get abandoned. Most users never
//     discover them. DevClaw's differentiator is its memory — it must
//     ship active.
//
//  3. Sprint 1 adds only schema and tool infrastructure — it does NOT
//     rewrite file paths or auto-migrate legacy data. A user upgrading
//     from v1.17.0 sees no data change. The only visible surface is new
//     bot commands (/wing, /room, /tree, /palace help) and new LLM tools
//     that the agent can use to organize memories going forward.
//
//  4. Incremental improvement: as the user interacts with the assistant,
//     new memories gradually get routed to wings (full integration in
//     Sprint 2). The palace fills up organically. No config edits, no
//     migrations, no surprises.
//
// Users who need v1.17.0 byte-identical behavior can still opt out by
// setting memory.hierarchy.enabled: false in their YAML.
func DefaultHierarchyConfig() HierarchyConfig {
	return HierarchyConfig{
		Enabled:                true, // ON by default — see doc comment above
		DefaultWing:            "",   // empty = legacy wing=NULL
		L1Budget:               400,
		L2Budget:               300,
		WingBoostMatch:         1.3,
		WingBoostPenalty:       0.4,
		AutoRoomCap:            30,
		AutoRoomDedupeDistance: 2,
		IdentityPath:           "", // empty = use ~/.devclaw/identity.md
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Metrics scaffold (ADR-008)
// ─────────────────────────────────────────────────────────────────────────────

// HierarchyMetrics holds process-level counters for palace-aware features.
// Each counter is atomic and safe for concurrent use. Counters are exposed
// via EmitSnapshot which writes structured log lines that a log aggregator
// can scrape into Prometheus time series.
//
// Sprint 1 scope: scaffolding and a minimal counter set. Sprint 2 adds the
// layer_tokens histogram. Sprint 3 adds KG-related counters.
type HierarchyMetrics struct {
	// ContextRouterMapped counts Resolve calls that returned SourceMapped.
	ContextRouterMapped uint64

	// ContextRouterHeuristic counts Resolve calls that returned SourceHeuristic.
	ContextRouterHeuristic uint64

	// ContextRouterDefault counts Resolve calls that returned SourceDefault.
	ContextRouterDefault uint64

	// ContextRouterDisabled counts Resolve calls made while the flag is off.
	ContextRouterDisabled uint64

	// SearchWithWingFilter counts searches that used an explicit wing filter.
	// Sprint 2 activates this counter when wing boost lands.
	SearchWithWingFilter uint64

	// SearchWithoutWingFilter counts searches that omitted the wing filter.
	SearchWithoutWingFilter uint64

	// ToolCallsByName is incremented by the palace tool handlers with the
	// tool name as a label. Sprint 1 exposes the count via EmitSnapshot.
	// Using a fixed set of counters avoids the need for a map in hot paths.
	ListWingsCalls    uint64
	ListRoomsCalls    uint64
	GetTaxonomyCalls  uint64
	WingPinCalls      uint64
	WingUnpinCalls    uint64
	WingStatusCalls   uint64
}

// globalMetrics is the process-wide HierarchyMetrics instance. Callers
// either use the package-level Inc* helpers (which touch globalMetrics)
// or pass a per-request *HierarchyMetrics for testing.
var globalMetrics = &HierarchyMetrics{}

// Global returns the process-wide metrics instance. Prefer the Inc*
// helpers for single-counter updates.
func (HierarchyMetrics) Global() *HierarchyMetrics {
	return globalMetrics
}

// IncContextRouterMapped atomically increments the mapped counter.
func IncContextRouterMapped() { atomic.AddUint64(&globalMetrics.ContextRouterMapped, 1) }

// IncContextRouterHeuristic atomically increments the heuristic counter.
func IncContextRouterHeuristic() { atomic.AddUint64(&globalMetrics.ContextRouterHeuristic, 1) }

// IncContextRouterDefault atomically increments the default counter.
func IncContextRouterDefault() { atomic.AddUint64(&globalMetrics.ContextRouterDefault, 1) }

// IncContextRouterDisabled atomically increments the disabled counter.
func IncContextRouterDisabled() { atomic.AddUint64(&globalMetrics.ContextRouterDisabled, 1) }

// IncSearchWithWingFilter atomically increments the wing-filtered search counter.
func IncSearchWithWingFilter() { atomic.AddUint64(&globalMetrics.SearchWithWingFilter, 1) }

// IncSearchWithoutWingFilter atomically increments the unfiltered search counter.
func IncSearchWithoutWingFilter() { atomic.AddUint64(&globalMetrics.SearchWithoutWingFilter, 1) }

// IncToolCall atomically increments the counter for a given palace tool name.
// Unknown names are ignored (no-op) rather than creating new counters at
// runtime — this keeps the metric cardinality bounded.
func IncToolCall(name string) {
	switch name {
	case "memory_list_wings":
		atomic.AddUint64(&globalMetrics.ListWingsCalls, 1)
	case "memory_list_rooms":
		atomic.AddUint64(&globalMetrics.ListRoomsCalls, 1)
	case "memory_get_taxonomy":
		atomic.AddUint64(&globalMetrics.GetTaxonomyCalls, 1)
	case "memory_wing_pin":
		atomic.AddUint64(&globalMetrics.WingPinCalls, 1)
	case "memory_wing_unpin":
		atomic.AddUint64(&globalMetrics.WingUnpinCalls, 1)
	case "memory_wing_status":
		atomic.AddUint64(&globalMetrics.WingStatusCalls, 1)
	}
}

// EmitSnapshot logs the current counter values via slog at INFO level.
// Called periodically (e.g., every 5 minutes from a goroutine) to produce
// scrape-friendly log lines.
//
// The log format is:
//
//	metric=<name> value=<n> component=palace
//
// Log aggregators can parse these lines directly into Prometheus counters.
func (m *HierarchyMetrics) EmitSnapshot(logger *slog.Logger) {
	if m == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	emit := func(name string, value uint64) {
		logger.Info("palace metric",
			"metric", name,
			"value", value,
			"component", "palace",
		)
	}

	emit("memory.context_router.mapped_total", atomic.LoadUint64(&m.ContextRouterMapped))
	emit("memory.context_router.heuristic_total", atomic.LoadUint64(&m.ContextRouterHeuristic))
	emit("memory.context_router.default_total", atomic.LoadUint64(&m.ContextRouterDefault))
	emit("memory.context_router.disabled_total", atomic.LoadUint64(&m.ContextRouterDisabled))
	emit("memory.search.wing_filter_total", atomic.LoadUint64(&m.SearchWithWingFilter))
	emit("memory.search.no_wing_filter_total", atomic.LoadUint64(&m.SearchWithoutWingFilter))
	emit("memory.tools.list_wings_total", atomic.LoadUint64(&m.ListWingsCalls))
	emit("memory.tools.list_rooms_total", atomic.LoadUint64(&m.ListRoomsCalls))
	emit("memory.tools.get_taxonomy_total", atomic.LoadUint64(&m.GetTaxonomyCalls))
	emit("memory.tools.wing_pin_total", atomic.LoadUint64(&m.WingPinCalls))
	emit("memory.tools.wing_unpin_total", atomic.LoadUint64(&m.WingUnpinCalls))
	emit("memory.tools.wing_status_total", atomic.LoadUint64(&m.WingStatusCalls))
}

// EmitHierarchyEnabled logs the feature flag state as a gauge. Called once
// at startup and on any config reload. Log format matches EmitSnapshot.
func EmitHierarchyEnabled(logger *slog.Logger, enabled bool) {
	if logger == nil {
		logger = slog.Default()
	}
	value := uint64(0)
	if enabled {
		value = 1
	}
	logger.Info("palace metric",
		"metric", "memory.hierarchy.enabled",
		"value", value,
		"component", "palace",
	)
}
