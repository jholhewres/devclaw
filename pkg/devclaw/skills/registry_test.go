package skills

import (
	"context"
	"testing"
)

// fakeSkill is a minimal Skill implementation for testing.
type fakeSkill struct {
	meta Metadata
}

func (f *fakeSkill) Metadata() Metadata                                 { return f.meta }
func (f *fakeSkill) Tools() []Tool                                      { return nil }
func (f *fakeSkill) SystemPrompt() string                               { return "" }
func (f *fakeSkill) Triggers() []string                                 { return nil }
func (f *fakeSkill) Init(_ context.Context, _ map[string]any) error     { return nil }
func (f *fakeSkill) Execute(_ context.Context, _ string) (string, error) { return "", nil }
func (f *fakeSkill) Shutdown() error                                    { return nil }

func newFake(name, category string, tags []string) *fakeSkill {
	return &fakeSkill{meta: Metadata{
		Name:        name,
		Version:     "1.0.0",
		Author:      "test",
		Description: "test skill " + name,
		Category:    category,
		Tags:        tags,
	}}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)

	s := newFake("calendar", "productivity", nil)
	if err := r.Register(s); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := r.Get("calendar")
	if !ok {
		t.Fatal("Get returned false for registered skill")
	}
	if got.Metadata().Name != "calendar" {
		t.Errorf("got name %q, want %q", got.Metadata().Name, "calendar")
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	s := newFake("dup", "test", nil)
	r.Register(s)

	err := r.Register(newFake("dup", "test", nil))
	if err == nil {
		t.Error("registering duplicate should return error")
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get should return false for unknown skill")
	}
}

func TestRegistry_List(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	r.Register(newFake("a", "cat1", nil))
	r.Register(newFake("b", "cat2", nil))

	list := r.List()
	if len(list) != 2 {
		t.Errorf("expected 2 skills, got %d", len(list))
	}
}

func TestRegistry_Search(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	r.Register(newFake("calendar", "productivity", []string{"schedule", "events"}))
	r.Register(newFake("github", "development", []string{"code"}))

	results := r.Search("calendar")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'calendar', got %d", len(results))
	}

	results = r.Search("code")
	if len(results) != 1 {
		t.Errorf("expected 1 result for tag 'code', got %d", len(results))
	}

	results = r.Search("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'nonexistent', got %d", len(results))
	}
}

func TestRegistry_EnableDisable(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	r.Register(newFake("skill1", "test", nil))

	if !r.IsEnabled("skill1") {
		t.Error("should be enabled by default")
	}

	if err := r.Disable("skill1"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if r.IsEnabled("skill1") {
		t.Error("should be disabled after Disable()")
	}

	if err := r.Enable("skill1"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !r.IsEnabled("skill1") {
		t.Error("should be enabled after Enable()")
	}
}

func TestRegistry_EnableNonexistent(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	if err := r.Enable("fake"); err == nil {
		t.Error("Enable nonexistent should return error")
	}
}

func TestRegistry_DisableNonexistent(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	if err := r.Disable("fake"); err == nil {
		t.Error("Disable nonexistent should return error")
	}
}

func TestRegistry_Remove(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	r.Register(newFake("removable", "test", nil))

	if !r.Remove("removable") {
		t.Error("Remove should return true")
	}
	if _, ok := r.Get("removable"); ok {
		t.Error("skill should not exist after Remove")
	}
}

func TestRegistry_RemoveNonexistent(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	if r.Remove("ghost") {
		t.Error("Remove nonexistent should return false")
	}
}

func TestRegistry_ByCategory(t *testing.T) {
	t.Parallel()
	r := NewRegistry(nil)
	r.Register(newFake("s1", "productivity", nil))
	r.Register(newFake("s2", "productivity", nil))
	r.Register(newFake("s3", "development", nil))

	results := r.ByCategory("productivity")
	if len(results) != 2 {
		t.Errorf("expected 2 productivity skills, got %d", len(results))
	}

	results = r.ByCategory("development")
	if len(results) != 1 {
		t.Errorf("expected 1 development skill, got %d", len(results))
	}

	results = r.ByCategory("empty")
	if len(results) != 0 {
		t.Errorf("expected 0 for unknown category, got %d", len(results))
	}
}
