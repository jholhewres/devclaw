// Package copilot — memory_hierarchy_config_test.go verifies that the
// palace-aware hierarchy config preserves its defaults when a user YAML
// omits the entire `memory.hierarchy` section. This is the critical
// retrocompat guarantee: users upgrading from v1.17.0 MUST NOT need to
// edit their devclaw.yaml for v1.18.0 to work — the defaults must kick
// in automatically.
//
// Related: Princípio Zero rule 3 "default off by default", rule 5
// "absence of yaml key = legacy-compatible behavior".
package copilot

import (
	"log/slog"
	"strings"
	"testing"
)

// TestHierarchyDefaultsAppliedWhenYAMLOmitsSection is the critical
// retrocompat test: user's YAML has NO `memory.hierarchy` section, and
// the defaults from DefaultHierarchyConfig() must be preserved.
func TestHierarchyDefaultsAppliedWhenYAMLOmitsSection(t *testing.T) {
	// Minimal YAML that does not mention hierarchy at all. A real user
	// YAML upgrading from v1.17.0 looks like this.
	yamlData := `
memory:
  type: sqlite
  path: /tmp/test.db
  max_messages: 100
`
	cfg, err := ParseConfig([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	h := cfg.Memory.Hierarchy
	// Every field that DefaultHierarchyConfig sets to a non-zero value
	// must remain non-zero after parsing a YAML that omits the section.
	// As of Sprint 1 amendment (2026-04-08), Enabled defaults to true.
	if h.Enabled != true {
		t.Errorf("Enabled: expected true (default), got %v", h.Enabled)
	}
	if h.DefaultWing != "" {
		t.Errorf("DefaultWing: expected empty (legacy first-class), got %q", h.DefaultWing)
	}
	if h.L1Budget != 400 {
		t.Errorf("L1Budget: expected 400, got %d — default NOT preserved", h.L1Budget)
	}
	if h.L2Budget != 300 {
		t.Errorf("L2Budget: expected 300, got %d — default NOT preserved", h.L2Budget)
	}
	if h.WingBoostMatch != 1.3 {
		t.Errorf("WingBoostMatch: expected 1.3, got %v — default NOT preserved", h.WingBoostMatch)
	}
	if h.WingBoostPenalty != 0.4 {
		t.Errorf("WingBoostPenalty: expected 0.4, got %v — default NOT preserved", h.WingBoostPenalty)
	}
	if h.AutoRoomCap != 30 {
		t.Errorf("AutoRoomCap: expected 30, got %d — default NOT preserved", h.AutoRoomCap)
	}
	if h.AutoRoomDedupeDistance != 2 {
		t.Errorf("AutoRoomDedupeDistance: expected 2, got %d — default NOT preserved", h.AutoRoomDedupeDistance)
	}
}

// TestHierarchyDefaultsAppliedWhenYAMLHasNoMemorySection verifies that
// a YAML with no `memory` section at all also gets the hierarchy defaults.
// This happens when a user has a minimal config for non-memory features.
func TestHierarchyDefaultsAppliedWhenYAMLHasNoMemorySection(t *testing.T) {
	yamlData := `
logging:
  level: info
`
	cfg, err := ParseConfig([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	h := cfg.Memory.Hierarchy
	if h.AutoRoomCap != 30 || h.WingBoostMatch != 1.3 || h.L1Budget != 400 {
		t.Errorf("hierarchy defaults not preserved without memory section: %+v", h)
	}
}

// TestHierarchyDefaultHeuristicsAndKeywordsAreEmpty verifies that
// DefaultHierarchyConfig ships zero heuristics and zero legacy keywords.
// This is the i18n + domain-neutrality guarantee: a fresh install classifies
// nothing via heuristics or legacy classifier until the user opts in via YAML.
func TestHierarchyDefaultHeuristicsAndKeywordsAreEmpty(t *testing.T) {
	cfg := DefaultHierarchyConfig()
	if len(cfg.Heuristics) != 0 {
		t.Errorf("DefaultHierarchyConfig().Heuristics: expected empty, got len=%d", len(cfg.Heuristics))
	}
	if len(cfg.LegacyKeywords) != 0 {
		t.Errorf("DefaultHierarchyConfig().LegacyKeywords: expected empty, got len=%d", len(cfg.LegacyKeywords))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Sprint 2 Room 2.5 — MemoryStackConfig + Phase C metric tests
// ─────────────────────────────────────────────────────────────────────────────

// TestMemoryStackConfigDefaults verifies that DefaultConfig returns
// ForceLegacy == false (stack active by default).
func TestMemoryStackConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Memory.Stack.ForceLegacy != false {
		t.Errorf("DefaultConfig().Memory.Stack.ForceLegacy: expected false, got %v", cfg.Memory.Stack.ForceLegacy)
	}
}

// TestMemoryStackConfigYAMLLoad verifies that memory.stack.force_legacy: true
// in YAML is correctly parsed.
func TestMemoryStackConfigYAMLLoad(t *testing.T) {
	yamlData := `
memory:
  stack:
    force_legacy: true
`
	cfg, err := ParseConfig([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if !cfg.Memory.Stack.ForceLegacy {
		t.Errorf("ForceLegacy: expected true after YAML load, got false")
	}
}

// TestMemoryStackConfigAbsentBlockDefaults verifies that a YAML without any
// memory.stack block yields ForceLegacy == false (stack active).
func TestMemoryStackConfigAbsentBlockDefaults(t *testing.T) {
	yamlData := `
memory:
  type: sqlite
`
	cfg, err := ParseConfig([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Memory.Stack.ForceLegacy {
		t.Errorf("ForceLegacy: expected false when stack block absent, got true")
	}
}

// TestEmitSnapshotIncludesSprintCTwoCounters verifies that all seven Phase C
// Sprint 2 counters appear in the EmitSnapshot output after being incremented.
func TestEmitSnapshotIncludesSprintCTwoCounters(t *testing.T) {
	// Use a bytes.Buffer-backed slog handler to capture log output.
	var buf strings.Builder
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Snapshot the global counters before modification and restore after.
	// We call the Inc* helpers and then read the snapshot from a fresh
	// HierarchyMetrics instance to keep the test isolated from process-global
	// state that other parallel tests may have accumulated.
	var m HierarchyMetrics
	m.layerTokensL0.Add(100)
	m.layerTokensL1.Add(200)
	m.layerTokensL2.Add(300)
	m.l1CacheHitTotal.Add(5)
	m.l1CacheMissTotal.Add(3)
	m.classifierPassTotal.Add(1)
	m.saveWingRoutedTotal.Add(7)

	m.EmitSnapshot(logger)

	got := buf.String()
	for _, want := range []string{
		"layer_tokens_l0",
		"layer_tokens_l1",
		"layer_tokens_l2",
		"l1_cache_hit_total",
		"l1_cache_miss_total",
		"classifier_pass_total",
		"save_wing_routed_total",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("EmitSnapshot output missing metric %q\nfull output:\n%s", want, got)
		}
	}
}

// TestHierarchyOverrideWithPartialYAMLPreservesUntouchedDefaults verifies
// that when a user sets ONLY some hierarchy fields, the untouched ones
// remain at defaults. This is the classic go-yaml footgun pattern — if
// yaml.v3 zeros untouched fields, we need to handle it like MediaConfig does.
func TestHierarchyOverrideWithPartialYAMLPreservesUntouchedDefaults(t *testing.T) {
	yamlData := `
memory:
  hierarchy:
    enabled: true
    l1_budget_tokens: 600
`
	cfg, err := ParseConfig([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	h := cfg.Memory.Hierarchy
	// Fields the user set (enabled=true is also the default, but YAML
	// explicitly sets it here):
	if h.Enabled != true {
		t.Errorf("Enabled: expected true (user set), got %v", h.Enabled)
	}
	if h.L1Budget != 600 {
		t.Errorf("L1Budget: expected 600 (user set), got %d", h.L1Budget)
	}
	// Fields the user did NOT set — MUST remain at defaults:
	if h.WingBoostMatch != 1.3 {
		t.Errorf("WingBoostMatch: expected 1.3 (default preserved), got %v", h.WingBoostMatch)
	}
	if h.AutoRoomCap != 30 {
		t.Errorf("AutoRoomCap: expected 30 (default preserved), got %d", h.AutoRoomCap)
	}
	if h.L2Budget != 300 {
		t.Errorf("L2Budget: expected 300 (default preserved), got %d", h.L2Budget)
	}
	if h.WingBoostPenalty != 0.4 {
		t.Errorf("WingBoostPenalty: expected 0.4 (default preserved), got %v", h.WingBoostPenalty)
	}
	if h.AutoRoomDedupeDistance != 2 {
		t.Errorf("AutoRoomDedupeDistance: expected 2 (default preserved), got %d", h.AutoRoomDedupeDistance)
	}
}
