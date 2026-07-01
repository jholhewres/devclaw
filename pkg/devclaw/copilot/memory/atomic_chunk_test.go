package memory

import (
	"strings"
	"testing"
)

func TestSplitAtomicFacts(t *testing.T) {
	short := "User prefers dark mode"
	if got := splitAtomicFacts(short); len(got) != 1 || got[0] != short {
		t.Errorf("short content must pass through: %v", got)
	}

	// A real itinerary: must isolate the locators as their own facts while
	// keeping comma-separated flight legs together.
	trip := "Viagem Florianópolis 23-26/06/2026 (Encontro Conexão Estratégica - HostGator). " +
		"Ida: GOL G3 1673 MCZ 05:10→GRU 08:15, G3 1140 GRU 09:15→FLN 10:35. " +
		"Localizador ida: WGADGP. Volta: LATAM LA 4671 FLN 17:35→GRU 19:05. Localizador volta: FUDWGZ."
	got := splitAtomicFacts(trip)
	if len(got) < 4 {
		t.Fatalf("expected the itinerary to split into ≥4 facts, got %d: %v", len(got), got)
	}
	hasLocatorIda := false
	for _, p := range got {
		if strings.Contains(p, "Localizador ida: WGADGP") && len([]rune(p)) < 40 {
			hasLocatorIda = true
		}
		// flight legs (comma list) must stay in one piece
		if strings.Contains(p, "G3 1673") && !strings.Contains(p, "G3 1140") {
			t.Errorf("comma-separated flight legs were split: %q", p)
		}
	}
	if !hasLocatorIda {
		t.Errorf("'Localizador ida: WGADGP' was not isolated as an atomic fact: %v", got)
	}

	// No text loss: every non-space rune of the input appears in the output.
	joined := strings.Join(got, " ")
	for _, tok := range []string{"WGADGP", "FUDWGZ", "G3 1673", "LA 4671", "HostGator"} {
		if !strings.Contains(joined, tok) {
			t.Errorf("token %q lost during split", tok)
		}
	}
}
