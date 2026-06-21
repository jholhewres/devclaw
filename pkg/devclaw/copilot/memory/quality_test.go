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
