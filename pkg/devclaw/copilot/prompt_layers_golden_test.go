// Package copilot — prompt_layers_golden_test.go locks buildMemoryLayer's
// output for a known empty-DB input to a byte-identical golden fixture.
// This is the retrocompat gate for Sprint 2 Room 2.4: when the memory
// stack is nil, absent, or all three layers (L0/L1/L2) render empty,
// buildMemoryLayer must produce byte-identical output to the v1.18.0
// baseline captured in testdata/prompts/v1.18.0-layermemory.golden.txt.
//
// First-run workflow:
//
//   - If the golden file does not exist (or is empty), the test writes
//     the current output to it and fails with a skip-style message
//     asking the caller to re-run.
//   - Subsequent runs require byte-identical output.
//
// Update workflow (after an INTENTIONAL change):
//
//   - Delete testdata/prompts/v1.18.0-layermemory.golden.txt.
//   - Re-run the test to regenerate. Commit the new fixture with the
//     ADR or commit message that justifies the change.
//
// DO NOT update the fixture as a side effect of adding new layer
// prefixes. The entire point of the test is to catch prepend-vs-rewrite
// regressions in prompt_layers.go.
package copilot

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goldenFixturePath = "testdata/prompts/v1.18.0-layermemory.golden.txt"

// goldenSessionInput is the fixed input used by every golden variant.
// Stable across runs — do not parameterize.
const (
	goldenSessionID = "golden-session-001"
	goldenChannel   = "golden-channel"
	goldenChatID    = "golden-chat"
	goldenInput     = "What did we discuss last week about the alpha project?"
)

// newGoldenComposer builds a PromptComposer with the minimum config
// required for buildMemoryLayer to run: no sqliteMemory, no memoryStore,
// no stack, no context router. A fresh *Config with default values is
// enough — the function only touches the memory-layer code path.
func newGoldenComposer(t *testing.T) *PromptComposer {
	t.Helper()
	cfg := &Config{}
	return NewPromptComposer(cfg)
}

// newGoldenSession builds a Session with the stable fixture identity
// and two session-level facts. The facts are generic placeholders
// ("fact-alpha", "fact-beta") so the rendered output is non-trivial
// without leaking domain vocabulary into the fixture.
func newGoldenSession() *Session {
	s := &Session{
		ID:      goldenSessionID,
		Channel: goldenChannel,
		ChatID:  goldenChatID,
	}
	s.AddFact("fact-alpha")
	s.AddFact("fact-beta")
	return s
}

// loadOrWriteGolden reads the golden fixture if it exists. When the
// file does not exist on disk (os.IsNotExist), it writes the supplied
// current output and signals the caller to skip with a first-run
// message. Returns (expected, true) when the caller should assert
// byte-equality — including the legitimate case where the fixture is
// a zero-byte file (empty DB → empty output is the v1.18.0 baseline).
func loadOrWriteGolden(t *testing.T, current string) (expected string, ready bool) {
	t.Helper()
	data, err := os.ReadFile(goldenFixturePath)
	if err == nil {
		return string(data), true
	}
	if !os.IsNotExist(err) {
		t.Fatalf("failed to read golden fixture: %v", err)
	}

	// Missing — write and skip. The second run will lock it.
	if mkdirErr := os.MkdirAll(filepath.Dir(goldenFixturePath), 0o755); mkdirErr != nil {
		t.Fatalf("failed to create golden fixture dir: %v", mkdirErr)
	}
	if writeErr := os.WriteFile(goldenFixturePath, []byte(current), 0o644); writeErr != nil {
		t.Fatalf("failed to write golden fixture: %v", writeErr)
	}
	t.Skipf("golden fixture did not exist — wrote %d bytes to %s, re-run to lock",
		len(current), goldenFixturePath)
	return "", false
}

// assertGoldenEqual fails the test if current does not byte-equal the
// loaded golden fixture. Uses line-diff style output for debuggability.
func assertGoldenEqual(t *testing.T, label, current string) {
	t.Helper()
	expected, ready := loadOrWriteGolden(t, current)
	if !ready {
		return
	}
	if current == expected {
		return
	}
	t.Errorf("%s: golden mismatch (want %d bytes, got %d bytes)",
		label, len(expected), len(current))
	// Diff the first differing line for a focused failure message.
	wantLines := strings.Split(expected, "\n")
	gotLines := strings.Split(current, "\n")
	limit := len(wantLines)
	if len(gotLines) < limit {
		limit = len(gotLines)
	}
	for i := 0; i < limit; i++ {
		if wantLines[i] != gotLines[i] {
			t.Errorf("first diff at line %d:\n  want: %q\n   got: %q",
				i+1, wantLines[i], gotLines[i])
			return
		}
	}
	if len(wantLines) != len(gotLines) {
		t.Errorf("line count differs: want %d, got %d", len(wantLines), len(gotLines))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Golden variants
// ─────────────────────────────────────────────────────────────────────────────

// TestBuildMemoryLayer_GoldenEmptyDB locks the empty-stack baseline.
// No stack, no memory stores, no context router — buildMemoryLayer
// should produce the v1.18.0 byte-identical output.
func TestBuildMemoryLayer_GoldenEmptyDB(t *testing.T) {
	composer := newGoldenComposer(t)
	session := newGoldenSession()

	got := composer.buildMemoryLayer(session, goldenInput)
	assertGoldenEqual(t, "empty-DB baseline", got)
}

// TestBuildMemoryLayer_GoldenWithStackPresentButAllLayersEmpty proves
// that a wired-up stack whose layers all return empty is
// indistinguishable from no stack at all.
func TestBuildMemoryLayer_GoldenWithStackPresentButAllLayersEmpty(t *testing.T) {
	composer := newGoldenComposer(t)
	session := newGoldenSession()

	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	stack := newStackFromLayers(
		&mockLayer{out: ""},
		&mockLayer{out: ""},
		&mockLayer{out: ""},
		DefaultStackConfig(),
		logger,
	)
	composer.SetMemoryStack(stack)

	got := composer.buildMemoryLayer(session, goldenInput)
	assertGoldenEqual(t, "stack present but empty", got)
}

// TestBuildMemoryLayer_GoldenWithStackForceLegacy proves the escape
// hatch works: a fully-populated stack with ForceLegacy=true must
// produce the same output as no stack at all.
func TestBuildMemoryLayer_GoldenWithStackForceLegacy(t *testing.T) {
	composer := newGoldenComposer(t)
	session := newGoldenSession()

	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	cfg := DefaultStackConfig()
	cfg.ForceLegacy = true
	stack := newStackFromLayers(
		&mockLayer{out: "L0-populated"},
		&mockLayer{out: "L1-populated"},
		&mockLayer{out: "L2-populated"},
		cfg,
		logger,
	)
	composer.SetMemoryStack(stack)

	got := composer.buildMemoryLayer(session, goldenInput)
	assertGoldenEqual(t, "stack force-legacy", got)
}

// TestBuildMemoryLayer_StackPrefixInjected proves the stack output is
// prepended to the legacy output. With a populated stack returning
// "IDENTITY_TOKEN", the output must START with that token and the
// remainder must byte-equal the empty-stack baseline.
func TestBuildMemoryLayer_StackPrefixInjected(t *testing.T) {
	composer := newGoldenComposer(t)
	session := newGoldenSession()

	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	stack := newStackFromLayers(
		&mockLayer{out: "IDENTITY_TOKEN"},
		&mockLayer{out: ""},
		&mockLayer{out: ""},
		DefaultStackConfig(),
		logger,
	)
	composer.SetMemoryStack(stack)

	got := composer.buildMemoryLayer(session, goldenInput)

	// Load the baseline from the golden fixture. If it does not exist,
	// the other tests will have already written it; this test just
	// asserts the prepend relationship.
	baseline, err := os.ReadFile(goldenFixturePath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("golden fixture not yet written — re-run after first run")
		}
		t.Fatalf("failed to read golden fixture: %v", err)
	}

	if !strings.HasPrefix(got, "IDENTITY_TOKEN") {
		t.Errorf("expected output to start with IDENTITY_TOKEN, got %q", got)
	}

	// The remainder after the stack prefix (and its separator) must
	// equal the empty-stack baseline verbatim. The stack output is
	// joined to the rest of buildMemoryLayer's parts via "\n".
	rest := strings.TrimPrefix(got, "IDENTITY_TOKEN")
	rest = strings.TrimPrefix(rest, "\n")
	if rest != string(baseline) {
		t.Errorf("expected everything after IDENTITY_TOKEN\\n to byte-equal baseline\n  baseline bytes=%d\n  rest bytes=%d",
			len(baseline), len(rest))
	}
}

// discardWriter is an io.Writer that ignores all writes, used so slog
// handlers in the golden tests don't clutter test output.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// Compile-time check: ensure we can build a context.Background for use
// in the file. (Prevents unused import complaints if the imports drift.)
var _ = context.Background
