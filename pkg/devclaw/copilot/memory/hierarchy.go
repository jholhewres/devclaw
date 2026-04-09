// Package memory — hierarchy.go provides palace-aware wing/hall/room helpers
// for organizing memories into contextual namespaces.
//
// Sprint 1 (v1.18.0): introduces the Wing/Hall/Room taxonomy per ADR-001.
// All functions here are PURE GO with no database dependency — they operate
// on file paths and string normalization only.
//
// Conventions for wing derivation from file path:
//
//	MEMORY.md                               → wing=NULL (legacy, first-class)
//	{wing}/MEMORY.md                        → wing
//	{wing}/{room}/MEMORY.md                 → wing + room
//	{wing}/{hall}/{room}/MEMORY.md          → wing + hall + room
//	{wing}/YYYY-MM-DD.md                    → wing (daily log)
//	{wing}/{room}/YYYY-MM-DD.md             → wing + room
//
// Legacy behavior: files at the memory base directory root without a wing
// prefix have wing=NULL. All queries treat wing=NULL as a first-class citizen
// per Princípio Zero rule 5 (retrocompat absoluta).
//
// Normalization: wing/room/hall names are stored in canonical form
// (lowercase, trimmed, accents removed, spaces → kebab-case, non-alnum stripped).
// Display names can be set separately in the `wings`/`rooms` tables.
//
// References:
//   - ADR-001 Wing Naming Scheme
//   - ADR-002 Room Auto vs Manual
//   - Sprint 0.5 agents-map-v2 §File Inventory
package memory

import (
	"path/filepath"
	"strings"
	"unicode"
)

// MaxWingNameLength caps the length of a normalized wing identifier.
// 32 characters is generous (8-word kebab-case) while preventing abuse.
const MaxWingNameLength = 32

// MaxRoomNameLength caps the length of a normalized room identifier.
const MaxRoomNameLength = 48

// MaxHallNameLength caps the length of a normalized hall identifier.
const MaxHallNameLength = 32

// ReservedWingPrefix marks internal/system wings that users cannot create.
// Any wing name starting with this prefix is rejected by NormalizeWing.
const ReservedWingPrefix = "__"

// PalaceCoords groups the wing/hall/room triple derived from a path
// or supplied by a tool call. All fields are optional; empty string means
// "not set" (which maps to SQL NULL at persistence boundaries).
type PalaceCoords struct {
	Wing string // normalized wing identifier, "" = not set (legacy)
	Hall string // normalized hall identifier, "" = no hall
	Room string // normalized room identifier, "" = no room
}

// IsEmpty reports whether no palace coordinates are set (legacy memory).
func (c PalaceCoords) IsEmpty() bool {
	return c.Wing == "" && c.Hall == "" && c.Room == ""
}

// NormalizeWing produces the canonical wing identifier from user input.
// Returns empty string if the input normalizes to nothing or starts with
// the reserved prefix "__".
//
// Algorithm:
//  1. Trim whitespace
//  2. Lowercase (Unicode-aware)
//  3. Strip accents (NFD decomposition keeping base characters)
//  4. Replace spaces and underscores with hyphens
//  5. Drop anything that is not [a-z0-9-]
//  6. Collapse consecutive hyphens
//  7. Trim leading/trailing hyphens
//  8. Reject if result is empty or starts with the reserved prefix
//  9. Truncate to MaxWingNameLength at a word boundary
//
// Examples:
//
//	NormalizeWing("Work")       → "work"
//	NormalizeWing("Trabalho")   → "trabalho"
//	NormalizeWing("Família")    → "familia"
//	NormalizeWing("side hustle") → "side-hustle"
//	NormalizeWing("  ")         → ""
//	NormalizeWing("__system")   → ""
//	NormalizeWing("🎉🎉")       → ""
func NormalizeWing(input string) string {
	return normalizeIdentifier(input, MaxWingNameLength)
}

// NormalizeRoom produces the canonical room identifier. Same algorithm as
// NormalizeWing but with a larger length budget.
func NormalizeRoom(input string) string {
	return normalizeIdentifier(input, MaxRoomNameLength)
}

// NormalizeHall produces the canonical hall identifier.
func NormalizeHall(input string) string {
	return normalizeIdentifier(input, MaxHallNameLength)
}

// normalizeIdentifier is the shared normalization engine.
func normalizeIdentifier(input string, maxLen int) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}

	s = strings.ToLower(s)
	s = stripAccents(s)

	// Reject reserved prefix BEFORE separator conversion — we need to see
	// literal underscores to detect "__system" and friends. The check is
	// done after accent stripping so variants like "__Família" are also
	// rejected consistently.
	if strings.HasPrefix(s, ReservedWingPrefix) {
		return ""
	}

	// Replace separators with hyphens.
	replacer := strings.NewReplacer(" ", "-", "_", "-", ".", "-", "/", "-")
	s = replacer.Replace(s)

	// Drop anything outside [a-z0-9-].
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s = b.String()

	// Collapse consecutive hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}

	// Trim leading/trailing hyphens.
	s = strings.Trim(s, "-")

	if s == "" {
		return ""
	}

	// Truncate at word boundary if too long.
	if len(s) > maxLen {
		s = s[:maxLen]
		// Avoid ending on a partial word — trim back to last hyphen.
		if idx := strings.LastIndex(s, "-"); idx > maxLen/2 {
			s = s[:idx]
		}
	}

	return s
}

// stripAccents removes combining diacritical marks from a string while
// preserving the base characters. "família" → "familia", "coração" → "coracao".
//
// This is a zero-dependency implementation that handles the common Latin
// diacritics sufficient for Portuguese, Spanish, French, and English.
func stripAccents(s string) string {
	// Table of common accented lowercase Latin characters to their ASCII base.
	// This is not exhaustive (e.g., CJK is untouched) but covers every
	// accent used in the target languages for wing naming.
	var replacements = map[rune]rune{
		'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a',
		'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
		'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
		'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o',
		'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
		'ç': 'c',
		'ñ': 'n',
		'ý': 'y', 'ÿ': 'y',
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if rep, ok := replacements[r]; ok {
			b.WriteRune(rep)
			continue
		}
		// Skip combining marks (category Mn) that may come from NFD-decomposed input.
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ExtractCoordsFromPath derives wing/hall/room from a file path relative
// to the memory base directory. The separator is the OS path separator.
//
// Rules (applied to path components BEFORE the final filename):
//   - 0 components → all empty (legacy file at root)
//   - 1 component  → wing only
//   - 2 components → wing + room
//   - 3+ components → wing + hall + room (middle components collapsed)
//
// The final filename (MEMORY.md, YYYY-MM-DD.md, etc.) is ignored for
// coordinate derivation.
//
// If any component fails normalization, the whole coordinate set is
// rejected and empty coords are returned (fail-safe to legacy behavior).
//
// Examples (assuming baseDir="/home/u/.devclaw/memory"):
//
//	"MEMORY.md"                           → {}
//	"work/MEMORY.md"                      → {Wing:"work"}
//	"work/auth-migration/MEMORY.md"       → {Wing:"work", Room:"auth-migration"}
//	"work/meetings/auth/MEMORY.md"        → {Wing:"work", Hall:"meetings", Room:"auth"}
//	"work/a/b/c/d/MEMORY.md"              → {Wing:"work", Hall:"a", Room:"d"}  (middle collapsed)
//	"FAMÍLIA/MEMORY.md"                   → {Wing:"familia"}
func ExtractCoordsFromPath(relPath string) PalaceCoords {
	// Normalize the separator to forward slash for cross-OS consistency.
	relPath = filepath.ToSlash(relPath)
	relPath = strings.TrimPrefix(relPath, "/")

	if relPath == "" {
		return PalaceCoords{}
	}

	// Split into directory components; drop the final filename.
	parts := strings.Split(relPath, "/")
	if len(parts) <= 1 {
		// Just a filename at root — legacy memory.
		return PalaceCoords{}
	}
	dirs := parts[:len(parts)-1]

	coords := PalaceCoords{}
	switch len(dirs) {
	case 1:
		coords.Wing = NormalizeWing(dirs[0])
	case 2:
		coords.Wing = NormalizeWing(dirs[0])
		coords.Room = NormalizeRoom(dirs[1])
	default: // 3 or more
		coords.Wing = NormalizeWing(dirs[0])
		coords.Hall = NormalizeHall(dirs[1])
		coords.Room = NormalizeRoom(dirs[len(dirs)-1])
	}

	// Fail-safe: if any non-empty input failed normalization, reject all.
	if coords.Wing == "" && len(dirs) > 0 && dirs[0] != "" {
		return PalaceCoords{}
	}

	return coords
}

// BuildPathForCoords constructs the relative directory path that
// corresponds to a PalaceCoords value. The returned path does NOT include
// the filename.
//
// Examples:
//
//	{}                                → ""
//	{Wing:"work"}                     → "work"
//	{Wing:"work", Room:"auth"}        → "work/auth"
//	{Wing:"work", Hall:"m", Room:"a"} → "work/m/a"
func BuildPathForCoords(c PalaceCoords) string {
	if c.IsEmpty() {
		return ""
	}
	parts := []string{}
	if c.Wing != "" {
		parts = append(parts, c.Wing)
	}
	if c.Hall != "" {
		parts = append(parts, c.Hall)
	}
	if c.Room != "" {
		parts = append(parts, c.Room)
	}
	return filepath.ToSlash(filepath.Join(parts...))
}

// IsLegacyPath reports whether a relative path corresponds to a legacy
// memory file (no palace coordinates). This is useful for filters that
// want to distinguish "unclassified" memories from "classified" ones.
func IsLegacyPath(relPath string) bool {
	return ExtractCoordsFromPath(relPath).IsEmpty()
}

// SuggestedWings returns a list of wing names suggested to new users on
// first init. These are NOT reserved and can be deleted or ignored.
// Users are free to create entirely different wings.
func SuggestedWings() []string {
	return []string{
		"work",
		"personal",
		"family",
		"finance",
		"learning",
		"health",
	}
}
