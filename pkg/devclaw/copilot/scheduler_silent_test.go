package copilot

import "testing"

func TestIsSilentScheduledOutput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", true},
		{"whitespace only", "   \n\t ", true},
		{"normal text", "Reminder: drink water", false},
		{"schedule silent marker", "SCHEDULE_SILENT", true},
		{"schedule silent with trailing text", "SCHEDULE_SILENT reason: nothing to say", true},
		{"schedule silent leading whitespace", "  SCHEDULE_SILENT", true},
		{"partial marker does not count", "scheduled_silent", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isSilentScheduledOutput(c.in); got != c.want {
				t.Errorf("isSilentScheduledOutput(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
