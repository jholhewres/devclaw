package memory

import (
	"strings"
	"testing"
)

func TestRedactCredentials_NaturalLanguage(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		secret string // must be absent from output
		keep   string // must remain in output ("" to skip)
		redact bool   // expect a REDACTED marker
	}{
		{
			name:   "password for X is VALUE",
			in:     "User's password for integrabot.ai is 081082se",
			secret: "081082se",
			keep:   "integrabot.ai",
			redact: true,
		},
		{
			name:   "password VALUE without colon",
			in:     "User has an account on integrabot.ai with email jhol@integrabot.ai and password 081082se",
			secret: "081082se",
			keep:   "jhol@integrabot.ai", // email is not secret-shaped (no digit) → preserved
			redact: true,
		},
		{
			name:   "colon form still works",
			in:     "Database login uses senha: hunter2supersecret for staging",
			secret: "hunter2supersecret",
			redact: true,
		},
		{
			name:   "benign line without keyword is untouched",
			in:     "The deploy completed on build abc123def with no issues",
			secret: "",
			keep:   "abc123def", // no credential keyword on the line → not redacted
			redact: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := RedactCredentials(tc.in)
			if tc.secret != "" && strings.Contains(out, tc.secret) {
				t.Errorf("secret %q leaked through redaction: %q", tc.secret, out)
			}
			if tc.keep != "" && !strings.Contains(out, tc.keep) {
				t.Errorf("expected %q preserved, got %q", tc.keep, out)
			}
			if tc.redact && !strings.Contains(out, "REDACTED") {
				t.Errorf("expected a REDACTED marker, got %q", out)
			}
			if tc.secret != "" && !LooksLikeCredential(tc.in) {
				t.Errorf("LooksLikeCredential should flag %q", tc.in)
			}
		})
	}
}
