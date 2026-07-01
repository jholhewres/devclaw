package memory

import "testing"

func TestScoreQuality_DecisionBeatsShortEvent(t *testing.T) {
	decision := "We decided to adopt SQLite WAL mode for the memory store because it allows concurrent readers while serializing writers, which fixes the lock contention we saw under load."
	event := "deploy ok"

	decScore := ScoreQuality(decision, "decision", true)
	evtScore := ScoreQuality(event, "event", false)

	if decScore <= evtScore {
		t.Fatalf("decision (%.3f) should score higher than short event (%.3f)", decScore, evtScore)
	}
	if decScore < LowSignalThreshold {
		t.Fatalf("a rich decision should clear the low-signal threshold; got %.3f", decScore)
	}
}

func TestScoreQuality_TestOutputBelowThreshold(t *testing.T) {
	testOutput := "ran the suite: 42 passed, 0 failed — go test ./... all green"
	score := ScoreQuality(testOutput, "fact", true)
	if score >= LowSignalThreshold {
		t.Fatalf("test-output chatter should fall below %.2f; got %.3f", LowSignalThreshold, score)
	}
}

func TestClassifyQuality_LowSignalRule(t *testing.T) {
	v := ClassifyQuality("rodando testando", "event", false, false)
	if v.CurationStatus != CurationStatusLowSignal {
		t.Fatalf("expected low_signal status, got %q (score %.3f)", v.CurationStatus, v.Score)
	}
	if v.CurationRule != CurationRuleQuality {
		t.Fatalf("expected rule %q, got %q", CurationRuleQuality, v.CurationRule)
	}
}

func TestClassifyQuality_PinnedNeverDemoted(t *testing.T) {
	v := ClassifyQuality("x", "event", false, true) // tiny + ephemeral but pinned
	if v.CurationStatus != "" {
		t.Fatalf("pinned content must not be demoted; got status %q", v.CurationStatus)
	}
}

// TestScoreQuality_PersonalFactsRecallable is the v1.22.1 recalibration
// guardrail: a genuine personal-assistant fact must clear LowSignalThreshold
// even when it is short and carries no scope, so recall does not hide it.
func TestScoreQuality_PersonalFactsRecallable(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{
			name: "iscb proposal fact (58 chars, no scope)",
			text: "As empresas envolvidas na proposta são ISCB e/ou Alta Forja.",
		},
		{
			name: "typical one-line personal fact (no scope)",
			text: "O cliente prefere reuniões pela manhã e pagamento via PIX em até 7 dias.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score := ScoreQuality(tc.text, "fact", false) // no scope
			if score < LowSignalThreshold {
				t.Fatalf("personal fact must stay recallable (>= %.2f); got %.3f", LowSignalThreshold, score)
			}
		})
	}
}

// TestScoreQuality_GenuineNoiseBelowThreshold confirms the recalibration still
// demotes real noise: test-output chatter and near-empty fragments.
func TestScoreQuality_GenuineNoiseBelowThreshold(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		category string
	}{
		{name: "test output line", text: "go test ./... 12 passed", category: "fact"},
		{name: "three-word fragment", text: "fix the bug", category: "fact"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score := ScoreQuality(tc.text, tc.category, false)
			if score >= LowSignalThreshold {
				t.Fatalf("genuine noise must fall below %.2f; got %.3f", LowSignalThreshold, score)
			}
		})
	}
}

// TestQualityScorerVersionIsCurrent guards against forgetting to bump the
// version (which drives boot-time recuration).
func TestQualityScorerVersionIsCurrent(t *testing.T) {
	if QualityScorerVersion != 4 {
		t.Fatalf("QualityScorerVersion expected 4 after v1.22.1 recalibration; got %d", QualityScorerVersion)
	}
}
