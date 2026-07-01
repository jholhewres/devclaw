// Package memory — temporal_query.go resolves natural-language temporal cues in
// a recall query ("ontem", "na sexta", "dia 18", "18/06", "2026-06-18") into a
// concrete [start, end) time window. The window is used by HybridSearchWithOpts
// to hard-filter recall to chunks whose occurred_at falls inside it (US-003).
//
// TZ contract: occurred_at is stored as a real local-wall-clock instant (US-001
// parses the .md stamp with time.ParseInLocation(time.Local)). Every window this
// file builds is therefore anchored in now.Location() at DAY granularity so the
// boundaries line up with occurred_at. A single-day cue yields
// [localMidnight(day), localMidnight(day)+24h). We never mix UTC.
//
// Detection is deliberately CONSERVATIVE: resolveTemporalWindow returns ok=false
// unless a clear date cue is present, so a query with no temporal intent leaves
// normal recall completely unchanged.
package memory

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// reExplicitISO matches an explicit ISO date (YYYY-MM-DD), e.g. "2026-06-18".
var reExplicitISO = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)

// reDateDMY matches DD/MM/YYYY, e.g. "18/06/2026".
var reDateDMY = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})/(\d{4})\b`)

// reDateDM matches DD/MM (no year), e.g. "18/06".
var reDateDM = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})\b`)

// reDiaDM matches "dia DD/MM", e.g. "dia 18/06".
var reDiaDM = regexp.MustCompile(`\bdia\s+(\d{1,2})/(\d{1,2})\b`)

// reDiaN matches "dia N", a bare day-of-month, e.g. "dia 18".
var reDiaN = regexp.MustCompile(`\bdia\s+(\d{1,2})\b`)

// weekdayCue maps a PT-BR / EN weekday token to its time.Weekday.
var weekdayCue = map[string]time.Weekday{
	"segunda":       time.Monday,
	"segunda-feira": time.Monday,
	"monday":        time.Monday,
	"terca":         time.Tuesday, // accent-stripped form
	"terça":         time.Tuesday,
	"terca-feira":   time.Tuesday,
	"terça-feira":   time.Tuesday,
	"tuesday":       time.Tuesday,
	"quarta":        time.Wednesday,
	"quarta-feira":  time.Wednesday,
	"wednesday":     time.Wednesday,
	"quinta":        time.Thursday,
	"quinta-feira":  time.Thursday,
	"thursday":      time.Thursday,
	"sexta":         time.Friday,
	"sexta-feira":   time.Friday,
	"friday":        time.Friday,
	"sabado":        time.Saturday, // accent-stripped form
	"sábado":        time.Saturday,
	"saturday":      time.Saturday,
	"domingo":       time.Sunday,
	"sunday":        time.Sunday,
}

// wordReCache caches compiled word-boundary regexps for hasWord / hasPhrase.
// All fixed tokens (relative-day cues + weekday names + phrase fragments) are
// seeded at init time; ad-hoc tokens compile-on-miss under the write lock.
var (
	wordReCacheMu sync.RWMutex
	wordReCache   = map[string]*regexp.Regexp{}
)

func init() {
	// Pre-seed every fixed token used by hasWord so the hot recall path never
	// compiles a regexp at runtime.
	fixedTokens := []string{
		"anteontem", "ontem", "yesterday", "hoje", "today",
	}
	for _, tok := range fixedTokens {
		wordReCache[tok] = regexp.MustCompile(`\b` + regexp.QuoteMeta(tok) + `\b`)
	}
	for tok := range weekdayCue {
		wordReCache[tok] = regexp.MustCompile(`\b` + regexp.QuoteMeta(tok) + `\b`)
	}

	// Pre-seed every fixed phrase used by hasPhrase.
	fixedPhrases := []string{
		"semana passada", "last week",
		"esta semana", "this week", "essa semana",
		"mes passado", "mês passado", "last month",
	}
	for _, ph := range fixedPhrases {
		parts := strings.Fields(ph)
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		hasPhraseReCache[ph] = regexp.MustCompile(`\b` + strings.Join(parts, `\s+`) + `\b`)
	}
}

// compiledWordRe returns (creating if absent) the word-boundary regexp for tok.
func compiledWordRe(tok string) *regexp.Regexp {
	wordReCacheMu.RLock()
	re, ok := wordReCache[tok]
	wordReCacheMu.RUnlock()
	if ok {
		return re
	}
	compiled := regexp.MustCompile(`\b` + regexp.QuoteMeta(tok) + `\b`)
	wordReCacheMu.Lock()
	wordReCache[tok] = compiled
	wordReCacheMu.Unlock()
	return compiled
}

// ResolveTemporalWindow is the exported entry point for the recall wiring
// (copilot.handleMemorySearch). It delegates to the pure, table-tested
// resolveTemporalWindow. ok is false when no clear date cue is present, so
// callers leave the search window unset and normal recall is unchanged.
func ResolveTemporalWindow(query string, now time.Time) (start, end time.Time, ok bool) {
	return resolveTemporalWindow(query, now)
}

// resolveTemporalWindow detects a PT-BR or EN temporal cue in query and resolves
// it to a half-open [start, end) window in now.Location(). It returns ok=false
// when no clear date cue is present, so callers leave normal recall untouched.
//
// Resolution rules (all day-aligned in now.Location()):
//   - hoje/today                  → [today, today+24h)
//   - ontem/yesterday             → [today-1d, today)
//   - anteontem/day before yest.  → [today-2d, today-1d)
//   - weekday name (PT/EN)        → MOST RECENT PAST occurrence of that weekday;
//     when today IS that weekday, resolves to today (not 7 days ago)
//   - esta semana/this week       → current Mon..Sun week [Mon, nextMon)
//   - semana passada/last week    → previous Mon..Sun week [prevMon, Mon)
//   - mês passado/last month      → [firstOfPrevMonth, firstOfThisMonth)
//   - dia N                       → day N of the CURRENT month (most recent
//     occurrence: if N is in the future this month, use previous month)
//   - dia DD/MM, DD/MM            → that day in the current year (most recent:
//     if in the future, previous year)
//   - DD/MM/YYYY, YYYY-MM-DD      → that exact day
func resolveTemporalWindow(query string, now time.Time) (start, end time.Time, ok bool) {
	loc := now.Location()
	q := normalizeTemporalQuery(query)

	// Explicit absolute dates first — highest precision, lowest ambiguity.
	if m := reExplicitISO.FindStringSubmatch(q); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		if validYMD(y, mo, d) {
			return dayWindow(time.Date(y, time.Month(mo), d, 0, 0, 0, 0, loc))
		}
	}
	if m := reDateDMY.FindStringSubmatch(q); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		if validYMD(y, mo, d) {
			return dayWindow(time.Date(y, time.Month(mo), d, 0, 0, 0, 0, loc))
		}
	}
	// "dia DD/MM" and bare "DD/MM" — resolve to the most recent past occurrence.
	if m := reDiaDM.FindStringSubmatch(q); m != nil {
		return dayMonthWindow(m[1], m[2], now)
	}
	if m := reDateDM.FindStringSubmatch(q); m != nil {
		// Validate captured day/month before treating as a date cue. Without
		// this guard, "nginx/1.24", "porta 80/443", "v1/2" fire the regexp and
		// produce a bogus window (80>31 passes the outer reDateDM but must not
		// resolve). dayMonthWindow also validates, but we want ok=false not a
		// calendar-reject silent drop — same effect, just explicit.
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		if d < 1 || d > 31 || mo < 1 || mo > 12 {
			return time.Time{}, time.Time{}, false
		}
		// Require at least one component to have ≥2 digits. A bare single-
		// digit/single-digit match (e.g. "1/2", "endpoint 1/2 do fluxo") is
		// too ambiguous to be a reliable date cue; "5/10" and "18/06" are fine.
		if len(m[1]) < 2 && len(m[2]) < 2 {
			return time.Time{}, time.Time{}, false
		}
		return dayMonthWindow(m[1], m[2], now)
	}

	// Relative day cues. anteontem must be checked before ontem; both are
	// matched as whole words so "ontem" inside another token won't false-fire.
	switch {
	case hasWord(q, "anteontem"):
		return dayWindow(midnight(now, loc).AddDate(0, 0, -2))
	case hasWord(q, "ontem"), hasWord(q, "yesterday"):
		return dayWindow(midnight(now, loc).AddDate(0, 0, -1))
	case hasWord(q, "hoje"), hasWord(q, "today"):
		return dayWindow(midnight(now, loc))
	}

	// Week / month spans.
	switch {
	case hasPhrase(q, "semana passada"), hasPhrase(q, "last week"):
		thisMon := startOfWeek(now, loc)
		prevMon := thisMon.AddDate(0, 0, -7)
		return prevMon, thisMon, true
	case hasPhrase(q, "esta semana"), hasPhrase(q, "this week"), hasPhrase(q, "essa semana"):
		thisMon := startOfWeek(now, loc)
		return thisMon, thisMon.AddDate(0, 0, 7), true
	case hasPhrase(q, "mes passado"), hasPhrase(q, "mês passado"), hasPhrase(q, "last month"):
		firstThis := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		firstPrev := firstThis.AddDate(0, -1, 0)
		return firstPrev, firstThis, true
	}

	// Weekday names — most recent PAST occurrence (today if today matches).
	if wd, found := matchWeekday(q); found {
		day := mostRecentWeekday(now, loc, wd)
		return dayWindow(day)
	}

	// "dia N" — bare day-of-month, most recent occurrence.
	if m := reDiaN.FindStringSubmatch(q); m != nil {
		d, _ := strconv.Atoi(m[1])
		if d >= 1 && d <= 31 {
			return dayOfMonthWindow(d, now, loc)
		}
	}

	return time.Time{}, time.Time{}, false
}

// dayWindow returns the half-open [day, day+24h) window for a midnight-aligned t.
func dayWindow(day time.Time) (time.Time, time.Time, bool) {
	return day, day.AddDate(0, 0, 1), true
}

// midnight returns local midnight of t's calendar day in loc.
func midnight(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// startOfWeek returns local midnight of the Monday on or before t.
func startOfWeek(t time.Time, loc *time.Location) time.Time {
	m := midnight(t, loc)
	// Go's Weekday: Sunday=0..Saturday=6. Days since Monday:
	offset := (int(m.Weekday()) + 6) % 7
	return m.AddDate(0, 0, -offset)
}

// mostRecentWeekday returns local midnight of the most recent past (or today's)
// occurrence of wd relative to now.
func mostRecentWeekday(now time.Time, loc *time.Location, wd time.Weekday) time.Time {
	m := midnight(now, loc)
	diff := (int(m.Weekday()) - int(wd) + 7) % 7 // 0 when today == wd
	return m.AddDate(0, 0, -diff)
}

// dayOfMonthWindow resolves a bare day-of-month N to its most recent occurrence:
// day N of the current month if that day is today or in the past, otherwise day
// N of the previous month. Returns ok=false when N is invalid for the resolved
// month (e.g. day 31 of a 30-day month).
func dayOfMonthWindow(d int, now time.Time, loc *time.Location) (time.Time, time.Time, bool) {
	today := midnight(now, loc)
	candidate := time.Date(now.Year(), now.Month(), d, 0, 0, 0, 0, loc)
	if candidate.Day() != d {
		// Day N doesn't exist in this month — back off one month.
		candidate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).
			AddDate(0, -1, 0)
		candidate = time.Date(candidate.Year(), candidate.Month(), d, 0, 0, 0, 0, loc)
		if candidate.Day() != d {
			return time.Time{}, time.Time{}, false
		}
		return dayWindow(candidate)
	}
	if candidate.After(today) {
		// Future this month → previous month's day N.
		prev := candidate.AddDate(0, -1, 0)
		if prev.Day() != d {
			return time.Time{}, time.Time{}, false
		}
		return dayWindow(prev)
	}
	return dayWindow(candidate)
}

// dayMonthWindow resolves a DD/MM (no year) cue to its most recent occurrence:
// that day in the current year if today-or-past, otherwise the previous year.
func dayMonthWindow(dayStr, monthStr string, now time.Time) (time.Time, time.Time, bool) {
	loc := now.Location()
	d, _ := strconv.Atoi(dayStr)
	mo, _ := strconv.Atoi(monthStr)
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return time.Time{}, time.Time{}, false
	}
	today := midnight(now, loc)
	candidate := time.Date(now.Year(), time.Month(mo), d, 0, 0, 0, 0, loc)
	if candidate.Day() != d || candidate.Month() != time.Month(mo) {
		return time.Time{}, time.Time{}, false // invalid calendar date
	}
	if candidate.After(today) {
		candidate = candidate.AddDate(-1, 0, 0)
	}
	return dayWindow(candidate)
}

// validYMD reports whether y/mo/d forms a real calendar date.
func validYMD(y, mo, d int) bool {
	if y < 1 || mo < 1 || mo > 12 || d < 1 || d > 31 {
		return false
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	return t.Year() == y && t.Month() == time.Month(mo) && t.Day() == d
}

// normalizeTemporalQuery lowercases the query and drops the "última"/"ultima"
// qualifier ("última sexta" → "sexta") and the "na"/"no" preposition so the
// weekday and phrase matchers see a clean token stream. Accents are preserved;
// the weekdayCue map carries both accented and stripped forms.
func normalizeTemporalQuery(query string) string {
	q := strings.ToLower(query)
	q = strings.ReplaceAll(q, "última", "")
	q = strings.ReplaceAll(q, "ultima", "")
	q = strings.ReplaceAll(q, "último", "")
	q = strings.ReplaceAll(q, "ultimo", "")
	return q
}

// hasWord reports whether q contains word as a standalone token (word-boundary
// matched), so "ontem" does not match inside "anteontem" etc.
// The regexp for word is looked up from the precompiled cache (compile-on-miss).
func hasWord(q, word string) bool {
	return compiledWordRe(word).MatchString(q)
}

// hasPhraseReCache caches the compiled multi-word phrase regexps separately
// from single-token word regexps (different pattern structure).
var (
	hasPhraseReCacheMu sync.RWMutex
	hasPhraseReCache   = map[string]*regexp.Regexp{}
)

func compiledPhraseRe(phrase string) *regexp.Regexp {
	hasPhraseReCacheMu.RLock()
	re, ok := hasPhraseReCache[phrase]
	hasPhraseReCacheMu.RUnlock()
	if ok {
		return re
	}
	parts := strings.Fields(phrase)
	for i := range parts {
		parts[i] = regexp.QuoteMeta(parts[i])
	}
	compiled := regexp.MustCompile(`\b` + strings.Join(parts, `\s+`) + `\b`)
	hasPhraseReCacheMu.Lock()
	hasPhraseReCache[phrase] = compiled
	hasPhraseReCacheMu.Unlock()
	return compiled
}

// hasPhrase reports whether q contains the (possibly multi-word) phrase,
// tolerating arbitrary internal whitespace.
// The joined regexp is looked up from the precompiled cache (compile-on-miss).
func hasPhrase(q, phrase string) bool {
	return compiledPhraseRe(phrase).MatchString(q)
}

// matchWeekday returns the time.Weekday for the first weekday cue found in q.
// Longer tokens ("segunda-feira") are tried before their short forms so the
// "-feira" suffix is consumed by the right entry. Returns found=false when no
// weekday token appears.
func matchWeekday(q string) (time.Weekday, bool) {
	// Prefer the "-feira" / full forms first to avoid a short token matching a
	// substring of a longer one.
	ordered := []string{
		"segunda-feira", "terça-feira", "terca-feira", "quarta-feira",
		"quinta-feira", "sexta-feira",
		"segunda", "terça", "terca", "quarta", "quinta", "sexta",
		"sábado", "sabado", "domingo",
		"monday", "tuesday", "wednesday", "thursday", "friday",
		"saturday", "sunday",
	}
	for _, tok := range ordered {
		if hasWord(q, tok) {
			return weekdayCue[tok], true
		}
	}
	return time.Weekday(0), false
}
