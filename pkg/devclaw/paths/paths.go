// Package paths provides centralized path resolution for the DevClaw application.
// All data is stored in ./data/ relative to the project root.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// AppName is the application name used for the state directory.
const AppName = "devclaw"

// StateDirEnv is the environment variable for custom state directory.
const StateDirEnv = "DEVCLAW_STATE_DIR"

// ConfigPathEnv is the environment variable for custom config path.
const ConfigPathEnv = "DEVCLAW_CONFIG_PATH"

// ResolveStateDir returns the project root directory.
func ResolveStateDir() string {
	// Check environment variable first
	if dir := os.Getenv(StateDirEnv); dir != "" {
		return dir
	}
	return "."
}

// ResolveDataDir returns the data directory path.
func ResolveDataDir() string {
	return filepath.Join(ResolveStateDir(), "data")
}

// ResolveMediaDir returns the media directory path.
func ResolveMediaDir() string {
	return filepath.Join(ResolveStateDir(), "data", "media")
}

// ResolveSessionsDir returns the sessions directory path.
func ResolveSessionsDir() string {
	return filepath.Join(ResolveStateDir(), "sessions")
}

// ResolveSkillsDir returns the skills directory path.
func ResolveSkillsDir() string {
	return filepath.Join(ResolveStateDir(), "skills")
}

// ResolveWorkspacesDir returns the workspaces directory path.
func ResolveWorkspacesDir() string {
	return filepath.Join(ResolveStateDir(), "workspaces")
}

// ResolvePluginsDir returns the plugins directory path.
func ResolvePluginsDir() string {
	return filepath.Join(ResolveStateDir(), "plugins")
}

// ResolveConfigPath returns the config file path.
// Precedence: DEVCLAW_CONFIG_PATH > ./config.yaml
func ResolveConfigPath() string {
	// Check environment variable first
	if path := os.Getenv(ConfigPathEnv); path != "" {
		return path
	}

	// Check for config files in common locations
	localPaths := []string{
		"config.yaml",
		"config.yml",
		"devclaw.yaml",
		"devclaw.yml",
		"configs/config.yaml",
	}

	for _, p := range localPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return "config.yaml"
}

// ResolveVaultPath returns the vault file path.
func ResolveVaultPath() string {
	return filepath.Join(ResolveStateDir(), "."+AppName+".vault")
}

// ResolveDatabasePath returns the database file path.
func ResolveDatabasePath(filename string) string {
	return filepath.Join(ResolveDataDir(), filename)
}

// ResolveMediaPath returns the media path for a specific channel and session.
// Input is sanitized to prevent path traversal attacks.
func ResolveMediaPath(channel, sessionID string) string {
	// Sanitize inputs to prevent path traversal
	channel = sanitizePathComponent(channel)
	sessionID = sanitizePathComponent(sessionID)
	return filepath.Join(ResolveMediaDir(), channel, sessionID)
}

// sanitizePathComponent removes path traversal sequences from a path component.
func sanitizePathComponent(s string) string {
	// Remove any path separators and parent directory references
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, "..", "_")
	return s
}

// EnsureStateDirs creates the state directory structure if it doesn't exist.
func EnsureStateDirs() error {
	dirs := []string{
		ResolveStateDir(),
		ResolveDataDir(),
		ResolveMediaDir(),
		filepath.Join(ResolveMediaDir(), "whatsapp"),
		filepath.Join(ResolveMediaDir(), "telegram"),
		ResolveSessionsDir(),
		filepath.Join(ResolveSessionsDir(), "whatsapp"),
		filepath.Join(ResolveSessionsDir(), "telegram"),
		ResolveSkillsDir(),
		ResolveWorkspacesDir(),
		ResolvePluginsDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}
