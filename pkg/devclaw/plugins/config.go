package plugins

import (
	"fmt"
	"os"
)

// VaultReader reads secrets from the encrypted vault.
// Defined here to avoid circular imports with the skills package.
type VaultReader interface {
	Get(key string) (string, error)
	Has(key string) bool
}

// PluginsConfig holds configuration for the plugin system.
type PluginsConfig struct {
	// Dirs lists directories to scan for plugins (YAML-based).
	Dirs []string `yaml:"dirs"`

	// Dir is the legacy single-directory config (for backward compatibility with .so loader).
	Dir string `yaml:"dir"`

	// Enabled lists plugin IDs to load (empty = load all found).
	Enabled []string `yaml:"enabled"`

	// Disabled lists plugin IDs to skip.
	Disabled []string `yaml:"disabled"`

	// Overrides maps plugin ID to config overrides.
	Overrides map[string]map[string]any `yaml:"overrides"`
}

// Config is a type alias for backward compatibility with copilot/config.go.
type Config = PluginsConfig

// EffectiveDirs returns the merged list of directories to scan.
// Combines Dirs and the legacy Dir field, deduplicating entries.
func (c PluginsConfig) EffectiveDirs() []string {
	seen := make(map[string]bool)
	var dirs []string

	for _, d := range c.Dirs {
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}

	if c.Dir != "" && !seen[c.Dir] {
		dirs = append(dirs, c.Dir)
	}

	return dirs
}

// ResolveConfig resolves the final configuration for a plugin by merging
// sources in precedence order: overrides > vault > env > defaults.
func ResolveConfig(schema *PluginConfigSchema, overrides map[string]any, vault VaultReader) (map[string]any, error) {
	resolved := make(map[string]any)

	if schema == nil {
		return resolved, nil
	}

	for _, field := range schema.Fields {
		// 1. Check overrides (highest precedence).
		if v, ok := overrides[field.Key]; ok {
			resolved[field.Key] = v
			continue
		}

		// 2. Check vault.
		if field.VaultKey != "" && vault != nil {
			if v, err := vault.Get(field.VaultKey); err == nil {
				resolved[field.Key] = v
				continue
			}
		}

		// 3. Check environment variable.
		if field.EnvVar != "" {
			if v := os.Getenv(field.EnvVar); v != "" {
				resolved[field.Key] = v
				continue
			}
		}

		// 4. Use default.
		if field.Default != nil {
			resolved[field.Key] = field.Default
			continue
		}

		// 5. Required but missing.
		if field.Required {
			return nil, fmt.Errorf("required config field %q not provided", field.Key)
		}
	}

	return resolved, nil
}

// ValidateConfig validates resolved config against the schema.
func ValidateConfig(schema *PluginConfigSchema, resolved map[string]any) error {
	if schema == nil {
		return nil
	}

	for _, field := range schema.Fields {
		if field.Required {
			v, ok := resolved[field.Key]
			if !ok || v == nil || v == "" {
				return fmt.Errorf("required config field %q is missing or empty", field.Key)
			}
		}
	}

	return nil
}
