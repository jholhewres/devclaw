package memory

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

// RetryOpts controls sqliteExecWithRetry. The zero value disables retry so the
// wrapper is a transparent no-op on untouched call sites.
type RetryOpts struct {
	MaxAttempts int // <=1 disables retry (fn runs exactly once).
	JitterMinMs int // Minimum backoff per retry (clamped to 0 when negative).
	JitterMaxMs int // Maximum backoff per retry (clamped to min when smaller).
}

// DefaultRetryOpts is the recommended profile for contended write paths.
// 5 attempts with 20-150ms jitter disperses goroutines that return from the
// same event (e.g. a batched memory flush after a burst of conversation
// updates), complementing the SQLite driver's own busy_timeout handling.
func DefaultRetryOpts() RetryOpts {
	return RetryOpts{MaxAttempts: 5, JitterMinMs: 20, JitterMaxMs: 150}
}

// isSQLiteBusy reports whether err is an SQLITE_BUSY / SQLITE_LOCKED return.
// Detected via substring match so the helper stays independent of the driver
// import path (works with both mattn/go-sqlite3 and modernc.org/sqlite).
func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "SQLITE_BUSY") ||
		strings.Contains(s, "SQLITE_LOCKED") ||
		strings.Contains(s, "database is locked") ||
		strings.Contains(s, "database table is locked")
}

// sqliteExecWithRetry runs fn and retries on SQLITE_BUSY/LOCKED with jittered
// backoff. Non-busy errors return immediately. When opts.MaxAttempts <= 1,
// fn runs exactly once — guaranteeing byte-identical behavior vs. unwrapped
// callers.
//
// Jitter uses a uniform distribution across [JitterMinMs, JitterMaxMs]. Each
// attempt picks a fresh random offset, so concurrent writers that wake from
// the same event disperse instead of colliding on the retry cycle.
func sqliteExecWithRetry(ctx context.Context, fn func(ctx context.Context) error, opts RetryOpts) error {
	if opts.MaxAttempts <= 1 {
		return fn(ctx)
	}
	minMs := opts.JitterMinMs
	if minMs < 0 {
		minMs = 0
	}
	maxMs := opts.JitterMaxMs
	if maxMs < minMs {
		maxMs = minMs
	}

	var last error
	for i := 0; i < opts.MaxAttempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn(ctx)
		if err == nil {
			return nil
		}
		if !isSQLiteBusy(err) {
			return err
		}
		last = err
		if i == opts.MaxAttempts-1 {
			break
		}
		wait := minMs
		if maxMs > minMs {
			wait = minMs + rand.Intn(maxMs-minMs+1)
		}
		timer := time.NewTimer(time.Duration(wait) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return last
}
