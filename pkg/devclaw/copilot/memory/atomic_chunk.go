// Package memory — atomic_chunk.go splits rich multi-fact content into atomic
// fact strings so each is independently recallable. A long itinerary stored as
// one chunk dilutes its embedding; a narrow query ("qual o localizador") then
// loses to shorter, focused chunks. Splitting on line/sentence boundaries (never
// commas) keeps coherent lists intact while isolating each fact.
package memory

import "strings"

const (
	atomicSplitThreshold = 200 // only content longer than this is split
	atomicMinFragmentLen = 24  // fragments shorter than this merge into the previous
)

// splitAtomicFacts partitions content into atomic facts. Short or single-fact
// content is returned unchanged. The pieces concatenate back to the input
// (modulo whitespace) — no text is dropped.
func splitAtomicFacts(content string) []string {
	text := strings.TrimSpace(content)
	if len([]rune(text)) <= atomicSplitThreshold {
		return []string{text}
	}

	var raw []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		raw = append(raw, splitSentences(line)...)
	}

	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(out) > 0 && len([]rune(p)) < atomicMinFragmentLen {
			out[len(out)-1] += " " + p
			continue
		}
		out = append(out, p)
	}
	if len(out) <= 1 {
		return []string{text}
	}
	return out
}

// splitSentences splits a single line on ". " boundaries, keeping the period
// with each sentence. Periods without a trailing space (versions, times,
// decimals) do not split.
func splitSentences(line string) []string {
	line = strings.TrimSpace(line)
	parts := strings.Split(line, ". ")
	if len(parts) == 1 {
		return []string{line}
	}
	out := make([]string, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if i < len(parts)-1 {
			p += "."
		}
		out = append(out, p)
	}
	return out
}
