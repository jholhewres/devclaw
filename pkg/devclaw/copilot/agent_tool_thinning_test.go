package copilot

import (
	"encoding/json"
	"testing"
)

func TestEstimateToolDefTokens(t *testing.T) {
	tools := []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "web_search",
				Description: "Search the web for information using a query string.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"The search query"}},"required":["query"]}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "read_file",
				Description: "Read the contents of a file from the filesystem.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Absolute path to the file"},"offset":{"type":"integer","description":"Line offset to start reading from"}},"required":["path"]}`),
			},
		},
	}

	tokens := estimateToolDefTokens(tools, "glm-5-turbo")
	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
	// Sanity check: 2 tools should produce a reasonable estimate.
	// Total chars ~500, ratio ~1.625 → ~300 tokens.
	if tokens > 1000 {
		t.Errorf("token estimate seems too high for 2 tools: %d", tokens)
	}
}

func TestThinToolDefinitions_Level0_StripsDescriptions(t *testing.T) {
	tools := []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "bash",
				Description: "Execute a bash command on the system with full shell access.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"The command to execute"}},"required":["command"]}`),
			},
		},
	}

	thinned := thinToolDefinitions(tools, 0)

	// Original should be unmodified.
	if tools[0].Function.Description == "" {
		t.Error("original tool description was modified")
	}

	// Thinned should have empty description.
	if thinned[0].Function.Description != "" {
		t.Errorf("expected empty description, got %q", thinned[0].Function.Description)
	}

	// Parameters should be preserved.
	if len(thinned[0].Function.Parameters) == 0 {
		t.Error("parameters were lost")
	}

	// Name should be preserved.
	if thinned[0].Function.Name != "bash" {
		t.Errorf("expected name 'bash', got %q", thinned[0].Function.Name)
	}
}

func TestThinToolDefinitions_Level1_StripsParamDescriptions(t *testing.T) {
	tools := []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "edit_file",
				Description: "Edit a file by replacing text.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"description": "Parameters for editing a file",
					"properties": {
						"path": {
							"type": "string",
							"description": "The file path to edit"
						},
						"old_text": {
							"type": "string",
							"description": "The text to find and replace"
						}
					},
					"required": ["path", "old_text"]
				}`),
			},
		},
	}

	thinned := thinToolDefinitions(tools, 1)

	// Description should be stripped.
	if thinned[0].Function.Description != "" {
		t.Errorf("expected empty description, got %q", thinned[0].Function.Description)
	}

	// Parameter descriptions should be stripped.
	var schema map[string]interface{}
	if err := json.Unmarshal(thinned[0].Function.Parameters, &schema); err != nil {
		t.Fatalf("failed to parse thinned parameters: %v", err)
	}

	// Top-level description should be gone.
	if _, ok := schema["description"]; ok {
		t.Error("top-level description was not stripped from parameters")
	}

	// Property descriptions should be gone.
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties not found in schema")
	}
	for name, v := range props {
		prop, ok := v.(map[string]interface{})
		if !ok {
			t.Fatalf("property %s is not an object", name)
		}
		if _, hasDesc := prop["description"]; hasDesc {
			t.Errorf("property %q still has description", name)
		}
		// type should be preserved.
		if _, hasType := prop["type"]; !hasType {
			t.Errorf("property %q lost its type", name)
		}
	}

	// required should be preserved.
	if _, ok := schema["required"]; !ok {
		t.Error("required field was lost")
	}
}

func TestThinToolDefinitions_DoesNotModifyOriginal(t *testing.T) {
	original := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string","description":"query"}},"required":["q"]}`)
	tools := []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "search",
				Description: "Search for stuff",
				Parameters:  original,
			},
		},
	}

	origCopy := make(json.RawMessage, len(original))
	copy(origCopy, original)

	_ = thinToolDefinitions(tools, 1)

	// Verify original is untouched.
	if string(tools[0].Function.Parameters) != string(origCopy) {
		t.Error("original parameters were modified")
	}
	if tools[0].Function.Description != "Search for stuff" {
		t.Error("original description was modified")
	}
}

func TestStripParamDescriptions_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not valid json`)
	result := stripParamDescriptions(raw)
	if string(result) != string(raw) {
		t.Error("invalid JSON should be returned as-is")
	}
}

func TestStripParamDescriptions_Empty(t *testing.T) {
	result := stripParamDescriptions(nil)
	if result != nil {
		t.Error("nil input should return nil")
	}
}

func TestEstimateToolDefTokens_EmptyTools(t *testing.T) {
	tokens := estimateToolDefTokens(nil, "glm-5-turbo")
	if tokens != 0 {
		t.Errorf("expected 0 tokens for empty tools, got %d", tokens)
	}
}

func TestThinToolDefinitions_TokenReduction(t *testing.T) {
	// Create tools with substantial descriptions and parameter descriptions.
	tools := make([]ToolDefinition, 10)
	for i := range tools {
		desc := "This is a detailed description of the tool that explains what it does and how to use it properly."
		params := json.RawMessage(`{"type":"object","description":"The parameters object","properties":{"input":{"type":"string","description":"A detailed description of the input parameter that explains what value to provide"},"output":{"type":"string","description":"A detailed description of the output format"}},"required":["input"]}`)
		tools[i] = ToolDefinition{
			Type: "function",
			Function: FunctionDef{
				Name:        "tool_" + string(rune('a'+i)),
				Description: desc,
				Parameters:  params,
			},
		}
	}

	origTokens := estimateToolDefTokens(tools, "glm-5-turbo")
	level0 := thinToolDefinitions(tools, 0)
	level0Tokens := estimateToolDefTokens(level0, "glm-5-turbo")
	level1 := thinToolDefinitions(tools, 1)
	level1Tokens := estimateToolDefTokens(level1, "glm-5-turbo")

	// Level 0 should reduce tokens (descriptions stripped).
	if level0Tokens >= origTokens {
		t.Errorf("level 0 thinning did not reduce tokens: %d >= %d", level0Tokens, origTokens)
	}

	// Level 1 should reduce even more (param descriptions also stripped).
	if level1Tokens >= level0Tokens {
		t.Errorf("level 1 thinning did not reduce tokens further: %d >= %d", level1Tokens, level0Tokens)
	}

	t.Logf("Token reduction: original=%d, level0=%d (-%d%%), level1=%d (-%d%%)",
		origTokens, level0Tokens, (origTokens-level0Tokens)*100/origTokens,
		level1Tokens, (origTokens-level1Tokens)*100/origTokens)
}
