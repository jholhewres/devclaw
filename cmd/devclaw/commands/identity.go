package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// defaultIdentityTemplate is written to identity.md on first invocation when
// the file does not exist. Content is generic English — users are expected to
// customise it for their own context.
const defaultIdentityTemplate = `# DevClaw Identity

You are an AI assistant.

## Communication style
- Concise and direct
- Honest about uncertainty

## Background
(Add context about yourself, your role, and what matters to you here.)
`

// newIdentityCmd creates the `devclaw identity` command group.
func newIdentityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "identity",
		Short: "Manage the L0 identity fragment",
		Long: `Manage the L0 identity fragment — a short markdown file that anchors the
assistant's identity across conversations.

The identity file is loaded at startup and injected into every prompt. Edit it
to customise the assistant's persona, communication style, or background context.

Default location: ~/.devclaw/identity.md`,
	}

	cmd.AddCommand(newIdentityEditCmd())
	return cmd
}

// newIdentityEditCmd creates the `devclaw identity edit` subcommand.
func newIdentityEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open the identity file in $EDITOR",
		Long: `Open the identity file in your preferred editor ($VISUAL, $EDITOR, or vi).

On first invocation, if the file does not exist, a default template is written
before opening the editor. The file path can be overridden via the
memory.hierarchy.identity_path config field.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := resolveIdentityPath()
			if err != nil {
				return fmt.Errorf("identity edit: resolve path: %w", err)
			}

			if err := ensureIdentityFile(path); err != nil {
				return fmt.Errorf("identity edit: create default file: %w", err)
			}

			editor := resolveEditor()
			c := exec.Command(editor, path)
			c.Stdin = cmd.InOrStdin()
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.ErrOrStderr()
			if err := c.Run(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					// Propagate the editor's exit code.
					os.Exit(exitErr.ExitCode())
				}
				return fmt.Errorf("identity edit: editor exited with error: %w", err)
			}
			return nil
		},
	}
}

// resolveIdentityPath returns the absolute path to identity.md.
// It uses $HOME/.devclaw/identity.md. Room 2.4 will thread the configured
// IdentityPath through here when the config is available at CLI parse time.
func resolveIdentityPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".devclaw", "identity.md"), nil
}

// ensureIdentityFile creates the identity file with a default template if it
// does not exist. The parent directory is created as needed.
func ensureIdentityFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultIdentityTemplate), 0o644)
}

// resolveEditor returns the editor binary to use. Preference order:
// $VISUAL, $EDITOR, "vi".
func resolveEditor() string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "vi"
}
