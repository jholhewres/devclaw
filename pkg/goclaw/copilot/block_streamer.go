// Package copilot – block_streamer.go implements progressive message delivery
// for channels. Instead of waiting for the full LLM response, text is coalesced
// into blocks and sent as they become available, giving the user near-real-time
// feedback similar to OpenClaw's block streaming.
//
// Coalescing rules:
//   - Wait until at least MinChars are accumulated.
//   - Flush when MaxChars is reached or the idle timer fires.
//   - Always try to flush at a natural boundary (newline, sentence end).
package copilot

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jholhewres/goclaw/pkg/goclaw/channels"
)

// BlockStreamConfig configures the progressive message streaming behavior.
type BlockStreamConfig struct {
	// Enabled turns block streaming on/off (default: true).
	Enabled bool `yaml:"enabled"`

	// MinChars is the minimum characters to accumulate before sending a block (default: 600).
	MinChars int `yaml:"min_chars"`

	// MaxChars is the maximum characters per block before a forced flush (default: 1200).
	MaxChars int `yaml:"max_chars"`

	// IdleMs is the idle timeout in milliseconds: if no new tokens arrive within
	// this window, flush whatever is buffered (default: 1500).
	IdleMs int `yaml:"idle_ms"`
}

// DefaultBlockStreamConfig returns sensible defaults for block streaming.
// Tuned for WhatsApp/chat UX: send text quickly so the user sees progress.
func DefaultBlockStreamConfig() BlockStreamConfig {
	return BlockStreamConfig{
		Enabled:  true,
		MinChars: 150,  // Send small blocks early for real-time feel
		MaxChars: 800,  // Reasonable block size for chat apps
		IdleMs:   800,  // Flush quickly when LLM pauses (tool calls, thinking)
	}
}

// Effective returns a copy with defaults filled in for zero values.
func (c BlockStreamConfig) Effective() BlockStreamConfig {
	out := c
	if out.MinChars <= 0 {
		out.MinChars = 150
	}
	if out.MaxChars <= 0 {
		out.MaxChars = 800
	}
	if out.IdleMs <= 0 {
		out.IdleMs = 800
	}
	return out
}

// BlockStreamer accumulates LLM stream tokens and sends them progressively
// to a channel. It is tied to a single message exchange (one user message →
// one agent response).
type BlockStreamer struct {
	cfg        BlockStreamConfig
	channelMgr *channels.Manager
	channel    string
	chatID     string
	replyTo    string // original message ID for threading

	mu      sync.Mutex
	buf     strings.Builder
	sent    int  // total chars sent so far
	done    bool // Finish() was called
	flushed bool // at least one block was sent

	idleTimer *time.Timer
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewBlockStreamer creates a streamer that progressively sends blocks to the given channel.
func NewBlockStreamer(
	cfg BlockStreamConfig,
	channelMgr *channels.Manager,
	channel, chatID, replyTo string,
) *BlockStreamer {
	cfg = cfg.Effective()
	ctx, cancel := context.WithCancel(context.Background())
	return &BlockStreamer{
		cfg:        cfg,
		channelMgr: channelMgr,
		channel:    channel,
		chatID:     chatID,
		replyTo:    replyTo,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// StreamCallback returns a StreamCallback function suitable for AgentRun.SetStreamCallback.
func (bs *BlockStreamer) StreamCallback() StreamCallback {
	return func(chunk string) {
		bs.mu.Lock()
		defer bs.mu.Unlock()

		if bs.done {
			return
		}

		bs.buf.WriteString(chunk)

		// Reset idle timer on every token.
		bs.resetIdleTimer()

		// Check if we should flush.
		if bs.buf.Len() >= bs.cfg.MaxChars {
			bs.flushLocked()
		}
	}
}

// FlushNow immediately sends any buffered text to the channel, regardless of
// MinChars threshold. Use this before tool execution to ensure the user sees
// the LLM's intermediate text (thoughts/reasoning) before tools start running.
func (bs *BlockStreamer) FlushNow() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.done || bs.buf.Len() == 0 {
		return
	}
	if bs.idleTimer != nil {
		bs.idleTimer.Stop()
	}
	bs.flushLocked()
}

// Finish flushes any remaining buffer and marks the streamer as done.
func (bs *BlockStreamer) Finish() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.done {
		return // Already finished — idempotent.
	}

	bs.done = true
	if bs.idleTimer != nil {
		bs.idleTimer.Stop()
	}

	// IMPORTANT: Flush remaining text BEFORE cancelling the context.
	// The send operation uses bs.ctx, so cancelling first would silently
	// drop the final message — causing the user to never receive the response.
	if bs.buf.Len() > 0 {
		bs.flushLocked()
	}

	bs.cancel()
}

// HasSentBlocks returns true if at least one block was sent progressively.
func (bs *BlockStreamer) HasSentBlocks() bool {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.flushed
}

// resetIdleTimer resets the idle flush timer. Must be called with mu held.
func (bs *BlockStreamer) resetIdleTimer() {
	if bs.idleTimer != nil {
		bs.idleTimer.Stop()
	}

	idleDuration := time.Duration(bs.cfg.IdleMs) * time.Millisecond
	bs.idleTimer = time.AfterFunc(idleDuration, func() {
		bs.mu.Lock()
		defer bs.mu.Unlock()

		if bs.done {
			return
		}

		// Flush whatever is buffered when idle — don't wait for MinChars.
		// This ensures the user sees text promptly even for short responses.
		if bs.buf.Len() > 0 {
			bs.flushLocked()
		}
	})
}

// flushLocked sends the current buffer as a message block. Must be called with mu held.
func (bs *BlockStreamer) flushLocked() {
	text := bs.buf.String()
	if len(strings.TrimSpace(text)) == 0 {
		return
	}

	// Try to break at a natural boundary if we're mid-buffer and over MinChars.
	sendText := text
	remainder := ""

	if len(text) > bs.cfg.MinChars && !bs.done {
		// Look for a good break point near MinChars..MaxChars.
		breakIdx := findNaturalBreak(text, bs.cfg.MinChars, bs.cfg.MaxChars)
		if breakIdx > 0 && breakIdx < len(text) {
			sendText = text[:breakIdx]
			remainder = text[breakIdx:]
		}
	}

	// Format for channel.
	sendText = FormatForChannel(sendText, bs.channel)

	msg := &channels.OutgoingMessage{
		Content: strings.TrimSpace(sendText),
	}
	// Only reply-to the original on the first block.
	if !bs.flushed {
		msg.ReplyTo = bs.replyTo
	}

	if err := bs.channelMgr.Send(bs.ctx, bs.channel, bs.chatID, msg); err != nil {
		// Silently ignore send errors during streaming — the final sendReply
		// will attempt to send the complete message as fallback.
		return
	}

	bs.flushed = true
	bs.sent += len(sendText)

	// Reset buffer with remainder.
	bs.buf.Reset()
	if remainder != "" {
		bs.buf.WriteString(remainder)
	}
}

// findNaturalBreak finds a good text break point between minIdx and maxIdx.
// Prefers paragraph breaks > sentence ends > word boundaries.
func findNaturalBreak(text string, minIdx, maxIdx int) int {
	if maxIdx > len(text) {
		maxIdx = len(text)
	}
	if minIdx >= maxIdx {
		return maxIdx
	}

	region := text[minIdx:maxIdx]

	// Look for paragraph break (double newline).
	if idx := strings.LastIndex(region, "\n\n"); idx >= 0 {
		return minIdx + idx + 2
	}

	// Look for single newline.
	if idx := strings.LastIndex(region, "\n"); idx >= 0 {
		return minIdx + idx + 1
	}

	// Look for sentence end (. ! ?).
	for i := len(region) - 1; i >= 0; i-- {
		ch := region[i]
		if (ch == '.' || ch == '!' || ch == '?') && i+1 < len(region) && region[i+1] == ' ' {
			return minIdx + i + 2
		}
	}

	// Look for word boundary (space).
	if idx := strings.LastIndex(region, " "); idx >= 0 {
		return minIdx + idx + 1
	}

	return maxIdx
}
