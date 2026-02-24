// Package paths provides centralized path resolution for the DevClaw application.
// It supports a unified state directory layout (~/.devclaw) with backward compatibility
// for existing installations using the legacy ./data layout.
package paths

import (
	"os"
	"path/filepath"
)

// AppName is the application name used for the state directory.
const AppName = "devclaw"

// StateDirEnv is the environment variable for custom state directory.
const StateDirEnv = "DEVCLAW_STATE_DIR"

// ConfigPathEnv is the environment variable for custom config path.
const ConfigPathEnv = "DEVCLAW_CONFIG_PATH"

// ResolveStateDir returns the centralized state directory path.
// Precedence: DEVCLAW_STATE_DIR > ~/.devclaw > . (backward compat)
func ResolveStateDir() string {
	// 1. Check environment variable first
	if dir := os.Getenv(StateDirEnv); dir != "" {
		return dir
	}

	// 2. Try user home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return "." // fallback to current directory
	}

	newDir := filepath.Join(home, "."+AppName)

	// 3. If new directory exists, use it
	if _, err := os.Stat(newDir); err == nil {
		return newDir
	}

	// 4. If legacy ./data exists (old layout), maintain backward compatibility
	if _, err := os.Stat("./data"); err == nil {
		return "."
	}

	// 5. Default to new location
	return newDir
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
// Precedence: DEVCLAW_CONFIG_PATH > ~/.devclaw/config.yaml > ./config.yaml
func ResolveConfigPath() string {
	// 1. Check environment variable first
	if path := os.Getenv(ConfigPathEnv); path != "" {
		return path
	}

	// 2. Check for config in state directory
	statePath := filepath.Join(ResolveStateDir(), "config.yaml")
	if _, err := os.Stat(statePath); err == nil {
		return statePath
	}

	// 3. Check for local config (backward compat)
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

	// 4. Default to state directory
	return statePath
}

// ResolveVaultPath returns the vault file path.
// Prioritizes new location but maintains backward compatibility.
func ResolveVaultPath() string {
	newPath := filepath.Join(ResolveStateDir(), "."+AppName+".vault")
	oldPath := "./." + AppName + ".vault"

	// Check new location first
	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}

	// Check legacy location
	if _, err := os.Stat(oldPath); err == nil {
		return oldPath
	}

	// Default to new location
	return newPath
}

// ResolveDatabasePath returns the database file path.
func ResolveDatabasePath(filename string) string {
	return filepath.Join(ResolveDataDir(), filename)
}

// ResolveMediaPath returns the media path for a specific channel and session.
func ResolveMediaPath(channel, sessionID string) string {
	return filepath.Join(ResolveMediaDir(), channel, sessionID)
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
