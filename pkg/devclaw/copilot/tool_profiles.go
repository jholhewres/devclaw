// Package copilot – tool_profiles.go implements predefined tool permission profiles.
// Profiles simplify tool configuration by providing presets for common use cases.
package copilot

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ToolProfile defines a preset of allowed and denied tools.
type ToolProfile struct {
	// Name is the profile identifier (e.g., "minimal", "coding", "full").
	Name string `yaml:"name"`

	// Description explains what this profile is for.
	Description string `yaml:"description"`

	// Allow lists tools and groups that are permitted.
	// Supports: tool names, "group:name", wildcards like "git_*"
	// Empty means no allow list (use permission levels).
	Allow []string `yaml:"allow"`

	// Deny lists tools and groups that are always blocked.
	// Takes precedence over Allow.
	Deny []string `yaml:"deny"`
}

// BuiltInProfiles provides predefined tool profiles for common use cases.
//
// Design rules:
//   - Always use "group:xxx" references instead of individual tool names when
//     the intent is to include all tools in a category. This ensures new tools
//     added to a group are automatically picked up by profiles.
//   - Individual tool names are used only for tools that don't belong to any
//     group (e.g. "apply_patch") or when only a subset of a group is needed.
//   - Dispatcher tools (memory, vault, scheduler, browser) internally call
//     hidden sub-tools via executeByName which bypasses the profile guard.
//     Using the group (e.g. "group:vault") instead of just "vault" ensures
//     the sub-tools are also in the allow set for direct Execute() calls.
var BuiltInProfiles = map[string]ToolProfile{
	"minimal": {
		Name:        "minimal",
		Description: "Basic queries only - read-only access, no writes",
		Allow: []string{
			"group:web",    // web_search, web_fetch
			"group:memory", // memory dispatcher + sub-tools (save, search, list, index)
			"read_file",    // read-only filesystem
			"list_files",
			"search_files",
			"glob_files",
			"describe_image", // analyze images
		},
		Deny: []string{
			"group:runtime",   // bash, exec, ssh, scp, set_env
			"write_file",      // no writes
			"edit_file",       // no edits
			"group:skills",    // no skill management
			"group:scheduler", // no scheduling
			"group:vault",     // no secrets
			"group:subagents", // no subagents
			"group:daemon",    // no daemons
			"group:browser",   // no browser
			"group:teams",     // no teams
		},
	},
	"coding": {
		Name:        "coding",
		Description: "Software development - file access, skills, vault, scheduler, browser",
		Allow: []string{
			"group:fs",        // read_file, write_file, edit_file, list_files, search_files, glob_files
			"group:web",       // web_search, web_fetch
			"group:memory",    // memory dispatcher + sub-tools
			"group:scheduler", // scheduler dispatcher + sub-tools
			"group:vault",     // vault dispatcher + sub-tools
			"group:skills",    // get_skill_instructions, get_skill_reference, skill_list, etc.
			"group:sessions",  // sessions
			"group:subagents", // spawn, list, wait, stop subagents
			"group:daemon",    // daemon manager
			"group:media",     // describe_image, transcribe_audio, send_media
			"group:browser",   // browser dispatcher + sub-tools
			"group:skill_db",  // skill_db_query, skill_db_list_tables, etc.
			"bash",            // shell access
			"exec",            // sandboxed execution
			"apply_patch",     // multi-file patches
			"read_file",       // read files
			"write_file",      // write files
			"edit_file",       // edit files
			"list_files",      // list files
			"search_files",    // search files
			"glob_files",      // glob files
		},
		Deny: []string{
			"ssh", // no remote access
			"scp",
		},
	},
	"messaging": {
		Name:        "messaging",
		Description: "Chat channels (WhatsApp, Discord, Telegram, Slack) - nearly full access except ssh/daemon",
		Allow: []string{
			"group:web",       // web_search, web_fetch
			"group:memory",    // memory dispatcher + sub-tools
			"group:scheduler", // scheduler dispatcher + sub-tools
			"group:vault",     // vault dispatcher + sub-tools (save/get API keys)
			"group:skills",    // get_skill_instructions, get_skill_reference, skill_list, etc.
			"group:sessions",  // sessions
			"group:media",     // describe_image, transcribe_audio, send_media
			"group:skill_db",  // skill_db_query, skill_db_list_tables, etc.
			"group:fs",        // read_file, write_file, edit_file, list_files, search_files, glob_files
			"group:subagents", // spawn, list, wait, stop subagents
			"group:browser",   // browser automation for skills
			"group:teams",     // team_manage, team_agent, team_task, team_memory, team_comm
			"group:daemon",    // daemon manager
			"bash",            // shell access (curl, jq, etc. for API skills)
			"exec",            // sandboxed execution
			"apply_patch",     // multi-file patches
			"read_file",       // read files
			"write_file",      // write files
			"edit_file",       // edit files
			"list_files",      // list files
			"search_files",    // search files
			"glob_files",      // glob files
		},
		Deny: []string{
			"ssh",     // no remote access
			"scp",     // no remote copy
			"set_env", // no env modification
		},
	},
	"team": {
		Name:        "team",
		Description: "Team agent - team tools, web, memory, scheduler, vault, skills, full FS, bash",
		Allow: []string{
			"group:teams",     // team_manage, team_agent, team_task, team_memory, team_comm
			"group:web",       // web_search, web_fetch
			"group:memory",    // memory dispatcher + sub-tools
			"group:scheduler", // scheduler dispatcher + sub-tools
			"group:vault",     // vault dispatcher + sub-tools
			"group:skills",    // get_skill_instructions, get_skill_reference, skill_list, etc.
			"group:media",     // describe_image, transcribe_audio, send_media
			"group:skill_db",  // skill_db_query, skill_db_list_tables, etc.
			"group:fs",        // full filesystem access
			"group:sessions",  // sessions
			"group:browser",   // browser automation
			"bash",            // shell access (for team task execution)
			"exec",            // sandboxed execution
			"apply_patch",     // multi-file patches
		},
		Deny: []string{
			"group:subagents", // no subagents (team agents are agents themselves)
			"group:daemon",    // no daemon management
			"ssh",             // no remote access
			"scp",
		},
	},
	"full": {
		Name:        "full",
		Description: "Full access - all tools available (respect per-tool permissions)",
		Allow:       []string{"*"},
		Deny:        []string{},
	},
}

// InferProfileForChannel returns the default tool profile name for a channel.
// Messaging channels (WhatsApp, Discord, Telegram, Slack) get "messaging"
// to avoid exposing filesystem/runtime tools. WebUI and CLI get "full".
func InferProfileForChannel(channel string) string {
	switch strings.ToLower(channel) {
	case "whatsapp", "discord", "telegram", "slack":
		return "messaging"
	case "webui", "cli":
		return "full"
	default:
		return "full"
	}
}

// ExtendProfileWithSkills creates a copy of the profile that also allows
// tools from the given active skills. Skill tools follow the naming pattern
// "skillname_toolname", so we add "skillname_*" wildcards to the allow list.
// If the profile already allows all tools ("*"), returns it unchanged.
func ExtendProfileWithSkills(base *ToolProfile, activeSkills []string) *ToolProfile {
	if base == nil || len(activeSkills) == 0 {
		return base
	}

	// If allow list already contains "*", no need to extend.
	for _, a := range base.Allow {
		if a == "*" {
			return base
		}
	}

	extended := ToolProfile{
		Name:        base.Name,
		Description: base.Description,
		Allow:       make([]string, len(base.Allow), len(base.Allow)+len(activeSkills)),
		Deny:        append([]string(nil), base.Deny...),
	}
	copy(extended.Allow, base.Allow)

	for _, skill := range activeSkills {
		sanitized := sanitizeToolName(skill)
		extended.Allow = append(extended.Allow, sanitized+"_*")
	}

	return &extended
}

// ResolveProfile returns the allow and deny lists for a profile.
// Checks built-in profiles first, then custom profiles.
// Returns nil lists if profile not found.
func ResolveProfile(name string, customProfiles map[string]ToolProfile) (allow, deny []string) {
	// Check built-in profiles first
	if profile, ok := BuiltInProfiles[name]; ok {
		return profile.Allow, profile.Deny
	}

	// Check custom profiles
	if customProfiles != nil {
		if profile, ok := customProfiles[name]; ok {
			return profile.Allow, profile.Deny
		}
	}

	return nil, nil
}

// GetProfile returns a profile by name (built-in or custom).
func GetProfile(name string, customProfiles map[string]ToolProfile) *ToolProfile {
	if profile, ok := BuiltInProfiles[name]; ok {
		return &profile
	}
	if customProfiles != nil {
		if profile, ok := customProfiles[name]; ok {
			profile := profile // copy
			return &profile
		}
	}
	return nil
}

// ListProfiles returns all available profile names.
func ListProfiles(customProfiles map[string]ToolProfile) []string {
	names := make([]string, 0, len(BuiltInProfiles)+len(customProfiles))

	// Add built-in profiles
	for name := range BuiltInProfiles {
		names = append(names, name)
	}

	// Add custom profiles
	for name := range customProfiles {
		names = append(names, name)
	}

	return names
}

// ExpandProfileList expands a profile's allow/deny lists into tool names.
// Handles groups ("group:name") and wildcards ("git_*").
func ExpandProfileList(items []string, allTools []string) []string {
	var result []string

	for _, item := range items {
		// Wildcard pattern
		if strings.HasSuffix(item, "*") {
			prefix := strings.TrimSuffix(item, "*")
			for _, tool := range allTools {
				if strings.HasPrefix(tool, prefix) {
					result = append(result, tool)
				}
			}
			continue
		}

		// Group reference
		if strings.HasPrefix(item, "group:") {
			if tools, ok := ToolGroups[item]; ok {
				result = append(result, tools...)
			}
			continue
		}

		// Special case: "*" means all tools
		if item == "*" {
			result = append(result, allTools...)
			continue
		}

		// Direct tool name
		result = append(result, item)
	}

	return result
}

// ProfileChecker checks if tools are allowed/denied by a profile.
type ProfileChecker struct {
	allowSet map[string]bool
	denySet  map[string]bool
}

// NewProfileChecker creates a checker from allow/deny lists.
func NewProfileChecker(allow, deny []string, allTools []string) *ProfileChecker {
	pc := &ProfileChecker{
		allowSet: make(map[string]bool),
		denySet:  make(map[string]bool),
	}

	// Expand and populate deny set (deny takes precedence)
	expandedDeny := ExpandProfileList(deny, allTools)
	for _, tool := range expandedDeny {
		pc.denySet[tool] = true
	}

	// Expand and populate allow set
	expandedAllow := ExpandProfileList(allow, allTools)
	for _, tool := range expandedAllow {
		pc.allowSet[tool] = true
	}

	return pc
}

// IsDenied returns true if the tool is in the deny list.
func (pc *ProfileChecker) IsDenied(toolName string) bool {
	return pc.denySet[toolName]
}

// IsAllowed returns true if the tool is in the allow list.
// If allow list is empty, all tools are allowed (respecting deny).
func (pc *ProfileChecker) IsAllowed(toolName string) bool {
	// Empty allow list = all allowed
	if len(pc.allowSet) == 0 {
		return true
	}
	return pc.allowSet[toolName]
}

// Check returns whether a tool is permitted by the profile.
// Returns (allowed, reason) where reason explains why if not allowed.
func (pc *ProfileChecker) Check(toolName string) (allowed bool, reason string) {
	// Check deny first (takes precedence)
	if pc.IsDenied(toolName) {
		return false, "denied by profile"
	}

	// Check allow
	if !pc.IsAllowed(toolName) {
		return false, "not in profile allow list"
	}

	return true, ""
}

// MatchesPattern checks if a tool name matches a pattern.
// Supports glob-style wildcards: "git_*" matches "git_status", "git_commit", etc.
func MatchesPattern(toolName, pattern string) bool {
	// Exact match
	if pattern == toolName {
		return true
	}

	// Wildcard suffix
	if prefix, found := strings.CutSuffix(pattern, "*"); found {
		if strings.HasPrefix(toolName, prefix) {
			return true
		}
	}

	// Glob pattern (simple implementation)
	matched, err := filepath.Match(pattern, toolName)
	if err != nil {
		return false
	}
	return matched
}

// ---------- Tool Categorization for Prompt ----------

// InferToolCategory determines the category of a tool from its name.
// Used for grouping tools in the system prompt and list_capabilities output.
func InferToolCategory(name string) string {
	switch {
	// Skills management (must precede Filesystem — skill_edit would match "edit",
	// skill_test would match "test", skill_db_* would match "read"/"write")
	case strings.HasPrefix(name, "skill_") || name == "skill_manage" ||
		strings.HasSuffix(name, "_skill"):
		return "Skills"

	// Filesystem operations
	case strings.Contains(name, "read") ||
		strings.Contains(name, "write") ||
		strings.Contains(name, "edit") ||
		strings.Contains(name, "list_files") ||
		strings.Contains(name, "glob") ||
		strings.Contains(name, "search_files"):
		return "Filesystem"

	// Shell/execution
	case name == "bash" ||
		name == "exec" ||
		name == "ssh" ||
		name == "scp" ||
		name == "set_env":
		return "Execution"

	// Web operations
	case strings.Contains(name, "web_") ||
		strings.Contains(name, "fetch"):
		return "Web"

	// Memory/knowledge
	case strings.Contains(name, "memory"):
		return "Memory"

	// Scheduling
	case strings.HasPrefix(name, "scheduler_") || name == "scheduler" || strings.HasPrefix(name, "cron_"):
		return "Scheduling"

	// Vault/secrets
	case name == "vault" || strings.HasPrefix(name, "vault_"):
		return "Vault"

	// Sessions/agents
	case name == "sessions" || strings.HasPrefix(name, "sessions_") ||
		strings.Contains(name, "subagent"):
		return "Agents"

	// Git/version control
	case strings.Contains(name, "git_") ||
		name == "git":
		return "Git"

	// Docker/containers
	case strings.Contains(name, "docker") ||
		strings.Contains(name, "kubectl") ||
		strings.Contains(name, "kubernetes"):
		return "Containers"

	// Cloud/infrastructure
	case strings.Contains(name, "aws_") ||
		strings.Contains(name, "gcloud_") ||
		strings.Contains(name, "azure_") ||
		strings.Contains(name, "terraform"):
		return "Cloud"

	// Development tools
	case strings.Contains(name, "claude-code") ||
		strings.Contains(name, "test") ||
		strings.Contains(name, "debug"):
		return "Development"

	// Team tools
	case strings.HasPrefix(name, "team_"):
		return "Team"

	// Daemon management
	case name == "daemon":
		return "Daemon"

	// Media
	case name == "send_media" ||
		strings.Contains(name, "image") ||
		strings.Contains(name, "audio") ||
		strings.Contains(name, "video") ||
		strings.Contains(name, "transcribe"):
		return "Media"

	// Capabilities
	case name == "list_capabilities":
		return "Capabilities"

	default:
		return "Other"
	}
}

// CategorizeTools groups tool definitions by category for display purposes.
func CategorizeTools(tools []ToolDefinition) map[string][]ToolDefinition {
	categories := make(map[string][]ToolDefinition)
	for _, tool := range tools {
		cat := InferToolCategory(tool.Function.Name)
		categories[cat] = append(categories[cat], tool)
	}
	return categories
}

// CategorizeToolNames groups tool names by category.
func CategorizeToolNames(names []string) map[string][]string {
	categories := make(map[string][]string)
	for _, name := range names {
		cat := InferToolCategory(name)
		categories[cat] = append(categories[cat], name)
	}
	return categories
}

// FormatToolsForPrompt formats tools as a compact list for the system prompt.
// Groups by category and truncates descriptions to fit within budget.
func FormatToolsForPrompt(tools []ToolDefinition, maxDescLen int) string {
	categories := CategorizeTools(tools)

	// Sort categories for consistent output
	var cats []string
	for cat := range categories {
		cats = append(cats, cat)
	}
	// Sort alphabetically
	for i := 0; i < len(cats); i++ {
		for j := i + 1; j < len(cats); j++ {
			if cats[j] < cats[i] {
				cats[i], cats[j] = cats[j], cats[i]
			}
		}
	}

	var b strings.Builder
	for _, cat := range cats {
		b.WriteString(fmt.Sprintf("\n### %s\n", cat))
		for _, tool := range categories[cat] {
			desc := tool.Function.Description
			if len(desc) > maxDescLen {
				desc = desc[:maxDescLen-3] + "..."
			}
			// Clean up description (remove newlines)
			desc = strings.ReplaceAll(desc, "\n", " ")
			b.WriteString(fmt.Sprintf("- %s: %s\n", tool.Function.Name, desc))
		}
	}

	return b.String()
}

// FormatToolNamesForPrompt formats tool names as a compact list for the system prompt.
// Groups by category for better readability.
func FormatToolNamesForPrompt(names []string) string {
	categories := CategorizeToolNames(names)

	// Sort categories
	var cats []string
	for cat := range categories {
		cats = append(cats, cat)
	}
	for i := 0; i < len(cats); i++ {
		for j := i + 1; j < len(cats); j++ {
			if cats[j] < cats[i] {
				cats[i], cats[j] = cats[j], cats[i]
			}
		}
	}

	var b strings.Builder
	for _, cat := range cats {
		b.WriteString(fmt.Sprintf("\n### %s\n", cat))
		for _, name := range categories[cat] {
			b.WriteString(fmt.Sprintf("- %s\n", name))
		}
	}

	return b.String()
}
