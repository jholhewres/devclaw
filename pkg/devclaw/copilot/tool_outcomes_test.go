package copilot

import (
	"testing"
	"time"
)

func TestProvenanceReason_FactEchoingFailure_Rejected(t *testing.T) {
	outcomes := []ToolOutcome{
		{
			Name:      "bash",
			Args:      `{"cmd":"ssh deploy@host-alpha.example.com"}`,
			Error:     true,
			Content:   "Exit code: 255\nhost-alpha.example.com: Permission denied (publickey).",
			Timestamp: time.Now(),
		},
	}

	reason := ProvenanceReason("host-alpha.example.com is unreachable", outcomes)
	if reason == "" {
		t.Fatalf("expected rejection: fact echoes failed bash call")
	}
}

func TestProvenanceReason_FactUnrelatedToFailure_Allowed(t *testing.T) {
	outcomes := []ToolOutcome{
		{
			Name:      "bash",
			Args:      `{"cmd":"ssh deploy@host-alpha.example.com"}`,
			Error:     true,
			Content:   "Permission denied",
			Timestamp: time.Now(),
		},
	}

	// Different subject entirely — no token overlap with host-alpha/deploy/ssh.
	reason := ProvenanceReason("User prefers short replies on weekends", outcomes)
	if reason != "" {
		t.Fatalf("expected allow (no overlap), got rejection: %s", reason)
	}
}

func TestProvenanceReason_LaterSuccessSupersedesFailure_Allowed(t *testing.T) {
	base := time.Now()
	outcomes := []ToolOutcome{
		{
			Name:      "bash",
			Args:      `{"cmd":"ssh deploy@host-alpha.example.com"}`,
			Error:     true,
			Content:   "Permission denied",
			Timestamp: base,
		},
		{
			Name:      "bash",
			Args:      `{"cmd":"sshpass -p *** ssh deploy@host-alpha.example.com id"}`,
			Error:     false,
			Content:   "uid=1000(deploy) gid=1000(deploy) groups=1000(deploy)",
			Timestamp: base.Add(10 * time.Second),
		},
	}

	reason := ProvenanceReason("host-alpha.example.com accepts deploy over password-ssh", outcomes)
	if reason != "" {
		t.Fatalf("expected allow (later success supersedes failure), got: %s", reason)
	}
}

func TestProvenanceReason_EmptyLog_Allowed(t *testing.T) {
	if reason := ProvenanceReason("anything", nil); reason != "" {
		t.Fatalf("empty log must allow, got: %s", reason)
	}
}

func TestToolOutcomeLog_Bounded(t *testing.T) {
	l := NewToolOutcomeLog(3)
	for i := 0; i < 10; i++ {
		l.Record(ToolOutcome{Name: "t", Timestamp: time.Now()})
	}
	snap := l.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("expected log capped at 3, got %d", len(snap))
	}
}

func TestIdentifierTokens_ExtractsHostnamesAndPaths(t *testing.T) {
	toks := identifierTokens("ssh deploy@host-alpha.example.com /var/log/app.log")
	for _, want := range []string{"ssh", "deploy", "host-alpha.example.com", "/var/log/app.log"} {
		if _, ok := toks[want]; !ok {
			t.Errorf("missing token %q in %v", want, toks)
		}
	}
}

func TestIdentifierTokens_DropsShortAndStopwords(t *testing.T) {
	toks := identifierTokens("the server is up")
	for _, drop := range []string{"the", "is", "up"} {
		if _, ok := toks[drop]; ok {
			t.Errorf("stopword/short token %q should have been dropped, got %v", drop, toks)
		}
	}
	if _, ok := toks["server"]; !ok {
		t.Errorf("expected 'server' in tokens, got %v", toks)
	}
}

func TestProvenanceReason_PreferenceNotAffected(t *testing.T) {
	// Caller side (handleMemorySave) only calls ProvenanceReason for
	// category=fact; this test documents that preferences are passed as
	// plain content and no structural rejection happens without a failed
	// tool in the log.
	outcomes := []ToolOutcome{
		{Name: "other", Error: false, Content: "ok"},
	}
	if reason := ProvenanceReason("user likes dark mode", outcomes); reason != "" {
		t.Fatalf("no failure in log should allow, got: %s", reason)
	}
}
