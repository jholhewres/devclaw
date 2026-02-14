package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/jholhewres/goclaw/pkg/goclaw/copilot"
	"github.com/jholhewres/goclaw/pkg/goclaw/skills"
	"github.com/spf13/cobra"
)

// newSkillCmd creates the `copilot skill` command for managing skills.
func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage assistant skills",
		Long: `Manage installed and available skills. Install from ClawHub,
GitHub, URLs, or local paths.

Sources:
  copilot skill install steipete/trello              # ClawHub slug
  copilot skill install https://clawhub.ai/user/skill # ClawHub URL
  copilot skill install https://github.com/user/repo  # GitHub repo
  copilot skill install https://example.com/skill.zip  # URL (zip or SKILL.md)
  copilot skill install ./my-local-skill               # Local path

Other:
  copilot skill list                                   # List installed skills
  copilot skill search calendar                        # Search ClawHub
  copilot skill info <name>                            # Show skill details
  copilot skill remove <name>                          # Remove a skill
  copilot skill update --all                           # Update all GitHub skills`,
	}

	cmd.AddCommand(
		newSkillListCmd(),
		newSkillSearchCmd(),
		newSkillInstallCmd(),
		newSkillDefaultsCmd(),
		newSkillUpdateCmd(),
		newSkillRemoveCmd(),
		newSkillInfoCmd(),
	)

	return cmd
}

// getSkillsDir returns the configured skills directory.
func getSkillsDir(cmd *cobra.Command) string {
	cfg, _, err := loadConfig(cmd)
	if err == nil && len(cfg.Skills.ClawdHubDirs) > 0 {
		return cfg.Skills.ClawdHubDirs[0]
	}
	return "./skills"
}

// loadSkillRegistry creates and loads the skill registry from config.
func loadSkillRegistry(cmd *cobra.Command) (*skills.Registry, *copilot.Config, error) {
	cfg, _, err := loadConfig(cmd)
	if err != nil {
		cfg = copilot.DefaultConfig()
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	registry := skills.NewRegistry(logger)

	// Add builtin loader.
	if len(cfg.Skills.Builtin) > 0 {
		registry.AddLoader(skills.NewBuiltinLoader(cfg.Skills.Builtin, logger))
	}

	// Add ClawdHub loader for local skill directories.
	dirs := cfg.Skills.ClawdHubDirs
	if len(dirs) == 0 {
		dirs = []string{"./skills"}
	}
	registry.AddLoader(skills.NewClawdHubLoader(dirs, logger))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = registry.LoadAll(ctx)
	return registry, cfg, nil
}

func newSkillDefaultsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defaults",
		Short: "Install recommended default skills (interactive or --all)",
		Long: `Interactively select and install recommended default skills that come
bundled with GoClaw. Use --all to install all of them at once.

These are productivity-focused skills like web-search, weather, notes,
reminders, timer, translate, and more — ready to use without any setup.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			skillsDir := getSkillsDir(cmd)
			allFlag, _ := cmd.Flags().GetBool("all")

			defaults := skills.DefaultSkills()

			var selectedNames []string

			if allFlag {
				// Install all defaults without prompting.
				for _, d := range defaults {
					selectedNames = append(selectedNames, d.Name)
				}
			} else {
				// Interactive multi-select.
				opts := make([]huh.Option[string], 0, len(defaults))
				for _, d := range defaults {
					opts = append(opts, huh.NewOption(d.Label, d.Name))
				}

				err := huh.NewForm(
					huh.NewGroup(
						huh.NewMultiSelect[string]().
							Title("Default skills").
							Description("Select skills to install (Space to toggle, Enter to confirm)").
							Options(opts...).
							Value(&selectedNames),
					),
				).WithTheme(huh.ThemeDracula()).Run()
				if err != nil {
					return err
				}
			}

			if len(selectedNames) == 0 {
				fmt.Println("No skills selected.")
				return nil
			}

			fmt.Printf("Installing %d skill(s) to %s...\n\n", len(selectedNames), skillsDir)

			installed, skipped, failed := skills.InstallDefaultSkills(skillsDir, selectedNames)

			if failed > 0 {
				fmt.Printf("\n%d installed, %d skipped, %d failed.\n", installed, skipped, failed)
			} else if skipped > 0 {
				fmt.Printf("\n%d installed, %d already existed.\n", installed, skipped)
			} else {
				fmt.Printf("\n%d skill(s) installed successfully.\n", installed)
			}

			fmt.Println("\nSkills are available on the next start of 'copilot serve' or 'copilot chat'.")
			return nil
		},
	}

	cmd.Flags().Bool("all", false, "install all default skills without prompting")
	return cmd
}

func newSkillListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		RunE: func(cmd *cobra.Command, _ []string) error {
			registry, _, err := loadSkillRegistry(cmd)
			if err != nil {
				return err
			}

			allSkills := registry.List()
			if len(allSkills) == 0 {
				fmt.Println("No skills installed.")
				fmt.Println()
				fmt.Println("Install one with:")
				fmt.Println("  copilot skill install steipete/trello")
				fmt.Println("  copilot skill search calendar")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "NAME\tVERSION\tCATEGORY\tDESCRIPTION\n")
			fmt.Fprintf(w, "────\t───────\t────────\t───────────\n")
			for _, meta := range allSkills {
				desc := meta.Description
				if len(desc) > 50 {
					desc = desc[:47] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", meta.Name, meta.Version, meta.Category, desc)
			}
			w.Flush()
			fmt.Printf("\n%d skill(s) installed.\n", len(allSkills))
			return nil
		},
	}
}

func newSkillSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search skills on ClawHub",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			query := args[0]
			fmt.Printf("Searching ClawHub for %q...\n\n", query)

			client := skills.NewClawHubClient("")
			result, err := client.Search(query, 20)
			if err != nil {
				return fmt.Errorf("ClawHub search failed: %w", err)
			}

			if len(result.Skills) == 0 {
				fmt.Println("No skills found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "SLUG\tDESCRIPTION\tSTARS\tDOWNLOADS\n")
			fmt.Fprintf(w, "────\t───────────\t─────\t─────────\n")
			for _, s := range result.Skills {
				desc := s.Description
				if len(desc) > 50 {
					desc = desc[:47] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", s.Slug, desc, s.Stars, s.Downloads)
			}
			w.Flush()
			fmt.Printf("\n%d result(s). Install with: copilot skill install <slug>\n", len(result.Skills))
			return nil
		},
	}
}

func newSkillInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <source>",
		Short: "Install a skill from ClawHub, GitHub, URL, or local path",
		Long: `Install a skill from various sources. The source type is auto-detected:

  ClawHub slug:   steipete/trello
  ClawHub URL:    https://clawhub.ai/steipete/trello
  GitHub repo:    https://github.com/user/repo
  GitHub prefix:  github:user/repo/path/to/skill
  URL (zip):      https://example.com/skill.zip
  URL (SKILL.md): https://raw.github.../SKILL.md
  Local path:     ./my-skill

Explicit prefixes: clawhub:<slug>, github:<user/repo>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			skillsDir := getSkillsDir(cmd)

			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
			installer := skills.NewInstaller(skillsDir, logger)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			result, err := installer.Install(ctx, source)
			if err != nil {
				return fmt.Errorf("install failed: %w", err)
			}

			fmt.Println()
			if result.IsNew {
				fmt.Printf("Installed: %s\n", result.Name)
			} else {
				fmt.Printf("Updated: %s\n", result.Name)
			}
			fmt.Printf("  Path:   %s\n", result.Path)
			fmt.Printf("  Source: %s\n", result.Source)
			fmt.Println()
			fmt.Println("The skill will be available on the next start of 'copilot serve' or 'copilot chat'.")

			return nil
		},
	}
}

func newSkillUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [name]",
		Short: "Update installed skills (git-based)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			skillsDir := getSkillsDir(cmd)

			if !all && len(args) == 0 {
				return fmt.Errorf("specify a skill name or use --all")
			}

			entries, err := os.ReadDir(skillsDir)
			if err != nil {
				return fmt.Errorf("reading skills directory: %w", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			updated := 0
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}

				name := entry.Name()
				if !all && len(args) > 0 && args[0] != name {
					continue
				}

				skillDir := skillsDir + "/" + name

				// Check if it's a git repo.
				if _, err := os.Stat(skillDir + "/.git"); err != nil {
					if !all {
						fmt.Printf("  %s: not a git repo, skipping\n", name)
					}
					continue
				}

				fmt.Printf("Updating %s...\n", name)
				gitCmd := exec.CommandContext(ctx, "git", "-C", skillDir, "pull", "--ff-only")
				out, err := gitCmd.CombinedOutput()
				if err != nil {
					fmt.Printf("  %s: update failed: %s\n", name, strings.TrimSpace(string(out)))
				} else {
					fmt.Printf("  %s: %s\n", name, strings.TrimSpace(string(out)))
					updated++
				}
			}

			fmt.Printf("\n%d skill(s) updated.\n", updated)
			return nil
		},
	}

	cmd.Flags().Bool("all", false, "update all installed skills")
	return cmd
}

func newSkillRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			skillsDir := getSkillsDir(cmd)
			targetDir := skillsDir + "/" + name

			if _, err := os.Stat(targetDir); os.IsNotExist(err) {
				return fmt.Errorf("skill %q not found in %s", name, skillsDir)
			}

			if err := os.RemoveAll(targetDir); err != nil {
				return fmt.Errorf("removing skill: %w", err)
			}

			fmt.Printf("Removed skill: %s\n", name)
			return nil
		},
	}
}

func newSkillInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show details about an installed skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, _, err := loadSkillRegistry(cmd)
			if err != nil {
				return err
			}

			skill, ok := registry.Get(args[0])
			if !ok {
				return fmt.Errorf("skill %q not found", args[0])
			}

			meta := skill.Metadata()
			fmt.Printf("Name:        %s\n", meta.Name)
			fmt.Printf("Version:     %s\n", meta.Version)
			fmt.Printf("Author:      %s\n", meta.Author)
			fmt.Printf("Category:    %s\n", meta.Category)
			fmt.Printf("Description: %s\n", meta.Description)
			if len(meta.Tags) > 0 {
				fmt.Printf("Tags:        %s\n", strings.Join(meta.Tags, ", "))
			}

			tools := skill.Tools()
			if len(tools) > 0 {
				fmt.Printf("\nTools (%d):\n", len(tools))
				for _, t := range tools {
					fmt.Printf("  - %s: %s\n", t.Name, t.Description)
				}
			}

			triggers := skill.Triggers()
			if len(triggers) > 0 {
				fmt.Printf("\nTriggers: %s\n", strings.Join(triggers, ", "))
			}

			if sp := skill.SystemPrompt(); sp != "" {
				fmt.Printf("\nSystem prompt:\n  %s\n", sp)
			}

			return nil
		},
	}
}
