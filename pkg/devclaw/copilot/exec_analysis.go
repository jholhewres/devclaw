// Package copilot – exec_analysis.go implements command risk analysis
// for bash/exec tool calls. Commands are categorized by risk level and
// appropriate actions are taken (allow, log, require approval, deny).
package copilot

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// RiskLevel represents the risk category of a command.
type RiskLevel string

const (
	RiskSafe      RiskLevel = "safe"       // Execute immediately
	RiskModerate  RiskLevel = "moderate"   // Log and execute
	RiskDangerous RiskLevel = "dangerous"  // Require approval
	RiskBlocked   RiskLevel = "blocked"    // Always deny
)

// RiskAction represents the action to take for a risk level.
type RiskAction string

const (
	ActionAllow          RiskAction = "allow"
	ActionAllowLog       RiskAction = "allow_log"
	ActionRequireApproval RiskAction = "require_approval"
	ActionDeny           RiskAction = "deny"
)

// RiskCategoryConfig configures a risk category.
type RiskCategoryConfig struct {
	// Patterns are glob-like patterns to match commands.
	Patterns []string `yaml:"patterns"`

	// Action is the action to take for this category.
	Action RiskAction `yaml:"action"`

	// Notify lists who to notify (e.g., ["owners"], ["admins"]).
	Notify []string `yaml:"notify"`

	// Message is a custom denial/approval message.
	Message string `yaml:"message"`
}

// TrustConfig configures trust levels per user role.
type TrustConfig struct {
	// Owner is the maximum risk level the owner can execute without approval.
	Owner RiskLevel `yaml:"owner"`

	// Admin is the maximum risk level admins can execute without approval.
	Admin RiskLevel `yaml:"admin"`

	// User is the maximum risk level regular users can execute without approval.
	User RiskLevel `yaml:"user"`
}

// ExecAnalysisConfig configures the exec analysis system.
type ExecAnalysisConfig struct {
	// Enabled turns the analysis on/off.
	Enabled bool `yaml:"enabled"`

	// Categories configures each risk category.
	Categories map[RiskLevel]RiskCategoryConfig `yaml:"categories"`

	// SafeBins lists binary paths that are always safe.
	SafeBins []string `yaml:"safe_bins"`

	// Trust configures per-role trust levels.
	Trust TrustConfig `yaml:"trust"`

	// SuspiciousPatterns are regex patterns for suspicious constructs.
	SuspiciousPatterns []string `yaml:"suspicious_patterns"`

	// DefaultAction is the action for commands that don't match any category.
	DefaultAction RiskAction `yaml:"default_action"`
}

// ExecAnalyzer analyzes commands for risk.
type ExecAnalyzer struct {
	config             ExecAnalysisConfig
	suspiciousRegexes  []*regexp.Regexp
	categoryRegexes    map[RiskLevel][]*regexp.Regexp
	safeBins           map[string]bool
	logger             *slog.Logger
}

// ExecAnalysisResult contains the analysis result for a command.
type ExecAnalysisResult struct {
	// Risk is the determined risk level.
	Risk RiskLevel

	// Action is the action to take.
	Action RiskAction

	// Reason explains why this risk level was assigned.
	Reason string

	// MatchedPattern is the pattern that matched (if any).
	MatchedPattern string

	// IsSuspicious indicates if suspicious constructs were found.
	IsSuspicious bool

	// SuspiciousMatches lists the suspicious patterns found.
	SuspiciousMatches []string
}

// NewExecAnalyzer creates a new command analyzer.
func NewExecAnalyzer(cfg ExecAnalysisConfig, logger *slog.Logger) *ExecAnalyzer {
	if logger == nil {
		logger = slog.Default()
	}

	// Compile suspicious patterns.
	suspiciousRegexes := make([]*regexp.Regexp, 0, len(cfg.SuspiciousPatterns))
	for _, p := range cfg.SuspiciousPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			logger.Warn("invalid suspicious pattern", "pattern", p, "error", err)
			continue
		}
		suspiciousRegexes = append(suspiciousRegexes, re)
	}

	// Compile category patterns.
	categoryRegexes := make(map[RiskLevel][]*regexp.Regexp)
	for level, cat := range cfg.Categories {
		regexes := make([]*regexp.Regexp, 0, len(cat.Patterns))
		for _, p := range cat.Patterns {
			re, err := patternToRegex(p)
			if err != nil {
				logger.Warn("invalid category pattern", "level", level, "pattern", p, "error", err)
				continue
			}
			regexes = append(regexes, re)
		}
		categoryRegexes[level] = regexes
	}

	// Index safe bins.
	safeBins := make(map[string]bool)
	for _, bin := range cfg.SafeBins {
		safeBins[bin] = true
	}

	logger.Info("exec analyzer initialized",
		"enabled", cfg.Enabled,
		"categories", len(cfg.Categories),
		"suspicious_patterns", len(suspiciousRegexes),
		"safe_bins", len(safeBins))

	return &ExecAnalyzer{
		config:            cfg,
		suspiciousRegexes: suspiciousRegexes,
		categoryRegexes:   categoryRegexes,
		safeBins:          safeBins,
		logger:            logger.With("component", "exec_analyzer"),
	}
}

// Analyze analyzes a command and returns the risk assessment.
func (a *ExecAnalyzer) Analyze(command string) ExecAnalysisResult {
	if !a.config.Enabled {
		return ExecAnalysisResult{
			Risk:   RiskSafe,
			Action: ActionAllow,
			Reason: "analysis disabled",
		}
	}

	// Check for suspicious patterns first.
	suspiciousMatches := a.checkSuspicious(command)

	// Check each risk category in order of severity.
	// Check blocked first, then dangerous, moderate, safe.
	for _, level := range []RiskLevel{RiskBlocked, RiskDangerous, RiskModerate, RiskSafe} {
		if matched, pattern := a.matchesCategory(command, level); matched {
			action := a.getActionForLevel(level)
			if len(suspiciousMatches) > 0 && action == ActionAllow {
				// Escalate if suspicious patterns found.
				action = ActionAllowLog
			}
			return ExecAnalysisResult{
				Risk:              level,
				Action:            action,
				Reason:            fmt.Sprintf("matched category %s", level),
				MatchedPattern:    pattern,
				IsSuspicious:      len(suspiciousMatches) > 0,
				SuspiciousMatches: suspiciousMatches,
			}
		}
	}

	// Check if using a safe binary.
	if a.isSafeBin(command) {
		return ExecAnalysisResult{
			Risk:         RiskSafe,
			Action:       ActionAllow,
			Reason:       "uses safe binary",
			IsSuspicious: len(suspiciousMatches) > 0,
			SuspiciousMatches: suspiciousMatches,
		}
	}

	// Use default action.
	defaultAction := a.config.DefaultAction
	if defaultAction == "" {
		defaultAction = ActionAllowLog
	}

	return ExecAnalysisResult{
		Risk:         RiskModerate,
		Action:       defaultAction,
		Reason:       "no category matched, using default",
		IsSuspicious: len(suspiciousMatches) > 0,
		SuspiciousMatches: suspiciousMatches,
	}
}

// AnalyzeForRole analyzes a command considering the user's role.
func (a *ExecAnalyzer) AnalyzeForRole(command string, role string) ExecAnalysisResult {
	result := a.Analyze(command)

	// If already blocked, don't escalate.
	if result.Action == ActionDeny {
		return result
	}

	// Check trust level for role.
	maxRisk := a.getMaxRiskForRole(role)
	if maxRisk == "" {
		return result
	}

	// If user's trust level doesn't cover this risk, require approval.
	if !a.riskAllowed(result.Risk, maxRisk) && result.Action != ActionRequireApproval {
		result.Action = ActionRequireApproval
		result.Reason = fmt.Sprintf("risk level %s exceeds trust for role %s", result.Risk, role)
	}

	return result
}

// checkSuspicious checks for suspicious patterns in the command.
func (a *ExecAnalyzer) checkSuspicious(command string) []string {
	var matches []string
	for _, re := range a.suspiciousRegexes {
		if re.MatchString(command) {
			matches = append(matches, re.String())
		}
	}
	return matches
}

// matchesCategory checks if the command matches a risk category.
func (a *ExecAnalyzer) matchesCategory(command string, level RiskLevel) (bool, string) {
	regexes := a.categoryRegexes[level]
	for _, re := range regexes {
		if re.MatchString(command) {
			return true, re.String()
		}
	}
	return false, ""
}

// isSafeBin checks if the command uses a safe binary.
func (a *ExecAnalyzer) isSafeBin(command string) bool {
	// Extract the first word (binary name or path).
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return false
	}

	bin := parts[0]

	// Check if it's an absolute path to a safe bin.
	if a.safeBins[bin] {
		return true
	}

	// Check common safe commands by name.
	safeCommands := []string{"ls", "cat", "echo", "grep", "find", "head", "tail", "wc", "sort", "uniq", "cut", "awk", "sed", "tr", "date", "pwd", "whoami", "id", "uname"}
	for _, safe := range safeCommands {
		if bin == safe || strings.HasSuffix(bin, "/"+safe) {
			return true
		}
	}

	return false
}

// getActionForLevel returns the action for a risk level.
func (a *ExecAnalyzer) getActionForLevel(level RiskLevel) RiskAction {
	if cat, ok := a.config.Categories[level]; ok && cat.Action != "" {
		return cat.Action
	}

	// Default actions per level.
	switch level {
	case RiskSafe:
		return ActionAllow
	case RiskModerate:
		return ActionAllowLog
	case RiskDangerous:
		return ActionRequireApproval
	case RiskBlocked:
		return ActionDeny
	default:
		return ActionAllowLog
	}
}

// getMaxRiskForRole returns the maximum risk level allowed for a role.
func (a *ExecAnalyzer) getMaxRiskForRole(role string) RiskLevel {
	switch strings.ToLower(role) {
	case "owner":
		if a.config.Trust.Owner != "" {
			return a.config.Trust.Owner
		}
		return RiskDangerous
	case "admin":
		if a.config.Trust.Admin != "" {
			return a.config.Trust.Admin
		}
		return RiskModerate
	case "user":
		if a.config.Trust.User != "" {
			return a.config.Trust.User
		}
		return RiskSafe
	default:
		return RiskSafe
	}
}

// riskAllowed checks if a risk level is allowed within the max risk.
func (a *ExecAnalyzer) riskAllowed(risk, maxRisk RiskLevel) bool {
	riskOrder := map[RiskLevel]int{
		RiskSafe:      0,
		RiskModerate:  1,
		RiskDangerous: 2,
		RiskBlocked:   3,
	}

	return riskOrder[risk] <= riskOrder[maxRisk]
}

// patternToRegex converts a glob-like pattern to a regex.
func patternToRegex(pattern string) (*regexp.Regexp, error) {
	// Simple glob conversion: * becomes .*
	// Escape regex special chars except *
	escaped := regexp.QuoteMeta(pattern)
	// Convert escaped * back to .*
	regex := "^" + strings.ReplaceAll(escaped, `\*`, `.*`) + "$"
	return regexp.Compile(regex)
}

// DefaultExecAnalysisConfig returns sensible defaults.
func DefaultExecAnalysisConfig() ExecAnalysisConfig {
	return ExecAnalysisConfig{
		Enabled: true,
		Categories: map[RiskLevel]RiskCategoryConfig{
			RiskSafe: {
				Patterns: []string{"ls*", "cat *", "echo *", "grep *", "head *", "tail *", "wc *", "pwd", "whoami", "id", "date*"},
				Action:   ActionAllow,
			},
			RiskModerate: {
				Patterns: []string{"npm *", "yarn *", "git *", "docker *", "go *", "python *", "node *", "make *"},
				Action:   ActionAllowLog,
			},
			RiskDangerous: {
				Patterns: []string{"rm *", "sudo *", "chmod *", "chown *", "mv /*", "cp /*"},
				Action:   ActionRequireApproval,
			},
			RiskBlocked: {
				Patterns: []string{"mkfs*", "dd if=*", "shutdown*", "reboot*", "halt*", "init 0", "init 6", "> /dev/sd*", "mv /dev/*"},
				Action:   ActionDeny,
				Message:  "This command is blocked for safety reasons",
			},
		},
		SafeBins: []string{
			"/usr/bin/ls",
			"/usr/bin/cat",
			"/usr/bin/grep",
			"/usr/bin/find",
			"/usr/bin/head",
			"/usr/bin/tail",
		},
		Trust: TrustConfig{
			Owner: RiskDangerous,
			Admin: RiskModerate,
			User:  RiskSafe,
		},
		SuspiciousPatterns: []string{
			`\$\([^)]*\)`,       // Command substitution $(...)
			"`[^`]*`",           // Backtick execution
			`\|\s*(sh|bash)`,    // Pipe to shell
			`&&\s*(rm|sudo)`,    // Chained dangerous commands
			`;\s*(rm|sudo)`,     // Semicolon dangerous commands
			`>\s*/etc/`,         // Writing to /etc
			`>\s*/dev/`,         // Writing to /dev
		},
		DefaultAction: ActionAllowLog,
	}
}
