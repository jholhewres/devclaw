# DevClaw v1.19.0-rc1 Release Notes

**Release date:** 2026-04-08
**Branch:** `feature/long-memory-memplace`
**Status:** Release Candidate 1

---

## Summary

v1.19.0 ships the complete Sprint 2 layered memory system. The most visible
change is that every conversation now gets a structured memory prefix composed
of three new layers (L0 identity, L1 essential story, L2 on-demand retrieval)
in addition to the existing v1.18.0 hybrid-search block. All new behavior is
default-on and retrocompat-proven. An escape hatch (`force_legacy`) reverts to
v1.18.0 behavior without any database migration.

---

## What Is New

### Layered Memory Stack (MemoryStack)

The `MemoryStack` type (`pkg/devclaw/copilot/memory_stack.go`) assembles three
new layers into a prefix that is prepended to the existing L3 legacy output:

| Layer | Source | Budget |
|-------|--------|--------|
| L0 Identity | `~/.devclaw/identity.md` | Never trimmed |
| L1 Essential | Per-wing SQL template cache (6h TTL) | Trimmed second |
| L2 On-demand | Per-turn entity detection + SQL lookup | Trimmed first |

When all three layers render empty, the output is byte-identical to v1.18.0.
This is the retrocompat gate, enforced by `prompt_layers_golden_test.go`.

### `devclaw identity edit`

New CLI subcommand. Opens `$EDITOR` on `~/.devclaw/identity.md`, creating a
default template on first run. File changes are hot-reloaded via `fsnotify`
without restarting the daemon.

### Wing-Aware Hybrid Search

`HybridSearch` and `HybridSearchWithOptions` now route through
`HybridSearchWithOpts` internally. When a wing is resolved for the session,
matching-wing documents get a ×1.3 score boost and mismatched non-NULL wing
documents get a ×0.4 penalty. Files with `wing IS NULL` remain neutral
(multiplier 1.0) to preserve Sprint 1 retrocompat for legacy memories.

### Automatic Wing Routing on Save

`memory_save` now assigns `files.wing` automatically. Priority order:
1. Explicit `wing` argument in the LLM tool call
2. `ContextRouter` resolution from the session's `channel` + `chatID`
3. Nothing — `wing` stays `NULL` (legacy behavior; zero cost for CLI/MCP)

### Legacy Classifier in the Dream Cycle

The background `DreamConsolidator` now runs a `RunLegacyClassificationPass`
over `wing IS NULL` files on each dream cycle, using the keyword map the user
provides in `memory.hierarchy.legacy_keywords`. The binary ships zero keywords
— the classifier is a no-op until the user opts in. No LLM calls.

### New Telemetry Counters

Seven new counters appear in the `EmitSnapshot` slog output:

| Metric | Meaning |
|--------|---------|
| `layer_tokens_l0` | Cumulative bytes rendered by L0 per Build call |
| `layer_tokens_l1` | Cumulative bytes rendered by L1 per Build call |
| `layer_tokens_l2` | Cumulative bytes rendered by L2 per Build call |
| `l1_cache_hit_total` | L1 essential story served from cache |
| `l1_cache_miss_total` | L1 essential story required regeneration |
| `classifier_pass_total` | Dream classifier phases run |
| `save_wing_routed_total` | `memory_save` calls that assigned a wing |

### `memory.stack.force_legacy` Escape Hatch

A new YAML key bypasses the layered stack entirely:

```yaml
memory:
  stack:
    force_legacy: true
```

When set, `MemoryStack.Build()` returns `""` immediately, and `buildMemoryLayer`
produces byte-identical output to v1.18.0. No migration, no data loss. Remove
the key to re-enable the stack.

---

## Migration Notes

**No database migration required.** The `essential_stories` table is created
via `CREATE TABLE IF NOT EXISTS` on first startup. Existing rows in all tables
are never rewritten. Users upgrading from v1.18.0 will:

1. Get background dream consolidation for the first time (the `DreamConsolidator`
   was wired to the runtime in Sprint 2, not v1.17.0 when it was originally written).
2. See L0/L1/L2 prefixes in their prompts when `memory.hierarchy.enabled: true`
   (the default). If `identity.md` is absent and no wing is active, the prefix is
   empty and behavior is byte-identical to v1.18.0.

**Config file is fully backward-compatible.** Any v1.18.0 `devclaw.yaml` loads
without errors. The new `memory.stack` block defaults to `ForceLegacy: false`
(stack enabled) when absent. All existing `memory.hierarchy.*` keys are
unchanged.

---

## Rollback Instructions

If you need to revert to v1.18.0 behavior without downgrading the binary:

1. Add `memory.stack.force_legacy: true` to `devclaw.yaml`
2. Send a SIGHUP or restart the daemon to reload config
3. The layered stack is bypassed immediately — no data changes

To fully downgrade to the v1.18.0 binary:
1. Replace the binary
2. The `essential_stories` table remains in the database but is never read by
   v1.18.0 code — it is harmless dead weight (~bytes per wing)

---

## What to Test First (Maintainers)

- [ ] `go test -race -count=1 ./pkg/devclaw/copilot/... ./pkg/devclaw/copilot/memory/...` — all green
- [ ] `go run ./cmd/devclaw --help` — `identity` subcommand visible in output
- [ ] Start daemon, send a message, confirm L0/L1/L2 slog lines appear
- [ ] Set `memory.stack.force_legacy: true`, restart, confirm golden test still passes
- [ ] Check `EmitSnapshot` log output contains all 7 new metric names
- [ ] Upgrade a v1.18.0 database; confirm `essential_stories` table is created and
      existing memory rows are untouched
- [ ] Set `memory.hierarchy.legacy_keywords` to a small map, run a dream cycle,
      confirm `classifier_pass_total` increments
- [ ] Run `devclaw identity edit`, write some content, confirm next turn includes
      the identity prefix in the context

---

## Files Changed (Sprint 2 Room 2.5)

| File | Change |
|------|--------|
| `pkg/devclaw/copilot/config.go` | Added `MemoryStackConfig` struct + `Stack` field on `MemoryConfig` + `DefaultConfig()` initialization |
| `pkg/devclaw/copilot/memory_hierarchy_config.go` | Added 7 `atomic.Int64` counters, 7 `Inc*` helpers, extended `EmitSnapshot` |
| `pkg/devclaw/copilot/memory_stack.go` | Wired `IncLayerTokensL0/L1/L2` after Build telemetry; added `init()` to inject L1 cache callbacks |
| `pkg/devclaw/copilot/memory/layer_essential.go` | Added `l1CacheHitFn`/`l1CacheMissFn` vars + `SetL1CacheHitFn`/`SetL1CacheMissFn` setters; wired calls in `Render()` |
| `pkg/devclaw/copilot/assistant.go` | Changed `NewMemoryStack` call to read `a.config.Memory.Stack.ForceLegacy` |
| `pkg/devclaw/copilot/dream.go` | Added `IncClassifierPass()` call after `runClassifierPhase` |
| `pkg/devclaw/copilot/memory_tools.go` | Added `IncSaveWingRouted()` on successful `AssignWingToFile` |
| `pkg/devclaw/copilot/memory_hierarchy_config_test.go` | Added 4 new tests: defaults, YAML load, absent block, EmitSnapshot coverage |
| `docs/memory-system.md` | Added `## Layered Memory Stack (Sprint 2, v1.19.0+)` section |
| `CHANGELOG.md` | Added `[v1.19.0-rc1]` entry at top |
| `docs/devclaw-palace-memory/release-notes-v1.19.0-rc1.md` | This file |
