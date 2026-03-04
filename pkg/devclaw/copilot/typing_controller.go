// Package copilot – typing_controller.go provides a lifecycle-aware typing
// indicator controller that replaces the simple goroutine+ticker pattern.
//
// State machine:
//
//	Started → Active → RunComplete → DispatchIdle → Sealed
//
// - Started:      Controller created, not yet sending.
// - Active:       Ticker running, sending typing indicators at regular intervals.
// - RunComplete:  Agent execution finished; if block streamer is still dispatching
//                 buffered output we keep typing active for a grace period.
// - DispatchIdle: Block streamer has finished; typing stops but can be restarted
//                 if a followup run begins within the grace window.
// - Sealed:       Terminal state; no further typing will be sent.
//
// The controller also enforces a TTL so a stuck agent cannot send typing
// indicators indefinitely.
package copilot

import (
	"context"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

// TypingState represents the lifecycle state of the typing controller.
type TypingState int

const (
	TypingStarted      TypingState = iota // Created, not yet active.
	TypingActive                          // Sending typing indicators.
	TypingRunComplete                     // Agent done, grace period for dispatch.
	TypingDispatchIdle                    // Dispatch finished, idle.
	TypingSealed                          // Terminal, no further sends.
)

const (
	defaultTypingIntervalSec = 6    // Send typing every 6 seconds.
	defaultTypingTTLMs       = 120000 // Hard TTL: 2 minutes max.
	defaultTypingGraceMs     = 10000  // Grace period after run complete: 10 seconds.
)

// TypingController manages typing indicator lifecycle with state awareness.
type TypingController struct {
	state       TypingState
	channelMgr  *channels.Manager
	channel     string
	chatID      string
	intervalSec int
	ttlMs       int
	graceMs     int
	startedAt   time.Time
	ticker      *time.Ticker
	done        chan struct{}
	mu          sync.Mutex
}

// NewTypingController creates a new controller in the Started state.
func NewTypingController(channelMgr *channels.Manager, channel, chatID string) *TypingController {
	return &TypingController{
		state:       TypingStarted,
		channelMgr:  channelMgr,
		channel:     channel,
		chatID:      chatID,
		intervalSec: defaultTypingIntervalSec,
		ttlMs:       defaultTypingTTLMs,
		graceMs:     defaultTypingGraceMs,
	}
}

// Start transitions to Active and begins sending typing indicators.
// The context is used to cancel the typing loop if the parent context is cancelled.
func (tc *TypingController) Start(ctx context.Context) {
	tc.mu.Lock()
	if tc.state != TypingStarted {
		tc.mu.Unlock()
		return
	}
	tc.state = TypingActive
	tc.startedAt = time.Now()
	tc.done = make(chan struct{})
	tc.ticker = time.NewTicker(time.Duration(tc.intervalSec) * time.Second)
	tc.mu.Unlock()

	go tc.loop(ctx)
}

// loop is the typing heartbeat goroutine.
func (tc *TypingController) loop(ctx context.Context) {
	ttl := time.NewTimer(time.Duration(tc.ttlMs) * time.Millisecond)
	defer ttl.Stop()
	defer tc.ticker.Stop()

	for {
		select {
		case <-tc.done:
			return
		case <-ctx.Done():
			tc.sealInternal()
			return
		case <-ttl.C:
			// Hard TTL expired — seal to prevent indefinite typing.
			tc.sealInternal()
			return
		case <-tc.ticker.C:
			tc.mu.Lock()
			state := tc.state
			tc.mu.Unlock()

			switch state {
			case TypingActive, TypingRunComplete:
				tc.channelMgr.SendTyping(ctx, tc.channel, tc.chatID)
			case TypingDispatchIdle, TypingSealed:
				// Don't send in idle or sealed states.
				return
			}
		}
	}
}

// MarkRunComplete transitions to RunComplete state.
// Typing continues during a grace period to cover block streamer dispatch.
func (tc *TypingController) MarkRunComplete() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.state == TypingActive {
		tc.state = TypingRunComplete
		// Schedule transition to DispatchIdle after grace period.
		// Uses tc.done to cancel early if Seal() is called.
		go func() {
			timer := time.NewTimer(time.Duration(tc.graceMs) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
				tc.mu.Lock()
				if tc.state == TypingRunComplete {
					tc.state = TypingDispatchIdle
				}
				tc.mu.Unlock()
			case <-tc.done:
				// Sealed before grace period expired, nothing to do.
			}
		}()
	}
}

// MarkDispatchIdle transitions to DispatchIdle, stopping typing indicators.
func (tc *TypingController) MarkDispatchIdle() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.state == TypingRunComplete || tc.state == TypingActive {
		tc.state = TypingDispatchIdle
	}
}

// Seal transitions to the terminal Sealed state and stops the typing loop.
func (tc *TypingController) Seal() {
	tc.sealInternal()
}

// sealInternal transitions to Sealed and closes the done channel.
func (tc *TypingController) sealInternal() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.state == TypingSealed {
		return
	}
	tc.state = TypingSealed
	if tc.done != nil {
		select {
		case <-tc.done:
			// Already closed.
		default:
			close(tc.done)
		}
	}
}

// ShouldSuppress returns true if typing should be suppressed for the given
// content. Empty or internal-only content (heartbeats, etc.) should not
// restart typing indicators.
func (tc *TypingController) ShouldSuppress(content string) bool {
	return content == "" || content == "NO_REPLY" || content == "HEARTBEAT_OK"
}
