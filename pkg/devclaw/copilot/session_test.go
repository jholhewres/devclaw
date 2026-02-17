package copilot

import "testing"

func TestParseSessionKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		channel string
		chatID  string
		branch  string
	}{
		{"two parts", "whatsapp:123456", "whatsapp", "123456", ""},
		{"three parts", "whatsapp:123456:topic", "whatsapp", "123456", "topic"},
		{"one part", "justchatid", "", "justchatid", ""},
		{"empty branch", "discord:abc:", "discord", "abc", ""},
		{"colons in chatid", "webui:user@host:branch", "webui", "user@host", "branch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sk := ParseSessionKey(tt.input)
			if sk.Channel != tt.channel {
				t.Errorf("Channel = %q, want %q", sk.Channel, tt.channel)
			}
			if sk.ChatID != tt.chatID {
				t.Errorf("ChatID = %q, want %q", sk.ChatID, tt.chatID)
			}
			if sk.Branch != tt.branch {
				t.Errorf("Branch = %q, want %q", sk.Branch, tt.branch)
			}
		})
	}
}

func TestSessionKey_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sk   SessionKey
		want string
	}{
		{SessionKey{Channel: "whatsapp", ChatID: "123"}, "whatsapp:123"},
		{SessionKey{Channel: "whatsapp", ChatID: "123", Branch: "topic"}, "whatsapp:123:topic"},
		{SessionKey{ChatID: "only"}, ":only"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.sk.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionKey_Hash_Deterministic(t *testing.T) {
	t.Parallel()
	sk := SessionKey{Channel: "whatsapp", ChatID: "123"}
	h1 := sk.Hash()
	h2 := sk.Hash()
	if h1 != h2 {
		t.Errorf("same key should produce same hash: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestSessionKey_Hash_Different(t *testing.T) {
	t.Parallel()
	sk1 := SessionKey{Channel: "whatsapp", ChatID: "123"}
	sk2 := SessionKey{Channel: "whatsapp", ChatID: "456"}
	if sk1.Hash() == sk2.Hash() {
		t.Error("different keys should produce different hashes")
	}
}

func TestMakeSessionID(t *testing.T) {
	t.Parallel()
	id := MakeSessionID("whatsapp", "123")
	if id == "" {
		t.Error("MakeSessionID should return non-empty string")
	}
	// Same inputs should produce same ID.
	id2 := MakeSessionID("whatsapp", "123")
	if id != id2 {
		t.Error("MakeSessionID should be deterministic")
	}
	// Different inputs should produce different IDs.
	id3 := MakeSessionID("whatsapp", "456")
	if id == id3 {
		t.Error("different inputs should produce different IDs")
	}
}
