package plugins

import (
	"context"
	"slices"
	"time"
)

// PluginState represents the lifecycle state of a plugin instance.
type PluginState string

const (
	StateDiscovered PluginState = "discovered"
	StateLoaded     PluginState = "loaded"
	StateRegistered PluginState = "registered"
	StateStarted    PluginState = "started"
	StateStopped    PluginState = "stopped"
	StateError      PluginState = "error"
)

// PluginInstance holds the runtime state of a loaded plugin.
type PluginInstance struct {
	// Manifest is the parsed plugin.yaml.
	Manifest *PluginManifest

	// Provider is the unified interface for this plugin.
	Provider PluginProvider

	// Dir is the plugin directory path.
	Dir string

	// State is the current lifecycle state.
	State PluginState

	// Enabled indicates whether the plugin is enabled.
	Enabled bool

	// ErrorMsg holds the error message when State == StateError.
	ErrorMsg string

	// Config is the resolved configuration (after merging defaults, env, vault, overrides).
	Config map[string]any

	// RegisteredTools tracks the names of tools registered by this plugin.
	RegisteredTools []string

	// RegisteredHooks tracks the names of hooks registered by this plugin.
	RegisteredHooks []string

	// RegisteredAgents tracks the IDs of agents registered by this plugin.
	RegisteredAgents []string

	// RegisteredChannels tracks the names of channels registered by this plugin.
	RegisteredChannels []string

	// RegisteredSkills tracks the names of skills registered by this plugin.
	RegisteredSkills []string

	// nativeHandle holds the loaded .so plugin handle (nil if no native lib).
	nativeHandle *nativePlugin

	// serviceCtx and serviceCancel control running services.
	serviceCtx    context.Context
	serviceCancel context.CancelFunc

	// LoadedAt is when the plugin was loaded.
	LoadedAt time.Time

	// StartedAt is when the plugin services were started.
	StartedAt time.Time
}

// nativePlugin holds references to symbols loaded from a native .so.
type nativePlugin struct {
	plugin  Plugin // generic Plugin interface (optional)
	channel any    // channel implementation (optional, type-asserted at registration)
	raw     any    // raw *plugin.Plugin handle
}

// PluginInfo is a JSON-serializable summary of a plugin instance.
type PluginInfo struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	Description string      `json:"description"`
	Author      string      `json:"author"`
	State       PluginState `json:"state"`
	Enabled     bool        `json:"enabled"`
	ErrorMsg    string      `json:"error,omitempty"`
	Dir         string      `json:"dir"`

	Tools    []string `json:"tools,omitempty"`
	Hooks    []string `json:"hooks,omitempty"`
	Agents   []string `json:"agents,omitempty"`
	Channels []string `json:"channels,omitempty"`
	Skills   []string `json:"skills,omitempty"`

	// UI configuration for the settings panel.
	UI *PluginUIConfig `json:"ui,omitempty"`

	// ConfigSchema describes the plugin's configurable fields (for form generation).
	ConfigSchema *PluginConfigSchema `json:"config_schema,omitempty"`

	// ConfigValues holds the current resolved config values (secrets redacted).
	ConfigValues map[string]any `json:"config_values,omitempty"`

	LoadedAt  time.Time `json:"loaded_at"`
	StartedAt time.Time `json:"started_at,omitzero"`
}

// Info returns a JSON-serializable summary of the plugin instance.
func (pi *PluginInstance) Info() PluginInfo {
	info := PluginInfo{
		ID:        pi.Manifest.ID,
		Name:      pi.Manifest.Name,
		Version:   pi.Manifest.Version,
		Author:    pi.Manifest.Author,
		State:     pi.State,
		Enabled:   pi.Enabled,
		ErrorMsg:  pi.ErrorMsg,
		Dir:       pi.Dir,
		LoadedAt:  pi.LoadedAt,
		StartedAt: pi.StartedAt,
	}
	if pi.Manifest.Description != "" {
		info.Description = pi.Manifest.Description
	}
	info.Tools = slices.Clone(pi.RegisteredTools)
	info.Hooks = slices.Clone(pi.RegisteredHooks)
	info.Agents = slices.Clone(pi.RegisteredAgents)
	info.Channels = slices.Clone(pi.RegisteredChannels)
	info.Skills = slices.Clone(pi.RegisteredSkills)

	// Include UI and config for frontend form generation.
	if pi.Manifest.UI != nil {
		info.UI = pi.Manifest.UI
	}
	if pi.Manifest.Config != nil {
		info.ConfigSchema = pi.Manifest.Config
		info.ConfigValues = redactConfig(pi.Config, pi.Manifest.Config)
	}

	return info
}

// redactConfig returns a copy of config values with secret fields masked.
func redactConfig(values map[string]any, schema *PluginConfigSchema) map[string]any {
	if values == nil || schema == nil {
		return nil
	}
	secrets := make(map[string]bool)
	for _, f := range schema.Fields {
		if f.Type == "secret" {
			secrets[f.Key] = true
		}
	}
	redacted := make(map[string]any, len(values))
	for k, v := range values {
		if secrets[k] {
			if s, ok := v.(string); ok && len(s) > 0 {
				redacted[k] = "••••••••"
			} else {
				redacted[k] = ""
			}
		} else {
			redacted[k] = v
		}
	}
	return redacted
}
