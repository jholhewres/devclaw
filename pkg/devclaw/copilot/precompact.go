package copilot

import (
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// preCompactSnapshotTTL bounds how long a working-context snapshot stays
// relevant. It is operational memory: useful right after a compaction so the
// agent does not re-derive its current goal/activity, and noise afterwards.
const preCompactSnapshotTTL = 24 * time.Hour

// buildPreCompactSnapshot assembles a compact, operational working-context
// snapshot from recent conversation history so the agent does not lose its goal
// and recent activity across a compaction (and re-derive work it already did —
// e.g. rediscovering the same bug hours later). Returns ok=false when there is
// nothing worth snapshotting.
//
// The snapshot is deterministic (no extra LLM call) and is stored as an
// operational memory with Origin=precompact and a short TTL, so retrieval can
// surface the latest one right after compaction and retention drops it after.
func buildPreCompactSnapshot(history []ConversationEntry, now time.Time) (memory.Entry, bool) {
	if len(history) == 0 {
		return memory.Entry{}, false
	}

	last := history[len(history)-1]

	// Goal = most recent non-empty user message.
	goal := strings.TrimSpace(last.UserMessage)
	if goal == "" {
		for i := len(history) - 1; i >= 0; i-- {
			if g := strings.TrimSpace(history[i].UserMessage); g != "" {
				goal = g
				break
			}
		}
	}
	if goal == "" {
		return memory.Entry{}, false
	}

	// Collect distinct tools used across recent turns (most recent first).
	seen := map[string]bool{}
	var tools []string
	addTool := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] || len(tools) >= 12 {
			return
		}
		seen[name] = true
		tools = append(tools, name)
	}
	for i := len(history) - 1; i >= 0 && len(tools) < 12; i-- {
		if len(history[i].ToolCalls) > 0 {
			for _, tc := range history[i].ToolCalls {
				addTool(tc.Name)
			}
		} else if history[i].ToolSummary != "" {
			for _, name := range strings.Split(history[i].ToolSummary, ",") {
				addTool(name)
			}
		}
	}

	var b strings.Builder
	b.WriteString("[precompact] Working-context snapshot before compaction.\n")
	b.WriteString("Goal: ")
	b.WriteString(truncateForCapture(goal, 400))
	if len(tools) > 0 {
		b.WriteString("\nRecent tools: ")
		b.WriteString(strings.Join(tools, ", "))
	}
	if resp := strings.TrimSpace(last.AssistantResponse); resp != "" {
		b.WriteString("\nLast action: ")
		b.WriteString(truncateForCapture(resp, 400))
	}

	exp := now.Add(preCompactSnapshotTTL)
	return memory.Entry{
		Content:     b.String(),
		Source:      "system",
		Category:    "summary",
		Timestamp:   now,
		ExpiresAt:   &exp,
		Origin:      "precompact",
		MemoryType:  "operational",
		ContextTier: "L1",
		Importance:  0.9,
	}, true
}
