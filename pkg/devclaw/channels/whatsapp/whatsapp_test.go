package whatsapp

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

func TestNew(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("creates instance with defaults", func(t *testing.T) {
		cfg := DefaultConfig()
		w := New(cfg, logger)

		if w == nil {
			t.Fatal("expected non-nil WhatsApp instance")
		}
		if w.Name() != "whatsapp" {
			t.Errorf("expected name 'whatsapp', got %s", w.Name())
		}
		if w.getState() != StateDisconnected {
			t.Errorf("expected initial state 'disconnected', got %s", w.getState())
		}
	})

	t.Run("uses default logger if nil", func(t *testing.T) {
		cfg := DefaultConfig()
		w := New(cfg, nil)

		if w == nil {
			t.Fatal("expected non-nil WhatsApp instance")
		}
		if w.logger == nil {
			t.Error("expected logger to be set")
		}
	})

	t.Run("applies reconnect backoff default", func(t *testing.T) {
		cfg := Config{
			SessionDir: "./sessions",
		}
		w := New(cfg, logger)

		if w.cfg.ReconnectBackoff != 5*time.Second {
			t.Errorf("expected default backoff 5s, got %v", w.cfg.ReconnectBackoff)
		}
	})

	t.Run("accepts DatabasePath for shared database", func(t *testing.T) {
		cfg := Config{
			DatabasePath: "./data/devclaw.db",
		}
		w := New(cfg, logger)

		if w.cfg.DatabasePath != "./data/devclaw.db" {
			t.Errorf("expected DatabasePath './data/devclaw.db', got %q", w.cfg.DatabasePath)
		}
	})
}

func TestStateManagement(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultConfig()
	w := New(cfg, logger)

	t.Run("initial state is disconnected", func(t *testing.T) {
		if w.getState() != StateDisconnected {
			t.Errorf("expected 'disconnected', got %s", w.getState())
		}
	})

	t.Run("setState updates state", func(t *testing.T) {
		w.setState(StateConnecting)
		if w.getState() != StateConnecting {
			t.Errorf("expected 'connecting', got %s", w.getState())
		}

		w.setState(StateConnected)
		if w.getState() != StateConnected {
			t.Errorf("expected 'connected', got %s", w.getState())
		}
	})

	t.Run("GetState returns current state", func(t *testing.T) {
		w.setState(StateWaitingQR)
		if w.GetState() != StateWaitingQR {
			t.Errorf("expected 'waiting_qr', got %s", w.GetState())
		}
	})
}

func TestQRSubscription(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultConfig()
	w := New(cfg, logger)

	t.Run("subscribe receives events", func(t *testing.T) {
		ch, unsubscribe := w.SubscribeQR()
		defer unsubscribe()

		// Send a test event.
		w.notifyQR(QREvent{
			Type:    "code",
			Code:    "test-qr-code",
			Message: "Scan the QR code",
		})

		select {
		case evt := <-ch:
			if evt.Type != "code" {
				t.Errorf("expected type 'code', got %s", evt.Type)
			}
			if evt.Code != "test-qr-code" {
				t.Errorf("expected code 'test-qr-code', got %s", evt.Code)
			}
		case <-time.After(1 * time.Second):
			t.Error("timeout waiting for QR event")
		}
	})

	t.Run("unsubscribe stops receiving events", func(t *testing.T) {
		// Clear any cached QR first.
		w.lastQR = nil

		ch, unsubscribe := w.SubscribeQR()

		// Unsubscribe immediately.
		unsubscribe()

		// Send an event after unsubscribe.
		w.notifyQR(QREvent{
			Type:    "code",
			Code:    "should-not-receive",
			Message: "Test",
		})

		// Channel should be closed.
		select {
		case _, ok := <-ch:
			if ok {
				t.Error("expected channel to be closed after unsubscribe")
			}
		default:
			// Channel was closed.
		}
	})

	t.Run("multiple observers receive same event", func(t *testing.T) {
		// Clear any cached QR first.
		w.lastQR = nil

		ch1, unsub1 := w.SubscribeQR()
		ch2, unsub2 := w.SubscribeQR()
		defer unsub1()
		defer unsub2()

		w.notifyQR(QREvent{
			Type:    "success",
			Message: "Connected!",
		})

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			select {
			case evt := <-ch1:
				if evt.Type != "success" {
					t.Errorf("ch1: expected 'success', got %s", evt.Type)
				}
			case <-time.After(1 * time.Second):
				t.Error("ch1: timeout")
			}
		}()

		go func() {
			defer wg.Done()
			select {
			case evt := <-ch2:
				if evt.Type != "success" {
					t.Errorf("ch2: expected 'success', got %s", evt.Type)
				}
			case <-time.After(1 * time.Second):
				t.Error("ch2: timeout")
			}
		}()

		wg.Wait()
	})

	t.Run("late observer receives cached QR", func(t *testing.T) {
		// Send a QR code first.
		w.notifyQR(QREvent{
			Type:    "code",
			Code:    "cached-qr",
			Message: "Scan me",
		})

		// Subscribe after the event.
		ch, unsubscribe := w.SubscribeQR()
		defer unsubscribe()

		select {
		case evt := <-ch:
			if evt.Code != "cached-qr" {
				t.Errorf("expected cached QR, got %s", evt.Code)
			}
		case <-time.After(1 * time.Second):
			t.Error("expected to receive cached QR")
		}
	})

	t.Run("success clears QR cache", func(t *testing.T) {
		// Send a QR code.
		w.notifyQR(QREvent{
			Type:    "code",
			Code:    "to-be-cleared",
			Message: "Scan me",
		})

		// Send success.
		w.notifyQR(QREvent{
			Type:    "success",
			Message: "Connected!",
		})

		if w.lastQR != nil {
			t.Error("expected lastQR to be cleared on success")
		}
	})
}

func TestConnectionObserver(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultConfig()
	w := New(cfg, logger)

	t.Run("observer receives connection changes", func(t *testing.T) {
		var receivedEvent ConnectionEvent
		var mu sync.Mutex

		obs := &testConnectionObserver{
			onChange: func(evt ConnectionEvent) {
				mu.Lock()
				receivedEvent = evt
				mu.Unlock()
			},
		}

		w.AddConnectionObserver(obs)

		// Simulate connection change.
		w.notifyConnectionChange(ConnectionEvent{
			State:     StateConnected,
			Previous:  StateDisconnected,
			Timestamp: time.Now(),
			Reason:    "test",
		})

		// Wait for async notification.
		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		if receivedEvent.State != StateConnected {
			t.Errorf("expected state 'connected', got %s", receivedEvent.State)
		}
		if receivedEvent.Reason != "test" {
			t.Errorf("expected reason 'test', got %s", receivedEvent.Reason)
		}
		mu.Unlock()
	})
}

func TestHealth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultConfig()
	w := New(cfg, logger)

	t.Run("returns health status", func(t *testing.T) {
		health := w.Health()

		if health.Connected {
			t.Error("expected not connected initially")
		}
		if health.Details["state"] != string(StateDisconnected) {
			t.Errorf("expected state in details, got %v", health.Details)
		}
	})

	t.Run("tracks error count", func(t *testing.T) {
		w.errorCount.Store(5)
		health := w.Health()

		if health.ErrorCount != 5 {
			t.Errorf("expected error count 5, got %d", health.ErrorCount)
		}
	})
}

func TestNeedsQR(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultConfig()
	w := New(cfg, logger)

	t.Run("needs QR when no client", func(t *testing.T) {
		// No client set.
		if w.NeedsQR() {
			t.Error("expected NeedsQR=false when client is nil")
		}
	})
}

func TestIsConnected(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultConfig()
	w := New(cfg, logger)

	t.Run("not connected initially", func(t *testing.T) {
		if w.IsConnected() {
			t.Error("expected not connected initially")
		}
	})

	t.Run("connected flag works", func(t *testing.T) {
		w.connected.Store(true)
		if !w.IsConnected() {
			t.Error("expected connected after setting flag")
		}
		w.connected.Store(false)
	})
}

func TestDisconnect(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultConfig()
	w := New(cfg, logger)

	t.Run("disconnect updates state", func(t *testing.T) {
		// Reset state for clean test
		w.messages = make(chan *channels.IncomingMessage, 256)
		w.ctx, w.cancel = context.WithCancel(context.Background())
		w.connected.Store(true)
		w.setState(StateConnected)

		err := w.Disconnect()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if w.getState() != StateDisconnected {
			t.Errorf("expected state 'disconnected', got %s", w.getState())
		}
		if w.IsConnected() {
			t.Error("expected connected=false after disconnect")
		}
	})
}

func TestSendWhenDisconnected(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultConfig()
	w := New(cfg, logger)

	t.Run("send fails when disconnected", func(t *testing.T) {
		ctx := context.Background()
		err := w.Send(ctx, "5511999999999", &channels.OutgoingMessage{
			Content: "test",
		})

		if err != channels.ErrChannelDisconnected {
			t.Errorf("expected ErrChannelDisconnected, got %v", err)
		}
	})

	t.Run("send media fails when disconnected", func(t *testing.T) {
		ctx := context.Background()
		err := w.SendMedia(ctx, "5511999999999", &channels.MediaMessage{
			Type: channels.MessageImage,
		})

		if err != channels.ErrChannelDisconnected {
			t.Errorf("expected ErrChannelDisconnected, got %v", err)
		}
	})

	t.Run("send reaction fails when disconnected", func(t *testing.T) {
		ctx := context.Background()
		err := w.SendReaction(ctx, "5511999999999", "msg-id", "👍")

		if err != channels.ErrChannelDisconnected {
			t.Errorf("expected ErrChannelDisconnected, got %v", err)
		}
	})
}

func TestRequestNewQR(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := DefaultConfig()
	w := New(cfg, logger)

	t.Run("fails when already connected", func(t *testing.T) {
		w.connected.Store(true)
		err := w.RequestNewQR(context.Background())

		if err == nil {
			t.Error("expected error when already connected")
		}
		w.connected.Store(false)
	})

	t.Run("fails when client not initialized", func(t *testing.T) {
		err := w.RequestNewQR(context.Background())

		if err == nil {
			t.Error("expected error when client not initialized")
		}
	})
}

func TestNormalizeJID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Companion device stripping
		{"strip companion device", "5511999999999:95@s.whatsapp.net", "5511999999999@s.whatsapp.net"},
		{"strip companion device :0", "5511999999999:0@s.whatsapp.net", "5511999999999@s.whatsapp.net"},

		// Brazilian number normalization (12 digits -> 13 digits)
		{"BR 12-digit with @", "551199999999@s.whatsapp.net", "5511999999999@s.whatsapp.net"},
		{"BR 12-digit bare", "551199999999", "5511999999999@s.whatsapp.net"},
		{"BR 12-digit + companion", "551199999999:5@s.whatsapp.net", "5511999999999@s.whatsapp.net"},

		// Brazilian number already 13 digits (no change)
		{"BR 13-digit with @", "5511999999999@s.whatsapp.net", "5511999999999@s.whatsapp.net"},
		{"BR 13-digit bare", "5511999999999", "5511999999999@s.whatsapp.net"},

		// Non-Brazilian numbers (no change)
		{"US number", "12025551234@s.whatsapp.net", "12025551234@s.whatsapp.net"},
		{"other country", "4915123456789@s.whatsapp.net", "4915123456789@s.whatsapp.net"},

		// LID format (no BR normalization)
		{"LID format", "123456:1@lid", "123456@lid"},
		{"LID without device", "123456@lid", "123456@lid"},

		// Group JIDs (no BR normalization)
		{"group JID", "120363012345678901@g.us", "120363012345678901@g.us"},

		// Edge cases
		{"bare number with spaces", "  5511999999999  ", "5511999999999@s.whatsapp.net"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeJID(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeJID(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeBRPhone(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"12-digit SP", "551199999999", "5511999999999"},
		{"12-digit RJ", "552199999999", "5521999999999"},
		{"12-digit MG", "553199999999", "5531999999999"},
		{"13-digit already", "5511999999999", "5511999999999"},
		{"non-BR number", "12025551234", "12025551234"},
		{"short number", "5511", "5511"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeBRPhone(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeBRPhone(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// Test helper types

type testConnectionObserver struct {
	onChange func(evt ConnectionEvent)
}

func (o *testConnectionObserver) OnConnectionChange(evt ConnectionEvent) {
	if o.onChange != nil {
		o.onChange(evt)
	}
}
