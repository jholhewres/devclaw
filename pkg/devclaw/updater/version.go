// Package updater implements auto-update checking and installation for DevClaw.
package updater

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a parsed semantic version with optional git-describe metadata.
type Version struct {
	Major, Minor, Patch int
	// Pre is the number of commits after the tag (git-describe format: v0.0.4-9 → Pre=9).
	// Zero means this is an exact tag match.
	Pre int
	Raw string
}

// ParseVersion parses a version string like "v1.2.3", "1.2.3", or "v1.2.3-9-gabcdef" into a Version.
func ParseVersion(s string) (Version, error) {
	raw := s
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")

	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version format: %q (expected MAJOR.MINOR.PATCH)", raw)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version: %w", err)
	}

	// Split patch from suffix (e.g. "4-9-gabcdef-dirty" → patch "4", rest "9-gabcdef-dirty").
	patchParts := strings.SplitN(parts[2], "-", 2)
	patch, err := strconv.Atoi(patchParts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version: %w", err)
	}

	// Parse optional commit count from git-describe suffix (e.g. "9-gabcdef-dirty" → 9).
	var pre int
	if len(patchParts) == 2 {
		preStr := strings.SplitN(patchParts[1], "-", 2)[0]
		if n, err := strconv.Atoi(preStr); err == nil {
			pre = n
		}
	}

	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
		Pre:   pre,
		Raw:   raw,
	}, nil
}

// IsNewerThan returns true if v is a newer version than other.
func (v Version) IsNewerThan(other Version) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor > other.Minor
	}
	if v.Patch != other.Patch {
		return v.Patch > other.Patch
	}
	return v.Pre > other.Pre
}

// String returns the raw version string.
func (v Version) String() string {
	if v.Raw != "" {
		return v.Raw
	}
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}
