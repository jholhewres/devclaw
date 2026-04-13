// Package copilot — palace_bot_commands.go parses and handles slash commands
// for palace-aware memory operations directly, without invoking the LLM.
//
// This is a pre-agent text parser: the assistant (or channel router) can
// call HandlePalaceBotCommand at the start of message processing. If the
// command is handled, the reply is returned directly and the LLM is never
// invoked. If the input is not a palace command, handled=false and the
// message continues through the normal LLM path.
//
// Supported commands (all slash-prefixed):
//
//	/wing                          Show current wing for this chat
//	/wing list                     List all wings with counts
//	/wing set <name>               Pin this chat to <name>
//	/wing unset                    Remove the pin
//	/wing merge <from> <to>        Merge one wing into another
//	/room                          Show rooms in the current chat's wing
//	/room list [wing]              List rooms (filtered by wing if given)
//	/tree                          Show full palace taxonomy
//	/palace help                   Show help text
//
// Design notes:
//
//   - All commands check the HierarchyConfig.Enabled flag. When off, they
//     reply with a "disabled" message that tells the user how to enable.
//   - Commands that modify state (set, unset, merge) call through the
//     ContextRouter and SQLiteStore. Read-only commands go directly to
//     the store.
//   - Responses are plain text suitable for Telegram/WhatsApp rendering.
//     No markdown escaping needed because channels re-escape on send.
//   - Bot commands never wait for LLM provider I/O, so they're cheap and
//     fast — good for rapid iteration on wing config.
//
// Per Sprint 0.5 critic feedback MAJOR-5: the `/wing merge` command MUST
// ship in Sprint 1 (was originally planned for Sprint 4 WebUI) because
// users in Telegram/WhatsApp have no other way to fix wing duplicates.
package copilot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// PalaceBotConfig bundles the dependencies a command handler needs.
// Typically constructed once at startup and passed to
// HandlePalaceBotCommand on every message.
type PalaceBotConfig struct {
	Store   *memory.SQLiteStore
	Router  *ContextRouter
	Enabled bool
}

// PalaceBotReply is the result of handling a command. When Handled is
// false, Reply should be ignored and the message should continue through
// the normal LLM path.
type PalaceBotReply struct {
	Handled bool
	Reply   string
	Err     error
}

// HandlePalaceBotCommand inspects an incoming chat message and, if it is
// a palace-aware slash command, handles it directly and returns the reply
// text. Callers invoke this before any LLM processing.
//
// Parameters:
//   - ctx: request context
//   - cfg: palace bot config (store, router, enabled flag)
//   - channel: the channel identifier (e.g., "telegram", "whatsapp")
//   - chatID: the external chat ID from the channel
//   - input: the raw message text from the user
//
// Returns a PalaceBotReply. If Handled is false, the caller should
// process the message normally. If Handled is true, the caller should
// send Reply back to the user via the channel and skip LLM invocation.
func HandlePalaceBotCommand(ctx context.Context, cfg PalaceBotConfig, channel, chatID, input string) PalaceBotReply {
	trimmed := strings.TrimSpace(input)

	// Fast path: not a slash command — not handled.
	if !strings.HasPrefix(trimmed, "/") {
		return PalaceBotReply{Handled: false}
	}

	// Strip leading slash and tokenize.
	trimmed = strings.TrimPrefix(trimmed, "/")
	tokens := strings.Fields(trimmed)
	if len(tokens) == 0 {
		return PalaceBotReply{Handled: false}
	}

	// Identify the root command. Anything else is not our concern.
	root := strings.ToLower(tokens[0])
	var sub string
	if len(tokens) > 1 {
		sub = strings.ToLower(tokens[1])
	}

	switch root {
	case "wing":
		return handleWingCommand(ctx, cfg, channel, chatID, sub, tokens[1:])
	case "room":
		return handleRoomCommand(ctx, cfg, channel, chatID, sub, tokens[1:])
	case "tree":
		return handleTreeCommand(cfg)
	case "palace":
		if sub == "help" {
			return PalaceBotReply{Handled: true, Reply: palaceHelpText()}
		}
		// /palace without subcommand → help.
		return PalaceBotReply{Handled: true, Reply: palaceHelpText()}
	}

	// Not one of our commands.
	return PalaceBotReply{Handled: false}
}

// ─────────────────────────────────────────────────────────────────────────────
// /wing command family
// ─────────────────────────────────────────────────────────────────────────────

// handleWingCommand dispatches /wing subcommands.
// args are the tokens AFTER /wing, with sub being args[0] in lowercase.
func handleWingCommand(ctx context.Context, cfg PalaceBotConfig, channel, chatID, sub string, args []string) PalaceBotReply {
	// Feature flag check applies to all /wing subcommands.
	if !cfg.Enabled {
		return PalaceBotReply{Handled: true, Reply: disabledMessage()}
	}

	// No subcommand → status.
	if sub == "" {
		return handleWingStatusCmd(ctx, cfg, channel, chatID)
	}

	switch sub {
	case "list":
		return handleWingListCmd(cfg)
	case "set":
		// /wing set <name>
		if len(args) < 2 {
			return PalaceBotReply{Handled: true, Reply: "usage: /wing set <name>"}
		}
		return handleWingSetCmd(cfg, channel, chatID, args[1])
	case "unset":
		return handleWingUnsetCmd(cfg, channel, chatID)
	case "merge":
		// /wing merge <from> <to>
		if len(args) < 3 {
			return PalaceBotReply{Handled: true, Reply: "usage: /wing merge <from> <to>"}
		}
		return handleWingMergeCmd(cfg, args[1], args[2])
	case "help":
		return PalaceBotReply{Handled: true, Reply: wingHelpText()}
	}

	// Unknown subcommand.
	return PalaceBotReply{Handled: true, Reply: fmt.Sprintf("unknown /wing subcommand: %s\n\n%s", sub, wingHelpText())}
}

func handleWingStatusCmd(ctx context.Context, cfg PalaceBotConfig, channel, chatID string) PalaceBotReply {
	if cfg.Router != nil {
		res := cfg.Router.Resolve(ctx, channel, chatID, "")
		if res.IsEmpty() {
			return PalaceBotReply{Handled: true, Reply: fmt.Sprintf(
				"This chat has no wing (source=%s). Memories are stored as legacy.\n\n"+
					"Use /wing set <name> to pin this chat to a specific wing.",
				res.Source)}
		}
		return PalaceBotReply{Handled: true, Reply: fmt.Sprintf(
			"This chat is pinned to wing=%s (confidence %.2f, source=%s).\n\n"+
				"Use /wing set <name> to change it, /wing unset to remove.",
			res.Wing, res.Confidence, res.Source)}
	}

	// No router available — direct store lookup.
	if cfg.Store == nil {
		return PalaceBotReply{Handled: true, Reply: "palace memory store is not configured"}
	}
	mapping, err := cfg.Store.GetChannelWing(channel, chatID)
	// Use errors.Is for future-proofing against error wrapping (ME-6).
	if errors.Is(err, memory.ErrChannelWingNotFound) {
		return PalaceBotReply{Handled: true, Reply: "This chat has no wing. Use /wing set <name> to pin."}
	}
	if err != nil {
		return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf("error: %v", err)}
	}
	return PalaceBotReply{Handled: true, Reply: fmt.Sprintf("wing=%s (confidence %.2f, source=%s)",
		mapping.Wing, mapping.Confidence, mapping.Source)}
}

func handleWingListCmd(cfg PalaceBotConfig) PalaceBotReply {
	if cfg.Store == nil {
		return PalaceBotReply{Handled: true, Reply: "palace memory store is not configured"}
	}
	wings, err := cfg.Store.ListWings()
	if err != nil {
		return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf("error: %v", err)}
	}
	if len(wings) == 0 {
		return PalaceBotReply{Handled: true, Reply: "No wings registered. Use /wing set <name> to create one."}
	}
	var sb strings.Builder
	sb.WriteString("Wings:\n")
	for _, w := range wings {
		suggested := ""
		if w.IsSuggested && w.MemoryCount == 0 {
			suggested = " (suggested)"
		}
		sb.WriteString(fmt.Sprintf("  • %s%s — %d memories, %d rooms\n",
			w.Name, suggested, w.MemoryCount, w.RoomCount))
	}
	legacy, _ := cfg.Store.TotalLegacyFiles()
	if legacy > 0 {
		sb.WriteString(fmt.Sprintf("\n  [legacy] — %d unclassified memories\n", legacy))
	}
	return PalaceBotReply{Handled: true, Reply: sb.String()}
}

func handleWingSetCmd(cfg PalaceBotConfig, channel, chatID, wingName string) PalaceBotReply {
	normalized := memory.NormalizeWing(wingName)
	if normalized == "" {
		return PalaceBotReply{Handled: true, Reply: fmt.Sprintf(
			"invalid wing name %q. Must contain letters/digits/hyphens. Reserved prefix __ rejected.", wingName)}
	}

	if cfg.Router != nil {
		if err := cfg.Router.Pin(channel, chatID, normalized); err != nil {
			return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf("error: %v", err)}
		}
	} else if cfg.Store != nil {
		if err := cfg.Store.SetChannelWing(channel, chatID, normalized, "manual", 1.0); err != nil {
			return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf("error: %v", err)}
		}
	} else {
		return PalaceBotReply{Handled: true, Reply: "palace memory store is not configured"}
	}

	return PalaceBotReply{Handled: true, Reply: fmt.Sprintf(
		"Pinned this chat to wing=%s. New memories will be routed there.", normalized)}
}

func handleWingUnsetCmd(cfg PalaceBotConfig, channel, chatID string) PalaceBotReply {
	if cfg.Router != nil {
		if err := cfg.Router.Unpin(channel, chatID); err != nil {
			return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf("error: %v", err)}
		}
	} else if cfg.Store != nil {
		if err := cfg.Store.DeleteChannelWing(channel, chatID); err != nil {
			return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf("error: %v", err)}
		}
	} else {
		return PalaceBotReply{Handled: true, Reply: "palace memory store is not configured"}
	}

	return PalaceBotReply{Handled: true, Reply: "Unset. This chat's wing will re-resolve via heuristics or default."}
}

// handleWingMergeCmd merges `from` wing into `to`. In Sprint 1 this is a
// registry-level merge only: it updates the `wings` table but does NOT
// migrate `files.wing` values en masse. That migration is Sprint 2 scope
// because it interacts with the markdown file layout.
//
// The command is still useful in Sprint 1 because it:
//   - Removes a duplicate suggested wing from the registry
//   - Updates channel_wing_map rows pointing at `from` to point at `to`
//   - Returns a status message with what was changed
//
// Sprint 2 will extend this to physically move files between directories.
func handleWingMergeCmd(cfg PalaceBotConfig, fromName, toName string) PalaceBotReply {
	if cfg.Store == nil {
		return PalaceBotReply{Handled: true, Reply: "palace memory store is not configured"}
	}

	from := memory.NormalizeWing(fromName)
	to := memory.NormalizeWing(toName)
	if from == "" || to == "" {
		return PalaceBotReply{Handled: true, Reply: fmt.Sprintf(
			"invalid wing name(s): from=%q to=%q", fromName, toName)}
	}
	if from == to {
		return PalaceBotReply{Handled: true, Reply: "cannot merge a wing into itself"}
	}

	// 1. Update channel_wing_map rows pointing at `from` to point at `to`.
	mappings, err := cfg.Store.ListChannelWings(from)
	if err != nil {
		return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf("error: %v", err)}
	}
	remapped := 0
	for _, m := range mappings {
		if err := cfg.Store.SetChannelWing(m.Channel, m.ExternalID, to, m.Source, m.Confidence); err != nil {
			return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf(
				"partial merge: remapped %d mappings before error: %v", remapped, err)}
		}
		remapped++
	}

	// 2. Ensure `to` wing exists in the registry.
	if err := cfg.Store.UpsertWing(to, "", ""); err != nil {
		return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf("error: %v", err)}
	}

	// 3. Delete the `from` wing from the registry (force=true because we
	//    just remapped its channel entries and Sprint 1 does not migrate files).
	//    NOTE: files with wing=from will still carry that label until Sprint 2
	//    adds physical file migration. We warn the user.
	if err := cfg.Store.DeleteWing(from, true); err != nil {
		// Non-fatal — the remap is already done.
		return PalaceBotReply{Handled: true, Reply: fmt.Sprintf(
			"Merged %d channel mapping(s) from %s → %s.\n\n"+
				"Note: the %s wing registry entry could not be deleted (%v). "+
				"File-level migration will happen in Sprint 2.",
			remapped, from, to, from, err)}
	}

	return PalaceBotReply{Handled: true, Reply: fmt.Sprintf(
		"Merged %d channel mapping(s) from %s → %s.\n\n"+
			"Note: any existing memories still carrying wing=%s label will be "+
			"migrated in Sprint 2. Search with wing=%s remains valid until then.",
		remapped, from, to, from, from)}
}

// ─────────────────────────────────────────────────────────────────────────────
// /room command family
// ─────────────────────────────────────────────────────────────────────────────

func handleRoomCommand(ctx context.Context, cfg PalaceBotConfig, channel, chatID, sub string, args []string) PalaceBotReply {
	if !cfg.Enabled {
		return PalaceBotReply{Handled: true, Reply: disabledMessage()}
	}

	if cfg.Store == nil {
		return PalaceBotReply{Handled: true, Reply: "palace memory store is not configured"}
	}

	// /room or /room list → show rooms in the current chat's wing.
	if sub == "" || sub == "list" {
		// Determine the target wing: if an explicit arg is passed after
		// "list", use that; otherwise resolve from current chat.
		var targetWing string
		if sub == "list" && len(args) > 1 {
			targetWing = memory.NormalizeWing(args[1])
		} else if cfg.Router != nil {
			res := cfg.Router.Resolve(ctx, channel, chatID, "")
			targetWing = res.Wing
		}

		rooms, err := cfg.Store.ListRooms(targetWing)
		if err != nil {
			return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf("error: %v", err)}
		}
		if len(rooms) == 0 {
			if targetWing == "" {
				return PalaceBotReply{Handled: true, Reply: "No rooms in any wing yet."}
			}
			return PalaceBotReply{Handled: true, Reply: fmt.Sprintf("No rooms in wing %q.", targetWing)}
		}
		var sb strings.Builder
		if targetWing == "" {
			sb.WriteString("All rooms:\n")
		} else {
			sb.WriteString(fmt.Sprintf("Rooms in %s:\n", targetWing))
		}
		for _, r := range rooms {
			sb.WriteString(fmt.Sprintf("  • %s/%s — %d memories\n", r.Wing, r.Name, r.MemoryCount))
		}
		return PalaceBotReply{Handled: true, Reply: sb.String()}
	}

	return PalaceBotReply{Handled: true, Reply: "usage: /room [list [wing]]"}
}

// ─────────────────────────────────────────────────────────────────────────────
// /tree command
// ─────────────────────────────────────────────────────────────────────────────

func handleTreeCommand(cfg PalaceBotConfig) PalaceBotReply {
	if !cfg.Enabled {
		return PalaceBotReply{Handled: true, Reply: disabledMessage()}
	}
	if cfg.Store == nil {
		return PalaceBotReply{Handled: true, Reply: "palace memory store is not configured"}
	}

	tree, err := cfg.Store.GetTaxonomy()
	if err != nil {
		return PalaceBotReply{Handled: true, Err: err, Reply: fmt.Sprintf("error: %v", err)}
	}
	legacy, _ := cfg.Store.TotalLegacyFiles()

	if len(tree) == 0 && legacy == 0 {
		return PalaceBotReply{Handled: true, Reply: "Palace is empty. Save some memories first."}
	}

	var sb strings.Builder
	sb.WriteString("Palace:\n")
	for _, entry := range tree {
		sb.WriteString(fmt.Sprintf("  %s (%d memories)\n", entry.Wing.Name, entry.Wing.MemoryCount))
		for _, r := range entry.Rooms {
			sb.WriteString(fmt.Sprintf("    ├─ %s (%d)\n", r.Name, r.MemoryCount))
		}
	}
	if legacy > 0 {
		sb.WriteString(fmt.Sprintf("\n  [legacy] — %d unclassified memories\n", legacy))
	}
	return PalaceBotReply{Handled: true, Reply: sb.String()}
}

// ─────────────────────────────────────────────────────────────────────────────
// Help + disabled messages
// ─────────────────────────────────────────────────────────────────────────────

func palaceHelpText() string {
	return `Palace-aware memory commands:

/wing                  Show current wing for this chat
/wing list             List all wings with counts
/wing set <name>       Pin this chat to <name>
/wing unset            Remove the pin
/wing merge <from> <to> Merge one wing into another

/room                  Show rooms in current chat's wing
/room list [wing]      List rooms (optionally filtered)

/tree                  Full palace taxonomy

/palace help           This message

Wings organize memories by context (work, personal, family, ...).
Rooms are fine-grained topics inside a wing (e.g., work/auth-migration).`
}

func wingHelpText() string {
	return `/wing                  Show current wing for this chat
/wing list             List all wings
/wing set <name>       Pin this chat to <name>
/wing unset            Remove the pin
/wing merge <from> <to> Merge wings`
}

func disabledMessage() string {
	return "Palace-aware memory is disabled. Enable via config memory.hierarchy.enabled=true."
}
