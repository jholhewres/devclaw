package copilot

import (
	"strings"
	"testing"
)

func TestLooksLikeCredential_QuotedValues(t *testing.T) {
	shouldFlag := []string{
		`password: "a long phrase with spaces"`,
		`api_key: 'abcdef123456'`,
		`secret_key = "abcdef-123-456"`,
		`token: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ4In0.AbCdEfGh"`,
	}
	for _, c := range shouldFlag {
		if !looksLikeCredential(c) {
			t.Errorf("expected credential flag on %q", c)
		}
	}

	shouldPass := []string{
		`the password of the user needs review`,
		`senha do servidor foi alterada`,
		`password for the admin`,
	}
	for _, c := range shouldPass {
		if looksLikeCredential(c) {
			t.Errorf("stopword follow-up should suppress flag on %q", c)
		}
	}
}

func TestLooksLikeCredential_ProviderFormats(t *testing.T) {
	shouldFlag := []string{
		"deploy token ghp_" + strings.Repeat("A", 36),
		"gemini AIza" + strings.Repeat("b", 35),
		"AWS access key AKIA" + strings.Repeat("0", 16),
		"gh fine-grained github_pat_" + strings.Repeat("x", 82),
	}
	for _, c := range shouldFlag {
		if !looksLikeCredential(c) {
			t.Errorf("expected credential flag on %q", c)
		}
	}
}

func TestRedactCredentials_ReplacesValueKeepsLabel(t *testing.T) {
	in := `password: "supersecret phrase 42"`
	out := redactCredentials(in)
	if strings.Contains(out, "supersecret") {
		t.Fatalf("redaction leaked value: %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "password") {
		t.Fatalf("redaction should keep label, got %q", out)
	}
}
