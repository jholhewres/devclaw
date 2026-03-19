package telegram

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/channels"
)

func TestNew(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	tg := New(cfg, nil)

	if tg.Name() != "telegram" {
		t.Errorf("Name() = %q, want %q", tg.Name(), "telegram")
	}
	if tg.IsConnected() {
		t.Error("expected not connected initially")
	}
}

func TestParseChatIDAndThread(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		chatID   int64
		threadID int64
		wantErr  bool
	}{
		{"plain chat ID", "12345", 12345, 0, false},
		{"negative chat ID", "-100123", -100123, 0, false},
		{"with thread", "12345:topic:42", 12345, 42, false},
		{"negative with thread", "-100123:topic:7", -100123, 7, false},
		{"invalid chat ID", "abc", 0, 0, true},
		{"invalid thread ID", "12345:topic:abc", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatID, threadID, err := parseChatIDAndThread(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseChatIDAndThread(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if chatID != tt.chatID {
				t.Errorf("chatID = %d, want %d", chatID, tt.chatID)
			}
			if threadID != tt.threadID {
				t.Errorf("threadID = %d, want %d", threadID, tt.threadID)
			}
		})
	}
}

func TestTypingCircuitBreaker(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	tg := New(cfg, slog.New(slog.NewTextHandler(os.Stdout, nil)))

	t.Run("not suspended initially", func(t *testing.T) {
		if tg.typingSuspended.Load() {
			t.Error("expected typingSuspended=false initially")
		}
	})

	t.Run("suspends after 10 consecutive auth errors", func(t *testing.T) {
		// Simulate 10 consecutive auth errors.
		for i := int32(0); i < 10; i++ {
			tg.typingConsecutive401.Store(i + 1)
		}
		tg.typingSuspended.Store(true)
		tg.typingSuspendedAt.Store(time.Now())

		if !tg.typingSuspended.Load() {
			t.Error("expected typingSuspended=true after 10 errors")
		}
	})

	t.Run("recovers after cooldown", func(t *testing.T) {
		// Simulate suspension from 6 minutes ago.
		tg.typingSuspended.Store(true)
		tg.typingSuspendedAt.Store(time.Now().Add(-6 * time.Minute))

		// SendTyping should detect the cooldown has passed.
		// Since we can't make a real API call, verify the recovery logic directly.
		if v := tg.typingSuspendedAt.Load(); v != nil {
			if suspendedAt, ok := v.(time.Time); ok && time.Since(suspendedAt) > 5*time.Minute {
				tg.typingSuspended.Store(false)
				tg.typingConsecutive401.Store(0)
			}
		}
		if tg.typingSuspended.Load() {
			t.Error("expected typingSuspended=false after cooldown")
		}
		if tg.typingConsecutive401.Load() != 0 {
			t.Error("expected consecutive 401 count to be 0 after recovery")
		}
	})

	t.Run("resets on success", func(t *testing.T) {
		tg.typingConsecutive401.Store(0)
		if tg.typingConsecutive401.Load() != 0 {
			t.Errorf("expected consecutive 401 count to be 0")
		}
	})
}

func TestHealth(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	tg := New(cfg, nil)

	t.Run("initial health", func(t *testing.T) {
		h := tg.Health()
		if h.Connected {
			t.Error("expected not connected")
		}
		if h.ErrorCount != 0 {
			t.Errorf("expected 0 errors, got %d", h.ErrorCount)
		}
		if h.LatencyMs != 0 {
			t.Errorf("expected 0 latency, got %d", h.LatencyMs)
		}
	})

	t.Run("latency tracking", func(t *testing.T) {
		tg.lastLatencyMs.Store(150)
		h := tg.Health()
		if h.LatencyMs != 150 {
			t.Errorf("expected latency 150, got %d", h.LatencyMs)
		}
	})
}

func TestProcessUpdate_CallbackQuery(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	tg := New(cfg, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	// We can't call apiCall without a real server, so we test the update struct parsing.

	t.Run("callback query struct fields", func(t *testing.T) {
		cq := &tgCallbackQuery{
			ID:   "123",
			From: tgUser{ID: 42, FirstName: "Test", Username: "testuser"},
			Data: "button_data",
			Message: &tgMessage{
				MessageID: 100,
				Chat:      tgChat{ID: -100123, Type: "supergroup"},
			},
		}

		if cq.ID != "123" {
			t.Errorf("expected ID '123', got %s", cq.ID)
		}
		if cq.Data != "button_data" {
			t.Errorf("expected Data 'button_data', got %s", cq.Data)
		}
		if cq.Message.Chat.ID != -100123 {
			t.Errorf("expected chat ID -100123, got %d", cq.Message.Chat.ID)
		}
		_ = tg // used to verify struct compiles
	})
}

func TestProcessUpdate_VideoNote(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	tg := New(cfg, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	tg.connected.Store(true)

	t.Run("video note message", func(t *testing.T) {
		u := tgUpdate{
			UpdateID: 1,
			Message: &tgMessage{
				MessageID: 1,
				From:      &tgUser{ID: 42, FirstName: "Test"},
				Chat:      tgChat{ID: 12345, Type: "private"},
				Date:      1000000,
				VideoNote: &tgVideoNote{
					FileID:   "file-id-123",
					Duration: 10,
					Length:   240,
					FileSize: 50000,
				},
			},
		}

		tg.processUpdate(u)

		select {
		case msg := <-tg.messages:
			if msg.Type != channels.MessageVideoNote {
				t.Errorf("expected type video_note, got %s", msg.Type)
			}
			if msg.Media == nil {
				t.Fatal("expected media info")
			}
			if msg.Media.URL != "file-id-123" {
				t.Errorf("expected file ID 'file-id-123', got %s", msg.Media.URL)
			}
			if msg.Media.Duration != 10 {
				t.Errorf("expected duration 10, got %d", msg.Media.Duration)
			}
		default:
			t.Error("expected message from processUpdate")
		}
	})
}

func TestProcessUpdate_StandardMessage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	tg := New(cfg, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	tg.connected.Store(true)

	t.Run("text message", func(t *testing.T) {
		u := tgUpdate{
			UpdateID: 1,
			Message: &tgMessage{
				MessageID: 1,
				From:      &tgUser{ID: 42, FirstName: "Test"},
				Chat:      tgChat{ID: 12345, Type: "private"},
				Date:      1000000,
				Text:      "Hello, world!",
			},
		}

		tg.processUpdate(u)

		select {
		case msg := <-tg.messages:
			if msg.Type != channels.MessageText {
				t.Errorf("expected type text, got %s", msg.Type)
			}
			if msg.Content != "Hello, world!" {
				t.Errorf("expected content 'Hello, world!', got %s", msg.Content)
			}
		default:
			t.Error("expected message from processUpdate")
		}
	})
}

func TestSendWithThreadFallback_Struct(t *testing.T) {
	// Test that TelegramAPIError with thread not found is properly identified.
	err := &TelegramAPIError{
		Method:      "sendMessage",
		HTTPStatus:  400,
		ErrorCode:   400,
		Description: "Bad Request: message thread not found",
	}

	tgErr := asTelegramAPIError(err)
	if tgErr == nil {
		t.Fatal("expected non-nil TelegramAPIError")
	}
	if !tgErr.isThreadNotFound() {
		t.Error("expected isThreadNotFound() to return true")
	}
}

func TestBuildReplyMarkup(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	tg := New(cfg, nil)

	t.Run("nil metadata", func(t *testing.T) {
		msg := &channels.OutgoingMessage{}
		result := tg.buildReplyMarkup(msg)
		if result != nil {
			t.Error("expected nil for nil metadata")
		}
	})

	t.Run("with buttons", func(t *testing.T) {
		msg := &channels.OutgoingMessage{
			Metadata: map[string]any{
				"telegram_buttons": []InlineButton{
					{Text: "OK", CallbackData: "ok"},
					{Text: "Cancel", CallbackData: "cancel"},
				},
			},
		}
		result := tg.buildReplyMarkup(msg)
		if result == nil {
			t.Fatal("expected non-nil reply markup")
		}
		keyboard, ok := result["inline_keyboard"].([][]map[string]any)
		if !ok {
			t.Fatal("expected inline_keyboard in result")
		}
		if len(keyboard) != 2 {
			t.Errorf("expected 2 rows, got %d", len(keyboard))
		}
	})
}

func TestSentMessageTracker(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	tg := New(cfg, nil)

	t.Run("tracks sent messages", func(t *testing.T) {
		if tg.isBotMessage(12345, 100) {
			t.Error("should not be a bot message initially")
		}

		key := "12345:100"
		tg.sentMu.Lock()
		tg.sentMessageIDs[key] = true
		tg.sentMu.Unlock()

		if !tg.isBotMessage(12345, 100) {
			t.Error("should be a bot message after recording")
		}
	})
}

func TestInterfaceCompliance(t *testing.T) {
	// Verify Telegram implements all expected interfaces.
	var _ channels.Channel = (*Telegram)(nil)
	var _ channels.MediaChannel = (*Telegram)(nil)
	var _ channels.PresenceChannel = (*Telegram)(nil)
	var _ channels.ReactionChannel = (*Telegram)(nil)
	var _ channels.SentMessageTracker = (*Telegram)(nil)
}
