package memory

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSqliteExecWithRetry_ZeroOptsRunsOnce(t *testing.T) {
	var calls int
	err := sqliteExecWithRetry(context.Background(), func(context.Context) error {
		calls++
		return errors.New("database is locked")
	}, RetryOpts{}) // zero-value = MaxAttempts<=1 = no retry
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls != 1 {
		t.Errorf("MaxAttempts<=1 must run exactly once, ran %d times", calls)
	}
}

func TestSqliteExecWithRetry_NonBusyErrorReturnsImmediately(t *testing.T) {
	var calls int
	wantErr := errors.New("constraint violation")
	err := sqliteExecWithRetry(context.Background(), func(context.Context) error {
		calls++
		return wantErr
	}, DefaultRetryOpts())
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wantErr, got %v", err)
	}
	if calls != 1 {
		t.Errorf("non-busy error should not retry, ran %d times", calls)
	}
}

func TestSqliteExecWithRetry_RetriesOnBusyAndSucceeds(t *testing.T) {
	var calls int
	err := sqliteExecWithRetry(context.Background(), func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("database is locked")
		}
		return nil
	}, RetryOpts{MaxAttempts: 5, JitterMinMs: 1, JitterMaxMs: 2})
	if err != nil {
		t.Errorf("expected nil after retry, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected exactly 3 calls, got %d", calls)
	}
}

func TestSqliteExecWithRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	var calls int
	err := sqliteExecWithRetry(context.Background(), func(context.Context) error {
		calls++
		return errors.New("SQLITE_BUSY: database is locked")
	}, RetryOpts{MaxAttempts: 3, JitterMinMs: 1, JitterMaxMs: 2})
	if err == nil {
		t.Fatal("expected error after max attempts")
	}
	if calls != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", calls)
	}
}

func TestSqliteExecWithRetry_ContextCancellationStopsEarly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls int
	done := make(chan error, 1)
	go func() {
		done <- sqliteExecWithRetry(ctx, func(context.Context) error {
			calls++
			return errors.New("database is locked")
		}, RetryOpts{MaxAttempts: 10, JitterMinMs: 50, JitterMaxMs: 60})
	}()

	// Cancel after the first attempt has definitely run.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("retry did not honor context cancellation within 1s")
	}
	if calls >= 10 {
		t.Errorf("retry should have stopped early after cancel, ran %d times", calls)
	}
}

func TestIsSQLiteBusy_Variants(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"SQLITE_BUSY: database is locked", true},
		{"database is locked", true},
		{"SQLITE_LOCKED", true},
		{"database table is locked", true},
		{"constraint failed", false},
		{"", false},
	}
	for _, c := range cases {
		t.Run(c.msg, func(t *testing.T) {
			var err error
			if c.msg != "" {
				err = fmt.Errorf("%s", c.msg)
			}
			if got := isSQLiteBusy(err); got != c.want {
				t.Errorf("isSQLiteBusy(%q) = %v, want %v", c.msg, got, c.want)
			}
		})
	}
}
