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

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
	"github.com/jholhewres/devclaw/pkg/devclaw/plugins"
	"github.com/spf13/cobra"
)

// newPluginCmd creates the `devclaw plugin` command for managing plugins.
func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage plugins",
		Long: `Manage DevClaw plugins. Install from GitHub repositories or local paths.

Sources:
  devclaw plugin install user/repo                     # GitHub shorthand
  devclaw plugin install https://github.com/user/repo  # GitHub URL
  devclaw plugin install ./my-local-plugin             # Local path

Other:
  devclaw plugin list                                  # List installed plugins
  devclaw plugin remove <name>                         # Remove a plugin
  devclaw plugin update --all                          # Update all git-based plugins`,
	}

	cmd.AddCommand(
		newPluginInstallCmd(),
		newPluginListCmd(),
		newPluginRemoveCmd(),
		newPluginUpdateCmd(),
	)

	return cmd
}

// getPluginsDir returns the first configured plugins directory.
func getPluginsDir(cmd *cobra.Command) string {
	cfg, _, err := loadConfig(cmd)
	if err == nil {
		dirs := cfg.Plugins.EffectiveDirs()
		if len(dirs) > 0 {
			return dirs[0]
		}
	}
	return "./plugins"
}

func newPluginInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <source>",
		Short: "Install a plugin from GitHub or local path",
		Long: `Install a plugin from various sources. The source type is auto-detected:

  GitHub shorthand: user/repo or github:user/repo
  GitHub URL:       https://github.com/user/repo
  Local path:       ./my-plugin or /path/to/plugin

The plugin must contain a valid plugin.yaml manifest.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			pluginsDir := getPluginsDir(cmd)

			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
			installer := plugins.NewPluginInstaller(pluginsDir, logger)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			result, err := installer.Install(ctx, source)
			if err != nil {
				return fmt.Errorf("install failed: %w", err)
			}

			fmt.Println()
			if result.IsNew {
				fmt.Printf("Installed: %s (%s)\n", result.Name, result.ID)
			} else {
				fmt.Printf("Updated: %s (%s)\n", result.Name, result.ID)
			}
			fmt.Printf("  Version: %s\n", result.Version)
			fmt.Printf("  Path:    %s\n", result.Path)
			fmt.Printf("  Source:  %s\n", result.Source)
			fmt.Println()
			fmt.Println("Restart DevClaw to load the plugin.")

			return nil
		},
	}
}

func newPluginListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, err := loadConfig(cmd)
			if err != nil {
				cfg = copilot.DefaultConfig()
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
			loader := plugins.NewLoader(cfg.Plugins, logger)

			discovered, err := loader.Discover()
			if err != nil {
				return fmt.Errorf("discovery failed: %w", err)
			}

			if len(discovered) == 0 {
				fmt.Println("No plugins found.")
				fmt.Println()
				fmt.Println("Install one with:")
				fmt.Println("  devclaw plugin install user/repo")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tNAME\tVERSION\tDIR\n")
			fmt.Fprintf(w, "──\t────\t───────\t───\n")
			for _, inst := range discovered {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					inst.Manifest.ID, inst.Manifest.Name, inst.Manifest.Version, inst.Dir)
			}
			w.Flush()
			fmt.Printf("\n%d plugin(s) found.\n", len(discovered))
			return nil
		},
	}
}

func newPluginRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			pluginsDir := getPluginsDir(cmd)

			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
			installer := plugins.NewPluginInstaller(pluginsDir, logger)

			if err := installer.Remove(name); err != nil {
				return err
			}

			fmt.Printf("Removed plugin: %s\n", name)
			fmt.Println("Restart DevClaw to apply changes.")
			return nil
		},
	}
}

func newPluginUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [name]",
		Short: "Update installed plugins (git-based)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			pluginsDir := getPluginsDir(cmd)

			if !all && len(args) == 0 {
				return fmt.Errorf("specify a plugin name or use --all")
			}

			entries, err := os.ReadDir(pluginsDir)
			if err != nil {
				return fmt.Errorf("reading plugins directory: %w", err)
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

				pluginDir := pluginsDir + "/" + name

				// Check if it's a git repo.
				if _, err := os.Stat(pluginDir + "/.git"); err != nil {
					if !all {
						fmt.Printf("  %s: not a git repo, skipping\n", name)
					}
					continue
				}

				fmt.Printf("Updating %s...\n", name)
				gitCmd := exec.CommandContext(ctx, "git", "-C", pluginDir, "pull", "--ff-only")
				out, err := gitCmd.CombinedOutput()
				if err != nil {
					fmt.Printf("  %s: update failed: %s\n", name, strings.TrimSpace(string(out)))
				} else {
					fmt.Printf("  %s: %s\n", name, strings.TrimSpace(string(out)))
					updated++
				}
			}

			fmt.Printf("\n%d plugin(s) updated.\n", updated)
			return nil
		},
	}

	cmd.Flags().Bool("all", false, "update all installed plugins")
	return cmd
}
