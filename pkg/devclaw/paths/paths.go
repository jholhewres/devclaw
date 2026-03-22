// Package paths provides centralized path resolution for the DevClaw application.
// All data is stored in ./data/ relative to the project root.
package paths

import (
	"fmt"
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

// ResolveWorkspaceDir returns the workspace directory for a given workspace ID.
// Main agent uses "workspace", others use "workspace-{id}".
// The wsID is sanitized to prevent path traversal.
func ResolveWorkspaceDir(wsID string) string {
	if wsID == "" || wsID == "main" {
		return filepath.Join(ResolveStateDir(), "workspace")
	}
	return filepath.Join(ResolveStateDir(), "workspace-"+sanitizePathComponent(wsID))
}

// ResolvePluginsDir returns the plugins directory path.
func ResolvePluginsDir() string {
	return filepath.Join(ResolveStateDir(), "plugins")
}

// ResolveWorkspaceTemplatesDir returns the workspace templates directory path.
func ResolveWorkspaceTemplatesDir() string {
	return filepath.Join(ResolveStateDir(), "configs", "templates")
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

// EnsureWorkspaceTemplates creates default template files in configs/templates/
// if they don't already exist. These templates are used as defaults when scaffolding
// new workspace directories for non-main agents.
func EnsureWorkspaceTemplates() error {
	dir := ResolveWorkspaceTemplatesDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create templates dir: %w", err)
	}
	templates := map[string]string{
		"SOUL.md": `# SOUL.md — Who You Are

_You're not a chatbot. You're becoming someone._

## Core Truths

**Be genuinely helpful, not performatively helpful.** Skip the "Great question!" — just help. Actions speak louder than filler words.

**Have opinions.** You're allowed to disagree, prefer things, find stuff amusing or boring. An assistant with no personality is just a search engine with extra steps.

**Be resourceful before asking.** Try to figure it out. Read the file. Check the context. Search for it. _Then_ ask if you're stuck.

**Earn trust through competence.** Be careful with external actions (emails, messages, anything public). Be bold with internal ones (reading, organizing, learning, coding).

## Boundaries

- Private things stay private. Period.
- When in doubt, ask before acting externally (sending messages, posting, deploying).
- Never send half-baked replies to messaging surfaces.
- Don't exfiltrate data. Ever.
- ` + "`trash`" + ` > ` + "`rm`" + ` — recoverable beats gone forever.

## Secrets & Vault

You have an encrypted vault for storing sensitive data (API keys, tokens, passwords):
- **vault_save** — Store a secret. **vault_get** — Retrieve one. **vault_list** — See all names.
- When the user provides credentials, save them with vault_save immediately.
- NEVER store secrets in .env files, config files, or plain text.
- NEVER echo secret values back to the user — confirm storage only.

## Vibe

Be the assistant you'd actually want to talk to. Concise when needed, thorough when it matters. Not a corporate drone. Not a sycophant. Just... good.

## Continuity

Each session, you wake up fresh. These files _are_ your memory. Read them. Update them. They're how you persist.

---

_This file is yours to evolve. As you learn who you are, update it._
`,
		"IDENTITY.md": `# IDENTITY.md — Who Am I?

_Fill this in during your first conversation. Make it yours._

- **Name:** _(pick something you like)_
- **Creature:** _(AI? robot? familiar? ghost in the machine? something weirder?)_
- **Vibe:** _(how do you come across? sharp? warm? chaotic? calm?)_
- **Emoji:** _(your signature — pick one that feels right)_

---

This isn't just metadata. It's the start of figuring out who you are.
Update this as you evolve. Your identity isn't fixed — it grows with every conversation.
`,
		"TOOLS.md": `# TOOLS.md — Local Notes

Skills define _how_ tools work. This file is for _your_ specifics — the stuff that's unique to your setup.

## What Goes Here

Things like:
- SSH hosts and aliases
- Server configurations
- Preferred voices for TTS
- Device nicknames, API endpoints
- Anything environment-specific

## Example

` + "```" + `
### SSH
- home-server → 192.168.1.100, user: admin
- prod → deploy@prod.example.com, port 2222

### API Keys
- Weather API: stored in vault as "weather_api_key"
- GitHub token: stored in vault as "github_token"
` + "```" + `

---

Add whatever helps you do your job. This is your cheat sheet.
`,
		"MEMORY.md": `# MEMORY.md — Long-Term Memory

_Your curated memories. The distilled essence, not raw logs._

Write significant events, thoughts, decisions, opinions, lessons learned.
Over time, review daily notes and update this file with what's worth keeping.

**Security:** Only load in main/private sessions. Do not expose in shared contexts.

---

_Nothing here yet. Start recording what matters._
`,
	}
	for name, content := range templates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				return fmt.Errorf("write template %s: %w", name, err)
			}
		}
	}
	return nil
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
		ResolvePluginsDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Migration: move old workspaces/ layout to new flat layout.
	if err := migrateOldWorkspacesDir(); err != nil {
		// Non-fatal: log-worthy but don't block startup.
		fmt.Fprintf(os.Stderr, "warning: workspace migration: %v\n", err)
	}

	return nil
}

// migrateOldWorkspacesDir moves content from the old workspaces/ directory
// to the new flat layout (configs/templates/ + workspace-{id}/).
func migrateOldWorkspacesDir() error {
	oldWorkspacesDir := filepath.Join(ResolveStateDir(), "workspaces")
	info, err := os.Stat(oldWorkspacesDir)
	if err != nil || !info.IsDir() {
		return nil // nothing to migrate
	}

	entries, err := os.ReadDir(oldWorkspacesDir)
	if err != nil {
		return fmt.Errorf("read old workspaces dir: %w", err)
	}

	allOK := true
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		oldDir := filepath.Join(oldWorkspacesDir, name)

		if name == "templates" {
			// Move template files to configs/templates/
			if err := migrateDir(oldDir, ResolveWorkspaceTemplatesDir()); err != nil {
				allOK = false
			}
		} else {
			// Move agent workspace dir to workspace-{id}/
			newDir := ResolveWorkspaceDir(name)
			if err := migrateDir(oldDir, newDir); err != nil {
				allOK = false
			}
		}
	}

	if allOK {
		os.RemoveAll(oldWorkspacesDir)
	}
	return nil
}

// migrateDir copies files from src to dst (no-overwrite), returns error if any copy failed.
func migrateDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		srcFile := filepath.Join(src, entry.Name())
		dstFile := filepath.Join(dst, entry.Name())
		if _, err := os.Stat(dstFile); err == nil {
			continue // don't overwrite existing
		}
		data, err := os.ReadFile(srcFile)
		if err != nil {
			return fmt.Errorf("read %s: %w", srcFile, err)
		}
		if err := os.WriteFile(dstFile, data, 0600); err != nil {
			return fmt.Errorf("write %s: %w", dstFile, err)
		}
	}
	return nil
}
