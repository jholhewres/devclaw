// Package copilot — memory_hierarchy_tools.go registers the palace-aware
// memory tools introduced in Sprint 1 (v1.18.0).
//
// These tools are ADDITIVE: they extend the existing memory toolset
// (memory_save, memory_search, memory_list, memory_index) with five new
// focused tools that expose the wing/room hierarchy and the context router.
//
// The tools are registered separately from the core memory tools so that:
//   - The core set continues to work when hierarchy is disabled
//   - Enabling/disabling hierarchy is a single call site change
//   - Tests can register one set without the other
//
// Feature flag: each tool checks cfg.Enabled before doing any work. When
// disabled, the tool is still registered (so the LLM discovers it), but
// returns a helpful error explaining that the feature is off. This is
// better UX than hiding the tool entirely — the LLM learns the capability
// exists and the user can opt in.
//
// New tools added in this file:
//
//	memory_list_wings       — enumerate wings with counts
//	memory_list_rooms       — enumerate rooms inside a wing
//	memory_get_taxonomy     — full wing → rooms tree
//	memory_wing_pin         — pin (channel, chatID) → wing mapping
//	memory_wing_unpin       — remove a pinned mapping
//	memory_wing_status      — show current wing for a channel/chat
//
// NOT added in this PR (deferred):
//
//   - memory_save wing/room params — requires rerouting markdown file path
//     into wing subdirectories, which is a larger change. Sprint 1 PR #4+.
//   - memory_search wing filter — requires wing boost in hybrid fusion,
//     which affects score semantics. Sprint 2.
//   - memory_wing_merge / memory_room_merge — bot commands first, see PR #4.
//
// The deferral is intentional: PR #3 adds the taxonomy/routing primitives
// without touching the hot path of save/search. This keeps retrocompat
// airtight while delivering user-visible functionality.
package copilot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// MemoryHierarchyDispatcherConfig holds configuration for the palace-aware
// memory tools. Callers construct one with a reference to the SQLite store
// and a feature flag; the registrar wires it into the ToolExecutor.
type MemoryHierarchyDispatcherConfig struct {
	// SQLiteStore is the memory store that exposes the palace operations
	// (UpsertWing, ListRooms, GetChannelWing, ...). Must not be nil when
	// Enabled is true; if nil, the tools return errors uniformly.
	SQLiteStore *memory.SQLiteStore

	// Router resolves (channel, chatID) to wings. May be nil if the caller
	// only wants the read-side tools. When nil, pin/unpin tools return
	// a "router unavailable" error.
	Router *ContextRouter

	// Enabled gates every tool behavior. When false, tools are still
	// registered but return a feature-disabled error on invocation.
	Enabled bool

	// Logger is used for structured logging of tool calls. Defaults to
	// slog.Default() if nil.
	Logger *slog.Logger
}

// errHierarchyDisabled is returned by tools when the feature flag is off.
// It carries a user-facing explanation of how to enable the feature.
var errHierarchyDisabled = errors.New(
	"palace-aware memory is disabled. Enable via config `memory.hierarchy.enabled: true` " +
		"or CLI flag --memory-hierarchy-enabled. See docs/memory-system.md for details.")

// errHierarchyStoreUnavailable is returned when the store was not
// configured (misconfiguration at startup). Distinct from feature-disabled
// so monitoring can alert.
var errHierarchyStoreUnavailable = errors.New(
	"palace-aware memory store unavailable: SQLite store failed to initialize " +
		"during startup. Check logs for schema errors.")

// RegisterMemoryHierarchyTools registers the six palace-aware memory tools
// on the given ToolExecutor. This is additive — existing memory tools are
// not touched.
//
// The registration happens unconditionally (even when cfg.Enabled is false)
// so that the LLM always sees the tools in its schema. At call time, each
// tool checks the flag and returns errHierarchyDisabled if off.
func RegisterMemoryHierarchyTools(executor *ToolExecutor, cfg MemoryHierarchyDispatcherConfig) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// ── memory_list_wings ──
	executor.RegisterHidden(
		MakeToolDefinition("memory_list_wings",
			"List all wings (top-level contextual namespaces) registered "+
				"in the palace-aware memory. Each wing groups memories by "+
				"context: work, personal, family, etc. Returns counts of "+
				"memories and rooms per wing.",
			map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		func(ctx context.Context, _ map[string]any) (any, error) {
			return handleListWings(ctx, cfg)
		},
	)

	// ── memory_list_rooms ──
	executor.RegisterHidden(
		MakeToolDefinition("memory_list_rooms",
			"List rooms inside a specific wing. Rooms are fine-grained "+
				"topical groupings (e.g., wing=work, room=auth-migration). "+
				"Pass an empty wing to list all rooms across wings.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"wing": map[string]any{
						"type":        "string",
						"description": "Wing name to list rooms from. Empty = all wings.",
					},
				},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleListRooms(ctx, cfg, args)
		},
	)

	// ── memory_get_taxonomy ──
	executor.RegisterHidden(
		MakeToolDefinition("memory_get_taxonomy",
			"Return the full wing → rooms tree with counts. Use this to "+
				"get an overview of how memories are organized across the "+
				"palace. Also reports the count of legacy (unclassified) memories.",
			map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		func(ctx context.Context, _ map[string]any) (any, error) {
			return handleGetTaxonomy(ctx, cfg)
		},
	)

	// ── memory_wing_pin ──
	executor.RegisterHidden(
		MakeToolDefinition("memory_wing_pin",
			"Pin a (channel, chat_id) pair to a specific wing. Subsequent "+
				"memories arriving from that chat will be routed to the pinned "+
				"wing with confidence 1.0. Use this when the heuristic "+
				"misclassifies a channel.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel": map[string]any{
						"type":        "string",
						"description": "Channel identifier: telegram, whatsapp, cli, mcp, discord, slack",
					},
					"chat_id": map[string]any{
						"type":        "string",
						"description": "External chat/session identifier (e.g., Telegram chat ID)",
					},
					"wing": map[string]any{
						"type":        "string",
						"description": "Wing name to pin. Will be normalized (lowercase, kebab-case, accents removed).",
					},
				},
				"required": []string{"channel", "chat_id", "wing"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleWingPin(ctx, cfg, args)
		},
	)

	// ── memory_wing_unpin ──
	executor.RegisterHidden(
		MakeToolDefinition("memory_wing_unpin",
			"Remove a pinned (channel, chat_id) → wing mapping. Subsequent "+
				"resolutions fall back to heuristics or the default wing.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel": map[string]any{
						"type":        "string",
						"description": "Channel identifier",
					},
					"chat_id": map[string]any{
						"type":        "string",
						"description": "External chat/session identifier",
					},
				},
				"required": []string{"channel", "chat_id"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleWingUnpin(ctx, cfg, args)
		},
	)

	// ── memory_wing_status ──
	executor.RegisterHidden(
		MakeToolDefinition("memory_wing_status",
			"Show the current wing mapping for a (channel, chat_id) pair. "+
				"Returns the wing, confidence, and source (mapped/heuristic/default).",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel": map[string]any{
						"type":        "string",
						"description": "Channel identifier",
					},
					"chat_id": map[string]any{
						"type":        "string",
						"description": "External chat/session identifier",
					},
				},
				"required": []string{"channel", "chat_id"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleWingStatus(ctx, cfg, args)
		},
	)

	logger.Debug("palace-aware memory tools registered", "enabled", cfg.Enabled)
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool handlers
// ─────────────────────────────────────────────────────────────────────────────

// enforceEnabled returns the gating error if the feature is off or the store
// is missing. Called at the start of every handler.
func enforceEnabled(cfg MemoryHierarchyDispatcherConfig) error {
	if !cfg.Enabled {
		return errHierarchyDisabled
	}
	if cfg.SQLiteStore == nil {
		return errHierarchyStoreUnavailable
	}
	return nil
}

// handleListWings renders a human-readable list of wings with counts.
func handleListWings(_ context.Context, cfg MemoryHierarchyDispatcherConfig) (any, error) {
	IncToolCall("memory_list_wings")
	if err := enforceEnabled(cfg); err != nil {
		return nil, err
	}

	wings, err := cfg.SQLiteStore.ListWings()
	if err != nil {
		return nil, fmt.Errorf("list wings: %w", err)
	}
	legacyCount, _ := cfg.SQLiteStore.TotalLegacyFiles()

	if len(wings) == 0 && legacyCount == 0 {
		return "No wings registered yet and no legacy memories. The palace is empty.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Palace contains %d wings:\n\n", len(wings)))
	for _, w := range wings {
		display := w.DisplayName
		if display == "" {
			display = w.Name
		}
		suggested := ""
		if w.IsSuggested && w.MemoryCount == 0 {
			suggested = " (suggested, unused)"
		}
		sb.WriteString(fmt.Sprintf("  • %s%s — %d memories, %d rooms\n",
			display, suggested, w.MemoryCount, w.RoomCount))
	}
	if legacyCount > 0 {
		sb.WriteString(fmt.Sprintf("\nLegacy (unclassified): %d memories\n", legacyCount))
		sb.WriteString("These are preserved from v1.17.0 and are a first-class citizen per ADR-006.\n")
	}
	return sb.String(), nil
}

// handleListRooms renders rooms inside a wing (or all wings if empty).
func handleListRooms(_ context.Context, cfg MemoryHierarchyDispatcherConfig, args map[string]any) (any, error) {
	IncToolCall("memory_list_rooms")
	if err := enforceEnabled(cfg); err != nil {
		return nil, err
	}

	wing, _ := args["wing"].(string)

	rooms, err := cfg.SQLiteStore.ListRooms(wing)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}

	if len(rooms) == 0 {
		if wing == "" {
			return "No rooms registered in any wing yet.", nil
		}
		return fmt.Sprintf("No rooms in wing %q.", wing), nil
	}

	var sb strings.Builder
	if wing == "" {
		sb.WriteString(fmt.Sprintf("All rooms (%d):\n\n", len(rooms)))
	} else {
		sb.WriteString(fmt.Sprintf("Rooms in wing %q (%d):\n\n", wing, len(rooms)))
	}
	for _, r := range rooms {
		hallNote := ""
		if r.Hall != "" {
			hallNote = fmt.Sprintf(" [hall: %s]", r.Hall)
		}
		sourceNote := ""
		if r.Source != "manual" {
			sourceNote = fmt.Sprintf(" (%s, conf %.2f)", r.Source, r.Confidence)
		}
		sb.WriteString(fmt.Sprintf("  • %s/%s%s%s — %d memories, reused %d×\n",
			r.Wing, r.Name, hallNote, sourceNote, r.MemoryCount, r.ReuseCount))
	}
	return sb.String(), nil
}

// handleGetTaxonomy renders the full wing → rooms tree.
func handleGetTaxonomy(_ context.Context, cfg MemoryHierarchyDispatcherConfig) (any, error) {
	IncToolCall("memory_get_taxonomy")
	if err := enforceEnabled(cfg); err != nil {
		return nil, err
	}

	tree, err := cfg.SQLiteStore.GetTaxonomy()
	if err != nil {
		return nil, fmt.Errorf("get taxonomy: %w", err)
	}
	legacyCount, _ := cfg.SQLiteStore.TotalLegacyFiles()

	if len(tree) == 0 && legacyCount == 0 {
		return "Palace is empty. No wings or memories registered.", nil
	}

	var sb strings.Builder
	sb.WriteString("Palace taxonomy:\n\n")
	for _, entry := range tree {
		wingDisplay := entry.Wing.DisplayName
		if wingDisplay == "" {
			wingDisplay = entry.Wing.Name
		}
		sb.WriteString(fmt.Sprintf("  %s — %d memories\n", wingDisplay, entry.Wing.MemoryCount))
		for _, r := range entry.Rooms {
			sb.WriteString(fmt.Sprintf("    ├─ %s (%d)\n", r.Name, r.MemoryCount))
		}
	}
	if legacyCount > 0 {
		sb.WriteString(fmt.Sprintf("\n  [legacy] — %d unclassified memories (v1.17.0)\n", legacyCount))
	}
	return sb.String(), nil
}

// handleWingPin creates or updates a channel → wing mapping.
func handleWingPin(_ context.Context, cfg MemoryHierarchyDispatcherConfig, args map[string]any) (any, error) {
	IncToolCall("memory_wing_pin")
	if err := enforceEnabled(cfg); err != nil {
		return nil, err
	}

	channel := strings.TrimSpace(stringArg(args, "channel"))
	chatID := strings.TrimSpace(stringArg(args, "chat_id"))
	wing := strings.TrimSpace(stringArg(args, "wing"))

	if channel == "" || chatID == "" || wing == "" {
		return nil, fmt.Errorf("channel, chat_id, and wing are required")
	}

	if cfg.Router == nil {
		// Fall back to direct store write if no router is attached.
		if err := cfg.SQLiteStore.SetChannelWing(channel, chatID, wing, "manual", 1.0); err != nil {
			return nil, fmt.Errorf("pin wing: %w", err)
		}
	} else {
		if err := cfg.Router.Pin(channel, chatID, wing); err != nil {
			return nil, fmt.Errorf("pin wing: %w", err)
		}
	}

	return fmt.Sprintf("Pinned %s:%s → wing=%s (confidence 1.0)",
		channel, chatID, memory.NormalizeWing(wing)), nil
}

// handleWingUnpin removes a channel → wing mapping.
func handleWingUnpin(_ context.Context, cfg MemoryHierarchyDispatcherConfig, args map[string]any) (any, error) {
	IncToolCall("memory_wing_unpin")
	if err := enforceEnabled(cfg); err != nil {
		return nil, err
	}

	channel := strings.TrimSpace(stringArg(args, "channel"))
	chatID := strings.TrimSpace(stringArg(args, "chat_id"))
	if channel == "" || chatID == "" {
		return nil, fmt.Errorf("channel and chat_id are required")
	}

	if cfg.Router != nil {
		if err := cfg.Router.Unpin(channel, chatID); err != nil {
			return nil, fmt.Errorf("unpin: %w", err)
		}
	} else {
		if err := cfg.SQLiteStore.DeleteChannelWing(channel, chatID); err != nil {
			return nil, fmt.Errorf("unpin: %w", err)
		}
	}

	return fmt.Sprintf("Unpinned %s:%s. Next messages will re-resolve via heuristics or default.",
		channel, chatID), nil
}

// handleWingStatus shows the current resolution for a (channel, chat_id) pair.
func handleWingStatus(ctx context.Context, cfg MemoryHierarchyDispatcherConfig, args map[string]any) (any, error) {
	IncToolCall("memory_wing_status")
	if err := enforceEnabled(cfg); err != nil {
		return nil, err
	}

	channel := strings.TrimSpace(stringArg(args, "channel"))
	chatID := strings.TrimSpace(stringArg(args, "chat_id"))
	if channel == "" || chatID == "" {
		return nil, fmt.Errorf("channel and chat_id are required")
	}

	// Prefer the router's resolution (exercises the full pipeline) but fall
	// back to direct store lookup if no router is attached.
	if cfg.Router != nil {
		res := cfg.Router.Resolve(ctx, channel, chatID, "")
		if res.IsEmpty() {
			return fmt.Sprintf("%s:%s → (no wing, source=%s) — memories would be stored as legacy",
				channel, chatID, res.Source), nil
		}
		return fmt.Sprintf("%s:%s → wing=%s (confidence %.2f, source=%s)",
			channel, chatID, res.Wing, res.Confidence, res.Source), nil
	}

	mapping, err := cfg.SQLiteStore.GetChannelWing(channel, chatID)
	if errors.Is(err, memory.ErrChannelWingNotFound) {
		return fmt.Sprintf("%s:%s → (no wing) — memories would be stored as legacy",
			channel, chatID), nil
	}
	if err != nil {
		return nil, err
	}
	return fmt.Sprintf("%s:%s → wing=%s (confidence %.2f, source=%s)",
		channel, chatID, mapping.Wing, mapping.Confidence, mapping.Source), nil
}

// stringArg extracts a string argument from a tool args map, returning
// empty string if missing or of the wrong type. This matches the style
// used in memory_tools.go.
func stringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}
