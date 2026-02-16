// Package copilot – group_chat.go implements enhanced group chat features
// Activation modes, intro messages, context injection,
// participant tracking, and quiet hours.
package copilot

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// GroupManager handles group-specific behavior and state.
type GroupManager struct {
	cfg GroupConfig

	// participants tracks recent participants per group (chatID -> names).
	participants map[string]*participantRing

	// introduced tracks groups where the intro message has been sent.
	introduced map[string]bool

	// compiled ignore patterns.
	ignorePatterns []*regexp.Regexp

	mu sync.RWMutex
}

// participantRing is a bounded ring buffer of recent participant names.
type participantRing struct {
	names []string
	max   int
}

func newParticipantRing(max int) *participantRing {
	if max <= 0 {
		max = 20
	}
	return &participantRing{names: make([]string, 0, max), max: max}
}

func (pr *participantRing) Add(name string) {
	// Remove if already present to move to end (most recent).
	for i, n := range pr.names {
		if n == name {
			pr.names = append(pr.names[:i], pr.names[i+1:]...)
			break
		}
	}
	pr.names = append(pr.names, name)
	if len(pr.names) > pr.max {
		pr.names = pr.names[len(pr.names)-pr.max:]
	}
}

func (pr *participantRing) List() []string {
	out := make([]string, len(pr.names))
	copy(out, pr.names)
	return out
}

// NewGroupManager creates a new group manager with the given config.
func NewGroupManager(cfg GroupConfig) *GroupManager {
	gm := &GroupManager{
		cfg:          cfg,
		participants: make(map[string]*participantRing),
		introduced:   make(map[string]bool),
	}

	// Compile ignore patterns.
	for _, pattern := range cfg.IgnorePatterns {
		if re, err := regexp.Compile(pattern); err == nil {
			gm.ignorePatterns = append(gm.ignorePatterns, re)
		}
	}

	return gm
}

// ShouldRespond determines if the bot should respond to a group message.
// Returns (respond bool, reason string).
func (gm *GroupManager) ShouldRespond(chatID, senderName, messageText, botName, trigger string) (bool, string) {
	gm.mu.RLock()
	cfg := gm.cfg
	gm.mu.RUnlock()

	// Check quiet hours.
	if cfg.QuietHours != "" && gm.isQuietHour(cfg.QuietHours) {
		return false, "quiet hours"
	}

	// Check ignore patterns.
	for _, re := range gm.ignorePatterns {
		if re.MatchString(messageText) {
			return false, "ignored pattern"
		}
	}

	// Check activation mode.
	mode := strings.ToLower(cfg.ActivationMode)
	if mode == "" {
		mode = "always"
	}

	switch mode {
	case "always":
		return true, ""
	case "mention":
		lowerMsg := strings.ToLower(messageText)
		lowerName := strings.ToLower(botName)
		lowerTrigger := strings.ToLower(trigger)
		if strings.Contains(lowerMsg, lowerName) {
			return true, "name mentioned"
		}
		if lowerTrigger != "" && strings.Contains(lowerMsg, lowerTrigger) {
			return true, "trigger mentioned"
		}
		return false, "not mentioned"
	case "reply":
		// Reply detection must be handled upstream (depends on channel-specific reply metadata).
		// Default to true; the caller should check for direct reply before calling.
		return true, ""
	default:
		return true, ""
	}
}

// TrackParticipant records a participant's activity in a group.
func (gm *GroupManager) TrackParticipant(chatID, name string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	ring, ok := gm.participants[chatID]
	if !ok {
		max := gm.cfg.MaxParticipants
		if max <= 0 {
			max = 20
		}
		ring = newParticipantRing(max)
		gm.participants[chatID] = ring
	}
	ring.Add(name)
}

// GetParticipants returns the recent participant names for a group.
func (gm *GroupManager) GetParticipants(chatID string) []string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	ring, ok := gm.participants[chatID]
	if !ok {
		return nil
	}
	return ring.List()
}

// GetIntroMessage returns the intro message for a group, or empty if already sent.
// Marks the group as introduced so the message is only sent once.
func (gm *GroupManager) GetIntroMessage(chatID, botName, trigger string) string {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.introduced[chatID] {
		return ""
	}
	if gm.cfg.IntroMessage == "" {
		return ""
	}

	gm.introduced[chatID] = true

	msg := gm.cfg.IntroMessage
	msg = strings.ReplaceAll(msg, "{{name}}", botName)
	msg = strings.ReplaceAll(msg, "{{trigger}}", trigger)
	return msg
}

// GetContextInjection returns any group-specific context for the given chatID.
func (gm *GroupManager) GetContextInjection(chatID string) string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	if ctx, ok := gm.cfg.ContextInjection[chatID]; ok {
		return ctx
	}
	return ""
}

// BuildGroupPromptContext builds a group-specific section for the system prompt.
func (gm *GroupManager) BuildGroupPromptContext(chatID, botName string) string {
	var b strings.Builder

	participants := gm.GetParticipants(chatID)
	if len(participants) > 0 {
		b.WriteString("\n## Group Participants (recent)\n")
		for i, name := range participants {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, name))
		}
		b.WriteString("\nWhen multiple people are talking, address them by name for clarity.\n")
	}

	ctx := gm.GetContextInjection(chatID)
	if ctx != "" {
		b.WriteString("\n## Group Context\n")
		b.WriteString(ctx)
		b.WriteString("\n")
	}

	return b.String()
}

// isQuietHour checks if the current time falls within quiet hours (format: "HH:MM-HH:MM").
func (gm *GroupManager) isQuietHour(spec string) bool {
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return false
	}

	startTime, err1 := time.Parse("15:04", strings.TrimSpace(parts[0]))
	endTime, err2 := time.Parse("15:04", strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return false
	}

	now := time.Now()
	nowMinutes := now.Hour()*60 + now.Minute()
	startMinutes := startTime.Hour()*60 + startTime.Minute()
	endMinutes := endTime.Hour()*60 + endTime.Minute()

	if startMinutes <= endMinutes {
		return nowMinutes >= startMinutes && nowMinutes < endMinutes
	}
	// Wraps midnight (e.g. 23:00-07:00).
	return nowMinutes >= startMinutes || nowMinutes < endMinutes
}
