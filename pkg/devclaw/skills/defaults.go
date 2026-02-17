// Package skills – defaults.go provides embedded default skill templates
// that can be installed via the setup wizard, CLI commands, chat commands,
// or agent tools. Each skill is a SKILL.md file (ClawdHub/OpenClaw format).
package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultSkill holds a default skill template.
type DefaultSkill struct {
	Name        string // Unique identifier (directory name).
	Label       string // Human-readable label for interactive selection.
	Description string // Short description.
	Category    string // Category: "development", "data", "productivity", "infra".
	StarterPack bool   // If true, pre-selected during setup wizard.
	Content     string // Full SKILL.md file content.
}

// DefaultSkills returns the list of default skill templates available for installation.
func DefaultSkills() []DefaultSkill {
	return defaultSkillList
}

// DefaultSkillNames returns just the names of all available default skills.
func DefaultSkillNames() []string {
	names := make([]string, len(defaultSkillList))
	for i, s := range defaultSkillList {
		names[i] = s.Name
	}
	return names
}

// GetDefaultSkill returns a default skill by name, or nil if not found.
func GetDefaultSkill(name string) *DefaultSkill {
	for _, s := range defaultSkillList {
		if s.Name == name {
			return &s
		}
	}
	return nil
}

// InstallDefaultSkill installs a single default skill to the given skills directory.
// Returns true if installed, false if already existed.
func InstallDefaultSkill(skillsDir, name string) (bool, error) {
	tmpl := GetDefaultSkill(name)
	if tmpl == nil {
		return false, fmt.Errorf("unknown default skill: %s", name)
	}

	targetDir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return false, fmt.Errorf("creating skill directory: %w", err)
	}

	skillFile := filepath.Join(targetDir, "SKILL.md")

	// Don't overwrite existing skills.
	if _, err := os.Stat(skillFile); err == nil {
		return false, nil // already exists
	}

	if err := os.WriteFile(skillFile, []byte(tmpl.Content), 0o644); err != nil {
		return false, fmt.Errorf("writing SKILL.md: %w", err)
	}

	return true, nil
}

// InstallDefaultSkills installs multiple default skills and returns counts.
func InstallDefaultSkills(skillsDir string, names []string) (installed, skipped, failed int) {
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return 0, 0, len(names)
	}

	for _, name := range names {
		isNew, err := InstallDefaultSkill(skillsDir, name)
		if err != nil {
			failed++
		} else if isNew {
			installed++
		} else {
			skipped++
		}
	}

	return installed, skipped, failed
}

// InstallAllDefaults installs all available default skills.
func InstallAllDefaults(skillsDir string) (installed, skipped, failed int) {
	return InstallDefaultSkills(skillsDir, DefaultSkillNames())
}

// StarterPackNames returns the names of skills marked as starter pack.
func StarterPackNames() []string {
	var names []string
	for _, s := range defaultSkillList {
		if s.StarterPack {
			names = append(names, s.Name)
		}
	}
	return names
}

// InstallStarterPack installs only starter pack skills.
func InstallStarterPack(skillsDir string) (installed, skipped, failed int) {
	return InstallDefaultSkills(skillsDir, StarterPackNames())
}
