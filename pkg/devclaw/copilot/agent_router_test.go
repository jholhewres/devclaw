package copilot

import (
	"log/slog"
	"os"
	"testing"
)

func TestNewAgentRouter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name          string
		cfg           AgentsConfig
		wantProfiles  int
		wantChannels  int
		wantUsers     int
		wantGroups    int
		wantDefault   string
	}{
		{
			name: "empty config",
			cfg:  AgentsConfig{},
			wantProfiles:  0,
			wantChannels:  0,
			wantUsers:     0,
			wantGroups:    0,
			wantDefault:   "",
		},
		{
			name: "single profile with channel routing",
			cfg: AgentsConfig{
				Profiles: []AgentProfileConfig{
					{
						ID:       "coding",
						Model:    "claude-sonnet-4",
						Channels: []string{"discord", "telegram"},
					},
				},
				Routing: RoutingConfig{
					Default: "coding",
				},
			},
			wantProfiles:  1,
			wantChannels:  2,
			wantUsers:     0,
			wantGroups:    0,
			wantDefault:   "coding",
		},
		{
			name: "multiple profiles with mixed routing",
			cfg: AgentsConfig{
				Profiles: []AgentProfileConfig{
					{
						ID:       "support",
						Model:    "gpt-4o-mini",
						Channels: []string{"whatsapp"},
						Users:    []string{"5511999999999"},
					},
					{
						ID:      "devops",
						Model:   "claude-sonnet-4",
						Groups:  []string{"120363xxx@g.us"},
					},
				},
				Routing: RoutingConfig{
					Default: "support",
				},
			},
			wantProfiles:  2,
			wantChannels:  1,
			wantUsers:     1,
			wantGroups:    1,
			wantDefault:   "support",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewAgentRouter(tt.cfg, logger)

			if got := len(r.profiles); got != tt.wantProfiles {
				t.Errorf("profiles count = %d, want %d", got, tt.wantProfiles)
			}
			if got := len(r.byChannel); got != tt.wantChannels {
				t.Errorf("channels count = %d, want %d", got, tt.wantChannels)
			}
			if got := len(r.byUser); got != tt.wantUsers {
				t.Errorf("users count = %d, want %d", got, tt.wantUsers)
			}
			if got := len(r.byGroup); got != tt.wantGroups {
				t.Errorf("groups count = %d, want %d", got, tt.wantGroups)
			}
			if r.defaultID != tt.wantDefault {
				t.Errorf("default = %q, want %q", r.defaultID, tt.wantDefault)
			}
		})
	}
}

func TestAgentRouter_Route(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := AgentsConfig{
		Profiles: []AgentProfileConfig{
			{
				ID:       "support",
				Model:    "gpt-4o-mini",
				Channels: []string{"whatsapp"},
			},
			{
				ID:      "coding",
				Model:   "claude-sonnet-4",
				Channels: []string{"discord", "telegram"},
			},
			{
				ID:     "vip",
				Model:  "gpt-4o",
				Users:  []string{"5511999999999", "5511888888888"},
			},
			{
				ID:     "devgroup",
				Model:  "claude-sonnet-4",
				Groups: []string{"120363xxx@g.us"},
			},
		},
		Routing: RoutingConfig{
			Default: "support",
		},
	}

	r := NewAgentRouter(cfg, logger)

	tests := []struct {
		name        string
		channel     string
		userJID     string
		groupJID    string
		wantProfile string
	}{
		// Channel routing
		{"route by whatsapp channel", "whatsapp", "", "", "support"},
		{"route by discord channel", "discord", "", "", "coding"},
		{"route by telegram channel", "telegram", "", "", "coding"},

		// User routing (higher priority)
		{"user routing overrides channel", "discord", "5511999999999", "", "vip"},
		{"user routing with resource", "whatsapp", "5511888888888@s.whatsapp.net", "", "vip"},

		// Group routing
		{"group routing", "", "", "120363xxx@g.us", "devgroup"},
		{"user overrides group", "", "5511999999999", "120363xxx@g.us", "vip"},

		// Default fallback
		{"default fallback", "unknown", "unknown@user.com", "", "support"},
		{"no match uses default", "", "", "", "support"},

		// Case insensitive channel
		{"case insensitive channel", "WhatsApp", "", "", "support"},
		{"case insensitive channel 2", "DISCORD", "", "", "coding"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := r.Route(tt.channel, tt.userJID, tt.groupJID)

			if tt.wantProfile == "" {
				if profile != nil {
					t.Errorf("expected nil profile, got %q", profile.ID)
				}
				return
			}

			if profile == nil {
				t.Errorf("expected profile %q, got nil", tt.wantProfile)
				return
			}

			if profile.ID != tt.wantProfile {
				t.Errorf("profile = %q, want %q", profile.ID, tt.wantProfile)
			}
		})
	}
}

func TestAgentRouter_Route_Priority(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Set up profiles with overlapping routing to test priority.
	cfg := AgentsConfig{
		Profiles: []AgentProfileConfig{
			{
				ID:       "channel-agent",
				Model:    "model-a",
				Channels: []string{"test-channel"},
			},
			{
				ID:     "user-agent",
				Model:  "model-b",
				Users:  []string{"test-user"},
			},
			{
				ID:     "group-agent",
				Model:  "model-c",
				Groups: []string{"test-group"},
			},
		},
		Routing: RoutingConfig{
			Default: "channel-agent",
		},
	}

	r := NewAgentRouter(cfg, logger)

	// Test 1: User routing has highest priority.
	profile := r.Route("test-channel", "test-user", "test-group")
	if profile.ID != "user-agent" {
		t.Errorf("user should have highest priority, got %q", profile.ID)
	}

	// Test 2: Group routing has second priority.
	profile = r.Route("test-channel", "", "test-group")
	if profile.ID != "group-agent" {
		t.Errorf("group should have second priority, got %q", profile.ID)
	}

	// Test 3: Channel routing has lowest priority.
	profile = r.Route("test-channel", "", "")
	if profile.ID != "channel-agent" {
		t.Errorf("channel should have lowest priority, got %q", profile.ID)
	}
}

func TestAgentRouter_GetProfile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := AgentsConfig{
		Profiles: []AgentProfileConfig{
			{ID: "profile1", Model: "model-a"},
			{ID: "profile2", Model: "model-b"},
		},
	}

	r := NewAgentRouter(cfg, logger)

	// Existing profile.
	p := r.GetProfile("profile1")
	if p == nil {
		t.Error("expected profile1, got nil")
	} else if p.ID != "profile1" {
		t.Errorf("profile ID = %q, want profile1", p.ID)
	}

	// Non-existing profile.
	p = r.GetProfile("nonexistent")
	if p != nil {
		t.Errorf("expected nil for nonexistent profile, got %q", p.ID)
	}
}

func TestAgentRouter_ListProfiles(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := AgentsConfig{
		Profiles: []AgentProfileConfig{
			{ID: "zebra", Model: "model-a"},
			{ID: "alpha", Model: "model-b"},
			{ID: "middle", Model: "model-c"},
		},
	}

	r := NewAgentRouter(cfg, logger)

	list := r.ListProfiles()
	if len(list) != 3 {
		t.Errorf("expected 3 profiles, got %d", len(list))
	}

	// Should be sorted.
	expected := []string{"alpha", "middle", "zebra"}
	for i, v := range expected {
		if list[i] != v {
			t.Errorf("list[%d] = %q, want %q", i, list[i], v)
		}
	}
}

func TestAgentProfileConfig_MergeConfig(t *testing.T) {
	tests := []struct {
		name             string
		profile          AgentProfileConfig
		baseModel        string
		baseInstructions string
		baseSkills       []string
		wantModel        string
		wantInstructions string
		wantSkills       []string
	}{
		{
			name:             "empty profile uses base",
			profile:          AgentProfileConfig{ID: "test"},
			baseModel:        "gpt-4o",
			baseInstructions: "Base instructions",
			baseSkills:       []string{"skill1", "skill2"},
			wantModel:        "gpt-4o",
			wantInstructions: "Base instructions",
			wantSkills:       []string{"skill1", "skill2"},
		},
		{
			name: "profile overrides model",
			profile: AgentProfileConfig{
				ID:    "test",
				Model: "claude-sonnet-4",
			},
			baseModel:        "gpt-4o",
			baseInstructions: "Base instructions",
			baseSkills:       []string{"skill1"},
			wantModel:        "claude-sonnet-4",
			wantInstructions: "Base instructions",
			wantSkills:       []string{"skill1"},
		},
		{
			name: "profile overrides instructions",
			profile: AgentProfileConfig{
				ID:           "test",
				Instructions: "Custom agent instructions",
			},
			baseModel:        "gpt-4o",
			baseInstructions: "Base instructions",
			baseSkills:       []string{"skill1"},
			wantModel:        "gpt-4o",
			wantInstructions: "Custom agent instructions",
			wantSkills:       []string{"skill1"},
		},
		{
			name: "profile overrides skills",
			profile: AgentProfileConfig{
				ID:     "test",
				Skills: []string{"custom-skill"},
			},
			baseModel:        "gpt-4o",
			baseInstructions: "Base instructions",
			baseSkills:       []string{"skill1", "skill2"},
			wantModel:        "gpt-4o",
			wantInstructions: "Base instructions",
			wantSkills:       []string{"custom-skill"},
		},
		{
			name: "profile overrides all",
			profile: AgentProfileConfig{
				ID:           "test",
				Model:        "claude-sonnet-4",
				Instructions: "Custom instructions",
				Skills:       []string{"custom1", "custom2"},
			},
			baseModel:        "gpt-4o",
			baseInstructions: "Base instructions",
			baseSkills:       []string{"skill1"},
			wantModel:        "claude-sonnet-4",
			wantInstructions: "Custom instructions",
			wantSkills:       []string{"custom1", "custom2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, instructions, skills := tt.profile.MergeConfig(
				tt.baseModel, tt.baseInstructions, tt.baseSkills,
			)

			if model != tt.wantModel {
				t.Errorf("model = %q, want %q", model, tt.wantModel)
			}
			if instructions != tt.wantInstructions {
				t.Errorf("instructions = %q, want %q", instructions, tt.wantInstructions)
			}
			if !equalSlices(skills, tt.wantSkills) {
				t.Errorf("skills = %v, want %v", skills, tt.wantSkills)
			}
		})
	}
}

func TestNormalizeJID(t *testing.T) {
	// normalizeJID strips device suffixes (e.g., ":5") and whitespace.
	// It also normalizes Brazilian phone numbers (removes extra 9th digit).
	// It's defined in access.go and handles WhatsApp-specific normalization.
	tests := []struct {
		input    string
		expected string
	}{
		// Non-Brazilian JID unchanged
		{"12125551234@s.whatsapp.net", "12125551234@s.whatsapp.net"},
		// Device suffix stripped (non-Brazilian)
		{"12125551234:5@s.whatsapp.net", "12125551234@s.whatsapp.net"},
		// Brazilian number normalized (9th digit removed)
		{"5511999999999@s.whatsapp.net", "551199999999@s.whatsapp.net"},
		// Brazilian with device suffix
		{"5511999999999:5@s.whatsapp.net", "551199999999@s.whatsapp.net"},
		// Whitespace stripped
		{"  12125551234@s.whatsapp.net  ", "12125551234@s.whatsapp.net"},
		// Simple string unchanged
		{"simple", "simple"},
		// Empty string
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeJID(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeJID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Helper function to compare string slices.
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
