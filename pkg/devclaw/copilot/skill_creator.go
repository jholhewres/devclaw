// Package copilot – skill_creator.go implements tools that allow the agent
// to create, edit, and manage skills via chat. Skills are created as
// ClawdHub-compatible SKILL.md files in the workspace skills directory.
//
// The agent can use these tools to:
//   - Initialize a new skill with a SKILL.md template
//   - Edit an existing skill's instructions
//   - Add scripts (Python, Node, Shell) to a skill
//   - List installed skills
//   - Test a skill by executing it
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/skills"
)

// SkillReloadCallback reloads skills from disk, initializes them with
// the sandbox runner, and re-registers their tools. Returns the total
// number of skills loaded.
type SkillReloadCallback func(ctx context.Context) (int, error)

// RegisterSkillCreatorTools registers individual skill management tools.
// Replaces the old dispatcher pattern with focused tools:
// skill_init, skill_edit, skill_add_script, skill_list, skill_test,
// skill_install, skill_defaults_list, skill_defaults_install, skill_remove.
func RegisterSkillCreatorTools(executor *ToolExecutor, registry *skills.Registry, skillsDir string, skillDB *SkillDB, reloadCb SkillReloadCallback, logger *slog.Logger) {
	if skillsDir == "" {
		skillsDir = "./skills"
	}
	if logger == nil {
		logger = slog.Default()
	}

	installer := skills.NewInstaller(skillsDir, logger)

	// withAdminCheck wraps a handler to require admin access for write operations.
	withAdminCheck := func(handler func(context.Context, map[string]any) (any, error)) func(context.Context, map[string]any) (any, error) {
		return func(ctx context.Context, args map[string]any) (any, error) {
			level := CallerLevelFromContext(ctx)
			if level != AccessOwner && level != AccessAdmin {
				return nil, fmt.Errorf("this action requires admin access (current: %s)", level)
			}
			return handler(ctx, args)
		}
	}

	// ── skill_init ──
	executor.Register(
		MakeToolDefinition("skill_init",
			"Create a new skill with a SKILL.md template. Optionally creates a database table for structured data storage.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":        map[string]any{"type": "string", "description": "Skill name (will be sanitized to lowercase-hyphenated)"},
					"description": map[string]any{"type": "string", "description": "Brief description of what the skill does"},
					"instructions": map[string]any{"type": "string", "description": "Agent instructions in Markdown format"},
					"emoji":       map[string]any{"type": "string", "description": "Emoji icon for the skill"},
					"with_database": map[string]any{"type": "boolean", "description": "Create a database table for structured data storage"},
					"database_table": map[string]any{"type": "string", "description": "Database table name (default: 'data')"},
					"database_schema": map[string]any{
						"type": "object", "description": "Column definitions as {name: type} pairs",
						"additionalProperties": map[string]any{"type": "string"},
					},
				},
				"required": []string{"name", "description"},
			}),
		withAdminCheck(func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillInit(registry, skillsDir, skillDB, args)
		}),
	)

	// ── skill_edit ── (hidden: admin tool, LLM uses edit_file directly)
	executor.RegisterHidden(
		MakeToolDefinition("skill_edit",
			"Edit an existing skill's SKILL.md content.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string", "description": "Skill name to edit"},
					"content": map[string]any{"type": "string", "description": "New SKILL.md content (full replacement)"},
				},
				"required": []string{"name", "content"},
			}),
		withAdminCheck(func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillEdit(skillsDir, args)
		}),
	)

	// ── skill_add_script ── (hidden: admin tool, rare direct use)
	executor.RegisterHidden(
		MakeToolDefinition("skill_add_script",
			"Add an executable script (Python, Node.js, Shell) to an existing skill.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"skill_name":  map[string]any{"type": "string", "description": "Target skill name"},
					"script_name": map[string]any{"type": "string", "description": "Script filename (e.g. main.py, run.sh)"},
					"content":     map[string]any{"type": "string", "description": "Script source code"},
				},
				"required": []string{"skill_name", "script_name", "content"},
			}),
		withAdminCheck(func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillAddScript(skillsDir, args)
		}),
	)

	// ── skill_list ──
	executor.Register(
		MakeToolDefinition("skill_list",
			"List all installed skills with name, version, author, description, category, and tags.",
			map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		func(_ context.Context, _ map[string]any) (any, error) {
			return handleSkillList(registry, skillsDir)
		},
	)

	// ── skill_test ── (hidden: frequent loop source, LLM tests via bash)
	executor.RegisterHidden(
		MakeToolDefinition("skill_test",
			"Test a skill by executing it with sample input.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":  map[string]any{"type": "string", "description": "Skill name to test"},
					"input": map[string]any{"type": "string", "description": "Test input to send to the skill"},
				},
				"required": []string{"name"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleSkillTest(ctx, registry, args)
		},
	)

	// ── skill_install ──
	executor.Register(
		MakeToolDefinition("skill_install",
			"Install a skill from ClawHub (slug), GitHub URL, or local path.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source": map[string]any{"type": "string", "description": "Install source: ClawHub slug, GitHub URL, or local path"},
				},
				"required": []string{"source"},
			}),
		withAdminCheck(func(ctx context.Context, args map[string]any) (any, error) {
			return handleSkillInstall(ctx, installer, reloadCb, args)
		}),
	)

	// ── skill_defaults_list ── (hidden: admin/setup only)
	executor.RegisterHidden(
		MakeToolDefinition("skill_defaults_list",
			"List available default skills that can be installed.",
			map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		func(_ context.Context, _ map[string]any) (any, error) {
			return handleSkillDefaultsList(skillsDir)
		},
	)

	// ── skill_defaults_install ── (hidden: admin/setup only)
	executor.RegisterHidden(
		MakeToolDefinition("skill_defaults_install",
			"Install one or more default skills. Pass [\"all\"] to install all defaults.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"names": map[string]any{
						"type": "array", "items": map[string]any{"type": "string"},
						"description": "Skill names to install, or [\"all\"] for all defaults",
					},
				},
				"required": []string{"names"},
			}),
		withAdminCheck(func(ctx context.Context, args map[string]any) (any, error) {
			return handleSkillDefaultsInstall(ctx, reloadCb, skillsDir, args)
		}),
	)

	// ── skill_remove ── (hidden: admin, destructive)
	executor.RegisterHidden(
		MakeToolDefinition("skill_remove",
			"Remove an installed skill and its directory.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "Skill name to remove"},
				},
				"required": []string{"name"},
			}),
		withAdminCheck(func(_ context.Context, args map[string]any) (any, error) {
			return handleSkillRemove(registry, skillsDir, args)
		}),
	)
}

func handleSkillInit(registry *skills.Registry, skillsDir string, skillDB *SkillDB, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	description, _ := args["description"].(string)
	instructions, _ := args["instructions"].(string)
	emoji, _ := args["emoji"].(string)
	withDatabase, _ := args["with_database"].(bool)
	databaseTable, _ := args["database_table"].(string)
	databaseSchemaRaw, _ := args["database_schema"].(map[string]any)

	if name == "" || description == "" {
		return nil, fmt.Errorf("name and description are required for init action")
	}

	displayName := name
	name = sanitizeSkillName(name)
	dbSkillName := strings.ReplaceAll(name, "-", "_")

	if withDatabase && skillDB != nil {
		existingTables, err := skillDB.ListTables("")
		if err == nil {
			for _, t := range existingTables {
				if t.SkillName == dbSkillName {
					existingSkillName := strings.ReplaceAll(t.SkillName, "_", "-")
					return nil, fmt.Errorf("skill name collision: '%s' would conflict with existing skill '%s' in database", name, existingSkillName)
				}
			}
		}
	}

	if _, exists := registry.Get(name); exists {
		return nil, fmt.Errorf("skill '%s' already exists. Use skill_edit to modify it", name)
	}

	skillDir := filepath.Join(skillsDir, name)
	if _, err := os.Stat(skillDir); err == nil {
		return nil, fmt.Errorf("skill directory '%s' already exists. Use skill_edit to modify it", skillDir)
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating skill directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		return nil, fmt.Errorf("creating scripts directory: %w", err)
	}

	var dbInfo string
	if withDatabase && skillDB != nil {
		if databaseTable == "" {
			databaseTable = "data"
		}
		databaseSchema := make(map[string]string)
		for k, v := range databaseSchemaRaw {
			if vs, ok := v.(string); ok {
				databaseSchema[k] = vs
			}
		}
		err := skillDB.CreateTable(dbSkillName, databaseTable, displayName, description, databaseSchema)
		if err != nil {
			os.RemoveAll(skillDir)
			return nil, fmt.Errorf("creating database table: %w", err)
		}
		dbInfo = fmt.Sprintf("\n\nDatabase table '%s_%s' created for storing data.", dbSkillName, databaseTable)
	}

	if instructions == "" {
		instructions = fmt.Sprintf("# %s\n\nDescribe how the agent should use this skill.", name)
	}

	if withDatabase && skillDB != nil {
		dbInstructions := fmt.Sprintf(`

## Database

This skill has a database table for storing structured data.

**IMPORTANT:**
- Always use skill_name="%s" (underscores, not hyphens)
- Use action="query" to LIST data (when user asks to "list", "show", "what are")
- NEVER show tool syntax in chat - respond naturally

### The skill_db Tool

`+"```"+`
# LIST records (use this when user asks to "list" or "show")
skill_db(action="query", skill_name="%s", table_name="%s")

# ADD a record
skill_db(action="insert", skill_name="%s", table_name="%s", data={"title": "Example"})

# FILTER records
skill_db(action="query", skill_name="%s", table_name="%s", where={"status": "active"})

# UPDATE a record
skill_db(action="update", skill_name="%s", table_name="%s", row_id="ID", data={"status": "done"})

# DELETE a record
skill_db(action="delete", skill_name="%s", table_name="%s", row_id="ID")
`+"```"+`

### Quick Reference
| User asks... | Use action=... |
|--------------|----------------|
| "list my X" / "show X" | query |
| "save/add/create X" | insert |
| "update/change X" | update |
| "delete/remove X" | delete |
`, dbSkillName, dbSkillName, databaseTable, dbSkillName, databaseTable, dbSkillName, databaseTable, dbSkillName, databaseTable, dbSkillName, databaseTable)
		instructions += dbInstructions
	}

	metadata := map[string]any{
		"openclaw": map[string]any{"emoji": emoji, "always": false},
		"database": withDatabase,
	}
	metaJSON, _ := json.Marshal(metadata)

	skillMD := fmt.Sprintf("---\nname: %s\ndescription: \"%s\"\nmetadata: %s\n---\n%s\n", name, description, string(metaJSON), instructions)

	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(skillMD), 0o644); err != nil {
		return nil, fmt.Errorf("writing SKILL.md: %w", err)
	}

	return fmt.Sprintf("Skill '%s' created at %s%s\n\nTo add scripts: use skill_add_script.\nTo test: use skill_test.", name, skillDir, dbInfo), nil
}

func handleSkillEdit(skillsDir string, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	content, _ := args["content"].(string)
	if name == "" || content == "" {
		return nil, fmt.Errorf("name and content are required for edit action")
	}
	skillFile := filepath.Join(skillsDir, sanitizeSkillName(name), "SKILL.md")
	if _, err := os.Stat(skillFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("skill '%s' not found at %s", name, skillFile)
	}
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("writing SKILL.md: %w", err)
	}
	return fmt.Sprintf("Skill '%s' updated.", name), nil
}

func handleSkillAddScript(skillsDir string, args map[string]any) (any, error) {
	skillName, _ := args["skill_name"].(string)
	scriptName, _ := args["script_name"].(string)
	content, _ := args["content"].(string)
	if skillName == "" || scriptName == "" || content == "" {
		return nil, fmt.Errorf("skill_name, script_name, and content are required for add_script action")
	}
	scriptsDir := filepath.Join(skillsDir, sanitizeSkillName(skillName), "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating scripts directory: %w", err)
	}
	scriptPath := filepath.Join(scriptsDir, scriptName)
	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return nil, fmt.Errorf("writing script: %w", err)
	}
	return fmt.Sprintf("Script '%s' added to skill '%s'.", scriptName, skillName), nil
}

func handleSkillList(registry *skills.Registry, skillsDir string) (any, error) {
	allSkills := registry.List()
	if len(allSkills) == 0 {
		return "No skills installed.", nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Installed skills (%d):\n\n", len(allSkills))
	for _, meta := range allSkills {
		fmt.Fprintf(&sb, "- **%s** v%s by %s\n  %s\n  Category: %s, Tags: %s\n",
			meta.Name, meta.Version, meta.Author, meta.Description,
			meta.Category, strings.Join(meta.Tags, ", "))
	}
	userSkills := listUserSkillDirs(skillsDir)
	if len(userSkills) > 0 {
		fmt.Fprintf(&sb, "\nUser skills directory (%d):\n", len(userSkills))
		for _, name := range userSkills {
			fmt.Fprintf(&sb, "- %s\n", name)
		}
	}
	return sb.String(), nil
}

func handleSkillTest(ctx context.Context, registry *skills.Registry, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	input, _ := args["input"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required for test action")
	}
	skill, ok := registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("skill '%s' not found in registry", name)
	}
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := skill.Execute(testCtx, input)
	if err != nil {
		return nil, fmt.Errorf("skill execution failed: %w", err)
	}
	return fmt.Sprintf("Skill '%s' test result:\n\n%s", name, result), nil
}

func handleSkillInstall(ctx context.Context, installer *skills.Installer, reloadCb SkillReloadCallback, args map[string]any) (any, error) {
	source, _ := args["source"].(string)
	if source == "" {
		return nil, fmt.Errorf("source is required for install action")
	}
	installCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result, err := installer.Install(installCtx, source)
	if err != nil {
		return nil, fmt.Errorf("install failed: %w", err)
	}
	reloadCtx, reloadCancel := context.WithTimeout(ctx, 10*time.Second)
	defer reloadCancel()
	reloaded, reloadErr := reloadCb(reloadCtx)
	reloadMsg := ""
	if reloadErr != nil {
		reloadMsg = fmt.Sprintf("\nWarning: skill catalog refresh failed: %v", reloadErr)
	} else {
		reloadMsg = fmt.Sprintf("\nSkill catalog refreshed (%d skills loaded).", reloaded)
	}
	status := "installed"
	if !result.IsNew {
		status = "updated"
	}
	return fmt.Sprintf("Skill '%s' %s successfully.\nPath: %s\nSource: %s%s",
		result.Name, status, result.Path, result.Source, reloadMsg), nil
}

func handleSkillDefaultsList(skillsDir string) (any, error) {
	defaults := skills.DefaultSkills()
	installed := listUserSkillDirs(skillsDir)
	installedSet := make(map[string]bool, len(installed))
	for _, n := range installed {
		installedSet[n] = true
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Default skills available (%d):\n\n", len(defaults))
	for _, d := range defaults {
		status := "not installed"
		if installedSet[d.Name] {
			status = "installed"
		}
		fmt.Fprintf(&sb, "- **%s** — %s [%s]\n", d.Name, d.Description, status)
	}
	sb.WriteString("\nUse skill_defaults_install(names=[...]) to install. Pass [\"all\"] for all.")
	return sb.String(), nil
}

func handleSkillDefaultsInstall(ctx context.Context, reloadCb SkillReloadCallback, skillsDir string, args map[string]any) (any, error) {
	rawNames, _ := args["names"].([]any)
	if len(rawNames) == 0 {
		return nil, fmt.Errorf("names is required for defaults_install action: pass skill names or [\"all\"]")
	}
	var names []string
	for _, v := range rawNames {
		if s, ok := v.(string); ok {
			names = append(names, s)
		}
	}
	if len(names) == 1 && strings.ToLower(names[0]) == "all" {
		names = skills.DefaultSkillNames()
	}
	installed, skipped, failed := skills.InstallDefaultSkills(skillsDir, names)
	reloadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	reloaded, reloadErr := reloadCb(reloadCtx)
	reloadMsg := ""
	if reloadErr != nil {
		reloadMsg = fmt.Sprintf("\nWarning: catalog refresh failed: %v", reloadErr)
	} else {
		reloadMsg = fmt.Sprintf("\nSkill catalog refreshed (%d skills loaded).", reloaded)
	}
	var sb strings.Builder
	sb.WriteString("Default skills installation complete.\n")
	fmt.Fprintf(&sb, "  Installed: %d\n", installed)
	if skipped > 0 {
		fmt.Fprintf(&sb, "  Already existed: %d\n", skipped)
	}
	if failed > 0 {
		fmt.Fprintf(&sb, "  Failed: %d\n", failed)
	}
	sb.WriteString(reloadMsg)
	return sb.String(), nil
}

func handleSkillRemove(registry *skills.Registry, skillsDir string, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required for remove action")
	}
	targetDir := filepath.Join(skillsDir, sanitizeSkillName(name))
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("skill '%s' not found at %s", name, targetDir)
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return nil, fmt.Errorf("removing skill: %w", err)
	}
	registry.Remove(sanitizeSkillName(name))
	return fmt.Sprintf("Skill '%s' removed successfully.", name), nil
}

// sanitizeSkillName normalizes a skill name to filesystem-safe format.
func sanitizeSkillName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	// Remove anything that's not alphanumeric or hyphen.
	var clean strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}

// listUserSkillDirs lists skill directories in the user skills folder.
func listUserSkillDirs(skillsDir string) []string {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	return names
}

