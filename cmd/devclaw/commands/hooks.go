package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newHooksCmd creates the `devclaw hooks` command group.
func newHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Manage lifecycle hooks",
		Long: `Inspect and manage lifecycle hooks configured in config.yaml.

Examples:
  devclaw hooks list
  devclaw hooks list --json
  devclaw hooks check
  devclaw hooks enable my-hook
  devclaw hooks disable my-hook`,
	}

	cmd.AddCommand(
		newHooksListCmd(),
		newHooksCheckCmd(),
		newHooksEnableCmd(),
		newHooksDisableCmd(),
	)

	return cmd
}

// newHooksListCmd creates the `devclaw hooks list` subcommand.
func newHooksListCmd() *cobra.Command {
	var jsonOutput bool
	var eligible bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured hooks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, err := loadConfig(cmd)
			if err != nil {
				return err
			}

			hooks := cfg.Hooks
			if !hooks.Enabled {
				fmt.Println("Hooks system is disabled. Set hooks.enabled: true in config.yaml")
				return nil
			}

			// Build a combined list of handlers and webhooks.
			type hookEntry struct {
				Name    string `json:"name"`
				Type    string `json:"type"`
				Event   string `json:"event"`
				Enabled bool   `json:"enabled"`
				Action  string `json:"action,omitempty"`
				URL     string `json:"url,omitempty"`
			}

			var entries []hookEntry

			for i, h := range hooks.Handlers {
				name := fmt.Sprintf("handler-%d", i+1)
				if eligible && !h.Enabled {
					continue
				}
				entries = append(entries, hookEntry{
					Name:    name,
					Type:    "handler",
					Event:   h.Event,
					Enabled: h.Enabled,
					Action:  h.Action,
				})
			}

			for i, w := range hooks.Webhooks {
				name := w.Name
				if name == "" {
					name = fmt.Sprintf("webhook-%d", i+1)
				}
				if eligible && !w.Enabled {
					continue
				}
				entries = append(entries, hookEntry{
					Name:    name,
					Type:    "webhook",
					Event:   strings.Join(eventsToStrings(w.Events), ","),
					Enabled: w.Enabled,
					URL:     w.URL,
				})
			}

			if jsonOutput {
				data, _ := json.MarshalIndent(entries, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(entries) == 0 {
				fmt.Println("No hooks configured.")
				return nil
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tTYPE\tEVENT\tENABLED\tACTION/URL")
			for _, e := range entries {
				target := e.Action
				if e.URL != "" {
					target = e.URL
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%s\n", e.Name, e.Type, e.Event, e.Enabled, target)
			}
			tw.Flush()

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	cmd.Flags().BoolVar(&eligible, "eligible", false, "show only enabled hooks")
	return cmd
}

// newHooksCheckCmd validates hook configurations.
func newHooksCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate hook configurations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, err := loadConfig(cmd)
			if err != nil {
				return err
			}

			hooks := cfg.Hooks
			issues := 0

			if !hooks.Enabled {
				fmt.Println("WARNING: hooks system is disabled")
				issues++
			}

			// Check handlers.
			for i, h := range hooks.Handlers {
				prefix := fmt.Sprintf("handler[%d]", i)
				if h.Event == "" {
					fmt.Printf("ERROR: %s has no event\n", prefix)
					issues++
				}
				if h.Action == "" {
					fmt.Printf("ERROR: %s has no action\n", prefix)
					issues++
				}
			}

			// Check webhooks.
			for i, w := range hooks.Webhooks {
				prefix := fmt.Sprintf("webhook[%d]", i)
				if w.URL == "" {
					fmt.Printf("ERROR: %s has no URL\n", prefix)
					issues++
				}
				if len(w.Events) == 0 {
					fmt.Printf("WARNING: %s has no events (will never fire)\n", prefix)
					issues++
				}
			}

			if issues == 0 {
				fmt.Printf("All %d hooks are valid.\n",
					len(hooks.Handlers)+len(hooks.Webhooks))
			} else {
				return fmt.Errorf("%d issue(s) found", issues)
			}
			return nil
		},
	}
}

// newHooksEnableCmd enables a hook by name in the config file.
func newHooksEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a hook by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return toggleHookInConfig(cmd, args[0], true)
		},
	}
}

// newHooksDisableCmd disables a hook by name in the config file.
func newHooksDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a hook by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return toggleHookInConfig(cmd, args[0], false)
		},
	}
}

// toggleHookInConfig enables/disables a webhook by name in the config file.
// Note: This performs a read-modify-write cycle without file-level locking.
// Acceptable for CLI (single user) but concurrent config writes may lose changes.
func toggleHookInConfig(cmd *cobra.Command, name string, enabled bool) error {
	configPath, _ := cmd.Root().PersistentFlags().GetString("config")
	if configPath == "" {
		configPath = copilot.FindConfigFile()
	}
	if configPath == "" {
		return fmt.Errorf("no config file found")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	// Navigate to hooks.webhooks and find the webhook by name.
	hooksNode := yamlGetPath(&doc, "hooks")
	if hooksNode == nil {
		return fmt.Errorf("no hooks section in config")
	}

	found := false

	// Search in webhooks by name field.
	webhooksNode := yamlGetPath(&doc, "hooks.webhooks")
	if webhooksNode != nil && webhooksNode.Kind == yaml.SequenceNode {
		for _, item := range webhooksNode.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			for i := 0; i < len(item.Content)-1; i += 2 {
				if item.Content[i].Value == "name" && item.Content[i+1].Value == name {
					// Found it — set enabled field.
					setMappingField(item, "enabled", fmt.Sprintf("%v", enabled))
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	if !found {
		return fmt.Errorf("hook %q not found in webhooks", name)
	}

	// Backup and save.
	bakPath := configPath + ".bak"
	_ = os.WriteFile(bakPath, data, 0o600)

	out, err := yaml.Marshal(doc.Content[0])
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(configPath, out, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	action := "enabled"
	if !enabled {
		action = "disabled"
	}
	fmt.Printf("Hook %q %s.\n", name, action)
	return nil
}

// setMappingField sets or adds a scalar field in a YAML mapping node.
func setMappingField(mapping *yaml.Node, key, value string) {
	tag := yamlAutoTag(value)
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1].Value = value
			mapping.Content[i+1].Tag = tag
			return
		}
	}
	// Key not found — add it.
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value, Tag: tag},
	)
}

// eventsToStrings converts WebhookConfig events to strings.
func eventsToStrings(events []string) []string {
	if len(events) == 0 {
		return []string{"*"}
	}
	return events
}
