// Package copilot – vault_tools.go implements individual vault tools.
// Each tool has a focused schema with only the parameters it needs.
package copilot

import (
	"context"
	"fmt"
	"strings"
)

// RegisterVaultTools registers individual vault tools.
// Replaces the old dispatcher pattern with focused tools:
// vault_status, vault_save, vault_get, vault_list, vault_delete.
func RegisterVaultTools(executor *ToolExecutor, vault *Vault) {

	// ── vault_status ──
	executor.RegisterHidden(
		MakeToolDefinition("vault_status",
			"Check the encrypted vault state: whether it exists, is locked/unlocked, and how many secrets are stored.",
			map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		func(_ context.Context, _ map[string]any) (any, error) {
			return handleVaultStatus(vault)
		},
	)

	// ── vault_save ──
	executor.RegisterHidden(
		MakeToolDefinition("vault_save",
			"Store a secret (API key, token, password) in the encrypted vault (AES-256-GCM + Argon2id). "+
				"Never store secrets in plain text files — always use this tool.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Secret name/key (e.g. OPENAI_API_KEY, SLACK_BOT_TOKEN)",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "Secret value to store",
					},
				},
				"required": []string{"name", "value"},
			}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleVaultSave(vault, args)
		},
	)

	// ── vault_get ──
	executor.RegisterHidden(
		MakeToolDefinition("vault_get",
			"Retrieve a secret value from the encrypted vault by name.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Secret name/key to retrieve",
					},
				},
				"required": []string{"name"},
			}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleVaultGet(vault, args)
		},
	)

	// ── vault_list ──
	executor.RegisterHidden(
		MakeToolDefinition("vault_list",
			"List all secret names stored in the encrypted vault (values are not shown).",
			map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		func(_ context.Context, _ map[string]any) (any, error) {
			return handleVaultList(vault)
		},
	)

	// ── vault_delete ──
	executor.RegisterHidden(
		MakeToolDefinition("vault_delete",
			"Remove a secret from the encrypted vault by name.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Secret name/key to delete",
					},
				},
				"required": []string{"name"},
			}),
		func(_ context.Context, args map[string]any) (any, error) {
			return handleVaultDelete(vault, args)
		},
	)
}

func handleVaultStatus(vault *Vault) (any, error) {
	status := map[string]any{
		"unlock_methods": []string{
			"Set DEVCLAW_VAULT_PASSWORD environment variable",
			"Run 'devclaw vault unlock'",
		},
	}

	if vault == nil {
		status["available"] = false
		status["exists"] = false
		status["locked"] = true
		status["secret_count"] = 0
		status["path"] = ""
		status["message"] = "Vault not configured. Run 'devclaw vault init' to create one."
		return status, nil
	}

	exists := vault.Exists()
	unlocked := vault.IsUnlocked()
	status["available"] = unlocked
	status["exists"] = exists
	status["locked"] = !unlocked
	status["path"] = vault.Path()

	if unlocked {
		keys, _ := vault.Keys()
		status["secret_count"] = len(keys)
		status["message"] = fmt.Sprintf("Vault unlocked with %d secrets.", len(keys))
	} else if exists {
		status["secret_count"] = 0
		status["message"] = "Vault exists but is locked. Unlock with DEVCLAW_VAULT_PASSWORD or 'devclaw vault unlock'."
	} else {
		status["secret_count"] = 0
		status["message"] = "Vault not initialized. Run 'devclaw vault init' to create one."
	}

	return status, nil
}

func handleVaultSave(vault *Vault, args map[string]any) (any, error) {
	if vault == nil {
		return nil, fmt.Errorf("vault not available — run 'devclaw vault init' to create one")
	}
	name, _ := args["name"].(string)
	value, _ := args["value"].(string)
	if name == "" || value == "" {
		return nil, fmt.Errorf("name and value are required")
	}
	if !vault.IsUnlocked() {
		return nil, fmt.Errorf("vault is locked — set DEVCLAW_VAULT_PASSWORD or run 'devclaw vault unlock'")
	}
	if err := vault.Set(name, value); err != nil {
		return nil, fmt.Errorf("failed to save to vault: %w", err)
	}
	return fmt.Sprintf("Secret '%s' saved to encrypted vault.", name), nil
}

func handleVaultGet(vault *Vault, args map[string]any) (any, error) {
	if vault == nil {
		return nil, fmt.Errorf("vault not available — run 'devclaw vault init' to create one")
	}
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	val, err := vault.Get(name)
	if err != nil {
		return nil, fmt.Errorf("failed to read from vault: %w", err)
	}
	if val == "" {
		return fmt.Sprintf("Secret '%s' not found in vault.", name), nil
	}
	return val, nil
}

func handleVaultList(vault *Vault) (any, error) {
	if vault == nil {
		return nil, fmt.Errorf("vault not available — run 'devclaw vault init' to create one")
	}
	names, err := vault.Keys()
	if err != nil {
		return nil, fmt.Errorf("failed to list vault keys: %w", err)
	}
	if len(names) == 0 {
		return "Vault is empty.", nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Vault contains %d secrets:\n", len(names))
	for _, name := range names {
		fmt.Fprintf(&sb, "- %s\n", name)
	}
	return sb.String(), nil
}

func handleVaultDelete(vault *Vault, args map[string]any) (any, error) {
	if vault == nil {
		return nil, fmt.Errorf("vault not available — run 'devclaw vault init' to create one")
	}
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := vault.Delete(name); err != nil {
		return nil, fmt.Errorf("failed to delete from vault: %w", err)
	}
	return fmt.Sprintf("Secret '%s' removed from vault.", name), nil
}
