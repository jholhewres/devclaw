// Package plugins implements the unified YAML-first plugin system for DevClaw.
// Plugins extend the runtime with agents, tools, hooks, services, channels, and skills.
//
// A plugin is a directory containing a plugin.yaml manifest and optional
// supporting files (prompts, skills, native .so libraries).
package plugins

import (
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// PluginManifest is the top-level struct parsed from plugin.yaml.
type PluginManifest struct {
	// ID is the unique plugin identifier (used for namespacing).
	ID string `yaml:"id" json:"id"`

	// Name is the human-readable plugin name.
	Name string `yaml:"name" json:"name"`

	// Version is the plugin version (semver recommended).
	Version string `yaml:"version" json:"version"`

	// Description is a short description of the plugin.
	Description string `yaml:"description" json:"description"`

	// Author is the plugin author or organization.
	Author string `yaml:"author" json:"author"`

	// License is the plugin license (e.g. "MIT", "Apache-2.0").
	License string `yaml:"license" json:"license,omitempty"`

	// MinDevclaw is the minimum DevClaw version required.
	MinDevclaw string `yaml:"min_devclaw" json:"min_devclaw,omitempty"`

	// Requires specifies external requirements (binaries, env vars, OS).
	Requires *PluginRequirements `yaml:"requires" json:"requires,omitempty"`

	// Config defines the plugin's configuration schema.
	Config *PluginConfigSchema `yaml:"config" json:"config,omitempty"`

	// Agents defines plugin-provided agent definitions.
	Agents []AgentDef `yaml:"agents" json:"agents,omitempty"`

	// Tools defines plugin-provided tool definitions.
	Tools []ToolDef `yaml:"tools" json:"tools,omitempty"`

	// Hooks defines plugin-provided hook definitions.
	Hooks []HookDef `yaml:"hooks" json:"hooks,omitempty"`

	// Services defines plugin-provided HTTP service endpoints.
	Services []ServiceDef `yaml:"services" json:"services,omitempty"`

	// Channels defines plugin-provided channel implementations.
	Channels []ChannelDef `yaml:"channels" json:"channels,omitempty"`

	// Skills defines plugin-provided skill definitions.
	Skills []SkillDef `yaml:"skills" json:"skills,omitempty"`

	// UI defines the plugin's settings panel layout (optional).
	UI *PluginUIConfig `yaml:"ui" json:"ui,omitempty"`

	// NativeLib is the path to an optional native .so library (relative to plugin dir).
	NativeLib string `yaml:"native_lib" json:"native_lib,omitempty"`
}

// PluginRequirements specifies external requirements for a plugin.
type PluginRequirements struct {
	// Bins lists required binaries that must be in PATH (all required).
	Bins []string `yaml:"bins" json:"bins,omitempty"`

	// AnyBins lists binaries where at least one must be in PATH.
	AnyBins []string `yaml:"any_bins" json:"any_bins,omitempty"`

	// Env lists required environment variables.
	Env []string `yaml:"env" json:"env,omitempty"`

	// OS lists supported operating systems (e.g. "linux", "darwin").
	OS []string `yaml:"os" json:"os,omitempty"`
}

// IsEligible returns true if the current environment satisfies all requirements.
func (r *PluginRequirements) IsEligible() bool {
	if r == nil {
		return true
	}

	// Check OS.
	if len(r.OS) > 0 {
		if !slices.ContainsFunc(r.OS, func(osName string) bool {
			return strings.EqualFold(osName, runtime.GOOS)
		}) {
			return false
		}
	}

	// Check required binaries.
	for _, bin := range r.Bins {
		if !binExists(bin) {
			return false
		}
	}

	// Check any-of binaries.
	if len(r.AnyBins) > 0 {
		if !slices.ContainsFunc(r.AnyBins, binExists) {
			return false
		}
	}

	// Check environment variables.
	for _, envVar := range r.Env {
		if os.Getenv(envVar) == "" {
			return false
		}
	}

	return true
}

// binExists checks if a binary is available in PATH.
func binExists(name string) bool {
	for dir := range strings.SplitSeq(os.Getenv("PATH"), string(os.PathListSeparator)) {
		path := dir + string(os.PathSeparator) + name
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// PluginConfigSchema defines the configuration schema for a plugin.
type PluginConfigSchema struct {
	// Fields is the list of configuration fields.
	Fields []PluginConfigField `yaml:"fields" json:"fields,omitempty"`
}

// PluginConfigField defines a single configuration field.
type PluginConfigField struct {
	// Key is the configuration key (used in resolved config map).
	Key string `yaml:"key" json:"key"`

	// Name is the human-readable field name.
	Name string `yaml:"name" json:"name"`

	// Description explains the field's purpose.
	Description string `yaml:"description" json:"description,omitempty"`

	// Type is the field type ("string", "int", "bool", "secret").
	Type string `yaml:"type" json:"type"`

	// Required indicates whether the field must be provided.
	Required bool `yaml:"required" json:"required,omitempty"`

	// Default is the default value if not provided.
	Default any `yaml:"default" json:"default,omitempty"`

	// EnvVar is the environment variable to read the value from.
	EnvVar string `yaml:"env_var" json:"env_var,omitempty"`

	// VaultKey is the vault key to read the value from.
	VaultKey string `yaml:"vault_key" json:"vault_key,omitempty"`
}

// AgentDef defines a plugin-provided agent.
type AgentDef struct {
	// ID is the agent identifier (unique within the plugin).
	ID string `yaml:"id" json:"id"`

	// Name is the human-readable agent name.
	Name string `yaml:"name" json:"name"`

	// Description explains the agent's purpose.
	Description string `yaml:"description" json:"description,omitempty"`

	// Instructions is inline system prompt text or a path to a .md file
	// (relative to plugin directory).
	Instructions string `yaml:"instructions" json:"instructions"`

	// Model overrides the LLM model for this agent (empty = use plugin/core default).
	Model string `yaml:"model" json:"model,omitempty"`

	// Triggers are keywords that activate this agent (matched against message content).
	Triggers []string `yaml:"triggers" json:"triggers,omitempty"`

	// Tools defines the tool allow/deny profile for this agent.
	Tools AgentToolProfile `yaml:"tools" json:"tools,omitzero"`

	// MaxTurns limits the agent loop turns (0 = unlimited).
	MaxTurns int `yaml:"max_turns" json:"max_turns,omitempty"`

	// TimeoutSec is the max execution time in seconds (0 = default).
	TimeoutSec int `yaml:"timeout_sec" json:"timeout_sec,omitempty"`

	// SessionMode controls session isolation ("isolated" or "shared").
	SessionMode string `yaml:"session_mode" json:"session_mode,omitempty"`

	// Escalation configures when/how the agent escalates to the main agent.
	Escalation *EscalationConfig `yaml:"escalation" json:"escalation,omitempty"`

	// Channels limits which channels this agent can be triggered from.
	Channels []string `yaml:"channels" json:"channels,omitempty"`
}

// AgentToolProfile defines which tools an agent can access.
type AgentToolProfile struct {
	// Allow lists tools the agent can use (empty = all available).
	Allow []string `yaml:"allow" json:"allow,omitempty"`

	// Deny lists tools the agent cannot use.
	Deny []string `yaml:"deny" json:"deny,omitempty"`
}

// EscalationConfig configures agent-to-main escalation.
type EscalationConfig struct {
	// Enabled turns escalation on/off.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Keywords are phrases that trigger automatic escalation.
	Keywords []string `yaml:"keywords" json:"keywords,omitempty"`

	// MaxTurns triggers escalation after this many turns without resolution.
	MaxTurns int `yaml:"max_turns" json:"max_turns,omitempty"`

	// OnFailure controls behavior on escalation failure ("retry", "drop").
	OnFailure string `yaml:"on_failure" json:"on_failure,omitempty"`

	// ExplicitOnly when true, only escalates via explicit escalate_to_main tool call.
	ExplicitOnly bool `yaml:"explicit_only" json:"explicit_only,omitempty"`
}

// ToolDef defines a plugin-provided tool.
type ToolDef struct {
	// Name is the tool name (will be namespaced as pluginID_name).
	Name string `yaml:"name" json:"name"`

	// Description explains what the tool does.
	Description string `yaml:"description" json:"description"`

	// Parameters is the JSON Schema for the tool's parameters.
	Parameters map[string]any `yaml:"parameters" json:"parameters,omitempty"`

	// Permission is the required access level ("owner", "admin", "user").
	Permission string `yaml:"permission" json:"permission,omitempty"`

	// Hidden excludes the tool from the LLM tool schema.
	Hidden bool `yaml:"hidden" json:"hidden,omitempty"`

	// Handler is a Go symbol name for native tool handlers (requires NativeLib).
	Handler string `yaml:"handler" json:"handler,omitempty"`

	// Endpoint is the HTTP URL for HTTP-based tool handlers.
	Endpoint string `yaml:"endpoint" json:"endpoint,omitempty"`

	// Method is the HTTP method (GET, POST, etc.) for HTTP-based tools.
	Method string `yaml:"method" json:"method,omitempty"`

	// Headers are additional HTTP headers for HTTP-based tools.
	Headers map[string]string `yaml:"headers" json:"headers,omitempty"`

	// Script is inline bash script for script-based tool handlers.
	Script string `yaml:"script" json:"script,omitempty"`
}

// HookDef defines a plugin-provided hook.
type HookDef struct {
	// Name is the hook identifier.
	Name string `yaml:"name" json:"name"`

	// Description explains the hook's purpose.
	Description string `yaml:"description" json:"description,omitempty"`

	// Events lists the events this hook listens to.
	Events []string `yaml:"events" json:"events"`

	// Priority controls execution order (lower = earlier, default: 100).
	Priority int `yaml:"priority" json:"priority,omitempty"`

	// Handler is a Go symbol name for native hook handlers.
	Handler string `yaml:"handler" json:"handler,omitempty"`

	// Script is inline bash script for script-based hook handlers.
	Script string `yaml:"script" json:"script,omitempty"`
}

// ServiceDef defines a plugin-provided HTTP service endpoint.
type ServiceDef struct {
	// ID is the service identifier.
	ID string `yaml:"id" json:"id"`

	// Name is the human-readable service name.
	Name string `yaml:"name" json:"name"`

	// Description explains the service's purpose.
	Description string `yaml:"description" json:"description,omitempty"`

	// Handler is a Go symbol name for native service handlers.
	Handler string `yaml:"handler" json:"handler,omitempty"`

	// Path is the HTTP path prefix (e.g. "/webhooks/myplugin").
	Path string `yaml:"path" json:"path,omitempty"`

	// Method is the HTTP method (GET, POST, etc.).
	Method string `yaml:"method" json:"method,omitempty"`
}

// ChannelDef defines a plugin-provided channel implementation.
type ChannelDef struct {
	// Name is the channel name.
	Name string `yaml:"name" json:"name"`

	// Handler is a Go symbol name for the channel implementation.
	Handler string `yaml:"handler" json:"handler"`
}

// SkillDef defines a plugin-provided skill.
type SkillDef struct {
	// Name is the skill name.
	Name string `yaml:"name" json:"name"`

	// Description explains the skill's purpose.
	Description string `yaml:"description" json:"description,omitempty"`

	// SkillMD is the relative path to the SKILL.md file within the plugin directory.
	SkillMD string `yaml:"skill_md" json:"skill_md"`
}

// ParseManifest reads and parses a plugin.yaml file.
func ParseManifest(path string) (*PluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m PluginManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if m.ID == "" {
		return nil, fmt.Errorf("manifest missing required field: id")
	}
	if m.Name == "" {
		m.Name = m.ID
	}
	if m.Version == "" {
		m.Version = "0.0.0"
	}

	return &m, nil
}
