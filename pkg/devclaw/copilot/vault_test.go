package copilot

import (
	"path/filepath"
	"sync"
	"testing"
)

func newTestVault(t *testing.T) (*Vault, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.vault")
	return NewVault(path), path
}

func TestVault_CreateAndExists(t *testing.T) {
	t.Parallel()
	v, _ := newTestVault(t)

	if v.Exists() {
		t.Fatal("vault should not exist before Create")
	}
	if err := v.Create("pass123"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !v.Exists() {
		t.Fatal("vault should exist after Create")
	}
	if !v.IsUnlocked() {
		t.Fatal("vault should be unlocked after Create")
	}
}

func TestVault_CreateDuplicate(t *testing.T) {
	t.Parallel()
	v, _ := newTestVault(t)

	if err := v.Create("pass"); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := v.Create("pass"); err == nil {
		t.Error("second Create should return error")
	}
}

func TestVault_UnlockCorrectPassword(t *testing.T) {
	t.Parallel()
	v, path := newTestVault(t)
	if err := v.Create("correct"); err != nil {
		t.Fatal(err)
	}
	v.Lock()

	v2 := NewVault(path)
	if err := v2.Unlock("correct"); err != nil {
		t.Fatalf("Unlock with correct password: %v", err)
	}
	if !v2.IsUnlocked() {
		t.Error("should be unlocked")
	}
}

func TestVault_UnlockWrongPassword(t *testing.T) {
	t.Parallel()
	v, path := newTestVault(t)
	if err := v.Create("correct"); err != nil {
		t.Fatal(err)
	}
	// Store something so verification entry is written.
	v.Set("key", "val")
	v.Lock()

	v2 := NewVault(path)
	if err := v2.Unlock("wrong"); err == nil {
		t.Error("Unlock with wrong password should fail")
	}
}

func TestVault_SetGetRoundtrip(t *testing.T) {
	t.Parallel()
	v, _ := newTestVault(t)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}

	if err := v.Set("api_key", "sk-12345"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	val, err := v.Get("api_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "sk-12345" {
		t.Errorf("Get = %q, want %q", val, "sk-12345")
	}
}

func TestVault_GetNonexistent(t *testing.T) {
	t.Parallel()
	v, _ := newTestVault(t)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}

	val, err := v.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get nonexistent: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty for nonexistent key, got %q", val)
	}
}

func TestVault_Delete(t *testing.T) {
	t.Parallel()
	v, _ := newTestVault(t)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}
	v.Set("key", "value")

	if err := v.Delete("key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	val, _ := v.Get("key")
	if val != "" {
		t.Errorf("expected empty after delete, got %q", val)
	}
}

func TestVault_KeysAndList(t *testing.T) {
	t.Parallel()
	v, _ := newTestVault(t)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}

	v.Set("a", "1")
	v.Set("b", "2")

	keys, err := v.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}

	// Should have a and b but NOT __verify__.
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	if !keySet["a"] || !keySet["b"] {
		t.Errorf("expected keys [a, b], got %v", keys)
	}
	if keySet["__verify__"] {
		t.Error("__verify__ should be excluded from Keys()")
	}

	list := v.List()
	if len(list) != len(keys) {
		t.Errorf("List and Keys should return same count: %d vs %d", len(list), len(keys))
	}
}

func TestVault_Lock(t *testing.T) {
	t.Parallel()
	v, _ := newTestVault(t)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}

	v.Lock()

	if v.IsUnlocked() {
		t.Error("should be locked after Lock()")
	}
	if _, err := v.Get("any"); err == nil {
		t.Error("Get on locked vault should return error")
	}
}

func TestVault_ChangePassword(t *testing.T) {
	t.Parallel()
	v, path := newTestVault(t)
	if err := v.Create("old"); err != nil {
		t.Fatal(err)
	}
	v.Set("key", "secret")

	if err := v.ChangePassword("new"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	v.Lock()

	// New password should work.
	v2 := NewVault(path)
	if err := v2.Unlock("new"); err != nil {
		t.Fatalf("Unlock with new password: %v", err)
	}
	val, _ := v2.Get("key")
	if val != "secret" {
		t.Errorf("value after password change = %q, want %q", val, "secret")
	}
	v2.Lock()

	// Old password should fail.
	v3 := NewVault(path)
	if err := v3.Unlock("old"); err == nil {
		t.Error("old password should fail after change")
	}
}

func TestVault_ConcurrentSetGet(t *testing.T) {
	t.Parallel()
	v, _ := newTestVault(t)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "key"
			v.Set(key, "value")
			v.Get(key)
		}(i)
	}
	wg.Wait()

	// If we get here without deadlock or panic, the test passes.
}

func TestVault_PersistenceAcrossInstances(t *testing.T) {
	t.Parallel()
	v, path := newTestVault(t)
	if err := v.Create("pass"); err != nil {
		t.Fatal(err)
	}
	v.Set("persist", "data123")
	v.Lock()

	// New instance, same file.
	v2 := NewVault(path)
	if err := v2.Unlock("pass"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	val, _ := v2.Get("persist")
	if val != "data123" {
		t.Errorf("persistence failed: got %q, want %q", val, "data123")
	}
}
