// Package copilot – queue_modes.go implements configurable queue modes that
// control how the agent handles incoming messages while a session is busy.
// Supports: collect, steer, followup, interrupt, steer-backlog.
package copilot

import (
	"fmt"
	"strings"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels"
)

// QueueMode defines how incoming messages are handled when the session is busy.
type QueueMode string

const (
	// QueueModeCollect groups all queued messages into a single prompt.
	// Processed as one agent run after the current run completes.
	QueueModeCollect QueueMode = "collect"

	// QueueModeSteer injects messages into the active agent run via interruptCh.
	// The agent sees the message between turns and adjusts behavior.
	QueueModeSteer QueueMode = "steer"

	// QueueModeFollowup enqueues each message as a separate agent run.
	// Processed in order after the current run completes.
	QueueModeFollowup QueueMode = "followup"

	// QueueModeInterrupt aborts the current run and processes the new message.
	QueueModeInterrupt QueueMode = "interrupt"

	// QueueModeSteerBacklog tries steer first; if no active run to inject into,
	// falls back to followup.
	QueueModeSteerBacklog QueueMode = "steer-backlog"
)

// QueueDropPolicy defines what happens when the queue exceeds max size.
type QueueDropPolicy string

const (
	// DropOld removes the oldest messages to make room.
	DropOld QueueDropPolicy = "old"

	// DropNew rejects new messages when the queue is full.
	DropNew QueueDropPolicy = "new"

	// DropSummarize uses the LLM to summarize dropped messages.
	DropSummarize QueueDropPolicy = "summarize"
)

// EffectiveQueueMode returns the queue mode for a given channel, falling back
// to the default mode from QueueConfig (defined in config.go).
func EffectiveQueueMode(qc QueueConfig, channelName string) QueueMode {
	if mode, ok := qc.ByChannel[channelName]; ok {
		return mode
	}
	if qc.DefaultMode != "" {
		return qc.DefaultMode
	}
	return QueueModeSteer
}

// ParseQueueMode parses a string into a QueueMode. Returns (mode, true) on
// success, ("", false) on unknown mode.
func ParseQueueMode(s string) (QueueMode, bool) {
	switch QueueMode(strings.ToLower(strings.TrimSpace(s))) {
	case QueueModeCollect:
		return QueueModeCollect, true
	case QueueModeSteer:
		return QueueModeSteer, true
	case QueueModeFollowup:
		return QueueModeFollowup, true
	case QueueModeInterrupt:
		return QueueModeInterrupt, true
	case QueueModeSteerBacklog:
		return QueueModeSteerBacklog, true
	default:
		return "", false
	}
}

// FormatCollectedMessages combines multiple messages into a single prompt
// (used by QueueModeCollect).
func FormatCollectedMessages(msgs []*channels.IncomingMessage) string {
	if len(msgs) == 0 {
		return ""
	}
	if len(msgs) == 1 {
		return msgs[0].Content
	}

	var b strings.Builder
	b.WriteString("[Queued messages while agent was busy]\n---\n")
	for i, m := range msgs {
		b.WriteString(fmt.Sprintf("Queued #%d: %s\n", i+1, strings.TrimSpace(m.Content)))
		if i < len(msgs)-1 {
			b.WriteString("---\n")
		}
	}
	return b.String()
}
