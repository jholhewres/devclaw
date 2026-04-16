package copilot

import (
	"bytes"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestSubagentManager builds a manager suitable for unit tests — no DB,
// modest limits, logger writes to the provided buffer so tests can inspect
// emitted log records.
func newTestSubagentManager(tb testing.TB, logBuf *bytes.Buffer) *SubagentManager {
	tb.Helper()
	handler := slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return NewSubagentManager(SubagentConfig{
		Enabled:        true,
		MaxConcurrent:  4,
		MaxTurns:       5,
		TimeoutSeconds: 30,
		MaxSpawnDepth:  2,
	}, slog.New(handler))
}

// newRun creates a pre-filled SubagentRun ready to be passed to completeRun.
func newRun(id string, scope DeliveryScope) *SubagentRun {
	return &SubagentRun{
		ID:            id,
		Label:         "test-" + id,
		Status:        SubagentStatusRunning,
		DeliveryScope: scope,
		StartedAt:     time.Now(),
		done:          make(chan struct{}),
	}
}

// registerRun inserts a run into the manager's in-memory map so completeRun
// can find it without going through Spawn.
func registerRun(m *SubagentManager, run *SubagentRun) {
	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()
}

func TestSubagentAnnounce_ParentIdle(t *testing.T) {
	// Contract under "idle parent" conditions: completeRun must fire the
	// announce callback exactly once via goroutine so the Assistant's
	// followup enqueue can run. Verified with a sync channel, no Sleep.
	mgr := newTestSubagentManager(t, new(bytes.Buffer))
	received := make(chan *SubagentRun, 1)
	mgr.SetAnnounceCallback(func(run *SubagentRun) {
		received <- run
	})

	run := newRun("idle-1", DeliveryScopeAll)
	registerRun(mgr, run)

	mgr.completeRun(run, "ok", nil)

	select {
	case got := <-received:
		if got.ID != "idle-1" {
			t.Errorf("callback received wrong run: got %q", got.ID)
		}
		if got.Status != SubagentStatusCompleted {
			t.Errorf("status = %q, want completed", got.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("announce callback did not fire within 1s")
	}
}

func TestSubagentAnnounce_ParentBusy(t *testing.T) {
	// Contract under "busy parent" conditions: the manager still fires the
	// callback — the Assistant is responsible for queueing the result when
	// IsProcessing=true. Invariant tested: completeRun always fires the
	// callback regardless of parent state, so Assistant owns the decision.
	mgr := newTestSubagentManager(t, new(bytes.Buffer))
	var calls int32
	done := make(chan struct{}, 2)
	mgr.SetAnnounceCallback(func(run *SubagentRun) {
		atomic.AddInt32(&calls, 1)
		done <- struct{}{}
	})

	for _, id := range []string{"busy-1", "busy-2"} {
		run := newRun(id, DeliveryScopeAll)
		registerRun(mgr, run)
		mgr.completeRun(run, "ok", nil)
	}

	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("callback did not fire for run %d within 1s", i)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("callback fired %d times, want 2", got)
	}
}

func TestSubagentAnnounce_EmptyOriginResolvesFromParentID(t *testing.T) {
	mgr := newTestSubagentManager(t, new(bytes.Buffer))
	ch, to, err := mgr.resolveSpawnOrigin(SpawnParams{
		ParentSessionID: "webui:user123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch != "webui" || to != "user123" {
		t.Errorf("got (%q, %q), want (webui, user123)", ch, to)
	}
}

func TestSubagentAnnounce_UnresolvableOriginFailsFast(t *testing.T) {
	mgr := newTestSubagentManager(t, new(bytes.Buffer))
	_, _, err := mgr.resolveSpawnOrigin(SpawnParams{
		ParentSessionID: "abcdef12", // hash-like, no colon
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not resolvable") {
		t.Errorf("error should mention 'not resolvable', got: %v", err)
	}
}

func TestSubagentAnnounce_NestedSpawnInheritsOrigin(t *testing.T) {
	// Nested spawn path: ParentSessionID="subagent:<runID>" should inherit
	// OriginChannel/OriginTo from the parent SubagentRun stored in the
	// manager's map.
	mgr := newTestSubagentManager(t, new(bytes.Buffer))
	parent := &SubagentRun{
		ID:            "parent-1",
		OriginChannel: "whatsapp",
		OriginTo:      "user@c.us",
		DeliveryScope: DeliveryScopeAll,
		StartedAt:     time.Now(),
		done:          make(chan struct{}),
	}
	registerRun(mgr, parent)

	ch, to, err := mgr.resolveSpawnOrigin(SpawnParams{
		ParentSessionID: "subagent:parent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch != "whatsapp" || to != "user@c.us" {
		t.Errorf("got (%q, %q), want (whatsapp, user@c.us)", ch, to)
	}
}

func TestSubagentAnnounce_DeliveryScopeExternal_SkipsParent(t *testing.T) {
	logBuf := new(bytes.Buffer)
	mgr := newTestSubagentManager(t, logBuf)
	called := make(chan struct{}, 1)
	mgr.SetAnnounceCallback(func(run *SubagentRun) {
		called <- struct{}{}
	})

	run := newRun("ext-1", DeliveryScopeExternal)
	registerRun(mgr, run)
	mgr.completeRun(run, "ok", nil)

	select {
	case <-called:
		t.Fatal("callback fired for delivery_scope=external, expected skip")
	case <-time.After(100 * time.Millisecond):
		// expected — no call
	}

	if !strings.Contains(logBuf.String(), "parent inject skipped") {
		t.Errorf("missing log about skipped inject; got: %s", logBuf.String())
	}
}

func TestSubagentAnnounce_DeliveryScopeAll_NotifiesBoth(t *testing.T) {
	mgr := newTestSubagentManager(t, new(bytes.Buffer))
	received := make(chan *SubagentRun, 1)
	mgr.SetAnnounceCallback(func(run *SubagentRun) {
		received <- run
	})

	run := newRun("all-1", DeliveryScopeAll)
	registerRun(mgr, run)
	mgr.completeRun(run, "ok", nil)

	select {
	case got := <-received:
		if got.DeliveryScope != DeliveryScopeAll {
			t.Errorf("scope = %q, want all", got.DeliveryScope)
		}
	case <-time.After(time.Second):
		t.Fatal("callback did not fire within 1s for scope=all")
	}
}

func TestSubagentAnnounce_SessionIDConsistency(t *testing.T) {
	id1 := MakeSessionID("whatsapp", "foo@c.us")
	id2 := MakeSessionID("whatsapp", "foo@c.us")
	id3 := MakeSessionID("whatsapp", "bar@c.us")
	id4 := MakeSessionID("telegram", "foo@c.us")

	if id1 == "" {
		t.Fatal("MakeSessionID returned empty")
	}
	if id1 != id2 {
		t.Errorf("not deterministic: id1=%q id2=%q", id1, id2)
	}
	if id1 == id3 {
		t.Errorf("same id for different chatIDs: id1=%q id3=%q", id1, id3)
	}
	if id1 == id4 {
		t.Errorf("same id for different channels: id1=%q id4=%q", id1, id4)
	}
}

func TestSubagentAnnounce_CallbackLogsOnFailure(t *testing.T) {
	// completeRun writes the Debug "announce decision" record synchronously
	// (before any goroutine fires). When no callback is registered the
	// decision log still emits, has_callback=false, and completeRun returns
	// without panic.
	logBuf := new(bytes.Buffer)
	mgr := newTestSubagentManager(t, logBuf)
	// No callback registered.

	run := newRun("nocb-1", DeliveryScopeAll)
	registerRun(mgr, run)
	mgr.completeRun(run, "ok", nil)

	s := logBuf.String()
	if !strings.Contains(s, "announce decision") {
		t.Errorf("missing announce decision log; got: %s", s)
	}
	if !strings.Contains(s, "has_callback=false") {
		t.Errorf("decision log should reflect has_callback=false; got: %s", s)
	}
}

func TestSubagentAnnounce_DefaultScopeIsAll(t *testing.T) {
	if DeliveryScopeDefault != DeliveryScopeAll {
		t.Errorf("DeliveryScopeDefault = %q, want %q", DeliveryScopeDefault, DeliveryScopeAll)
	}

	// A run created without explicit scope should pick up the default when
	// Spawn's zero-value branch runs. Verify the literal mapping since
	// Spawn itself requires a full LLM/executor setup.
	var zero DeliveryScope
	scope := zero
	if scope == "" {
		scope = DeliveryScopeDefault
	}
	if scope != DeliveryScopeAll {
		t.Errorf("zero-value resolution got %q, want all", scope)
	}
}
