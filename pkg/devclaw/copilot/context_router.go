// Package copilot — context_router.go routes incoming messages to the
// right palace wing based on (channel, external_id) context.
//
// Sprint 1 (v1.18.0), per ADR-001 + ADR-007-v2:
//
// When a message arrives from a channel (telegram, whatsapp, cli, mcp, ...),
// the context router decides which wing the resulting memory should be
// associated with. The decision uses a three-tier lookup:
//
//  1. EXPLICIT: the user (or a previous heuristic pass) mapped this
//     (channel, external_id) pair to a wing in the channel_wing_map table.
//     Confidence is whatever was recorded at mapping time (usually 1.0 for
//     manual entries, 0.5-0.8 for heuristic ones).
//
//  2. HEURISTIC: no explicit mapping exists. The router guesses a wing
//     based on user-configured patterns (HierarchyConfig.Heuristics).
//     The binary ships zero defaults — all heuristics are opt-in via YAML.
//     Confidence is 0.7 (constant). The guess is then persisted so
//     subsequent messages skip straight to tier 1.
//
//  3. DEFAULT: neither explicit nor heuristic. Returns an empty wing,
//     which propagates to wing=NULL in storage. This is a first-class
//     citizen per ADR-006: legacy and default-off behavior both live here.
//
// Retrocompat: the router NEVER returns an error. Every resolution path
// has a graceful fallback. Callers never need to handle "routing failed"
// — the worst case is a default (empty) wing which matches v1.17.0 behavior.
//
// Feature flag: this router is safe to instantiate regardless of the
// palace-aware flag state. When the flag is off, callers simply don't
// invoke Resolve — the router sits idle. When on, Resolve is called at
// the start of each turn for each incoming message.
package copilot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// WingResolutionSource identifies how a wing decision was reached.
// Useful for telemetry (ADR-008) and debugging.
type WingResolutionSource string

const (
	// SourceMapped — explicit entry in channel_wing_map.
	SourceMapped WingResolutionSource = "mapped"
	// SourceHeuristic — derived from channel pattern matching.
	SourceHeuristic WingResolutionSource = "heuristic"
	// SourceDefault — no mapping and no heuristic match; wing is empty.
	SourceDefault WingResolutionSource = "default"
	// SourceDisabled — context routing is off by feature flag; wing is empty.
	SourceDisabled WingResolutionSource = "disabled"
)

// WingResolution is what ContextRouter.Resolve returns. An empty Wing
// string is a valid, first-class result — it means "no wing", which maps
// to SQL NULL and matches the legacy behavior.
type WingResolution struct {
	Wing       string               // normalized wing identifier, "" = none
	Confidence float64              // 0.0 - 1.0; 0 for default/disabled
	Source     WingResolutionSource // where the decision came from
}

// IsEmpty reports whether this resolution yielded no wing.
// Callers use this to decide whether to pass wing through to downstream
// memory operations or to leave them unscoped.
func (w WingResolution) IsEmpty() bool {
	return w.Wing == ""
}

// routerCacheTTL is the time-to-live for entries in the ContextRouter's
// in-process cache. A value of 5 minutes keeps lookups O(1) during a
// realistic burst of traffic from channel reconnects (WhatsApp catching
// up 50+ queued messages in seconds) while keeping the cache tiny.
//
// This cache is the HI-1 fix from the Sprint 1 code review: without it,
// a reconnect storm can fire 50+ simultaneous SetChannelWing writes that
// all serialize behind SQLite's busy_timeout and inflate hot-path latency.
const routerCacheTTL = 5 * time.Minute

// routerCacheEntry is the value stored in ContextRouter.cache. It carries
// the full resolution plus an expiry timestamp so stale entries get
// lazily evicted on next access.
type routerCacheEntry struct {
	res       WingResolution
	expiresAt time.Time
}

// ContextRouter resolves (channel, external_id) pairs to palace wings.
// It is safe for concurrent use — all state lives in the SQLite store.
// An in-process cache reduces burst write pressure on the store.
type ContextRouter struct {
	store  *memory.SQLiteStore
	logger *slog.Logger
	cfg    HierarchyConfig

	// cache maps a (channel, externalID) key to a routerCacheEntry with
	// TTL-based eviction. sync.Map is used because the access pattern is
	// heavily read-skewed (one write per new key, many reads per existing
	// key) and entries are never deleted mid-read, which is sync.Map's
	// sweet spot.
	cache sync.Map // map[string]routerCacheEntry

	// cacheSingleflight deduplicates concurrent resolutions for the same
	// key. When the cache misses, the first goroutine acquires the mutex
	// for that key and does the full 3-tier lookup (including the store
	// write). Subsequent goroutines for the same key wait and then hit
	// the populated cache. This prevents the burst-write thundering herd.
	cacheSingleflight sync.Map // map[string]*sync.Mutex
}

// NewContextRouter creates a ContextRouter backed by the given memory store.
// The store may be nil — in that case, the router is a no-op that always
// returns a SourceDisabled resolution. This supports startup paths where
// the memory system failed to initialize but the channels are still running.
//
// cfg is the HierarchyConfig for this router instance. Heuristics are
// read from cfg.Heuristics — zero heuristics means tier 2 is a no-op.
func NewContextRouter(store *memory.SQLiteStore, logger *slog.Logger, cfg HierarchyConfig) *ContextRouter {
	if logger == nil {
		logger = slog.Default()
	}
	return &ContextRouter{
		store:  store,
		logger: logger,
		cfg:    cfg,
	}
}

// routerCacheKey produces the canonical string key for the cache. Kept
// in a helper so the cache format is in exactly one place and cannot
// drift between Resolve/Pin/Unpin.
func routerCacheKey(channel, externalID string) string {
	return channel + ":" + externalID
}

// invalidateCache removes a specific key from the cache. Called on Pin/Unpin
// to ensure a user's explicit wing change takes effect immediately rather
// than being shadowed by a stale entry until TTL expiry.
func (r *ContextRouter) invalidateCache(channel, externalID string) {
	r.cache.Delete(routerCacheKey(channel, externalID))
}

// Resolve determines which wing a message from (channel, externalID) should
// be associated with. This function NEVER returns an error — every failure
// mode falls through to SourceDefault with an empty wing.
//
// The hint parameter is an optional message preview that heuristics can
// inspect. Pass an empty string if no hint is available; heuristics will
// then rely purely on channel+externalID patterns.
//
// Resolve may write to the store: when a heuristic hit occurs, the result
// is persisted as a confidence<1.0 mapping so the next Resolve call for
// the same pair is a fast mapped lookup.
func (r *ContextRouter) Resolve(ctx context.Context, channel, externalID, hint string) WingResolution {
	// Guard rails: a nil store means we can't do any lookups. Return
	// a disabled resolution so the caller can still proceed.
	if r == nil || r.store == nil {
		IncContextRouterDisabled()
		return WingResolution{Source: SourceDisabled}
	}

	key := routerCacheKey(channel, externalID)

	// Cache tier (HI-1): check the in-process cache first. A hit means
	// we already resolved this key within the last routerCacheTTL and
	// can skip the store entirely.
	if v, ok := r.cache.Load(key); ok {
		entry := v.(routerCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			// Fresh hit — no store IO needed.
			return entry.res
		}
		// Stale — evict and fall through to recompute.
		r.cache.Delete(key)
	}

	// Singleflight: under burst load, prevent multiple goroutines from
	// all racing to do the full tier-1/2/3 resolution for the same key.
	// The first goroutine acquires the mutex and populates the cache;
	// subsequent goroutines wait, then hit the freshly-populated cache.
	mu := r.acquireSingleflight(key)
	mu.Lock()
	defer mu.Unlock()

	// Re-check the cache after acquiring the mutex: a previous holder
	// may have just populated it.
	if v, ok := r.cache.Load(key); ok {
		entry := v.(routerCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.res
		}
	}

	resolved := r.resolveUncached(ctx, channel, externalID, hint)
	r.cache.Store(key, routerCacheEntry{
		res:       resolved,
		expiresAt: time.Now().Add(routerCacheTTL),
	})
	return resolved
}

// acquireSingleflight returns (creating if necessary) the per-key mutex
// used to deduplicate concurrent misses. Uses LoadOrStore so the common
// path is lock-free.
func (r *ContextRouter) acquireSingleflight(key string) *sync.Mutex {
	if existing, ok := r.cacheSingleflight.Load(key); ok {
		return existing.(*sync.Mutex)
	}
	actual, _ := r.cacheSingleflight.LoadOrStore(key, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// resolveUncached performs the full 3-tier resolution without touching
// the cache. Callers must hold the singleflight mutex for the key.
func (r *ContextRouter) resolveUncached(ctx context.Context, channel, externalID, hint string) WingResolution {
	_ = ctx // reserved for future store calls that take context

	// Tier 1: explicit mapping lookup.
	mapping, err := r.store.GetChannelWing(channel, externalID)
	if err == nil {
		IncContextRouterMapped()
		return WingResolution{
			Wing:       mapping.Wing,
			Confidence: mapping.Confidence,
			Source:     SourceMapped,
		}
	}
	// If the error is not "not found", log and fall through.
	// Use errors.Is for future-proofing against error wrapping (ME-6).
	// Unexpected errors (disk full, DB locked, corruption) are logged at
	// WARN level so production operators see them without Debug enabled (ME-7).
	if !errors.Is(err, memory.ErrChannelWingNotFound) {
		r.logger.Warn("context router: store lookup failed unexpectedly",
			"channel", channel,
			"external_id", externalID,
			"error", err,
		)
	}

	// Tier 2: user-configured heuristic guess based on channel patterns.
	if wing, conf, ok := r.resolveByUserHeuristic(hint); ok {
		// Persist the guess so subsequent lookups skip straight to Tier 1.
		// Failures here are non-fatal — we still return the guess to the caller.
		if err := r.store.SetChannelWing(channel, externalID, wing, "heuristic", conf); err != nil {
			r.logger.Debug("context router: failed to persist heuristic mapping",
				"channel", channel,
				"external_id", externalID,
				"wing", wing,
				"error", err,
			)
		}
		IncContextRouterHeuristic()
		return WingResolution{
			Wing:       wing,
			Confidence: conf,
			Source:     SourceHeuristic,
		}
	}

	// Tier 3: no mapping, no heuristic match. Return empty wing (legacy).
	IncContextRouterDefault()
	return WingResolution{Source: SourceDefault}
}

// Pin creates or updates an explicit mapping for a (channel, externalID)
// pair. This is what the `/wing set` bot command and `devclaw wing map`
// CLI invoke. Confidence is always 1.0 because it's a user decision.
//
// Returns an error if the wing name is invalid or the store rejects the
// write. Callers (bot handlers, CLI) should surface the error to the user.
func (r *ContextRouter) Pin(channel, externalID, wing string) error {
	if r == nil || r.store == nil {
		return errRouterUnavailable
	}
	if err := r.store.SetChannelWing(channel, externalID, wing, "manual", 1.0); err != nil {
		return err
	}
	// Invalidate the cache so the next Resolve sees the new mapping
	// without waiting for routerCacheTTL to expire.
	r.invalidateCache(channel, externalID)
	return nil
}

// Unpin removes a mapping. Subsequent Resolve calls for the same pair will
// fall back to the heuristic tier or the default.
func (r *ContextRouter) Unpin(channel, externalID string) error {
	if r == nil || r.store == nil {
		return errRouterUnavailable
	}
	if err := r.store.DeleteChannelWing(channel, externalID); err != nil {
		return err
	}
	r.invalidateCache(channel, externalID)
	return nil
}

// errRouterUnavailable is returned by Pin/Unpin when the underlying store
// is nil. Distinct from ErrChannelWingNotFound because it indicates a
// configuration problem rather than a missing row.
var errRouterUnavailable = &routerError{msg: "context router: memory store is not configured"}

type routerError struct{ msg string }

func (e *routerError) Error() string { return e.msg }

// ─────────────────────────────────────────────────────────────────────────────
// Heuristics
// ─────────────────────────────────────────────────────────────────────────────

// userHeuristicConfidence is the fixed confidence score assigned to a wing
// resolved via a user-configured heuristic rule. It is deliberately constant
// (not tunable per rule) to keep the config surface minimal — telemetry can
// inform a future per-rule confidence field if needed.
const userHeuristicConfidence = 0.7

// resolveByUserHeuristic iterates cfg.Heuristics and checks whether the hint
// (channel name, group name, or display name from the channel) matches any
// user-configured pattern. First match wins.
//
// Returns ("", 0, false) when:
//   - cfg.Heuristics is nil or empty (no user config → no-op)
//   - hint is empty (nothing to match against)
//   - no pattern matches
//
// The hint is normalized to lowercase + accent-stripped before matching,
// consistent with how NormalizeWing treats wing names.
func (r *ContextRouter) resolveByUserHeuristic(hint string) (string, float64, bool) {
	if len(r.cfg.Heuristics) == 0 || hint == "" {
		return "", 0, false
	}

	// Normalize the hint: lowercase then strip common Latin accents so that
	// user-configured keywords match accented variants in the hint. Reuses
	// memory.StripAccents to keep normalization rules in a single source of
	// truth (Mn-combining-mark skip + replacement table).
	hintNorm := memory.StripAccents(strings.ToLower(hint))

	for _, h := range r.cfg.Heuristics {
		wing := memory.NormalizeWing(h.Wing)
		if wing == "" {
			continue
		}
		for _, kw := range h.MatchChannelName {
			if kw != "" && strings.Contains(hintNorm, strings.ToLower(kw)) {
				return wing, userHeuristicConfidence, true
			}
		}
		for _, kw := range h.MatchGroupName {
			if kw != "" && strings.Contains(hintNorm, strings.ToLower(kw)) {
				return wing, userHeuristicConfidence, true
			}
		}
	}
	return "", 0, false
}

