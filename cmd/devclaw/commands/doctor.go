package commands

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
	"github.com/spf13/cobra"
)

// doctorCheck represents a single diagnostic check.
type doctorCheck struct {
	Name    string
	Status  string // "ok", "warn", "fail", "fixed"
	Message string
}

// newDoctorCmd creates the `devclaw doctor` command.
func newDoctorCmd() *cobra.Command {
	var fix bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose common configuration and environment issues",
		Long: `Runs diagnostic checks on your DevClaw installation and reports
issues with configuration, dependencies, and environment setup.

Use --fix to attempt automatic remediation of common problems.

Examples:
  devclaw doctor
  devclaw doctor --fix`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd, fix)
		},
	}

	cmd.Flags().BoolVar(&fix, "fix", false, "attempt to auto-fix detected issues")
	return cmd
}

func runDoctor(cmd *cobra.Command, fix bool) error {
	fmt.Println("DevClaw Doctor")
	fmt.Println("==============")
	fmt.Println()

	var checks []doctorCheck

	// 1. Config file.
	checks = append(checks, checkConfigFile(cmd, fix)...)

	// 2. API key / secrets.
	checks = append(checks, checkAPIKeys()...)

	// 3. Dependencies.
	checks = append(checks, checkDependencies()...)

	// 4. Vault.
	checks = append(checks, checkVault()...)

	// 5. File permissions.
	checks = append(checks, checkPermissions()...)

	// Print results.
	fmt.Println()
	okCount, warnCount, failCount, fixedCount := 0, 0, 0, 0
	for _, c := range checks {
		icon := "✓"
		switch c.Status {
		case "ok":
			icon = "✓"
			okCount++
		case "warn":
			icon = "⚠"
			warnCount++
		case "fail":
			icon = "✗"
			failCount++
		case "fixed":
			icon = "✔"
			fixedCount++
		}
		fmt.Printf("  %s %s: %s\n", icon, c.Name, c.Message)
	}

	fmt.Println()
	fmt.Printf("Results: %d ok, %d warnings, %d failures", okCount, warnCount, failCount)
	if fixedCount > 0 {
		fmt.Printf(", %d fixed", fixedCount)
	}
	fmt.Println()

	if failCount > 0 {
		return fmt.Errorf("%d check(s) failed", failCount)
	}
	return nil
}

func checkConfigFile(cmd *cobra.Command, fix bool) []doctorCheck {
	var checks []doctorCheck

	configPath, _ := cmd.Root().PersistentFlags().GetString("config")
	if configPath == "" {
		configPath = copilot.FindConfigFile()
	}

	if configPath == "" {
		status := "fail"
		msg := "no config file found (run 'devclaw config init')"
		if fix {
			cfg := copilot.DefaultConfig()
			if err := copilot.SaveConfigToFile(cfg, "config.yaml"); err == nil {
				status = "fixed"
				msg = "created default config.yaml"
			}
		}
		return append(checks, doctorCheck{"Config file", status, msg})
	}

	checks = append(checks, doctorCheck{"Config file", "ok", configPath})

	// Validate config contents.
	cfg, err := copilot.LoadConfigFromFile(configPath)
	if err != nil {
		return append(checks, doctorCheck{"Config parse", "fail", err.Error()})
	}
	checks = append(checks, doctorCheck{"Config parse", "ok", "valid YAML"})

	// Check essential fields.
	if cfg.Model == "" {
		checks = append(checks, doctorCheck{"Config model", "warn", "no model specified (will use default)"})
	} else {
		checks = append(checks, doctorCheck{"Config model", "ok", cfg.Model})
	}

	if cfg.API.BaseURL == "" {
		checks = append(checks, doctorCheck{"Config API URL", "warn", "no base_url set (will use OpenAI default)"})
	} else {
		checks = append(checks, doctorCheck{"Config API URL", "ok", cfg.API.BaseURL})
	}

	// Check for deprecated fields.
	checks = append(checks, checkDeprecatedFields(configPath)...)

	return checks
}

func checkDeprecatedFields(configPath string) []doctorCheck {
	var checks []doctorCheck

	data, err := os.ReadFile(configPath)
	if err != nil {
		return checks
	}

	content := string(data)
	deprecated := map[string]string{
		"openai_key":        "use api.api_key or vault instead",
		"whatsapp_number":   "use channels.whatsapp.phone instead",
		"gpt_model":         "use model instead",
		"max_tokens":        "use agent.max_output_tokens instead",
		"system_prompt":     "use instructions instead",
	}

	for field, suggestion := range deprecated {
		if strings.Contains(content, field+":") {
			checks = append(checks, doctorCheck{
				"Deprecated field",
				"warn",
				fmt.Sprintf("%q found — %s", field, suggestion),
			})
		}
	}
	return checks
}

func checkAPIKeys() []doctorCheck {
	var checks []doctorCheck

	// Check common API key env vars.
	keyVars := []string{"DEVCLAW_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"}
	found := false
	for _, v := range keyVars {
		if os.Getenv(v) != "" {
			found = true
			checks = append(checks, doctorCheck{"API key (" + v + ")", "ok", "set"})
		}
	}

	if !found {
		// Check if vault exists — key might be there.
		vault := copilot.NewVault(copilot.VaultFile)
		if vault.Exists() {
			checks = append(checks, doctorCheck{"API key", "ok", "vault available (key may be stored there)"})
		} else if copilot.KeyringAvailable() {
			if copilot.GetKeyring("api_key") != "" {
				checks = append(checks, doctorCheck{"API key", "ok", "found in OS keyring"})
			} else {
				checks = append(checks, doctorCheck{"API key", "warn", "no API key found in env, keyring, or vault"})
			}
		} else {
			checks = append(checks, doctorCheck{"API key", "warn", "no API key found in env or vault"})
		}
	}

	return checks
}

func checkDependencies() []doctorCheck {
	var checks []doctorCheck

	// Go version.
	checks = append(checks, doctorCheck{"Go runtime", "ok", runtime.Version()})

	// Optional tools.
	optionalTools := []string{"git", "node", "npm"}
	for _, tool := range optionalTools {
		if path, err := exec.LookPath(tool); err == nil {
			checks = append(checks, doctorCheck{tool, "ok", path})
		} else {
			checks = append(checks, doctorCheck{tool, "warn", "not found (optional)"})
		}
	}

	return checks
}

func checkVault() []doctorCheck {
	var checks []doctorCheck

	vault := copilot.NewVault(copilot.VaultFile)
	if vault.Exists() {
		checks = append(checks, doctorCheck{"Vault", "ok", copilot.VaultFile + " exists"})
	} else {
		checks = append(checks, doctorCheck{"Vault", "warn",
			"not created (run 'devclaw config vault-init' for encrypted secret storage)"})
	}

	return checks
}

func checkPermissions() []doctorCheck {
	var checks []doctorCheck

	// Check config file permissions.
	configPath := copilot.FindConfigFile()
	if configPath != "" {
		info, err := os.Stat(configPath)
		if err == nil {
			mode := info.Mode().Perm()
			if mode&0o044 != 0 {
				checks = append(checks, doctorCheck{
					"Config permissions",
					"warn",
					fmt.Sprintf("%04o (recommend 0600, run: chmod 600 %s)", mode, configPath),
				})
			} else {
				checks = append(checks, doctorCheck{
					"Config permissions",
					"ok",
					fmt.Sprintf("%04o", mode),
				})
			}
		}
	}

	// Check vault permissions.
	if info, err := os.Stat(copilot.VaultFile); err == nil {
		mode := info.Mode().Perm()
		if mode&0o077 != 0 {
			checks = append(checks, doctorCheck{
				"Vault permissions",
				"warn",
				fmt.Sprintf("%04o (recommend 0600, run: chmod 600 %s)", mode, copilot.VaultFile),
			})
		} else {
			checks = append(checks, doctorCheck{
				"Vault permissions",
				"ok",
				fmt.Sprintf("%04o", mode),
			})
		}
	}

	return checks
}
