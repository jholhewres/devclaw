// Package memory — temporal_query_test.go is the table-driven proof for
// resolveTemporalWindow (US-003). All cases anchor on a fixed "now" of
// Sunday 2026-06-21 12:00 LOCAL so weekday and relative-day resolution is
// deterministic. Windows are asserted to be day-aligned and in now's location.
package memory

import (
	"testing"
	"time"
)

// fixedNow is Sunday, 2026-06-21 12:00 in time.Local. Using time.Local keeps
// the test consistent with occurred_at, which is itself a local instant.
func fixedNow() time.Time {
	return time.Date(2026, 6, 21, 12, 0, 0, 0, time.Local)
}

// localDay returns local midnight of the given Y-M-D.
func localDay(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

func TestResolveTemporalWindow(t *testing.T) {
	now := fixedNow()

	tests := []struct {
		name      string
		query     string
		wantOK    bool
		wantStart time.Time
		wantEnd   time.Time
	}{
		// Relative day cues.
		{"hoje", "o que rolou hoje", true, localDay(2026, 6, 21), localDay(2026, 6, 22)},
		{"today", "what happened today", true, localDay(2026, 6, 21), localDay(2026, 6, 22)},
		{"ontem", "o que falamos ontem", true, localDay(2026, 6, 20), localDay(2026, 6, 21)},
		{"yesterday", "the call yesterday", true, localDay(2026, 6, 20), localDay(2026, 6, 21)},
		{"anteontem", "anteontem combinamos", true, localDay(2026, 6, 19), localDay(2026, 6, 20)},

		// Weekday names — most recent PAST occurrence relative to Sunday 06-21.
		// Sunday today: sexta(Fri) = 06-19, quinta(Thu) = 06-18, etc.
		{"sexta", "o que rolou na sexta", true, localDay(2026, 6, 19), localDay(2026, 6, 20)},
		{"sexta-feira", "reunião de sexta-feira", true, localDay(2026, 6, 19), localDay(2026, 6, 20)},
		{"ultima sexta", "última sexta", true, localDay(2026, 6, 19), localDay(2026, 6, 20)},
		{"quinta", "na quinta", true, localDay(2026, 6, 18), localDay(2026, 6, 19)},
		{"quarta", "quarta-feira", true, localDay(2026, 6, 17), localDay(2026, 6, 18)},
		{"terca", "terça", true, localDay(2026, 6, 16), localDay(2026, 6, 17)},
		{"segunda", "segunda-feira passada", true, localDay(2026, 6, 15), localDay(2026, 6, 16)},
		{"sabado", "sábado", true, localDay(2026, 6, 20), localDay(2026, 6, 21)},
		{"domingo today", "domingo", true, localDay(2026, 6, 21), localDay(2026, 6, 22)},
		{"friday EN", "on friday", true, localDay(2026, 6, 19), localDay(2026, 6, 20)},
		{"monday EN", "monday", true, localDay(2026, 6, 15), localDay(2026, 6, 16)},

		// Week / month spans. Week is Monday..Sunday; the week containing
		// Sun 06-21 starts Mon 06-15.
		{"semana passada", "semana passada", true, localDay(2026, 6, 8), localDay(2026, 6, 15)},
		{"last week", "last week", true, localDay(2026, 6, 8), localDay(2026, 6, 15)},
		{"esta semana", "esta semana", true, localDay(2026, 6, 15), localDay(2026, 6, 22)},
		{"this week", "this week", true, localDay(2026, 6, 15), localDay(2026, 6, 22)},
		{"mes passado", "mês passado", true, localDay(2026, 5, 1), localDay(2026, 6, 1)},
		{"last month", "last month", true, localDay(2026, 5, 1), localDay(2026, 6, 1)},

		// Day-of-month and explicit dates. "dia 18" → 06-18 (past this month).
		{"dia 18", "o que aconteceu dia 18", true, localDay(2026, 6, 18), localDay(2026, 6, 19)},
		{"dia 25 future->prev month", "dia 25", true, localDay(2026, 5, 25), localDay(2026, 5, 26)},
		{"dia DD/MM", "dia 18/06", true, localDay(2026, 6, 18), localDay(2026, 6, 19)},
		{"DD/MM", "18/06", true, localDay(2026, 6, 18), localDay(2026, 6, 19)},
		{"DD/MM future->prev year", "25/12", true, localDay(2025, 12, 25), localDay(2025, 12, 26)},
		{"DD/MM/YYYY", "18/06/2026", true, localDay(2026, 6, 18), localDay(2026, 6, 19)},
		{"ISO date", "2026-06-18", true, localDay(2026, 6, 18), localDay(2026, 6, 19)},
		{"ISO date older year", "2025-01-03", true, localDay(2025, 1, 3), localDay(2025, 1, 4)},

		// NO-cue queries — must NOT fire so normal recall is unchanged.
		{"no-cue proposta", "proposta ISCB", false, time.Time{}, time.Time{}},
		{"no-cue config", "como configurar o gateway", false, time.Time{}, time.Time{}},
		{"no-cue empty", "", false, time.Time{}, time.Time{}},
		{"no-cue plain word", "memory recall design", false, time.Time{}, time.Time{}},
		{"no-cue number only", "porta 8080", false, time.Time{}, time.Time{}},
		{"no-cue substring ontem", "frontend", false, time.Time{}, time.Time{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end, ok := resolveTemporalWindow(tc.query, now)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (start=%v end=%v)", ok, tc.wantOK, start, end)
			}
			if !tc.wantOK {
				return
			}
			if !start.Equal(tc.wantStart) {
				t.Errorf("start = %v, want %v", start, tc.wantStart)
			}
			if !end.Equal(tc.wantEnd) {
				t.Errorf("end = %v, want %v", end, tc.wantEnd)
			}
			// Window must be in now's location and day-aligned [midnight, midnight).
			if start.Location() != now.Location() {
				t.Errorf("start location = %v, want %v", start.Location(), now.Location())
			}
			assertDayAligned(t, "start", start)
			assertDayAligned(t, "end", end)
			if !end.After(start) {
				t.Errorf("end %v must be after start %v", end, start)
			}
		})
	}
}

// assertDayAligned fails when t is not exactly local midnight (h=m=s=ns=0).
func assertDayAligned(t *testing.T, label string, ts time.Time) {
	t.Helper()
	if ts.Hour() != 0 || ts.Minute() != 0 || ts.Second() != 0 || ts.Nanosecond() != 0 {
		t.Errorf("%s %v is not day-aligned (must be local midnight)", label, ts)
	}
}

// TestResolveTemporalWindowExportedMatchesPure guards the thin exported wrapper.
func TestResolveTemporalWindowExportedMatchesPure(t *testing.T) {
	now := fixedNow()
	for _, q := range []string{"ontem", "na sexta", "dia 18", "proposta ISCB"} {
		s1, e1, ok1 := resolveTemporalWindow(q, now)
		s2, e2, ok2 := ResolveTemporalWindow(q, now)
		if ok1 != ok2 || !s1.Equal(s2) || !e1.Equal(e2) {
			t.Errorf("exported wrapper diverged for %q: pure(%v,%v,%v) exported(%v,%v,%v)",
				q, s1, e1, ok1, s2, e2, ok2)
		}
	}
}
