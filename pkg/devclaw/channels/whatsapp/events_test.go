package whatsapp

import (
	"context"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestHandleConnected(t *testing.T) {
	w := createTestWhatsApp()

	t.Run("sets connected state", func(t *testing.T) {
		w.setState(StateConnecting)
		w.handleConnected(&events.Connected{})

		if w.getState() != StateConnected {
			t.Errorf("expected state 'connected', got %s", w.getState())
		}
		// Note: connected flag is set by the event handler in real flow
	})

	t.Run("resets error count", func(t *testing.T) {
		w.errorCount.Store(10)
		w.handleConnected(&events.Connected{})

		if w.errorCount.Load() != 0 {
			t.Errorf("expected error count 0, got %d", w.errorCount.Load())
		}
	})

	t.Run("resets reconnect attempts", func(t *testing.T) {
		w.reconnectAttempts.Store(5)
		w.handleConnected(&events.Connected{})

		if w.reconnectAttempts.Load() != 0 {
			t.Errorf("expected reconnect attempts 0, got %d", w.reconnectAttempts.Load())
		}
	})
}

func TestHandleDisconnected(t *testing.T) {
	w := createTestWhatsApp()
	w.ctx, w.cancel = createTestContext()

	t.Run("sets disconnected state", func(t *testing.T) {
		w.setState(StateConnected)
		w.connected.Store(true)
		w.handleDisconnected(&events.Disconnected{})

		if w.getState() != StateDisconnected {
			t.Errorf("expected state 'disconnected', got %s", w.getState())
		}
		if w.IsConnected() {
			t.Error("expected connected=false")
		}
	})
}

func TestHandleStreamReplaced(t *testing.T) {
	w := createTestWhatsApp()
	w.ctx, w.cancel = createTestContext()

	t.Run("sets disconnected with reason", func(t *testing.T) {
		w.setState(StateConnected)
		w.connected.Store(true)
		w.handleStreamReplaced(&events.StreamReplaced{})

		if w.getState() != StateDisconnected {
			t.Errorf("expected state 'disconnected', got %s", w.getState())
		}
		if w.IsConnected() {
			t.Error("expected connected=false")
		}
	})
}

func TestHandleTemporaryBan(t *testing.T) {
	w := createTestWhatsApp()

	t.Run("sets banned state", func(t *testing.T) {
		w.setState(StateConnected)
		w.handleTemporaryBan(&events.TemporaryBan{
			Code:   1, // Any non-zero code
			Expire: time.Hour,
		})

		if w.getState() != StateBanned {
			t.Errorf("expected state 'banned', got %s", w.getState())
		}
		if w.IsConnected() {
			t.Error("expected connected=false")
		}
	})
}

func TestHandleKeepAliveTimeout(t *testing.T) {
	w := createTestWhatsApp()

	t.Run("increments error count", func(t *testing.T) {
		initialCount := w.errorCount.Load()
		w.handleKeepAliveTimeout(&events.KeepAliveTimeout{
			ErrorCount:  3,
			LastSuccess: time.Now(),
		})

		if w.errorCount.Load() != initialCount+1 {
			t.Errorf("expected error count to increment")
		}
	})
}

func TestHandleKeepAliveRestored(t *testing.T) {
	w := createTestWhatsApp()

	t.Run("resets error count", func(t *testing.T) {
		w.errorCount.Store(10)
		w.handleKeepAliveRestored(&events.KeepAliveRestored{})

		if w.errorCount.Load() != 0 {
			t.Errorf("expected error count 0, got %d", w.errorCount.Load())
		}
	})
}

func TestHandlePairSuccess(t *testing.T) {
	w := createTestWhatsApp()

	t.Run("logs pair info", func(t *testing.T) {
		jid := types.JID{User: "5511999999999", Server: "s.whatsapp.net"}
		w.handlePairSuccess(&events.PairSuccess{
			ID:           jid,
			Platform:     "android",
			BusinessName: "",
		})
		// Test passes if no panic.
	})
}

func TestConnectionEvent(t *testing.T) {
	t.Run("connection event structure", func(t *testing.T) {
		evt := ConnectionEvent{
			State:     StateConnected,
			Previous:  StateDisconnected,
			Timestamp: time.Now(),
			Reason:    "test",
			Details: map[string]any{
				"jid": "test@example.com",
			},
		}

		if evt.State != StateConnected {
			t.Errorf("expected state 'connected', got %s", evt.State)
		}
		if evt.Reason != "test" {
			t.Errorf("expected reason 'test', got %s", evt.Reason)
		}
	})
}

func TestQREventEnhanced(t *testing.T) {
	t.Run("enhanced QR event structure", func(t *testing.T) {
		now := time.Now()
		evt := QREventEnhanced{
			Type:        "code",
			Code:        "test-qr",
			Message:     "Scan me",
			ExpiresAt:   now.Add(60 * time.Second),
			SecondsLeft: 60,
			Attempts:    1,
		}

		if evt.Type != "code" {
			t.Errorf("expected type 'code', got %s", evt.Type)
		}
		if evt.SecondsLeft != 60 {
			t.Errorf("expected seconds_left 60, got %d", evt.SecondsLeft)
		}
	})
}

func TestConnectionStateConstants(t *testing.T) {
	states := []ConnectionState{
		StateDisconnected,
		StateConnecting,
		StateConnected,
		StateReconnecting,
		StateWaitingQR,
		StateQRScanned,
		StateLoggingOut,
		StateBanned,
	}

	expected := []string{
		"disconnected",
		"connecting",
		"connected",
		"reconnecting",
		"waiting_qr",
		"qr_scanned",
		"logging_out",
		"banned",
	}

	for i, state := range states {
		if string(state) != expected[i] {
			t.Errorf("expected state %q, got %q", expected[i], state)
		}
	}
}

// Test helpers

func createTestWhatsApp() *WhatsApp {
	cfg := DefaultConfig()
	return New(cfg, nil)
}

func createTestContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
