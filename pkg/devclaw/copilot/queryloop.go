// Package copilot – queryloop.go defines the Phase-based query loop architecture.
// This extracts the conceptual phases from the monolithic agent.go Run() method
// into composable, testable units.
//
// The existing Run() method continues to work unchanged. This module provides
// the interfaces and orchestrator for incremental migration to phased execution.
//
// Phases:
//
//	Prepare   → build messages, resolve tools, load context
//	APICall   → call LLM with streaming/non-streaming
//	ToolExec  → execute tool calls from LLM response
//	Decision  → decide: tool_use → loop back, text → proceed to stop
//	StopCheck → verify completion before returning
package copilot

import (
	"context"
	"fmt"
	"time"
)

// ── Phase Interface ──

// NextAction tells the QueryLoop what to do after a phase completes.
type NextAction int

const (
	// ActionContinue proceeds to the next phase in sequence.
	ActionContinue NextAction = iota

	// ActionLoopBack returns to the APICall phase (tool_use detected).
	ActionLoopBack

	// ActionStop ends the turn and returns the response.
	ActionStop

	// ActionInject re-injects a message and loops back to APICall.
	ActionInject
)

// String returns a human-readable name for the action.
func (a NextAction) String() string {
	switch a {
	case ActionContinue:
		return "continue"
	case ActionLoopBack:
		return "loop_back"
	case ActionStop:
		return "stop"
	case ActionInject:
		return "inject"
	default:
		return fmt.Sprintf("unknown(%d)", a)
	}
}

// TurnState carries mutable state through the phase pipeline.
// Each phase can read and modify this state before passing it to the next.
type TurnState struct {
	// Messages is the current conversation history.
	Messages []chatMessage

	// Tools is the tool definitions available for this turn.
	Tools []ToolDefinition

	// Response holds the LLM response (set by APICall phase).
	Response *LLMResponse

	// ToolResults holds results from tool execution (set by ToolExec phase).
	ToolResults []ToolResult

	// FinalText is the final response text (set when Decision chooses Stop).
	FinalText string

	// InjectedMessage is a message to inject (set when Decision chooses Inject).
	InjectedMessage string

	// Turn is the current turn number.
	Turn int

	// Usage tracks accumulated token usage.
	Usage LLMUsage

	// Metadata holds arbitrary phase-specific data.
	Metadata map[string]any
}

// Phase represents a single stage in the query loop pipeline.
// Each phase receives the turn state, performs its work, and returns
// the next action for the loop orchestrator.
type Phase interface {
	// Name returns a human-readable identifier for logging and debugging.
	Name() string

	// Execute runs this phase's logic on the given turn state.
	// Returns the next action and any error.
	Execute(ctx context.Context, state *TurnState) (NextAction, error)
}

// ── Query Loop Orchestrator ──

// QueryLoop orchestrates the phase pipeline for a single agent turn.
// It runs phases in sequence, handling loop-back and injection actions.
type QueryLoop struct {
	phases []Phase

	// maxIterations prevents infinite loops (default: 50).
	maxIterations int

	// onPhaseComplete is called after each phase for logging/metrics.
	onPhaseComplete func(phase string, action NextAction, duration time.Duration)
}

// NewQueryLoop creates a loop with the given phases.
// Phases are executed in order: [0] → [1] → [2] → [3] → [4]
// ActionLoopBack returns to phase index 1 (APICall).
func NewQueryLoop(phases ...Phase) *QueryLoop {
	return &QueryLoop{
		phases:        phases,
		maxIterations: 50,
	}
}

// SetMaxIterations sets the safety limit for loop iterations.
func (q *QueryLoop) SetMaxIterations(n int) {
	if n > 0 {
		q.maxIterations = n
	}
}

// SetOnPhaseComplete sets a callback for phase completion events.
func (q *QueryLoop) SetOnPhaseComplete(fn func(phase string, action NextAction, duration time.Duration)) {
	q.onPhaseComplete = fn
}

// RunTurn executes the phase pipeline for a single turn.
// Returns the final state after all phases complete, or an error.
func (q *QueryLoop) RunTurn(ctx context.Context, state *TurnState) (*TurnState, error) {
	if len(q.phases) == 0 {
		return state, fmt.Errorf("queryloop: no phases configured")
	}
	if state.Metadata == nil {
		state.Metadata = make(map[string]any)
	}

	iterations := 0
	phaseIdx := 0

	for phaseIdx < len(q.phases) {
		iterations++
		if iterations > q.maxIterations {
			return state, fmt.Errorf("queryloop: exceeded max iterations (%d)", q.maxIterations)
		}

		// Check context cancellation.
		if ctx.Err() != nil {
			return state, ctx.Err()
		}

		phase := q.phases[phaseIdx]
		start := time.Now()

		action, err := phase.Execute(ctx, state)
		duration := time.Since(start)

		if q.onPhaseComplete != nil {
			q.onPhaseComplete(phase.Name(), action, duration)
		}

		if err != nil {
			return state, fmt.Errorf("queryloop phase %q: %w", phase.Name(), err)
		}

		switch action {
		case ActionContinue:
			phaseIdx++
		case ActionLoopBack:
			// Return to phase 1 (APICall) for another tool iteration.
			if len(q.phases) > 1 {
				phaseIdx = 1
			} else {
				phaseIdx = 0
			}
			state.Turn++
		case ActionStop:
			return state, nil
		case ActionInject:
			// Inject a message and return to APICall.
			if state.InjectedMessage != "" {
				state.Messages = append(state.Messages, chatMessage{
					Role:    "user",
					Content: state.InjectedMessage,
				})
				state.InjectedMessage = ""
			}
			if len(q.phases) > 1 {
				phaseIdx = 1
			} else {
				phaseIdx = 0
			}
			state.Turn++
		}
	}

	return state, nil
}

// ── Built-in Phase Implementations ──

// PreparePhase handles message building and tool resolution.
type PreparePhase struct {
	buildMessages func(state *TurnState) error
}

// NewPreparePhase creates a prepare phase with a message builder function.
func NewPreparePhase(builder func(state *TurnState) error) *PreparePhase {
	return &PreparePhase{buildMessages: builder}
}

func (p *PreparePhase) Name() string { return "prepare" }

func (p *PreparePhase) Execute(ctx context.Context, state *TurnState) (NextAction, error) {
	if p.buildMessages != nil {
		if err := p.buildMessages(state); err != nil {
			return ActionStop, err
		}
	}
	return ActionContinue, nil
}

// DecisionPhase decides whether to loop back (tool_use) or stop (text response).
type DecisionPhase struct{}

func (d *DecisionPhase) Name() string { return "decision" }

func (d *DecisionPhase) Execute(ctx context.Context, state *TurnState) (NextAction, error) {
	if state.Response == nil {
		return ActionStop, fmt.Errorf("no LLM response available")
	}

	// If tool calls are present, loop back for execution.
	if len(state.Response.ToolCalls) > 0 {
		return ActionLoopBack, nil
	}

	// Text response → stop.
	state.FinalText = state.Response.Content
	return ActionStop, nil
}

// StopCheckPhase runs completion verification before allowing stop.
type StopCheckPhase struct {
	verifier     *StopHookVerifier
	maxInjects   int // Max injection attempts before giving up (default: 3).
	injectCount  int
}

// NewStopCheckPhase creates a stop check phase with the given verifier.
// Limits injection to 3 attempts to prevent infinite loops.
func NewStopCheckPhase(v *StopHookVerifier) *StopCheckPhase {
	return &StopCheckPhase{verifier: v, maxInjects: 3}
}

func (s *StopCheckPhase) Name() string { return "stop_check" }

func (s *StopCheckPhase) Execute(ctx context.Context, state *TurnState) (NextAction, error) {
	if s.verifier == nil {
		return ActionStop, nil
	}

	// Cap injections to prevent infinite loops.
	if s.injectCount >= s.maxInjects {
		return ActionStop, nil
	}

	incomplete := s.verifier.VerifyCompletion(state.Messages)
	if len(incomplete) == 0 {
		return ActionStop, nil
	}

	// Inject a reminder and continue.
	s.injectCount++
	state.InjectedMessage = FormatIncompleteWorkMessage(incomplete)
	return ActionInject, nil
}
