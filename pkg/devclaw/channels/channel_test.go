package channels

import "testing"

func TestValidateInstanceID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"empty is valid (default instance)", "", false},
		{"simple alphanumeric", "business", false},
		{"with underscore", "my_bot", false},
		{"with hyphen", "my-bot", false},
		{"mixed case", "MyBot123", false},
		{"max length 64", "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01", false},
		{"too long 65", "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz012", true},
		{"path traversal dot-dot", "../etc/passwd", true},
		{"path traversal slash", "foo/bar", true},
		{"path traversal backslash", "foo\\bar", true},
		{"path traversal just dots", "..", true},
		{"spaces", "my bot", true},
		{"special chars", "bot@home", true},
		{"colon", "bot:1", true},
		{"unicode", "böt", true},
		{"null byte", "bot\x00", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInstanceID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInstanceID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidChannelTypes(t *testing.T) {
	for _, ct := range []string{"whatsapp", "telegram", "discord", "slack"} {
		if !ValidChannelTypes[ct] {
			t.Errorf("expected %q to be a valid channel type", ct)
		}
	}
	for _, ct := range []string{"unknown", "whatsAPP", "TELEGRAM", "", "sms"} {
		if ValidChannelTypes[ct] {
			t.Errorf("expected %q to NOT be a valid channel type", ct)
		}
	}
}
