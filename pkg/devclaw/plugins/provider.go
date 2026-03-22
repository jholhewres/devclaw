package plugins

import "context"

// PluginProvider is the unified interface that all plugin types must satisfy.
// YAML-based plugins use ManifestProvider (auto-generated from plugin.yaml).
// Native plugins can implement this directly for full lifecycle control.
type PluginProvider interface {
	// Identity.
	ID() string
	Metadata() PluginMetadata

	// Configuration schema for UI generation.
	ConfigSchema() *PluginConfigSchema

	// UI configuration for the settings panel (nil = no custom UI).
	UIConfig() *PluginUIConfig

	// Capabilities — what the plugin provides.
	Tools() []ToolDef
	Hooks() []HookDef
	Agents() []AgentDef
	Skills() []SkillDef
	Channels() []ChannelDef
	Services() []ServiceDef

	// Lifecycle.
	Init(ctx context.Context, config map[string]any) error
	Shutdown() error
}

// PluginMetadata holds identity and descriptive information about a plugin.
type PluginMetadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	License     string `json:"license,omitempty"`
}

// ── UI Configuration ──

// PluginUIConfig defines how the plugin's settings panel is rendered.
// Declared in plugin.yaml under the `ui` key.
type PluginUIConfig struct {
	// Icon is a Lucide icon name for the plugin card (e.g. "bot", "globe", "zap").
	Icon string `yaml:"icon" json:"icon,omitempty"`

	// Category groups the plugin in the UI (e.g. "communication", "productivity").
	Category string `yaml:"category" json:"category,omitempty"`

	// Color is a hex accent color for the plugin card (e.g. "#3B82F6").
	Color string `yaml:"color" json:"color,omitempty"`

	// Sections defines the configuration form layout.
	// Each section references config field keys by name.
	Sections []UISection `yaml:"sections" json:"sections,omitempty"`

	// Actions defines quick-action buttons shown in the plugin detail view.
	Actions []UIAction `yaml:"actions" json:"actions,omitempty"`
}

// UISection groups related config fields in the settings form.
type UISection struct {
	// Title is the section heading.
	Title string `yaml:"title" json:"title"`

	// Description is an optional subtitle.
	Description string `yaml:"description" json:"description,omitempty"`

	// Fields references config field keys (from config.fields[].key).
	Fields []string `yaml:"fields" json:"fields"`

	// Collapsible makes the section collapsible in the UI.
	Collapsible bool `yaml:"collapsible" json:"collapsible,omitempty"`
}

// UIAction defines a quick-action button in the plugin detail view.
type UIAction struct {
	// Label is the button text.
	Label string `yaml:"label" json:"label"`

	// Description is shown as a tooltip.
	Description string `yaml:"description" json:"description,omitempty"`

	// Tool is the namespaced tool name to invoke (e.g. "hello-world_greet").
	Tool string `yaml:"tool" json:"tool"`

	// Confirm shows a confirmation dialog before executing.
	Confirm bool `yaml:"confirm" json:"confirm,omitempty"`

	// Icon is a Lucide icon name for the button.
	Icon string `yaml:"icon" json:"icon,omitempty"`
}

// ── ManifestProvider ──

// ManifestProvider wraps a PluginManifest to satisfy the PluginProvider interface.
// This is the default provider for YAML-based plugins.
type ManifestProvider struct {
	manifest *PluginManifest
	dir      string
	initFn   func(ctx context.Context, config map[string]any) error
	shutFn   func() error
}

// NewManifestProvider creates a provider from a parsed manifest.
func NewManifestProvider(m *PluginManifest, dir string) *ManifestProvider {
	return &ManifestProvider{manifest: m, dir: dir}
}

func (p *ManifestProvider) ID() string { return p.manifest.ID }

func (p *ManifestProvider) Metadata() PluginMetadata {
	return PluginMetadata{
		Name:        p.manifest.Name,
		Version:     p.manifest.Version,
		Description: p.manifest.Description,
		Author:      p.manifest.Author,
		License:     p.manifest.License,
	}
}

func (p *ManifestProvider) ConfigSchema() *PluginConfigSchema { return p.manifest.Config }
func (p *ManifestProvider) UIConfig() *PluginUIConfig         { return p.manifest.UI }
func (p *ManifestProvider) Tools() []ToolDef                  { return p.manifest.Tools }
func (p *ManifestProvider) Hooks() []HookDef                  { return p.manifest.Hooks }
func (p *ManifestProvider) Agents() []AgentDef                { return p.manifest.Agents }
func (p *ManifestProvider) Skills() []SkillDef                { return p.manifest.Skills }
func (p *ManifestProvider) Channels() []ChannelDef            { return p.manifest.Channels }
func (p *ManifestProvider) Services() []ServiceDef            { return p.manifest.Services }

func (p *ManifestProvider) Init(ctx context.Context, config map[string]any) error {
	if p.initFn != nil {
		return p.initFn(ctx, config)
	}
	return nil
}

func (p *ManifestProvider) Shutdown() error {
	if p.shutFn != nil {
		return p.shutFn()
	}
	return nil
}

// SetInitFunc sets a custom init function (used for native plugins).
func (p *ManifestProvider) SetInitFunc(fn func(ctx context.Context, config map[string]any) error) {
	p.initFn = fn
}

// SetShutdownFunc sets a custom shutdown function (used for native plugins).
func (p *ManifestProvider) SetShutdownFunc(fn func() error) {
	p.shutFn = fn
}
