package updater

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    Version
		wantErr bool
	}{
		{"v1.2.3", Version{1, 2, 3, 0, "v1.2.3"}, false},
		{"1.2.3", Version{1, 2, 3, 0, "1.2.3"}, false},
		{"v0.0.1", Version{0, 0, 1, 0, "v0.0.1"}, false},
		{"v10.20.30", Version{10, 20, 30, 0, "v10.20.30"}, false},
		{"v1.2.3-beta", Version{1, 2, 3, 0, "v1.2.3-beta"}, false},
		{"  v1.0.0  ", Version{1, 0, 0, 0, "  v1.0.0  "}, false},
		// git-describe format: commits after tag
		{"v0.0.4-9", Version{0, 0, 4, 9, "v0.0.4-9"}, false},
		{"v0.0.4-8-g3f1d951", Version{0, 0, 4, 8, "v0.0.4-8-g3f1d951"}, false},
		{"v0.0.4-8-g3f1d951-dirty", Version{0, 0, 4, 8, "v0.0.4-8-g3f1d951-dirty"}, false},
		{"v1.0.0-15-gabcdef", Version{1, 0, 0, 15, "v1.0.0-15-gabcdef"}, false},
		{"invalid", Version{}, true},
		{"v1.2", Version{}, true},
		{"v1.a.3", Version{}, true},
		{"", Version{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Major != tt.want.Major || got.Minor != tt.want.Minor || got.Patch != tt.want.Patch || got.Pre != tt.want.Pre {
					t.Errorf("ParseVersion(%q) = %+v, want %+v", tt.input, got, tt.want)
				}
			}
		})
	}
}

func TestVersionIsNewerThan(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"v2.0.0", "v1.0.0", true},
		{"v1.1.0", "v1.0.0", true},
		{"v1.0.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0", "v2.0.0", false},
		{"v1.0.0", "v1.1.0", false},
		{"v1.0.0", "v1.0.1", false},
		{"v1.2.3", "v1.2.2", true},
		{"v1.2.3", "v1.2.4", false},
		{"v2.0.0", "v1.9.9", true},
		// git-describe: same tag, more commits = newer
		{"v0.0.4-9", "v0.0.4-8", true},
		{"v0.0.4-9", "v0.0.4-8-g3f1d951-dirty", true},
		{"v0.0.4-8", "v0.0.4-9", false},
		{"v0.0.4-8", "v0.0.4-8", false},
		// exact tag vs commits after tag
		{"v0.0.4-1", "v0.0.4", true},
		{"v0.0.4", "v0.0.4-1", false},
		// higher patch always wins over pre
		{"v0.0.5", "v0.0.4-99", true},
		{"v0.0.4-99", "v0.0.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			a, _ := ParseVersion(tt.a)
			b, _ := ParseVersion(tt.b)
			if got := a.IsNewerThan(b); got != tt.want {
				t.Errorf("%s.IsNewerThan(%s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	v := Version{Major: 1, Minor: 2, Patch: 3, Raw: "v1.2.3"}
	if s := v.String(); s != "v1.2.3" {
		t.Errorf("String() = %q, want %q", s, "v1.2.3")
	}

	v2 := Version{Major: 1, Minor: 0, Patch: 0}
	if s := v2.String(); s != "v1.0.0" {
		t.Errorf("String() = %q, want %q", s, "v1.0.0")
	}
}
