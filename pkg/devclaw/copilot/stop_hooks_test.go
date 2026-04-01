package copilot

import (
	"strings"
	"testing"
)

func TestStopHookVerifier_NoMessages(t *testing.T) {
	v := NewStopHookVerifier()
	result := v.VerifyCompletion(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 items for nil messages, got %d", len(result))
	}
}

func TestStopHookVerifier_ShortConversation(t *testing.T) {
	v := NewStopHookVerifier()
	messages := []chatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	result := v.VerifyCompletion(messages)
	if len(result) != 0 {
		t.Errorf("expected 0 items for short conversation, got %d", len(result))
	}
}

func TestStopHookVerifier_CodeWithoutTests(t *testing.T) {
	v := NewStopHookVerifier()
	messages := []chatMessage{
		{Role: "user", Content: "Fix the bug in auth.go"},
		{Role: "assistant", Content: "I'll edit the file"},
		{Role: "assistant", Content: "Using edit_file to fix the function"},
		{Role: "tool", Content: "File edited successfully: func handleAuth()"},
		{Role: "assistant", Content: "I've fixed the authentication bug"},
	}

	result := v.VerifyCompletion(messages)

	foundTestCheck := false
	for _, item := range result {
		if item.CheckName == "tests_after_code_changes" {
			foundTestCheck = true
		}
	}
	if !foundTestCheck {
		t.Error("expected tests_after_code_changes to be detected as incomplete")
	}
}

func TestStopHookVerifier_CodeWithTests(t *testing.T) {
	v := NewStopHookVerifier()
	messages := []chatMessage{
		{Role: "user", Content: "Fix the bug in auth.go"},
		{Role: "assistant", Content: "I'll edit the file"},
		{Role: "assistant", Content: "Using edit_file to fix the function"},
		{Role: "tool", Content: "File edited successfully: func handleAuth()"},
		{Role: "assistant", Content: "Now let me run the tests"},
		{Role: "tool", Content: "go test ./... PASS ok all tests passed"},
		{Role: "assistant", Content: "All tests pass. The bug is fixed."},
	}

	result := v.VerifyCompletion(messages)

	for _, item := range result {
		if item.CheckName == "tests_after_code_changes" {
			t.Error("should NOT detect tests_after_code_changes when tests were run")
		}
	}
}

func TestStopHookVerifier_NoCodeChanges(t *testing.T) {
	v := NewStopHookVerifier()
	messages := []chatMessage{
		{Role: "user", Content: "What does this function do?"},
		{Role: "assistant", Content: "Let me read the file"},
		{Role: "tool", Content: "File contents: package main..."},
		{Role: "assistant", Content: "This function handles authentication"},
		{Role: "user", Content: "Thanks"},
	}

	result := v.VerifyCompletion(messages)
	if len(result) != 0 {
		t.Errorf("expected 0 items for read-only conversation, got %d", len(result))
	}
}

func TestFormatIncompleteWorkMessage(t *testing.T) {
	items := []IncompleteWork{
		{CheckName: "tests", Reminder: "Run tests please"},
		{CheckName: "build", Reminder: "Check the build"},
	}

	result := FormatIncompleteWorkMessage(items)
	if !strings.Contains(result, "Run tests please") {
		t.Error("expected reminder text in output")
	}
	if !strings.Contains(result, "1.") {
		t.Error("expected numbered list")
	}
}

func TestFormatIncompleteWorkMessageEmpty(t *testing.T) {
	result := FormatIncompleteWorkMessage(nil)
	if result != "" {
		t.Errorf("expected empty string for nil items, got %q", result)
	}
}
