package commands

import (
	"fmt"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/updater"
	"github.com/spf13/cobra"
)

const defaultAssetsURL = "https://assets-gatorclaw.hostgator.io"

func newUpdateCmd(version string) *cobra.Command {
	var assetsURL string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for and install updates",
		Long: `Check for new DevClaw versions and install updates.

Examples:
  devclaw update           # Check and install interactively
  devclaw update check     # Only check for updates
  devclaw update install   # Install latest version
  devclaw update install --force  # Install without confirmation`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateInteractive(version, assetsURL)
		},
	}

	cmd.PersistentFlags().StringVar(&assetsURL, "assets-url", defaultAssetsURL, "override the assets URL")

	cmd.AddCommand(newUpdateCheckCmd(version, &assetsURL))
	cmd.AddCommand(newUpdateInstallCmd(version, &assetsURL))

	return cmd
}

func newUpdateCheckCmd(version string, assetsURL *string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check if a new version is available",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateCheck(version, *assetsURL)
		},
	}
}

func newUpdateInstallCmd(version string, assetsURL *string) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Download and install the latest version",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateInstall(version, *assetsURL, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation prompt")
	return cmd
}

func runUpdateCheck(version, assetsURL string) error {
	if version == "dev" {
		fmt.Println("Running a development build — update check skipped.")
		return nil
	}

	fmt.Printf("Current version: %s\n", version)
	fmt.Print("Checking for updates... ")

	checker := updater.NewChecker(version, assetsURL, time.Hour, nil)
	info, err := checker.CheckNow()
	if err != nil {
		fmt.Println("failed")
		return fmt.Errorf("update check failed: %w", err)
	}

	if info.Available {
		fmt.Printf("\nNew version available: %s (current: %s)\n", info.LatestVersion, info.CurrentVersion)
		fmt.Println("Run 'devclaw update install' to update.")
	} else {
		fmt.Printf("up to date (%s)\n", info.LatestVersion)
	}

	return nil
}

func runUpdateInstall(version, assetsURL string, force bool) error {
	if version == "dev" {
		fmt.Println("Running a development build — cannot install updates.")
		return nil
	}

	// Check first.
	checker := updater.NewChecker(version, assetsURL, time.Hour, nil)
	info, err := checker.CheckNow()
	if err != nil {
		return fmt.Errorf("update check failed: %w", err)
	}

	if !info.Available {
		fmt.Printf("Already up to date (%s).\n", version)
		return nil
	}

	fmt.Printf("Update available: %s -> %s\n", info.CurrentVersion, info.LatestVersion)

	if !force {
		fmt.Print("Do you want to proceed? [y/N] ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Update cancelled.")
			return nil
		}
	}

	fmt.Println("Downloading and installing...")
	inst := updater.NewInstaller(assetsURL, nil)
	if err := inst.Install(); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Printf("Successfully updated to %s.\n", info.LatestVersion)
	fmt.Println("Restart the service to apply: pm2 restart devclaw")
	return nil
}

func runUpdateInteractive(version, assetsURL string) error {
	if version == "dev" {
		fmt.Println("Running a development build — update not available.")
		return nil
	}

	fmt.Printf("Current version: %s\n", version)
	fmt.Print("Checking for updates... ")

	checker := updater.NewChecker(version, assetsURL, time.Hour, nil)
	info, err := checker.CheckNow()
	if err != nil {
		fmt.Println("failed")
		return fmt.Errorf("update check failed: %w", err)
	}

	if !info.Available {
		fmt.Printf("up to date (%s)\n", info.LatestVersion)
		return nil
	}

	fmt.Printf("\nNew version available: %s\n", info.LatestVersion)
	fmt.Print("Install now? [y/N] ")
	var answer string
	fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Update cancelled.")
		return nil
	}

	fmt.Println("Downloading and installing...")
	inst := updater.NewInstaller(assetsURL, nil)
	if err := inst.Install(); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Printf("Successfully updated to %s.\n", info.LatestVersion)
	fmt.Println("Restart the service to apply: pm2 restart devclaw")
	return nil
}
