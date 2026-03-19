// Package telegram – errors.go defines structured error types for the
// Telegram Bot API, enabling callers to classify and handle API failures.
package telegram

import (
	"errors"
	"fmt"
	"strings"
)

// TelegramAPIError represents a structured error from the Telegram Bot API.
type TelegramAPIError struct {
	// Method is the Bot API method that failed (e.g. "sendMessage").
	Method string

	// HTTPStatus is the HTTP response status code.
	HTTPStatus int

	// ErrorCode is the Telegram-specific error code from the JSON response.
	ErrorCode int

	// Description is the human-readable error description from Telegram.
	Description string
}

// Error implements the error interface.
func (e *TelegramAPIError) Error() string {
	return fmt.Sprintf("telegram: %s: [%d] %s", e.Method, e.ErrorCode, e.Description)
}

// isRetryable returns true if the error is transient and the request can be retried.
// This includes rate limiting (429) and server errors (5xx).
func (e *TelegramAPIError) isRetryable() bool {
	return e.isRateLimited() || (e.HTTPStatus >= 500 && e.HTTPStatus < 600)
}

// isAuthError returns true if the error indicates an authentication/authorization failure.
// HTTP 401 (Unauthorized) or 403 (Forbidden) typically mean the bot token is invalid
// or the bot lacks permissions for the requested action.
func (e *TelegramAPIError) isAuthError() bool {
	return e.HTTPStatus == 401 || e.HTTPStatus == 403
}

// isRateLimited returns true if the error indicates rate limiting (HTTP 429).
func (e *TelegramAPIError) isRateLimited() bool {
	return e.HTTPStatus == 429 || e.ErrorCode == 429
}

// isThreadNotFound returns true if the error indicates a forum topic/thread
// was not found or is invalid. This happens when message_thread_id refers to
// a closed or deleted forum topic.
func (e *TelegramAPIError) isThreadNotFound() bool {
	desc := strings.ToLower(e.Description)
	return strings.Contains(desc, "thread not found") ||
		strings.Contains(desc, "message thread not found") ||
		strings.Contains(desc, "topic_closed") ||
		strings.Contains(desc, "topic_deleted")
}

// isMessageNotModified returns true if the error indicates that the message
// content was not modified (same content sent via editMessageText).
// This is a harmless error — the message already has the desired content.
func (e *TelegramAPIError) isMessageNotModified() bool {
	desc := strings.ToLower(e.Description)
	return strings.Contains(desc, "message is not modified")
}

// asTelegramAPIError attempts to extract a *TelegramAPIError from an error.
// Uses errors.As to support wrapped errors.
// Returns nil if the error is not a TelegramAPIError.
func asTelegramAPIError(err error) *TelegramAPIError {
	if err == nil {
		return nil
	}
	var tgErr *TelegramAPIError
	if errors.As(err, &tgErr) {
		return tgErr
	}
	return nil
}
