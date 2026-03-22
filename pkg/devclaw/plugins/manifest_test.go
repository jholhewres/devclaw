package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `
id: test-plugin
name: Test Plugin
version: 1.0.0
description: A test plugin
author: Test Author

config:
  fields:
    - key: api_key
      name: API Key
      type: secret
      required: true
      env_var: TEST_API_KEY

agents:
  - id: helper
    name: Helper Agent
    instructions: You are a helpful agent.
    triggers: ["help", "assist"]
    tools:
      allow: ["read_file"]
      deny: ["bash"]
    max_turns: 5
    escalation:
      enabled: true
      keywords: ["escalate"]
      max_turns: 3

tools:
  - name: greet
    description: Greet someone
    parameters:
      type: object
      properties:
        name:
          type: string
      required: ["name"]
    script: |
      echo "Hello, ${name}!"

hooks:
  - name: on-message
    events: ["user_prompt_submit"]
    priority: 100
    script: echo "hook fired"

skills:
  - name: greeting
    description: Greeting skill
    skill_md: skills/greeting/SKILL.md
`
	path := filepath.Join(dir, "plugin.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0600); err != nil {
		t.Fatal(err)
	}

	m, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest failed: %v", err)
	}

	if m.ID != "test-plugin" {
		t.Errorf("ID = %q, want %q", m.ID, "test-plugin")
	}
	if m.Name != "Test Plugin" {
		t.Errorf("Name = %q, want %q", m.Name, "Test Plugin")
	}
	if m.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", m.Version, "1.0.0")
	}
	if len(m.Agents) != 1 {
		t.Fatalf("Agents = %d, want 1", len(m.Agents))
	}
	if m.Agents[0].ID != "helper" {
		t.Errorf("Agent.ID = %q, want %q", m.Agents[0].ID, "helper")
	}
	if len(m.Agents[0].Triggers) != 2 {
		t.Errorf("Agent.Triggers = %d, want 2", len(m.Agents[0].Triggers))
	}
	if m.Agents[0].Escalation == nil || !m.Agents[0].Escalation.Enabled {
		t.Error("Agent.Escalation should be enabled")
	}
	if len(m.Tools) != 1 {
		t.Fatalf("Tools = %d, want 1", len(m.Tools))
	}
	if m.Tools[0].Name != "greet" {
		t.Errorf("Tool.Name = %q, want %q", m.Tools[0].Name, "greet")
	}
	if m.Tools[0].Script == "" {
		t.Error("Tool.Script should not be empty")
	}
	if len(m.Hooks) != 1 {
		t.Fatalf("Hooks = %d, want 1", len(m.Hooks))
	}
	if len(m.Skills) != 1 {
		t.Fatalf("Skills = %d, want 1", len(m.Skills))
	}
	if m.Config == nil || len(m.Config.Fields) != 1 {
		t.Fatal("Config should have 1 field")
	}
	if !m.Config.Fields[0].Required {
		t.Error("api_key field should be required")
	}
}

func TestParseManifest_MissingID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.yaml")
	if err := os.WriteFile(path, []byte("name: No ID Plugin\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := ParseManifest(path)
	if err == nil {
		t.Error("expected error for missing ID")
	}
}

func TestParseManifest_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.yaml")
	if err := os.WriteFile(path, []byte("id: minimal\n"), 0600); err != nil {
		t.Fatal(err)
	}

	m, err := ParseManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "minimal" {
		t.Errorf("Name should default to ID, got %q", m.Name)
	}
	if m.Version != "0.0.0" {
		t.Errorf("Version should default to 0.0.0, got %q", m.Version)
	}
}

func TestPluginRequirements_IsEligible(t *testing.T) {
	// nil requirements are always eligible.
	var r *PluginRequirements
	if !r.IsEligible() {
		t.Error("nil requirements should be eligible")
	}

	// Empty requirements are eligible.
	r = &PluginRequirements{}
	if !r.IsEligible() {
		t.Error("empty requirements should be eligible")
	}

	// Missing required env var.
	r = &PluginRequirements{Env: []string{"NONEXISTENT_VAR_FOR_TEST_12345"}}
	if r.IsEligible() {
		t.Error("should not be eligible with missing env var")
	}
}
