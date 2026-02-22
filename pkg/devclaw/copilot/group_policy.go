// Package copilot – group_policy.go implements group-specific policies
// including activation modes, quiet hours, and access control for groups.
package copilot

import (
	"log/slog"
	"slices"
	"strings"
	"time"
)

// GroupPolicy defines the access policy for a group.
type GroupPolicy string

const (
	// GroupPolicyOpen allows all group members to use the bot.
	GroupPolicyOpen GroupPolicy = "open"
	// GroupPolicyDisabled prevents the bot from responding in this group.
	GroupPolicyDisabled GroupPolicy = "disabled"
	// GroupPolicyAllowlist restricts access to allowed users only.
	GroupPolicyAllowlist GroupPolicy = "allowlist"
)

// ActivationMode defines how the bot activates in a group.
type ActivationMode string

const (
	// ActivationAlways responds to all messages.
	ActivationAlways ActivationMode = "always"
	// ActivationMention responds only when mentioned.
	ActivationMention ActivationMode = "mention"
	// ActivationReply responds only when replying to bot's messages.
	ActivationReply ActivationMode = "reply"
	// ActivationKeyword responds when keywords are detected.
	ActivationKeyword ActivationMode = "keyword"
)

// QuietHoursConfig defines quiet hours for a group or notification rule.
type QuietHoursConfig struct {
	// Enabled activates quiet hours.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Start is the start time in HH:MM format.
	Start string `json:"start" yaml:"start"`
	// End is the end time in HH:MM format.
	End string `json:"end" yaml:"end"`
	// Timezone is the timezone for quiet hours (default: UTC).
	Timezone string `json:"timezone" yaml:"timezone"`
	// Days are the days of week when quiet hours apply (0=Sunday, 6=Saturday).
	Days []int `json:"days,omitempty" yaml:"days,omitempty"`
}

// GroupPolicyConfig holds configuration for a specific group's policy.
type GroupPolicyConfig struct {
	// ID is the group JID.
	ID string `yaml:"id"`
	// Name is a human-readable name for the group.
	Name string `yaml:"name"`
	// Policy is the access policy for this group.
	Policy GroupPolicy `yaml:"policy"`
	// Activation is the activation mode.
	Activation ActivationMode `yaml:"activation"`
	// Keywords trigger the bot in keyword mode.
	Keywords []string `yaml:"keywords"`
	// Workspace is the workspace to use for this group.
	Workspace string `yaml:"workspace"`
	// QuietHours defines when the bot should be silent.
	QuietHours *QuietHoursConfig `yaml:"quiet_hours"`
	// MaxParticipants ignores messages in groups larger than this (0 = unlimited).
	MaxParticipants int `yaml:"max_participants"`
	// AllowedUsers is the list of allowed user JIDs for allowlist policy.
	AllowedUsers []string `yaml:"allowed_users"`
}

// GroupsPolicyConfig holds all group policy configuration.
type GroupsPolicyConfig struct {
	// DefaultPolicy is the policy for groups not explicitly configured.
	DefaultPolicy GroupPolicy `yaml:"default_policy"`
	// Groups is the list of group-specific configurations.
	Groups []GroupPolicyConfig `yaml:"groups"`
	// Blocked is the list of blocked group JIDs.
	Blocked []string `yaml:"blocked"`
}

// GroupPolicyManager manages group-specific policies.
type GroupPolicyManager struct {
	groups      map[string]*GroupPolicyConfig
	blocked     map[string]bool
	defaultMode GroupPolicy
	logger      *slog.Logger
}

// NewGroupPolicyManager creates a new group policy manager.
func NewGroupPolicyManager(cfg GroupsPolicyConfig, logger *slog.Logger) *GroupPolicyManager {
	m := &GroupPolicyManager{
		groups:      make(map[string]*GroupPolicyConfig),
		blocked:     make(map[string]bool),
		defaultMode: cfg.DefaultPolicy,
		logger:      logger,
	}

	// Index groups by ID.
	for i := range cfg.Groups {
		g := &cfg.Groups[i]
		m.groups[normalizeJID(g.ID)] = g

		// Normalize allowed users.
		normalizedUsers := make([]string, len(g.AllowedUsers))
		for i, u := range g.AllowedUsers {
			normalizedUsers[i] = normalizeJID(u)
		}
		g.AllowedUsers = normalizedUsers
	}

	// Index blocked groups.
	for _, id := range cfg.Blocked {
		m.blocked[normalizeJID(id)] = true
	}

	logger.Info("group policy manager initialized",
		"groups", len(m.groups),
		"blocked", len(m.blocked),
		"default_policy", m.defaultMode,
	)

	return m
}

// ShouldRespond determines if the bot should respond to a message in a group.
func (m *GroupPolicyManager) ShouldRespond(groupJID, userJID string, content string, isReplyToBot bool, trigger string) bool {
	groupJID = normalizeJID(groupJID)
	userJID = normalizeJID(userJID)

	// Check if group is blocked.
	if m.blocked[groupJID] {
		m.logger.Debug("group is blocked", "group", groupJID)
		return false
	}

	// Get group config (or use defaults).
	cfg := m.GetGroupConfig(groupJID)

	// Check policy.
	switch cfg.Policy {
	case GroupPolicyDisabled:
		return false
	case GroupPolicyAllowlist:
		if !slices.Contains(cfg.AllowedUsers, userJID) {
			m.logger.Debug("user not in allowlist", "group", groupJID, "user", userJID)
			return false
		}
	case GroupPolicyOpen:
		// Continue to check activation mode.
	}

	// Check quiet hours.
	if m.IsQuietHours(cfg) {
		m.logger.Debug("quiet hours active", "group", groupJID)
		return false
	}

	// Check activation mode.
	switch cfg.Activation {
	case ActivationAlways:
		return true
	case ActivationMention:
		return trigger != ""
	case ActivationReply:
		return isReplyToBot
	case ActivationKeyword:
		return m.matchesKeyword(cfg, content) || trigger != ""
	default:
		return trigger != ""
	}
}

// GetGroupConfig returns the configuration for a group.
// Returns a default config if the group is not explicitly configured.
func (m *GroupPolicyManager) GetGroupConfig(groupJID string) *GroupPolicyConfig {
	groupJID = normalizeJID(groupJID)

	if cfg, ok := m.groups[groupJID]; ok {
		return cfg
	}

	// Return default config.
	return &GroupPolicyConfig{
		ID:         groupJID,
		Policy:     m.defaultMode,
		Activation: ActivationMention, // Default: respond when mentioned
	}
}

// IsQuietHours checks if quiet hours are active for a group.
func (m *GroupPolicyManager) IsQuietHours(cfg *GroupPolicyConfig) bool {
	if cfg == nil || cfg.QuietHours == nil {
		return false
	}

	qh := cfg.QuietHours
	if qh.Start == "" || qh.End == "" {
		return false
	}

	// Parse timezone (default to UTC).
	tz := time.UTC
	if qh.Timezone != "" {
		var err error
		tz, err = time.LoadLocation(qh.Timezone)
		if err != nil {
			tz = time.UTC
		}
	}

	// Get current time in the group's timezone.
	now := time.Now().In(tz)
	currentMinutes := now.Hour()*60 + now.Minute()

	// Parse start and end times.
	startMinutes := parseTimeMinutes(qh.Start)
	endMinutes := parseTimeMinutes(qh.End)

	if startMinutes < 0 || endMinutes < 0 {
		return false
	}

	// Handle overnight quiet hours (e.g., 22:00 to 08:00).
	if startMinutes > endMinutes {
		// Overnight: active if current time is after start OR before end.
		return currentMinutes >= startMinutes || currentMinutes < endMinutes
	}

	// Same-day quiet hours.
	return currentMinutes >= startMinutes && currentMinutes < endMinutes
}

// GetWorkspace returns the workspace for a group, or empty string if not set.
func (m *GroupPolicyManager) GetWorkspace(groupJID string) string {
	cfg := m.GetGroupConfig(groupJID)
	if cfg != nil {
		return cfg.Workspace
	}
	return ""
}

// IsBlocked returns true if the group is blocked.
func (m *GroupPolicyManager) IsBlocked(groupJID string) bool {
	return m.blocked[normalizeJID(groupJID)]
}

// ListGroups returns all configured group IDs.
func (m *GroupPolicyManager) ListGroups() []string {
	ids := make([]string, 0, len(m.groups))
	for id := range m.groups {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// ListBlocked returns all blocked group IDs.
func (m *GroupPolicyManager) ListBlocked() []string {
	ids := make([]string, 0, len(m.blocked))
	for id := range m.blocked {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// matchesKeyword checks if the message content matches any keyword.
func (m *GroupPolicyManager) matchesKeyword(cfg *GroupPolicyConfig, content string) bool {
	if len(cfg.Keywords) == 0 {
		return false
	}

	contentLower := strings.ToLower(content)
	for _, kw := range cfg.Keywords {
		if strings.Contains(contentLower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// parseTimeMinutes parses a time string (HH:MM) into minutes since midnight.
// Returns -1 on error.
func parseTimeMinutes(timeStr string) int {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return -1
	}

	hours := 0
	minutes := 0
	if err := parseTimeString(parts[0], &hours, 0, 23); err != nil {
		return -1
	}
	if err := parseTimeString(parts[1], &minutes, 0, 59); err != nil {
		return -1
	}

	return hours*60 + minutes
}

// parseTimeString parses a time component string.
func parseTimeString(s string, result *int, min, max int) error {
	var val int
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		val = val*10 + int(c-'0')
	}
	if val < min || val > max {
		return ErrInvalidTime
	}
	*result = val
	return nil
}

// ErrInvalidTime is returned when a time string is invalid.
var ErrInvalidTime = errorString("invalid time format")

type errorString string

func (e errorString) Error() string { return string(e) }
