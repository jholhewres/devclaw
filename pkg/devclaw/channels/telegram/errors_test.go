package telegram

import (
	"fmt"
	"testing"
)

func TestTelegramAPIError_Error(t *testing.T) {
	err := &TelegramAPIError{
		Method:      "sendMessage",
		HTTPStatus:  400,
		ErrorCode:   400,
		Description: "Bad Request: chat not found",
	}
	got := err.Error()
	want := "telegram: sendMessage: [400] Bad Request: chat not found"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestTelegramAPIError_isRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      TelegramAPIError
		expected bool
	}{
		{"429 rate limit", TelegramAPIError{HTTPStatus: 429, ErrorCode: 429}, true},
		{"500 server error", TelegramAPIError{HTTPStatus: 500, ErrorCode: 500}, true},
		{"502 bad gateway", TelegramAPIError{HTTPStatus: 502, ErrorCode: 502}, true},
		{"503 unavailable", TelegramAPIError{HTTPStatus: 503, ErrorCode: 503}, true},
		{"400 bad request", TelegramAPIError{HTTPStatus: 400, ErrorCode: 400}, false},
		{"401 unauthorized", TelegramAPIError{HTTPStatus: 401, ErrorCode: 401}, false},
		{"403 forbidden", TelegramAPIError{HTTPStatus: 403, ErrorCode: 403}, false},
		{"404 not found", TelegramAPIError{HTTPStatus: 404, ErrorCode: 404}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.isRetryable(); got != tt.expected {
				t.Errorf("isRetryable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTelegramAPIError_isAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      TelegramAPIError
		expected bool
	}{
		{"401 unauthorized", TelegramAPIError{HTTPStatus: 401}, true},
		{"403 forbidden", TelegramAPIError{HTTPStatus: 403}, true},
		{"400 bad request", TelegramAPIError{HTTPStatus: 400}, false},
		{"429 rate limit", TelegramAPIError{HTTPStatus: 429}, false},
		{"500 server error", TelegramAPIError{HTTPStatus: 500}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.isAuthError(); got != tt.expected {
				t.Errorf("isAuthError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTelegramAPIError_isRateLimited(t *testing.T) {
	tests := []struct {
		name     string
		err      TelegramAPIError
		expected bool
	}{
		{"HTTP 429", TelegramAPIError{HTTPStatus: 429, ErrorCode: 429}, true},
		{"error_code 429", TelegramAPIError{HTTPStatus: 400, ErrorCode: 429}, true},
		{"HTTP 400", TelegramAPIError{HTTPStatus: 400, ErrorCode: 400}, false},
		{"HTTP 200", TelegramAPIError{HTTPStatus: 200, ErrorCode: 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.isRateLimited(); got != tt.expected {
				t.Errorf("isRateLimited() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTelegramAPIError_isThreadNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      TelegramAPIError
		expected bool
	}{
		{"thread not found", TelegramAPIError{Description: "Bad Request: thread not found"}, true},
		{"message thread not found", TelegramAPIError{Description: "Bad Request: message thread not found"}, true},
		{"topic closed", TelegramAPIError{Description: "Bad Request: TOPIC_CLOSED"}, true},
		{"topic deleted", TelegramAPIError{Description: "Bad Request: TOPIC_DELETED"}, true},
		{"other error", TelegramAPIError{Description: "Bad Request: chat not found"}, false},
		{"empty description", TelegramAPIError{Description: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.isThreadNotFound(); got != tt.expected {
				t.Errorf("isThreadNotFound() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTelegramAPIError_isMessageNotModified(t *testing.T) {
	tests := []struct {
		name     string
		err      TelegramAPIError
		expected bool
	}{
		{"message not modified", TelegramAPIError{Description: "Bad Request: message is not modified"}, true},
		{"uppercase", TelegramAPIError{Description: "Bad Request: MESSAGE IS NOT MODIFIED"}, true},
		{"other error", TelegramAPIError{Description: "Bad Request: chat not found"}, false},
		{"empty", TelegramAPIError{Description: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.isMessageNotModified(); got != tt.expected {
				t.Errorf("isMessageNotModified() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAsTelegramAPIError(t *testing.T) {
	t.Run("returns error for TelegramAPIError", func(t *testing.T) {
		err := &TelegramAPIError{Method: "test", ErrorCode: 400}
		got := asTelegramAPIError(err)
		if got == nil {
			t.Fatal("expected non-nil TelegramAPIError")
		}
		if got.Method != "test" {
			t.Errorf("Method = %q, want %q", got.Method, "test")
		}
	})

	t.Run("returns error for wrapped TelegramAPIError", func(t *testing.T) {
		inner := &TelegramAPIError{Method: "wrapped", ErrorCode: 429}
		err := fmt.Errorf("outer: %w", inner)
		got := asTelegramAPIError(err)
		if got == nil {
			t.Fatal("expected non-nil TelegramAPIError from wrapped error")
		}
		if got.Method != "wrapped" {
			t.Errorf("Method = %q, want %q", got.Method, "wrapped")
		}
	})

	t.Run("returns nil for other errors", func(t *testing.T) {
		err := fmt.Errorf("some error")
		got := asTelegramAPIError(err)
		if got != nil {
			t.Error("expected nil for non-TelegramAPIError")
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		got := asTelegramAPIError(nil)
		if got != nil {
			t.Error("expected nil for nil error")
		}
	})
}
