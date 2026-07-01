// Package memory — hierarchy_test.go covers the palace-aware normalization
// and path-derivation helpers. These tests protect Princípio Zero (retrocompat):
// legacy paths with no wing metadata must always return empty coordinates,
// and adversarial inputs must never produce a non-empty identifier.
package memory

import (
	"testing"
)

func TestNormalizeWing(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		// Happy path: English and Portuguese common cases.
		{"simple lowercase", "work", "work"},
		{"simple uppercase", "Work", "work"},
		{"mixed case", "WoRk", "work"},
		{"portuguese trabalho", "trabalho", "trabalho"},
		{"accented família → familia", "família", "familia"},
		{"multi-word spaces → kebab", "side hustle", "side-hustle"},
		{"multi-word mixed case", "Side Hustle", "side-hustle"},
		{"trim whitespace", "  work  ", "work"},
		{"underscore → hyphen", "side_project", "side-project"},
		{"dot → hyphen", "side.project", "side-project"},
		{"slash → hyphen", "side/project", "side-project"},
		{"collapse consecutive hyphens", "work---hard", "work-hard"},
		{"trim leading hyphen", "-work", "work"},
		{"trim trailing hyphen", "work-", "work"},

		// Accented Portuguese / Spanish / French characters.
		{"á → a", "áfrica", "africa"},
		{"ç → c", "coração", "coracao"},
		{"ñ → n", "mañana", "manana"},
		{"ü → u", "über", "uber"},

		// Legacy-preserving edge cases: must return empty so
		// caller treats as wing=NULL.
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
		{"single hyphen", "-", ""},
		{"only punctuation", "!!!", ""},
		{"only emoji", "🎉🎉🎉", ""},

		// Reserved prefix rejection.
		{"reserved __system", "__system", ""},
		{"reserved __internal", "__internal", ""},
		{"reserved double underscore only", "__", ""},

		// Length capping.
		{
			"long name capped at word boundary",
			"this-is-a-very-long-wing-name-that-exceeds-the-limit",
			"this-is-a-very-long-wing-name", // cap ~32 chars at word boundary
		},

		// Security: SQL / shell injection attempts become harmless.
		// "work';DROP TABLE--" → lowercase → "work';drop table--"
		// → replacer " "→"-" → "work';drop-table--"
		// → filter keeps [a-z0-9-] → "workdrop-table--"
		// → collapse "--" → "workdrop-table-" → trim → "workdrop-table"
		{"quotes stripped", "work';DROP TABLE--", "workdrop-table"},
		// "work;rm -rf /" → lowercase → replacer "/"→"-" " "→"-"
		// → "work;rm--rf--" → filter → "workrm--rf--"
		// → collapse → "workrm-rf-" → trim → "workrm-rf"
		{"semicolon stripped", "work;rm -rf /", "workrm-rf"},
		// "<script>work</script>" → lowercase → replacer "/"→"-"
		// → "<script>work<-script>" → filter → "scriptwork-script"
		{"angle brackets stripped", "<script>work</script>", "scriptwork-script"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeWing(c.input)
			if got != c.expected {
				t.Errorf("NormalizeWing(%q) = %q, want %q", c.input, got, c.expected)
			}
		})
	}
}

func TestNormalizeWingNeverStartsWithReservedPrefix(t *testing.T) {
	// Fuzz-ish check: any input that begins with "__" must normalize to "".
	inputs := []string{
		"__foo",
		"__bar",
		"__x",
		"  __trim",
		"__a-b-c",
	}
	for _, in := range inputs {
		got := NormalizeWing(in)
		if got != "" {
			t.Errorf("NormalizeWing(%q) = %q, want empty (reserved prefix)", in, got)
		}
	}
}

func TestNormalizeWingRespectsLengthCap(t *testing.T) {
	// Generate a very long input; result must not exceed MaxWingNameLength.
	longInput := ""
	for i := 0; i < 200; i++ {
		longInput += "word-"
	}
	got := NormalizeWing(longInput)
	if len(got) > MaxWingNameLength {
		t.Errorf("NormalizeWing truncation failed: len(%q) = %d, max %d", got, len(got), MaxWingNameLength)
	}
}

func TestNormalizeRoomAllowsLongerNames(t *testing.T) {
	// Rooms get a larger length budget than wings.
	input := "this-is-a-moderately-long-room-name-that-fits"
	got := NormalizeRoom(input)
	if len(got) > MaxRoomNameLength {
		t.Errorf("NormalizeRoom too long: len(%q) = %d, max %d", got, len(got), MaxRoomNameLength)
	}
	if got == "" {
		t.Errorf("NormalizeRoom should accept moderate-length input")
	}
}

func TestExtractCoordsFromPath(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		want    PalaceCoords
		isEmpty bool
	}{
		// Legacy: filename at root.
		{"legacy MEMORY.md", "MEMORY.md", PalaceCoords{}, true},
		{"legacy dated file", "2026-04-08.md", PalaceCoords{}, true},
		{"empty path", "", PalaceCoords{}, true},

		// Wing only.
		{"wing only", "work/MEMORY.md", PalaceCoords{Wing: "work"}, false},
		{"wing only dated", "work/2026-04-08.md", PalaceCoords{Wing: "work"}, false},

		// Wing + room (two components).
		{"wing + room", "work/auth-migration/MEMORY.md", PalaceCoords{Wing: "work", Room: "auth-migration"}, false},

		// Wing + hall + room (three components).
		{
			"wing + hall + room",
			"work/meetings/sprint-planning/MEMORY.md",
			PalaceCoords{Wing: "work", Hall: "meetings", Room: "sprint-planning"},
			false,
		},

		// Deeper paths collapse middle components.
		{
			"deep path, middle collapsed",
			"work/a/b/c/d/MEMORY.md",
			PalaceCoords{Wing: "work", Hall: "a", Room: "d"},
			false,
		},

		// Accent normalization from path.
		{
			"accented wing from path",
			"família/MEMORY.md",
			PalaceCoords{Wing: "familia"},
			false,
		},

		// Windows-style separators normalize correctly.
		{
			"forward slash cross-OS",
			"work/room/MEMORY.md",
			PalaceCoords{Wing: "work", Room: "room"},
			false,
		},

		// Edge case: path with leading slash.
		{
			"leading slash stripped",
			"/work/MEMORY.md",
			PalaceCoords{Wing: "work"},
			false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractCoordsFromPath(c.path)
			if got != c.want {
				t.Errorf("ExtractCoordsFromPath(%q) = %+v, want %+v", c.path, got, c.want)
			}
			if got.IsEmpty() != c.isEmpty {
				t.Errorf("IsEmpty mismatch for %q: got %v, want %v", c.path, got.IsEmpty(), c.isEmpty)
			}
		})
	}
}

func TestBuildPathForCoords(t *testing.T) {
	cases := []struct {
		name  string
		input PalaceCoords
		want  string
	}{
		{"empty", PalaceCoords{}, ""},
		{"wing only", PalaceCoords{Wing: "work"}, "work"},
		{"wing + room", PalaceCoords{Wing: "work", Room: "auth"}, "work/auth"},
		{"wing + hall + room", PalaceCoords{Wing: "work", Hall: "meetings", Room: "standup"}, "work/meetings/standup"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := BuildPathForCoords(c.input)
			if got != c.want {
				t.Errorf("BuildPathForCoords(%+v) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestIsLegacyPath(t *testing.T) {
	legacy := []string{
		"MEMORY.md",
		"2026-04-08.md",
		"daily.md",
		"",
	}
	for _, p := range legacy {
		if !IsLegacyPath(p) {
			t.Errorf("IsLegacyPath(%q) = false, want true", p)
		}
	}

	classified := []string{
		"work/MEMORY.md",
		"personal/2026-04-08.md",
		"family/schedule/MEMORY.md",
	}
	for _, p := range classified {
		if IsLegacyPath(p) {
			t.Errorf("IsLegacyPath(%q) = true, want false", p)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	// Coordinates → path → coordinates should be idempotent for normalized input.
	original := []PalaceCoords{
		{},
		{Wing: "work"},
		{Wing: "work", Room: "auth"},
		{Wing: "work", Hall: "meetings", Room: "standup"},
	}
	for _, orig := range original {
		path := BuildPathForCoords(orig)
		// Add a fake filename so ExtractCoordsFromPath has something to strip.
		pathWithFile := path
		if pathWithFile != "" {
			pathWithFile += "/MEMORY.md"
		} else {
			pathWithFile = "MEMORY.md"
		}
		got := ExtractCoordsFromPath(pathWithFile)
		if got != orig {
			t.Errorf("round trip: %+v → %q → %+v", orig, pathWithFile, got)
		}
	}
}

func TestSuggestedWingsAllNormalize(t *testing.T) {
	// Every suggested wing must normalize to itself (canonical form).
	for _, w := range SuggestedWings() {
		got := NormalizeWing(w)
		if got != w {
			t.Errorf("SuggestedWings contains non-canonical %q (normalizes to %q)", w, got)
		}
	}
}
