package copilot

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// slowStore wraps a FileStore and optionally stalls GetAll to force Run()
// to dwell inside the guarded region long enough for a second invocation
// to race it.
type slowStore struct {
	*memory.FileStore
	delay time.Duration
}

func (s *slowStore) GetAll() ([]memory.Entry, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return s.FileStore.GetAll()
}

// TestDreamGuard_RejectsConcurrentRun exercises the atomic CAS at the top of
// DreamConsolidator.Run. A slow store keeps the first Run inside the guarded
// region while a second Run is invoked from another goroutine; the second
// call must return immediately with an error reflecting the skip, and at
// no point may two Runs proceed past the guard concurrently.
func TestDreamGuard_RejectsConcurrentRun(t *testing.T) {
	tmp := t.TempDir()
	fs, err := memory.NewFileStore(tmp)
	if err != nil {
		t.Fatal(err)
	}

	store := &slowStore{FileStore: fs, delay: 400 * time.Millisecond}
	d := NewDreamConsolidator(DefaultDreamConfig(), store, t.TempDir(), nil)

	var (
		wg      sync.WaitGroup
		results = make(chan DreamResult, 2)
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		results <- d.Run(context.Background())
	}()
	// Give the first goroutine a head start so it captures the CAS.
	time.Sleep(50 * time.Millisecond)
	go func() {
		defer wg.Done()
		results <- d.Run(context.Background())
	}()

	wg.Wait()
	close(results)

	var skipped, ran int
	for r := range results {
		if r.Error != nil && strings.Contains(r.Error.Error(), "dream already running") {
			skipped++
		} else {
			ran++
		}
	}
	if skipped != 1 || ran != 1 {
		t.Fatalf("expected exactly one skip and one run, got skipped=%d ran=%d", skipped, ran)
	}
}
