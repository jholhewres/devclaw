package copilot

import (
	"context"
	"testing"
)

func TestNewDaemonManager_NilParentUsesBackground(t *testing.T) {
	dm := NewDaemonManager(nil)
	if dm.baseCtx == nil {
		t.Fatal("baseCtx must not be nil even when parent is nil")
	}
	if err := dm.baseCtx.Err(); err != nil {
		t.Errorf("nil parent should yield a live context, got err: %v", err)
	}
	dm.Shutdown()
}

func TestDaemonManager_ShutdownCancelsBaseCtx(t *testing.T) {
	dm := NewDaemonManager(context.Background())
	dm.Shutdown()
	if err := dm.baseCtx.Err(); err == nil {
		t.Error("Shutdown must cancel baseCtx so orphaned daemons terminate")
	}
}

func TestDaemonManager_DoubleShutdownIsSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("double Shutdown must not panic; got: %v", r)
		}
	}()
	dm := NewDaemonManager(context.Background())
	dm.Shutdown()
	dm.Shutdown() // second call — stopCh is already closed.
}

func TestDaemonManager_ParentCancelPropagatesToBaseCtx(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	dm := NewDaemonManager(parent)

	if err := dm.baseCtx.Err(); err != nil {
		t.Errorf("baseCtx live before parent cancel; got: %v", err)
	}
	cancel()
	// Parent cancel cascades: baseCtx.Err() becomes non-nil immediately
	// because context.WithCancel registers propagation.
	if err := dm.baseCtx.Err(); err == nil {
		t.Error("parent cancel must propagate to baseCtx")
	}
	dm.Shutdown()
}
