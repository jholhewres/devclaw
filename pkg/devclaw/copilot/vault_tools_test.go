package copilot

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// TestVaultToolsWithNilVault verifies that vault tools gracefully handle nil vault.
func TestVaultToolsWithNilVault(t *testing.T) {
	executor := NewToolExecutor(slog.Default())

	// Register vault tools with nil vault
	registerVaultTools(executor, nil)

	t.Run("vault_status with nil vault", func(t *testing.T) {
		tool, ok := executor.tools["vault_status"]
		if !ok {
			t.Fatal("vault_status tool not registered")
		}

		result, err := tool.Handler(context.Background(), map[string]any{})
		if err != nil {
			t.Fatalf("vault_status should not error with nil vault: %v", err)
		}

		status, ok := result.(map[string]any)
		if !ok {
			t.Fatal("vault_status should return map[string]any")
		}

		if status["available"].(bool) {
			t.Error("vault_status should show available=false with nil vault")
		}
		if status["exists"].(bool) {
			t.Error("vault_status should show exists=false with nil vault")
		}
		if !status["locked"].(bool) {
			t.Error("vault_status should show locked=true with nil vault")
		}

		// Verify unlock_methods is present
		methods, ok := status["unlock_methods"].([]string)
		if !ok || len(methods) == 0 {
			t.Error("vault_status should include unlock_methods")
		}
	})

	t.Run("vault_save with nil vault", func(t *testing.T) {
		tool, ok := executor.tools["vault_save"]
		if !ok {
			t.Fatal("vault_save tool not registered")
		}

		_, err := tool.Handler(context.Background(), map[string]any{
			"name":  "TEST_KEY",
			"value": "test_value",
		})

		if err == nil {
			t.Error("vault_save should error with nil vault")
		}

		if err != nil && !strings.Contains(err.Error(), "vault not available") {
			t.Errorf("error should mention 'vault not available', got: %v", err)
		}
	})

	t.Run("vault_get with nil vault", func(t *testing.T) {
		tool, ok := executor.tools["vault_get"]
		if !ok {
			t.Fatal("vault_get tool not registered")
		}

		_, err := tool.Handler(context.Background(), map[string]any{
			"name": "TEST_KEY",
		})

		if err == nil {
			t.Error("vault_get should error with nil vault")
		}
	})

	t.Run("vault_list with nil vault", func(t *testing.T) {
		tool, ok := executor.tools["vault_list"]
		if !ok {
			t.Fatal("vault_list tool not registered")
		}

		_, err := tool.Handler(context.Background(), map[string]any{})

		if err == nil {
			t.Error("vault_list should error with nil vault")
		}
	})

	t.Run("vault_delete with nil vault", func(t *testing.T) {
		tool, ok := executor.tools["vault_delete"]
		if !ok {
			t.Fatal("vault_delete tool not registered")
		}

		_, err := tool.Handler(context.Background(), map[string]any{
			"name": "TEST_KEY",
		})

		if err == nil {
			t.Error("vault_delete should error with nil vault")
		}
	})
}

// TestVaultToolsWithLockedVault verifies that vault tools gracefully handle locked vault.
func TestVaultToolsWithLockedVault(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")

	// Create vault (unlocked by default) then lock it
	vault := NewVault(vaultPath)
	if err := vault.Create("test-password"); err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}
	// Explicitly lock the vault for testing
	vault.Lock()

	executor := NewToolExecutor(slog.Default())
	registerVaultTools(executor, vault)

	t.Run("vault_status with locked vault", func(t *testing.T) {
		tool := executor.tools["vault_status"]
		result, err := tool.Handler(context.Background(), map[string]any{})
		if err != nil {
			t.Fatalf("vault_status should not error: %v", err)
		}

		status := result.(map[string]any)
		if status["available"].(bool) {
			t.Error("vault_status should show available=false when locked")
		}
		if !status["exists"].(bool) {
			t.Error("vault_status should show exists=true")
		}
		if !status["locked"].(bool) {
			t.Error("vault_status should show locked=true")
		}
	})

	t.Run("vault_save with locked vault", func(t *testing.T) {
		tool := executor.tools["vault_save"]
		_, err := tool.Handler(context.Background(), map[string]any{
			"name":  "TEST_KEY",
			"value": "test_value",
		})

		if err == nil {
			t.Error("vault_save should error with locked vault")
		}

		if err != nil && !strings.Contains(err.Error(), "vault is locked") {
			t.Errorf("error should mention 'vault is locked', got: %v", err)
		}

		// Should provide unlock instructions
		if err != nil && !strings.Contains(err.Error(), "DEVCLAW_VAULT_PASSWORD") {
			t.Errorf("error should mention DEVCLAW_VAULT_PASSWORD, got: %v", err)
		}
	})

	t.Run("vault_get with locked vault", func(t *testing.T) {
		tool := executor.tools["vault_get"]
		_, err := tool.Handler(context.Background(), map[string]any{
			"name": "TEST_KEY",
		})

		if err == nil {
			t.Error("vault_get should error with locked vault")
		}
	})

	t.Run("vault_list with locked vault", func(t *testing.T) {
		tool := executor.tools["vault_list"]
		_, err := tool.Handler(context.Background(), map[string]any{})

		if err == nil {
			t.Error("vault_list should error with locked vault")
		}
	})

	t.Run("vault_delete with locked vault", func(t *testing.T) {
		tool := executor.tools["vault_delete"]
		_, err := tool.Handler(context.Background(), map[string]any{
			"name": "TEST_KEY",
		})

		if err == nil {
			t.Error("vault_delete should error with locked vault")
		}
	})
}

// TestVaultToolsWithUnlockedVault verifies that vault tools work when unlocked.
func TestVaultToolsWithUnlockedVault(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")

	// Create and unlock vault
	vault := NewVault(vaultPath)
	if err := vault.Create("test-password"); err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}
	if err := vault.Unlock("test-password"); err != nil {
		t.Fatalf("failed to unlock vault: %v", err)
	}

	executor := NewToolExecutor(slog.Default())
	registerVaultTools(executor, vault)

	t.Run("vault_status with unlocked vault", func(t *testing.T) {
		tool := executor.tools["vault_status"]
		result, err := tool.Handler(context.Background(), map[string]any{})
		if err != nil {
			t.Fatalf("vault_status should not error: %v", err)
		}

		status := result.(map[string]any)
		if !status["available"].(bool) {
			t.Error("vault_status should show available=true when unlocked")
		}
		if !status["exists"].(bool) {
			t.Error("vault_status should show exists=true")
		}
		if status["locked"].(bool) {
			t.Error("vault_status should show locked=false when unlocked")
		}
	})

	t.Run("vault_save and vault_get", func(t *testing.T) {
		// Save
		saveTool := executor.tools["vault_save"]
		result, err := saveTool.Handler(context.Background(), map[string]any{
			"name":  "MY_API_KEY",
			"value": "sk-test12345",
		})
		if err != nil {
			t.Fatalf("vault_save should succeed when unlocked: %v", err)
		}

		if !strings.Contains(result.(string), "saved") {
			t.Errorf("expected success message, got: %v", result)
		}

		// Get
		getTool := executor.tools["vault_get"]
		val, err := getTool.Handler(context.Background(), map[string]any{
			"name": "MY_API_KEY",
		})
		if err != nil {
			t.Fatalf("vault_get should succeed: %v", err)
		}

		if val != "sk-test12345" {
			t.Errorf("expected 'sk-test12345', got: %v", val)
		}
	})

	t.Run("vault_list shows secrets", func(t *testing.T) {
		tool := executor.tools["vault_list"]
		result, err := tool.Handler(context.Background(), map[string]any{})
		if err != nil {
			t.Fatalf("vault_list should succeed: %v", err)
		}

		if !strings.Contains(result.(string), "MY_API_KEY") {
			t.Errorf("vault_list should include MY_API_KEY, got: %v", result)
		}
	})

	t.Run("vault_delete removes secret", func(t *testing.T) {
		// Delete
		deleteTool := executor.tools["vault_delete"]
		_, err := deleteTool.Handler(context.Background(), map[string]any{
			"name": "MY_API_KEY",
		})
		if err != nil {
			t.Fatalf("vault_delete should succeed: %v", err)
		}

		// Verify it's gone
		getTool := executor.tools["vault_get"]
		result, _ := getTool.Handler(context.Background(), map[string]any{
			"name": "MY_API_KEY",
		})

		if !strings.Contains(result.(string), "not found") {
			t.Errorf("expected 'not found' after delete, got: %v", result)
		}
	})
}

// TestVaultStatusJSONStructure verifies that vault_status returns properly structured JSON.
func TestVaultStatusJSONStructure(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "test.vault")

	vault := NewVault(vaultPath)
	vault.Create("password")
	vault.Unlock("password")
	vault.Set("test_key", "test_value")

	executor := NewToolExecutor(slog.Default())
	registerVaultTools(executor, vault)

	tool := executor.tools["vault_status"]
	result, err := tool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("vault_status failed: %v", err)
	}

	status := result.(map[string]any)

	// Verify all expected fields exist
	requiredFields := []string{"available", "exists", "locked", "secret_count", "path", "message"}
	for _, field := range requiredFields {
		if _, ok := status[field]; !ok {
			t.Errorf("vault_status missing field: %s", field)
		}
	}

	// Verify types
	if _, ok := status["available"].(bool); !ok {
		t.Error("available should be bool")
	}
	if _, ok := status["exists"].(bool); !ok {
		t.Error("exists should be bool")
	}
	if _, ok := status["locked"].(bool); !ok {
		t.Error("locked should be bool")
	}
	if _, ok := status["secret_count"].(int); !ok {
		t.Error("secret_count should be int")
	}
	if _, ok := status["path"].(string); !ok {
		t.Error("path should be string")
	}
	if _, ok := status["message"].(string); !ok {
		t.Error("message should be string")
	}

	// Test JSON serialization
	jsonBytes, err := json.Marshal(status)
	if err != nil {
		t.Errorf("vault_status result should be JSON serializable: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Errorf("vault_status JSON should be decodable: %v", err)
	}
}

// TestVaultStatusWithNonExistentVault verifies status when vault file doesn't exist.
func TestVaultStatusWithNonExistentVault(t *testing.T) {
	tmpDir := t.TempDir()
	vaultPath := filepath.Join(tmpDir, "nonexistent.vault")

	// Point to non-existent vault
	vault := NewVault(vaultPath)

	executor := NewToolExecutor(slog.Default())
	registerVaultTools(executor, vault)

	tool := executor.tools["vault_status"]
	result, err := tool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("vault_status should not error: %v", err)
	}

	status := result.(map[string]any)

	if status["exists"].(bool) {
		t.Error("exists should be false for non-existent vault")
	}
	if status["available"].(bool) {
		t.Error("available should be false when vault doesn't exist")
	}
}

