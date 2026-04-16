package copilot

import "testing"

func TestIndexBullet_RecognizesIndentedBullets(t *testing.T) {
	cases := map[string]int{
		"- item":                  0,
		"  - indented":            2,
		"\t- tabbed":              1,
		"not a bullet":            -1,
		"-missing-space":          -1,
		"    - [2026-04-16] fact": 4,
	}
	for in, want := range cases {
		if got := indexBullet(in); got != want {
			t.Errorf("indexBullet(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestStripEntryBrackets_PeelsTimestampCategoryExpiry(t *testing.T) {
	cases := map[string]string{
		"[2026-04-16 12:30] [fact] server accepts deploy":                       "server accepts deploy",
		"[2026-04-16 12:30] [summary] [expires:2026-05-16] weekly notes":        "weekly notes",
		"[2026-04-16 12:30] [fact] multi word content with [brackets] inside":   "multi word content with [brackets] inside",
		"plain content without brackets":                                        "plain content without brackets",
		"[only one] content":                                                    "content",
	}
	for in, want := range cases {
		if got := stripEntryBrackets(in); got != want {
			t.Errorf("stripEntryBrackets(%q) = %q, want %q", in, got, want)
		}
	}
}
