package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newSetupCmd creates the `copilot setup` command.
// The interactive CLI setup has been replaced by the web-based setup wizard.
// This command now just informs the user to use the web UI.
func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Configure DevClaw via web setup wizard",
		Long: `The setup wizard is now web-based for full compatibility with
headless servers, containers, and automation tools like pm2/systemd.

Just run 'copilot serve' — if no config.yaml exists, the web setup
wizard will start automatically at http://localhost:8090/setup`,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println()
			fmt.Println("╭────────────────────────────────────────────────╮")
			fmt.Println("│  DevClaw Setup is now web-based.                │")
			fmt.Println("│                                                │")
			fmt.Println("│  Run:  copilot serve                          │")
			fmt.Println("│                                                │")
			fmt.Println("│  If no config.yaml exists, the setup wizard   │")
			fmt.Println("│  will start at http://localhost:8090/setup     │")
			fmt.Println("╰────────────────────────────────────────────────╯")
			fmt.Println()
		},
	}
}
