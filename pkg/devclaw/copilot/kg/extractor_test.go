package kg

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg/patterns"
	"gopkg.in/yaml.v3"
)

//go:embed testdata/kg/pt-br-phrases.yaml
var ptbrPhrasesFS embed.FS

//go:embed patterns/pt-br.yaml
var ptbrPatternsYAML []byte

func loadPTBRPatternSet(t *testing.T) *patterns.PatternSet {
	t.Helper()
	ps, err := patterns.Load(ptbrPatternsYAML)
	if err != nil {
		t.Fatalf("load pt-br patterns: %v", err)
	}
	return ps
}

func newTestExtractor(t *testing.T) *Extractor {
	t.Helper()
	ps := loadPTBRPatternSet(t)
	ext, err := NewExtractor([]*patterns.PatternSet{ps}, slog.Default())
	if err != nil {
		t.Fatalf("NewExtractor: %v", err)
	}
	return ext
}

// phraseFixture mirrors the YAML structure of pt-br-phrases.yaml.
type phraseFixture struct {
	Phrases []phraseEntry `yaml:"phrases"`
}

type phraseEntry struct {
	Input    string           `yaml:"input"`
	Expected []expectedTriple `yaml:"expected"`
}

type expectedTriple struct {
	Subject   string `yaml:"subject"`
	Predicate string `yaml:"predicate"`
	Object    string `yaml:"object"`
}

func loadPhraseFixture(t *testing.T) []phraseEntry {
	t.Helper()
	data, err := ptbrPhrasesFS.ReadFile("testdata/kg/pt-br-phrases.yaml")
	if err != nil {
		t.Fatalf("read phrase fixture: %v", err)
	}
	var fixture phraseFixture
	if err := yaml.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("parse phrase fixture: %v", err)
	}
	return fixture.Phrases
}

func TestExtractor_LoadsPatternsFromYAML(t *testing.T) {
	ps := loadPTBRPatternSet(t)
	if ps.Language != "pt-br" {
		t.Errorf("expected language pt-br, got %q", ps.Language)
	}
	if len(ps.Patterns) < 5 {
		t.Fatalf("expected at least 5 patterns, got %d", len(ps.Patterns))
	}
	names := make(map[string]bool)
	for _, p := range ps.Patterns {
		if p.Name == "" {
			t.Error("pattern with empty name")
		}
		if p.Predicate == "" {
			t.Errorf("pattern %q: empty predicate", p.Name)
		}
		if len(p.Templates) == 0 {
			t.Errorf("pattern %q: no templates", p.Name)
		}
		names[p.Name] = true
	}
	for _, want := range []string{"works_at", "lives_in", "likes", "knows", "met"} {
		if !names[want] {
			t.Errorf("missing pattern %q", want)
		}
	}
}

func TestExtractor_PTBR_FixturePhrases(t *testing.T) {
	ext := newTestExtractor(t)
	phrases := loadPhraseFixture(t)
	if len(phrases) < 30 {
		t.Fatalf("expected at least 30 phrases in fixture, got %d", len(phrases))
	}
	for _, ph := range phrases {
		t.Run(ph.Input, func(t *testing.T) {
			results := ext.Extract(ph.Input)
			if len(ph.Expected) == 0 {
				if len(results) > 0 {
					t.Errorf("expected no extractions, got %d: %+v", len(results), results)
				}
				return
			}
			if len(results) != len(ph.Expected) {
				t.Errorf("expected %d extractions, got %d: %+v", len(ph.Expected), len(results), results)
				return
			}
			for _, exp := range ph.Expected {
				matched := false
				for _, r := range results {
					if r.Subject == exp.Subject && r.Predicate == exp.Predicate && r.Object == exp.Object {
						matched = true
						break
					}
				}
				if !matched {
					t.Errorf("expected triple {%s, %s, %s} not found in results %+v",
						exp.Subject, exp.Predicate, exp.Object, results)
				}
			}
		})
	}
}

func TestExtractor_Idempotent(t *testing.T) {
	k := newTestKG(t)
	ext := newTestExtractor(t)
	ctx := context.Background()

	text := "Maria trabalha na ACME"
	n1, err := ext.ExtractAndStore(ctx, k, text, "", "mem-001")
	if err != nil {
		t.Fatalf("first ExtractAndStore: %v", err)
	}
	if n1 == 0 {
		t.Fatal("first call should insert at least 1 triple")
	}

	n2, err := ext.ExtractAndStore(ctx, k, text, "", "mem-001")
	if err != nil {
		t.Fatalf("second ExtractAndStore: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second call should insert 0 triples (idempotent), got %d", n2)
	}
}

func TestExtractor_WingInheritance(t *testing.T) {
	k := newTestKG(t)
	ext := newTestExtractor(t)
	ctx := context.Background()

	text := "Maria mora em Sao Paulo"
	n, err := ext.ExtractAndStore(ctx, k, text, "work", "mem-wing-001")
	if err != nil {
		t.Fatalf("ExtractAndStore: %v", err)
	}
	if n == 0 {
		t.Fatal("expected at least 1 insertion")
	}

	var wing sql.NullString
	err = k.db.QueryRow(
		"SELECT wing FROM kg_triples WHERE wing IS NOT NULL LIMIT 1",
	).Scan(&wing)
	if err != nil {
		t.Fatalf("query wing: %v", err)
	}
	if !wing.Valid || wing.String != "work" {
		t.Errorf("expected wing='work', got %v", wing)
	}
}

func TestExtractor_NullWingPreserved(t *testing.T) {
	k := newTestKG(t)
	ext := newTestExtractor(t)
	ctx := context.Background()

	text := "Pedro gosta de cafe"
	n, err := ext.ExtractAndStore(ctx, k, text, "", "mem-null-wing")
	if err != nil {
		t.Fatalf("ExtractAndStore: %v", err)
	}
	if n == 0 {
		t.Fatal("expected at least 1 insertion")
	}

	var wing sql.NullString
	err = k.db.QueryRow(
		"SELECT wing FROM kg_triples LIMIT 1",
	).Scan(&wing)
	if err != nil {
		t.Fatalf("query wing: %v", err)
	}
	if wing.Valid {
		t.Errorf("expected NULL wing, got %q", wing.String)
	}
}

func TestExtractor_NoPanicOnMalformedInput(t *testing.T) {
	ext := newTestExtractor(t)

	cases := map[string]string{
		"empty":         "",
		"whitespace":    "   \t\n  ",
		"long_100kb":    strings.Repeat("a maria gosta de cafe ", 5000),
		"null_bytes":    "maria\x00gosta\x00de\x00cafe",
		"regex_breaker": `((((()))))))\1\2\3***???+++`,
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			// Must not panic.
			results := ext.Extract(input)
			_ = results
		})
	}
}

func TestExtractor_NoLocaleStringsInGoSource(t *testing.T) {
	localePattern := regexp.MustCompile(`(?:trabalha|mora|gosta|conhece|encontrou|funcionário|funcionária|reside|adora|curte|conheceu)`)
	root := "."
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skip("cannot resolve kg source directory")
	}
	violations := []string{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Skip string literals that are file paths (e.g. "patterns/pt-br.yaml")
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if localePattern.MatchString(line) {
				rel, _ := filepath.Rel(root, path)
				violations = append(violations, rel+": "+trimmed)
			}
		}
		return nil
	})
	if len(violations) > 0 {
		t.Errorf("found locale strings in Go source files (should be in YAML only):\n%s",
			strings.Join(violations, "\n"))
	}
}
