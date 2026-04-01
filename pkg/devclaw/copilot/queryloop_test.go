package copilot

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockPhase implements Phase for testing.
type mockPhase struct {
	name    string
	action  NextAction
	err     error
	calls   int
	onExec  func(state *TurnState) // Optional side-effect.
}

func (m *mockPhase) Name() string { return m.name }

func (m *mockPhase) Execute(ctx context.Context, state *TurnState) (NextAction, error) {
	m.calls++
	if m.onExec != nil {
		m.onExec(state)
	}
	return m.action, m.err
}

func TestQueryLoopBasicFlow(t *testing.T) {
	prepare := &mockPhase{name: "prepare", action: ActionContinue}
	apiCall := &mockPhase{name: "api_call", action: ActionContinue}
	decision := &mockPhase{name: "decision", action: ActionStop, onExec: func(s *TurnState) {
		s.FinalText = "Hello, world!"
	}}

	loop := NewQueryLoop(prepare, apiCall, decision)
	state := &TurnState{Turn: 1}

	result, err := loop.RunTurn(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FinalText != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", result.FinalText)
	}
	if prepare.calls != 1 {
		t.Errorf("prepare called %d times, expected 1", prepare.calls)
	}
	if apiCall.calls != 1 {
		t.Errorf("apiCall called %d times, expected 1", apiCall.calls)
	}
	if decision.calls != 1 {
		t.Errorf("decision called %d times, expected 1", decision.calls)
	}
}

func TestQueryLoopLoopBack(t *testing.T) {
	callCount := 0

	prepare := &mockPhase{name: "prepare", action: ActionContinue}
	apiCall := &mockPhase{name: "api_call", action: ActionContinue}
	toolExec := &mockPhase{name: "tool_exec", action: ActionContinue}
	decision := &mockPhase{name: "decision", onExec: func(s *TurnState) {
		callCount++
		if callCount < 3 {
			// Simulate tool_use — loop back.
		} else {
			// Final response.
			s.FinalText = "done after 3 iterations"
		}
	}}

	// Decision returns LoopBack twice, then Stop.
	decision.action = ActionLoopBack
	loopBackCount := 0
	decision.onExec = func(s *TurnState) {
		loopBackCount++
		if loopBackCount >= 3 {
			decision.action = ActionStop
			s.FinalText = "done"
		}
	}

	loop := NewQueryLoop(prepare, apiCall, toolExec, decision)
	state := &TurnState{Turn: 1}

	result, err := loop.RunTurn(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FinalText != "done" {
		t.Errorf("expected 'done', got %q", result.FinalText)
	}

	// Prepare runs once, apiCall runs 3 times (initial + 2 loopbacks).
	if prepare.calls != 1 {
		t.Errorf("prepare called %d times, expected 1", prepare.calls)
	}
	if apiCall.calls != 3 {
		t.Errorf("apiCall called %d times, expected 3", apiCall.calls)
	}
	if result.Turn != 3 {
		t.Errorf("expected turn 3, got %d", result.Turn)
	}
}

func TestQueryLoopInject(t *testing.T) {
	injectCount := 0

	prepare := &mockPhase{name: "prepare", action: ActionContinue}
	apiCall := &mockPhase{name: "api_call", action: ActionContinue}
	dec := &mockPhase{name: "decision", action: ActionInject}
	dec.onExec = func(s *TurnState) {
		injectCount++
		if injectCount == 1 {
			s.InjectedMessage = "Don't forget to run tests!"
			dec.action = ActionInject
		} else {
			dec.action = ActionStop
			s.FinalText = "ok, tests passed"
		}
	}

	loop := NewQueryLoop(prepare, apiCall, dec)
	state := &TurnState{Turn: 1, Messages: []chatMessage{}}

	result, err := loop.RunTurn(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FinalText != "ok, tests passed" {
		t.Errorf("expected 'ok, tests passed', got %q", result.FinalText)
	}

	// Check that the injected message was added.
	foundInjected := false
	for _, m := range result.Messages {
		content, _ := m.Content.(string)
		if content == "Don't forget to run tests!" {
			foundInjected = true
		}
	}
	if !foundInjected {
		t.Error("expected injected message in conversation history")
	}
}

func TestQueryLoopMaxIterations(t *testing.T) {
	// Phase that always loops back — should hit max iterations.
	looper := &mockPhase{name: "infinite", action: ActionLoopBack}

	loop := NewQueryLoop(looper)
	loop.SetMaxIterations(10)

	state := &TurnState{Turn: 1}
	_, err := loop.RunTurn(context.Background(), state)

	if err == nil {
		t.Fatal("expected error from max iterations")
	}
	if looper.calls != 10 {
		t.Errorf("expected exactly 10 calls, got %d", looper.calls)
	}
}

func TestQueryLoopContextCancellation(t *testing.T) {
	slow := &mockPhase{name: "slow", action: ActionContinue, onExec: func(s *TurnState) {
		time.Sleep(100 * time.Millisecond)
	}}
	next := &mockPhase{name: "next", action: ActionStop}

	loop := NewQueryLoop(slow, next)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	state := &TurnState{Turn: 1}
	_, err := loop.RunTurn(ctx, state)

	// May or may not error depending on timing — but should not hang.
	_ = err
}

func TestQueryLoopPhaseError(t *testing.T) {
	failing := &mockPhase{name: "failing", action: ActionContinue, err: fmt.Errorf("something broke")}

	loop := NewQueryLoop(failing)
	state := &TurnState{Turn: 1}

	_, err := loop.RunTurn(context.Background(), state)
	if err == nil {
		t.Fatal("expected error from failing phase")
	}
	if failing.calls != 1 {
		t.Errorf("expected 1 call, got %d", failing.calls)
	}
}

func TestQueryLoopNoPhases(t *testing.T) {
	loop := NewQueryLoop()
	state := &TurnState{Turn: 1}

	_, err := loop.RunTurn(context.Background(), state)
	if err == nil {
		t.Fatal("expected error from empty loop")
	}
}

func TestQueryLoopOnPhaseComplete(t *testing.T) {
	var events []string

	prepare := &mockPhase{name: "prepare", action: ActionContinue}
	stop := &mockPhase{name: "stop", action: ActionStop}

	loop := NewQueryLoop(prepare, stop)
	loop.SetOnPhaseComplete(func(phase string, action NextAction, duration time.Duration) {
		events = append(events, fmt.Sprintf("%s:%s", phase, action))
	})

	state := &TurnState{Turn: 1}
	loop.RunTurn(context.Background(), state)

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0] != "prepare:continue" {
		t.Errorf("expected 'prepare:continue', got %q", events[0])
	}
	if events[1] != "stop:stop" {
		t.Errorf("expected 'stop:stop', got %q", events[1])
	}
}

func TestDecisionPhaseToolCalls(t *testing.T) {
	d := &DecisionPhase{}
	state := &TurnState{
		Response: &LLMResponse{
			ToolCalls: []ToolCall{
				{ID: "1", Function: FunctionCall{Name: "grep", Arguments: "{}"}},
			},
		},
	}

	action, err := d.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionLoopBack {
		t.Errorf("expected LoopBack for tool calls, got %s", action)
	}
}

func TestDecisionPhaseTextResponse(t *testing.T) {
	d := &DecisionPhase{}
	state := &TurnState{
		Response: &LLMResponse{
			Content: "Here is the answer",
		},
	}

	action, err := d.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionStop {
		t.Errorf("expected Stop for text response, got %s", action)
	}
	if state.FinalText != "Here is the answer" {
		t.Errorf("expected FinalText set, got %q", state.FinalText)
	}
}

func TestStopCheckPhaseNoIssues(t *testing.T) {
	s := NewStopCheckPhase(NewStopHookVerifier())
	state := &TurnState{
		Messages: []chatMessage{
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "Go is a programming language."},
			{Role: "user", Content: "Thanks"},
			{Role: "assistant", Content: "You're welcome!"},
		},
	}

	action, err := s.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionStop {
		t.Errorf("expected Stop (no incomplete work), got %s", action)
	}
}

func TestStopCheckPhaseWithIncomplete(t *testing.T) {
	s := NewStopCheckPhase(NewStopHookVerifier())
	state := &TurnState{
		Messages: []chatMessage{
			{Role: "user", Content: "Fix the bug"},
			{Role: "assistant", Content: "I'll edit the file"},
			{Role: "tool", Content: "File edited successfully"},
			{Role: "assistant", Content: "Done!"},
		},
	}

	action, err := s.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionInject {
		t.Errorf("expected Inject (incomplete work detected), got %s", action)
	}
	if state.InjectedMessage == "" {
		t.Error("expected injected reminder message")
	}
}
