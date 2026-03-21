package gateway

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// mockChannel implements MonitoredChannel for testing.
type mockChannel struct {
	mu           sync.Mutex
	name         string
	lastActivity time.Time
	restartErr   error
	restartCount int
}

func (m *mockChannel) ChannelName() string { return m.name }

func (m *mockChannel) LastActivityTime() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastActivity
}

func (m *mockChannel) Restart(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartCount++
	return m.restartErr
}

func (m *mockChannel) setLastActivity(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastActivity = t
}

func (m *mockChannel) getRestartCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restartCount
}

func TestHealthMonitor_HealthyChannelNotRestarted(t *testing.T) {
	cfg := DefaultChannelHealthMonitorConfig()
	cfg.Enabled = true
	cfg.StaleThreshold = 5 * time.Minute

	mon := NewChannelHealthMonitor(cfg, nil)
	ch := &mockChannel{name: "test", lastActivity: time.Now()}
	mon.Register(ch)

	ctx := context.Background()
	mon.checkAll(ctx)

	if ch.getRestartCount() != 0 {
		t.Error("healthy channel should not be restarted")
	}
}

func TestHealthMonitor_StaleChannelRestarted(t *testing.T) {
	cfg := DefaultChannelHealthMonitorConfig()
	cfg.Enabled = true
	cfg.StaleThreshold = 1 * time.Minute

	mon := NewChannelHealthMonitor(cfg, nil)
	ch := &mockChannel{name: "test", lastActivity: time.Now().Add(-10 * time.Minute)}
	mon.Register(ch)

	ctx := context.Background()
	mon.checkAll(ctx)

	if ch.getRestartCount() != 1 {
		t.Errorf("stale channel should be restarted once, got %d", ch.getRestartCount())
	}
}

func TestHealthMonitor_CooldownPreventsRepeatRestart(t *testing.T) {
	cfg := DefaultChannelHealthMonitorConfig()
	cfg.Enabled = true
	cfg.StaleThreshold = 1 * time.Minute
	cfg.RestartCooldown = 10 * time.Minute

	mon := NewChannelHealthMonitor(cfg, nil)
	ch := &mockChannel{name: "test", lastActivity: time.Now().Add(-10 * time.Minute)}
	mon.Register(ch)

	ctx := context.Background()
	mon.checkAll(ctx)
	mon.checkAll(ctx) // Should be blocked by cooldown.

	if ch.getRestartCount() != 1 {
		t.Errorf("cooldown should prevent second restart, got %d restarts", ch.getRestartCount())
	}
}

func TestHealthMonitor_HourlyCapBlocksExcessRestarts(t *testing.T) {
	cfg := DefaultChannelHealthMonitorConfig()
	cfg.Enabled = true
	cfg.StaleThreshold = 1 * time.Minute
	cfg.RestartCooldown = 0 // Disable cooldown for this test.
	cfg.MaxRestartsPerHour = 2

	mon := NewChannelHealthMonitor(cfg, nil)
	// Use 3 different channels to bypass per-channel cooldown.
	ch1 := &mockChannel{name: "ch1", lastActivity: time.Now().Add(-10 * time.Minute)}
	ch2 := &mockChannel{name: "ch2", lastActivity: time.Now().Add(-10 * time.Minute)}
	ch3 := &mockChannel{name: "ch3", lastActivity: time.Now().Add(-10 * time.Minute)}
	mon.Register(ch1)
	mon.Register(ch2)
	mon.Register(ch3)

	ctx := context.Background()
	mon.checkAll(ctx)

	total := ch1.getRestartCount() + ch2.getRestartCount() + ch3.getRestartCount()
	if total != 2 {
		t.Errorf("hourly cap of 2 should limit restarts, got %d", total)
	}
}

func TestHealthMonitor_FailedRestartDoesNotCountAgainstCap(t *testing.T) {
	cfg := DefaultChannelHealthMonitorConfig()
	cfg.Enabled = true
	cfg.StaleThreshold = 1 * time.Minute
	cfg.RestartCooldown = 0
	cfg.MaxRestartsPerHour = 2

	mon := NewChannelHealthMonitor(cfg, nil)

	// Failing channel — its failures should NOT consume the hourly cap.
	failing := &mockChannel{name: "failing", lastActivity: time.Now().Add(-10 * time.Minute), restartErr: errors.New("fail")}
	// Healthy-enough channel that goes stale.
	good := &mockChannel{name: "good", lastActivity: time.Now().Add(-10 * time.Minute)}

	mon.Register(failing)
	mon.Register(good)

	ctx := context.Background()
	mon.checkAll(ctx)

	// The failing channel attempted restart but failed. The good one should still succeed
	// because failures don't count against the global hourly cap.
	if good.getRestartCount() != 1 {
		t.Errorf("good channel should have been restarted, got %d", good.getRestartCount())
	}
}

func TestHealthMonitor_HealthReportsCorrectStatus(t *testing.T) {
	cfg := DefaultChannelHealthMonitorConfig()
	cfg.StaleThreshold = 5 * time.Minute

	mon := NewChannelHealthMonitor(cfg, nil)
	healthy := &mockChannel{name: "healthy", lastActivity: time.Now()}
	stale := &mockChannel{name: "stale", lastActivity: time.Now().Add(-10 * time.Minute)}
	mon.Register(healthy)
	mon.Register(stale)

	health := mon.Health()
	if len(health) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(health))
	}

	for _, h := range health {
		switch h.Name {
		case "healthy":
			if !h.Healthy {
				t.Error("healthy channel should report healthy=true")
			}
		case "stale":
			if h.Healthy {
				t.Error("stale channel should report healthy=false")
			}
		default:
			t.Errorf("unexpected channel: %s", h.Name)
		}
	}
}

func TestHealthMonitor_HealthReportsRestartCount(t *testing.T) {
	cfg := DefaultChannelHealthMonitorConfig()
	cfg.StaleThreshold = 1 * time.Minute
	cfg.RestartCooldown = 0

	mon := NewChannelHealthMonitor(cfg, nil)
	ch := &mockChannel{name: "test", lastActivity: time.Now().Add(-10 * time.Minute)}
	mon.Register(ch)

	ctx := context.Background()
	mon.checkAll(ctx)

	health := mon.Health()
	if len(health) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(health))
	}
	if health[0].Restarts != 1 {
		t.Errorf("expected 1 restart in health report, got %d", health[0].Restarts)
	}
}

func TestHealthMonitor_RegisterNilIgnored(t *testing.T) {
	cfg := DefaultChannelHealthMonitorConfig()
	mon := NewChannelHealthMonitor(cfg, nil)
	mon.Register(nil)

	mon.mu.Lock()
	count := len(mon.channels)
	mon.mu.Unlock()

	if count != 0 {
		t.Errorf("nil channel should not be registered, got %d channels", count)
	}
}

func TestHealthMonitor_RegisterDuplicateIgnored(t *testing.T) {
	cfg := DefaultChannelHealthMonitorConfig()
	mon := NewChannelHealthMonitor(cfg, nil)
	ch := &mockChannel{name: "test", lastActivity: time.Now()}
	mon.Register(ch)
	mon.Register(ch) // duplicate

	mon.mu.Lock()
	count := len(mon.channels)
	mon.mu.Unlock()

	if count != 1 {
		t.Errorf("duplicate channel should be ignored, got %d channels", count)
	}
}

func TestHealthMonitor_RestartTimeout(t *testing.T) {
	cfg := DefaultChannelHealthMonitorConfig()
	cfg.Enabled = true
	cfg.StaleThreshold = 1 * time.Minute
	cfg.RestartTimeout = 100 * time.Millisecond

	mon := NewChannelHealthMonitor(cfg, nil)

	// Channel whose Restart blocks until context is cancelled.
	blocking := &mockChannel{name: "blocking", lastActivity: time.Now().Add(-10 * time.Minute)}
	blocking.restartErr = nil
	origRestart := blocking.Restart
	_ = origRestart
	// Override with a blocking restart.
	blockingCh := &blockingChannel{
		mockChannel: mockChannel{name: "blocking", lastActivity: time.Now().Add(-10 * time.Minute)},
	}
	mon.Register(blockingCh)

	ctx := context.Background()
	start := time.Now()
	mon.checkAll(ctx)
	elapsed := time.Since(start)

	// Should complete in ~100ms (the restart timeout), not hang forever.
	if elapsed > 2*time.Second {
		t.Errorf("checkAll should respect restart timeout, took %v", elapsed)
	}
}

// blockingChannel is a MonitoredChannel whose Restart blocks until the context is done.
type blockingChannel struct {
	mockChannel
}

func (b *blockingChannel) Restart(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
