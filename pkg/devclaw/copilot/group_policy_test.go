package copilot

import (
	"log/slog"
	"os"
	"testing"
)

func TestNewGroupPolicyManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name           string
		cfg            GroupsPolicyConfig
		wantGroups     int
		wantBlocked    int
		wantDefault    GroupPolicy
	}{
		{
			name: "empty config",
			cfg:  GroupsPolicyConfig{},
			wantGroups:  0,
			wantBlocked: 0,
			wantDefault: GroupPolicy(""),
		},
		{
			name: "single group with open policy",
			cfg: GroupsPolicyConfig{
				DefaultPolicy: GroupPolicyOpen,
				Groups: []GroupPolicyConfig{
					{ID: "120363xxx@g.us", Name: "Test Group", Policy: GroupPolicyOpen},
				},
			},
			wantGroups:  1,
			wantBlocked: 0,
			wantDefault: GroupPolicyOpen,
		},
		{
			name: "multiple groups with blocked list",
			cfg: GroupsPolicyConfig{
				DefaultPolicy: GroupPolicyOpen,
				Groups: []GroupPolicyConfig{
					{ID: "120363aaa@g.us", Name: "Group A", Policy: GroupPolicyOpen},
					{ID: "120363bbb@g.us", Name: "Group B", Policy: GroupPolicyAllowlist, AllowedUsers: []string{"5511999999999@s.whatsapp.net"}},
				},
				Blocked: []string{"120363bad@g.us"},
			},
			wantGroups:  2,
			wantBlocked: 1,
			wantDefault: GroupPolicyOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewGroupPolicyManager(tt.cfg, logger)

			if got := len(m.groups); got != tt.wantGroups {
				t.Errorf("groups count = %d, want %d", got, tt.wantGroups)
			}
			if got := len(m.blocked); got != tt.wantBlocked {
				t.Errorf("blocked count = %d, want %d", got, tt.wantBlocked)
			}
			if m.defaultMode != tt.wantDefault {
				t.Errorf("default mode = %q, want %q", m.defaultMode, tt.wantDefault)
			}
		})
	}
}

func TestGroupPolicyManager_ShouldRespond(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := GroupsPolicyConfig{
		DefaultPolicy: GroupPolicyOpen,
		Groups: []GroupPolicyConfig{
			{
				ID:         "120363open@g.us",
				Policy:     GroupPolicyOpen,
				Activation: ActivationAlways,
			},
			{
				ID:         "120363mention@g.us",
				Policy:     GroupPolicyOpen,
				Activation: ActivationMention,
			},
			{
				ID:         "120363keyword@g.us",
				Policy:     GroupPolicyOpen,
				Activation: ActivationKeyword,
				Keywords:   []string{"help", "urgent"},
			},
			{
				ID:           "120363allowlist@g.us",
				Policy:       GroupPolicyAllowlist,
				AllowedUsers: []string{"551199999999@s.whatsapp.net"},
				Activation:   ActivationAlways,
			},
			{
				ID:         "120363disabled@g.us",
				Policy:     GroupPolicyDisabled,
				Activation: ActivationAlways,
			},
		},
		Blocked: []string{"120363blocked@g.us"},
	}

	m := NewGroupPolicyManager(cfg, logger)

	tests := []struct {
		name        string
		groupJID    string
		userJID     string
		content     string
		isReply     bool
		trigger     string
		wantRespond bool
	}{
		// Blocked group
		{"blocked group", "120363blocked@g.us", "551199999999@s.whatsapp.net", "hello", false, "", false},

		// Disabled group
		{"disabled group", "120363disabled@g.us", "551199999999@s.whatsapp.net", "hello", false, "", false},

		// Open + Always activation
		{"open+always no trigger", "120363open@g.us", "551199999999@s.whatsapp.net", "hello", false, "", true},

		// Open + Mention activation
		{"mention mode without trigger", "120363mention@g.us", "551199999999@s.whatsapp.net", "hello", false, "", false},
		{"mention mode with trigger", "120363mention@g.us", "551199999999@s.whatsapp.net", "@bot hello", false, "@bot", true},

		// Keyword activation
		{"keyword mode no keyword no trigger", "120363keyword@g.us", "551199999999@s.whatsapp.net", "hello", false, "", false},
		{"keyword mode with keyword", "120363keyword@g.us", "551199999999@s.whatsapp.net", "I need help", false, "", true},
		{"keyword mode with trigger", "120363keyword@g.us", "551199999999@s.whatsapp.net", "hello", false, "@bot", true},

		// Allowlist policy
		{"allowlist allowed user", "120363allowlist@g.us", "551199999999@s.whatsapp.net", "hello", false, "", true},
		{"allowlist blocked user", "120363allowlist@g.us", "551188888888@s.whatsapp.net", "hello", false, "", false},

		// Unknown group uses default policy
		{"unknown group default open", "120363unknown@g.us", "551199999999@s.whatsapp.net", "@bot hello", false, "@bot", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.ShouldRespond(tt.groupJID, tt.userJID, tt.content, tt.isReply, tt.trigger)
			if got != tt.wantRespond {
				t.Errorf("ShouldRespond() = %v, want %v", got, tt.wantRespond)
			}
		})
	}
}

func TestGroupPolicyManager_IsBlocked(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := GroupsPolicyConfig{
		Blocked: []string{"120363blocked@g.us", "120363spam@g.us"},
	}

	m := NewGroupPolicyManager(cfg, logger)

	if !m.IsBlocked("120363blocked@g.us") {
		t.Error("expected group to be blocked")
	}
	if !m.IsBlocked("120363spam@g.us") {
		t.Error("expected group to be blocked")
	}
	if m.IsBlocked("120363ok@g.us") {
		t.Error("expected group to NOT be blocked")
	}
}

func TestGroupPolicyManager_GetWorkspace(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := GroupsPolicyConfig{
		Groups: []GroupPolicyConfig{
			{ID: "120363ws@g.us", Workspace: "team-alpha"},
		},
	}

	m := NewGroupPolicyManager(cfg, logger)

	if got := m.GetWorkspace("120363ws@g.us"); got != "team-alpha" {
		t.Errorf("GetWorkspace() = %q, want %q", got, "team-alpha")
	}
	if got := m.GetWorkspace("120363unknown@g.us"); got != "" {
		t.Errorf("GetWorkspace() for unknown group = %q, want empty", got)
	}
}

func TestGroupPolicyManager_ListGroups(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := GroupsPolicyConfig{
		Groups: []GroupPolicyConfig{
			{ID: "zebra@g.us"},
			{ID: "alpha@g.us"},
			{ID: "middle@g.us"},
		},
	}

	m := NewGroupPolicyManager(cfg, logger)

	list := m.ListGroups()
	if len(list) != 3 {
		t.Errorf("expected 3 groups, got %d", len(list))
	}

	// Should be sorted.
	expected := []string{"alpha@g.us", "middle@g.us", "zebra@g.us"}
	for i, v := range expected {
		if list[i] != v {
			t.Errorf("list[%d] = %q, want %q", i, list[i], v)
		}
	}
}

func TestGroupPolicyManager_ListBlocked(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := GroupsPolicyConfig{
		Blocked: []string{"c@g.us", "a@g.us", "b@g.us"},
	}

	m := NewGroupPolicyManager(cfg, logger)

	list := m.ListBlocked()
	if len(list) != 3 {
		t.Errorf("expected 3 blocked, got %d", len(list))
	}

	// Should be sorted.
	expected := []string{"a@g.us", "b@g.us", "c@g.us"}
	for i, v := range expected {
		if list[i] != v {
			t.Errorf("list[%d] = %q, want %q", i, list[i], v)
		}
	}
}

func TestParseTimeMinutes(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"00:00", 0},
		{"00:01", 1},
		{"01:00", 60},
		{"12:00", 720},
		{"23:59", 1439},
		{"invalid", -1},
		{"25:00", -1},   // hour out of range
		{"12:60", -1},   // minute out of range
		{"", -1},
		{"12", -1},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseTimeMinutes(tt.input)
			if got != tt.want {
				t.Errorf("parseTimeMinutes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestGroupPolicyManager_QuietHours(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Note: Testing quiet hours is tricky because it depends on the current time.
	// We test the parsing and configuration logic here.
	cfg := GroupsPolicyConfig{
		Groups: []GroupPolicyConfig{
			{
				ID:     "quiet-group@g.us",
				Policy: GroupPolicyOpen,
				QuietHours: &QuietHoursConfig{
					Start:    "22:00",
					End:      "08:00",
					Timezone: "UTC",
				},
			},
			{
				ID:     "invalid-quiet-group@g.us",
				Policy: GroupPolicyOpen,
				QuietHours: &QuietHoursConfig{
					Start: "invalid",
					End:   "invalid",
				},
			},
		},
	}

	m := NewGroupPolicyManager(cfg, logger)

	// Get config returns the right config
	groupCfg := m.GetGroupConfig("quiet-group@g.us")
	if groupCfg == nil {
		t.Fatal("expected config, got nil")
	}
	if groupCfg.QuietHours == nil {
		t.Fatal("expected quiet hours config, got nil")
	}
	if groupCfg.QuietHours.Start != "22:00" {
		t.Errorf("quiet hours start = %q, want %q", groupCfg.QuietHours.Start, "22:00")
	}

	// Invalid quiet hours should return false (not in quiet hours due to parse error)
	invalidCfg := m.GetGroupConfig("invalid-quiet-group@g.us")
	if m.IsQuietHours(invalidCfg) {
		t.Error("invalid quiet hours should not be active")
	}
}
