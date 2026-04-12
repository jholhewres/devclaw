// Package copilot — memory_categorizer.go classifies memory content
// into categories (event, summary, preference, fact) using keyword patterns.
// Zero LLM calls — pure regex matching in PT and EN.
package copilot

import "regexp"

// categoryPattern maps a compiled regex to its target category.
type categoryPattern struct {
	re       *regexp.Regexp
	category string
}

// compiledCategoryPatterns is built at init() from the raw pattern list.
var compiledCategoryPatterns []categoryPattern

func init() {
	patterns := []struct {
		pattern  string
		category string
	}{
		// SUMMARY patterns checked FIRST — "daily summary" must not match event's "daily".
		{`(?i)(daily|weekly|monthly).*(log|summary|report|relatório)`, "summary"},
		{`(?i)\b(resumo|summary|compacted|consolidado|consolidated)\b`, "summary"},
		{`(?i)\b(overview|balanço|relatório|recap)\b`, "summary"},

		// EVENT patterns — time-bound activities that expire (TTL 7d).
		{`(?i)\b(reunião|meeting|agenda|call|standup|sprint|daily)\b`, "event"},
		{`(?i)\b(lembrete|reminder|alerta|alert|aviso)\b`, "event"},
		{`(?i)\b\d{1,2}[/:h]\d{2}\b`, "event"},                                                               // time: 15:00, 6h30
		{`(?i)\b(hoje|amanhã|ontem|tomorrow|yesterday|today)\b`, "event"},                                      // relative dates
		{`(?i)\b(segunda|terça|quarta|quinta|sexta|sábado|domingo)\b`, "event"},                                 // PT weekdays
		{`(?i)\b(monday|tuesday|wednesday|thursday|friday|saturday|sunday)\b`, "event"},                         // EN weekdays
		{`(?i)\b(deploy|release|rollback|hotfix|incident|outage)\b`, "event"},                                   // ops events
		{`(?i)\b(comprou|pagou|transferiu|bought|paid|transferred|depositou|sacou)\b`, "event"},                 // financial events
		{`(?i)\b(saldo|fatura|conta|balance|invoice|bill)\b.*\b(R\$|BRL|\d+[.,]\d{2})\b`, "event"},             // financial with amount

		// (remaining summary patterns already declared above)

		// PREFERENCE patterns — user preferences that never expire.
		{`(?i)\b(prefere|prefer[es]?|gosta\s+de|likes?|sempre\s+usa|always\s+use)\b`, "preference"},
		{`(?i)\b(não\s+gosta|dislikes?|evita|avoids?|nunca|never)\b`, "preference"},
		{`(?i)\b(modo|mode|theme|layout)\b.*(escuro|dark|claro|light)`, "preference"},
		{`(?i)\b(favorit[oa]|favorite|preferid[oa]|preferred)\b`, "preference"},
	}

	for _, p := range patterns {
		compiledCategoryPatterns = append(compiledCategoryPatterns, categoryPattern{
			re:       regexp.MustCompile(p.pattern),
			category: p.category,
		})
	}
}

// categorizeMemory classifies memory content into a category based on
// keyword patterns. Returns "fact" as the safe default (never expires).
//
// Priority: event > summary > preference > fact.
// First match wins within each priority group.
func categorizeMemory(content string) string {
	// Priority order: check event first, then summary, then preference.
	for _, cp := range compiledCategoryPatterns {
		if cp.re.MatchString(content) {
			return cp.category
		}
	}
	return "fact"
}
