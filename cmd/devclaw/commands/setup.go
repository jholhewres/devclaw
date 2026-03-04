package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newSetupCmd creates the `devclaw setup` command.
// The interactive CLI setup has been replaced by the web-based setup wizard.
func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Configure DevClaw via web setup wizard",
		Long: `The setup wizard is now web-based for full compatibility with
headless servers, containers, and automation tools like pm2/systemd.

Just run 'devclaw serve' — if no config.yaml exists, the web setup
wizard will start automatically at http://localhost:47716/setup`,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println()
			fmt.Println("╭────────────────────────────────────────────────╮")
			fmt.Println("│  DevClaw Setup is now web-based.               │")
			fmt.Println("│                                                │")
			fmt.Println("│  Run:  devclaw serve                          │")
			fmt.Println("│                                                │")
			fmt.Println("│  If no config.yaml exists, the setup wizard   │")
			fmt.Println("│  will start at http://localhost:47716/setup    │")
			fmt.Println("╰────────────────────────────────────────────────╯")
			fmt.Println()
		},
	}
}
