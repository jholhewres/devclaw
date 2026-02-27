// Package gmail provides agent tools for Gmail operations.
package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/auth/profiles"
)

// ToolContext provides access to auth profiles for Gmail tools.
type ToolContext struct {
	ProfileManager profiles.ProfileManager
	ProfileID      string // Optional: specific profile to use
}

// resolveToken resolves a Gmail OAuth token from profiles.
func resolveToken(ctx context.Context, tc *ToolContext) (string, *profiles.AuthProfile, error) {
	if tc.ProfileManager == nil {
		return "", nil, fmt.Errorf("profile manager not available")
	}

	opts := profiles.ResolutionOptions{
		Provider:     "google-gmail",
		Preference:   profiles.PreferValid,
		RequireValid: true,
	}

	if tc.ProfileID != "" {
		opts.PreferredProfile = tc.ProfileID
	}

	result := tc.ProfileManager.Resolve(opts)
	if result.Error != nil {
		// Try generic google provider as fallback
		opts.Provider = "google"
		result = tc.ProfileManager.Resolve(opts)
		if result.Error != nil {
			return "", nil, fmt.Errorf("no valid Gmail profile found: %w", result.Error)
		}
	}

	if result.Profile == nil {
		return "", nil, fmt.Errorf("no Gmail profile found")
	}

	return result.Credential, result.Profile, nil
}

// SearchMessagesArgs contains arguments for searching messages.
type SearchMessagesArgs struct {
	Query      string `json:"query,omitempty"`       // Gmail search query
	MaxResults int    `json:"max_results,omitempty"` // Max results (default 10)
	Label      string `json:"label,omitempty"`       // Label to search in
	UnreadOnly bool   `json:"unread_only,omitempty"` // Only unread messages
}

// MessageSummary contains a summary of a message for display.
type MessageSummary struct {
	ID         string    `json:"id"`
	ThreadID   string    `json:"thread_id"`
	Subject    string    `json:"subject"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Date       time.Time `json:"date"`
	Snippet    string    `json:"snippet"`
	IsUnread   bool      `json:"is_unread"`
	LabelIDs   []string  `json:"label_ids"`
}

// SearchMessagesResult contains the result of searching messages.
type SearchMessagesResult struct {
	Messages          []MessageSummary `json:"messages"`
	Total             int              `json:"total"`
	HasMore           bool             `json:"has_more"`
	NextPageToken     string           `json:"next_page_token,omitempty"`
	ProfileUsed       string           `json:"profile_used,omitempty"`
	ProfileEmail      string           `json:"profile_email,omitempty"`
	Error             string           `json:"error,omitempty"`
}

// SearchMessages searches for messages in Gmail.
func SearchMessages(ctx context.Context, tc *ToolContext, args SearchMessagesArgs) (*SearchMessagesResult, error) {
	token, profile, err := resolveToken(ctx, tc)
	if err != nil {
		return &SearchMessagesResult{
			Error: err.Error(),
		}, nil
	}

	client := NewClient(token)

	// Build query
	query := args.Query
	if args.UnreadOnly {
		if query != "" {
			query = "is:unread " + query
		} else {
			query = "is:unread"
		}
	}

	maxResults := args.MaxResults
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 10
	}

	opts := SearchOptions{
		Query:      query,
		MaxResults: maxResults,
	}

	if args.Label != "" {
		opts.LabelIDs = []string{args.Label}
	}

	resp, err := client.ListMessages(ctx, opts)
	if err != nil {
		return &SearchMessagesResult{
			Error: fmt.Sprintf("failed to list messages: %v", err),
		}, nil
	}

	result := &SearchMessagesResult{
		Total:        resp.ResultSizeEstimate,
		HasMore:        resp.NextPageToken != "",
		NextPageToken:  resp.NextPageToken,
		ProfileUsed:    string(profile.ID),
		ProfileEmail:   profile.OAuth.Email,
	}

	// Fetch full details for each message
	for _, msg := range resp.Messages {
		fullMsg, err := client.GetMessageMetadata(ctx, msg.ID)
		if err != nil {
			continue // Skip messages we can't fetch
		}

		summary := MessageSummary{
			ID:       fullMsg.ID,
			ThreadID: fullMsg.ThreadID,
			Snippet:  fullMsg.Snippet,
			LabelIDs: fullMsg.LabelIDs,
		}

		// Check if unread
		for _, label := range fullMsg.LabelIDs {
			if label == "UNREAD" {
				summary.IsUnread = true
				break
			}
		}

		// Extract headers
		if fullMsg.Payload != nil {
			summary.Subject = GetHeader(fullMsg, "Subject")
			summary.From = GetHeader(fullMsg, "From")
			summary.To = GetHeader(fullMsg, "To")

			// Parse date
			dateStr := GetHeader(fullMsg, "Date")
			if dateStr != "" {
				if t, err := time.Parse(time.RFC1123Z, dateStr); err == nil {
					summary.Date = t
				} else if t, err := time.Parse(time.RFC1123, dateStr); err == nil {
					summary.Date = t
				}
			}
		}

		result.Messages = append(result.Messages, summary)
	}

	return result, nil
}

// ReadMessageArgs contains arguments for reading a message.
type ReadMessageArgs struct {
	MessageID string `json:"message_id"`
}

// MessageDetail contains detailed information about a message.
type MessageDetail struct {
	ID         string            `json:"id"`
	ThreadID   string            `json:"thread_id"`
	Subject    string            `json:"subject"`
	From       string            `json:"from"`
	To         string            `json:"to"`
	Cc         string            `json:"cc,omitempty"`
	Bcc        string            `json:"bcc,omitempty"`
	Date       time.Time         `json:"date"`
	Snippet    string            `json:"snippet"`
	BodyText   string            `json:"body_text"`
	BodyHTML   string            `json:"body_html,omitempty"`
	IsUnread   bool              `json:"is_unread"`
	LabelIDs   []string          `json:"label_ids"`
	Attachments []AttachmentInfo `json:"attachments,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// AttachmentInfo contains information about an attachment.
type AttachmentInfo struct {
	Filename string `json:"filename"`
	MIMEType string `json:"mime_type"`
	Size     int    `json:"size"`
	PartID   string `json:"part_id"`
}

// ReadMessage reads a specific message from Gmail.
func ReadMessage(ctx context.Context, tc *ToolContext, args ReadMessageArgs) (*MessageDetail, error) {
	if args.MessageID == "" {
		return &MessageDetail{
			Error: "message_id is required",
		}, nil
	}

	token, _, err := resolveToken(ctx, tc)
	if err != nil {
		return &MessageDetail{
			Error: err.Error(),
		}, nil
	}

	client := NewClient(token)

	msg, err := client.GetMessageFull(ctx, args.MessageID)
	if err != nil {
		return &MessageDetail{
			Error: fmt.Sprintf("failed to get message: %v", err),
		}, nil
	}

	detail := &MessageDetail{
		ID:       msg.ID,
		ThreadID: msg.ThreadID,
		Snippet:  msg.Snippet,
		LabelIDs: msg.LabelIDs,
	}

	// Check if unread
	for _, label := range msg.LabelIDs {
		if label == "UNREAD" {
			detail.IsUnread = true
			break
		}
	}

	if msg.Payload != nil {
		detail.Subject = GetHeader(msg, "Subject")
		detail.From = GetHeader(msg, "From")
		detail.To = GetHeader(msg, "To")
		detail.Cc = GetHeader(msg, "Cc")
		detail.Bcc = GetHeader(msg, "Bcc")

		// Parse date
		dateStr := GetHeader(msg, "Date")
		if dateStr != "" {
			if t, err := time.Parse(time.RFC1123Z, dateStr); err == nil {
				detail.Date = t
			} else if t, err := time.Parse(time.RFC1123, dateStr); err == nil {
				detail.Date = t
			}
		}

		// Extract body
		detail.BodyText = GetMessageBody(msg, "text/plain")
		detail.BodyHTML = GetMessageBody(msg, "text/html")

		// Find attachments
		findAttachments(msg.Payload, &detail.Attachments)
	}

	return detail, nil
}

// findAttachments recursively finds attachments in a message.
func findAttachments(part *MessagePart, attachments *[]AttachmentInfo) {
	if part == nil {
		return
	}

	// Check if this part is an attachment
	if part.Filename != "" && part.Filename != "" {
		*attachments = append(*attachments, AttachmentInfo{
			Filename: part.Filename,
			MIMEType: part.MIMEType,
			Size:     part.Body.Size,
			PartID:   part.PartID,
		})
	}

	// Recurse into child parts
	for _, child := range part.Parts {
		findAttachments(child, attachments)
	}
}

// ListLabelsArgs contains arguments for listing labels.
type ListLabelsArgs struct{}

// LabelInfo contains information about a label.
type LabelInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"` // system or user
	MessageTotal  int    `json:"message_total,omitempty"`
	MessageUnread int    `json:"message_unread,omitempty"`
}

// ListLabelsResult contains the result of listing labels.
type ListLabelsResult struct {
	Labels       []LabelInfo `json:"labels"`
	SystemLabels []LabelInfo `json:"system_labels"`
	UserLabels   []LabelInfo `json:"user_labels"`
	Error        string      `json:"error,omitempty"`
}

// ListLabels lists all labels in Gmail.
func ListLabels(ctx context.Context, tc *ToolContext, args ListLabelsArgs) (*ListLabelsResult, error) {
	token, _, err := resolveToken(ctx, tc)
	if err != nil {
		return &ListLabelsResult{
			Error: err.Error(),
		}, nil
	}

	client := NewClient(token)

	resp, err := client.ListLabels(ctx)
	if err != nil {
		return &ListLabelsResult{
			Error: fmt.Sprintf("failed to list labels: %v", err),
		}, nil
	}

	result := &ListLabelsResult{}

	for _, label := range resp.Labels {
		info := LabelInfo{
			ID:            label.ID,
			Name:          label.Name,
			Type:          label.Type,
			MessageTotal:  label.MessagesTotal,
			MessageUnread: label.MessagesUnread,
		}

		result.Labels = append(result.Labels, info)

		if label.Type == "system" {
			result.SystemLabels = append(result.SystemLabels, info)
		} else {
			result.UserLabels = append(result.UserLabels, info)
		}
	}

	return result, nil
}

// MarkReadArgs contains arguments for marking a message as read.
type MarkReadArgs struct {
	MessageID string `json:"message_id"`
}

// MarkReadResult contains the result of marking a message as read.
type MarkReadResult struct {
	Success    bool   `json:"success"`
	MessageID  string `json:"message_id"`
	Error      string `json:"error,omitempty"`
}

// MarkAsRead marks a message as read.
func MarkAsRead(ctx context.Context, tc *ToolContext, args MarkReadArgs) (*MarkReadResult, error) {
	if args.MessageID == "" {
		return &MarkReadResult{
			Success: false,
			Error:   "message_id is required",
		}, nil
	}

	token, _, err := resolveToken(ctx, tc)
	if err != nil {
		return &MarkReadResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	client := NewClient(token)

	if err := client.MarkAsRead(ctx, args.MessageID); err != nil {
		return &MarkReadResult{
			Success:   false,
			MessageID: args.MessageID,
			Error:     fmt.Sprintf("failed to mark as read: %v", err),
		}, nil
	}

	return &MarkReadResult{
		Success:   true,
		MessageID: args.MessageID,
	}, nil
}

// ArchiveMessageArgs contains arguments for archiving a message.
type ArchiveMessageArgs struct {
	MessageID string `json:"message_id"`
}

// ArchiveMessageResult contains the result of archiving a message.
type ArchiveMessageResult struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id"`
	Error     string `json:"error,omitempty"`
}

// ArchiveMessage archives a message (removes from inbox).
func ArchiveMessage(ctx context.Context, tc *ToolContext, args ArchiveMessageArgs) (*ArchiveMessageResult, error) {
	if args.MessageID == "" {
		return &ArchiveMessageResult{
			Success: false,
			Error:   "message_id is required",
		}, nil
	}

	token, _, err := resolveToken(ctx, tc)
	if err != nil {
		return &ArchiveMessageResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	client := NewClient(token)

	if err := client.ArchiveMessage(ctx, args.MessageID); err != nil {
		return &ArchiveMessageResult{
			Success:   false,
			MessageID: args.MessageID,
			Error:     fmt.Sprintf("failed to archive: %v", err),
		}, nil
	}

	return &ArchiveMessageResult{
		Success:   true,
		MessageID: args.MessageID,
	}, nil
}

// TrashMessageArgs contains arguments for trashing a message.
type TrashMessageArgs struct {
	MessageID string `json:"message_id"`
}

// TrashMessageResult contains the result of trashing a message.
type TrashMessageResult struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id"`
	Error     string `json:"error,omitempty"`
}

// TrashMessage moves a message to trash.
func TrashMessage(ctx context.Context, tc *ToolContext, args TrashMessageArgs) (*TrashMessageResult, error) {
	if args.MessageID == "" {
		return &TrashMessageResult{
			Success: false,
			Error:   "message_id is required",
		}, nil
	}

	token, _, err := resolveToken(ctx, tc)
	if err != nil {
		return &TrashMessageResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	client := NewClient(token)

	if err := client.TrashMessage(ctx, args.MessageID); err != nil {
		return &TrashMessageResult{
			Success:   false,
			MessageID: args.MessageID,
			Error:     fmt.Sprintf("failed to trash: %v", err),
		}, nil
	}

	return &TrashMessageResult{
		Success:   true,
		MessageID: args.MessageID,
	}, nil
}

// FormatMessageSummary formats a list of messages for display.
func FormatMessageSummary(result *SearchMessagesResult) string {
	if len(result.Messages) == 0 {
		return "No messages found."
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Found %d messages:\n", result.Total))

	for _, msg := range result.Messages {
		status := "✓"
		if msg.IsUnread {
			status = "●"
		}

		dateStr := ""
		if !msg.Date.IsZero() {
			dateStr = msg.Date.Format("Jan 02")
		}

		from := msg.From
		// Truncate long from addresses
		if len(from) > 30 {
			from = from[:27] + "..."
		}

		subject := msg.Subject
		if subject == "" {
			subject = "(no subject)"
		}

		lines = append(lines, fmt.Sprintf("%s [%s] %-30s %s",
			status, dateStr, from, subject))

		if msg.Snippet != "" {
			lines = append(lines, fmt.Sprintf("    %s", msg.Snippet))
		}

		lines = append(lines, "") // Empty line between messages
	}

	if result.HasMore {
		lines = append(lines, "(More results available...)")
	}

	return strings.Join(lines, "\n")
}

// FormatMessageDetail formats a message detail for display.
func FormatMessageDetail(detail *MessageDetail) string {
	if detail.Error != "" {
		return fmt.Sprintf("Error: %s", detail.Error)
	}

	var lines []string

	lines = append(lines, fmt.Sprintf("From: %s", detail.From))
	lines = append(lines, fmt.Sprintf("To: %s", detail.To))
	if detail.Cc != "" {
		lines = append(lines, fmt.Sprintf("Cc: %s", detail.Cc))
	}
	lines = append(lines, fmt.Sprintf("Subject: %s", detail.Subject))
	lines = append(lines, fmt.Sprintf("Date: %s", detail.Date.Format("Monday, January 2, 2006 at 3:04 PM")))

	if detail.IsUnread {
		lines = append(lines, "Status: Unread")
	}

	if len(detail.Attachments) > 0 {
		lines = append(lines, fmt.Sprintf("\nAttachments (%d):", len(detail.Attachments)))
		for _, att := range detail.Attachments {
			lines = append(lines, fmt.Sprintf("  - %s (%s, %d bytes)", att.Filename, att.MIMEType, att.Size))
		}
	}

	lines = append(lines, "\n---\n")

	// Prefer text body, fall back to HTML
	body := detail.BodyText
	if body == "" {
		body = detail.BodyHTML
	}

	if body != "" {
		lines = append(lines, body)
	} else {
		lines = append(lines, "(No message body)")
	}

	return strings.Join(lines, "\n")
}

// JSON helpers for tool results
func (r *SearchMessagesResult) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

func (r *MessageDetail) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

func (r *ListLabelsResult) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}
