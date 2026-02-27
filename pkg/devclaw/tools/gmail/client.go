// Package gmail provides tools for interacting with the Gmail API.
package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	gmailAPIBaseURL = "https://www.googleapis.com/gmail/v1"
)

// Client provides access to the Gmail API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Gmail API client.
func NewClient(accessToken string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &authTransport{
				accessToken: accessToken,
				base:        http.DefaultTransport,
			},
		},
		baseURL: gmailAPIBaseURL,
	}
}

// authTransport adds Authorization header to requests.
type authTransport struct {
	accessToken string
	base        http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.accessToken)
	return t.base.RoundTrip(req)
}

// SetAccessToken updates the access token for subsequent requests.
func (c *Client) SetAccessToken(token string) {
	c.httpClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &authTransport{
			accessToken: token,
			base:        http.DefaultTransport,
		},
	}
}

// doRequest performs an HTTP request and returns the response body.
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Message represents a Gmail message.
type Message struct {
	ID           string            `json:"id"`
	ThreadID     string            `json:"threadId"`
	LabelIDs     []string          `json:"labelIds,omitempty"`
	Snippet      string            `json:"snippet,omitempty"`
	HistoryID    uint64            `json:"historyId,omitempty"`
	InternalDate int64             `json:"internalDate,omitempty"`
	Payload      *MessagePart      `json:"payload,omitempty"`
	SizeEstimate int               `json:"sizeEstimate,omitempty"`
	Raw          string            `json:"raw,omitempty"`
}

// MessagePart represents a part of a Gmail message.
type MessagePart struct {
	PartID   string            `json:"partId,omitempty"`
	MIMEType string            `json:"mimeType,omitempty"`
	Filename string            `json:"filename,omitempty"`
	Headers  []MessageHeader   `json:"headers,omitempty"`
	Body     *MessagePartBody  `json:"body,omitempty"`
	Parts    []*MessagePart    `json:"parts,omitempty"`
}

// MessageHeader represents a message header.
type MessageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MessagePartBody represents the body of a message part.
type MessagePartBody struct {
	AttachmentID string `json:"attachmentId,omitempty"`
	Size         int    `json:"size,omitempty"`
	Data         string `json:"data,omitempty"`
}

// Thread represents a Gmail thread.
type Thread struct {
	ID          string     `json:"id"`
	HistoryID   uint64     `json:"historyId,omitempty"`
	Messages    []Message  `json:"messages,omitempty"`
}

// Label represents a Gmail label.
type Label struct {
	ID                     string `json:"id"`
	Name                   string `json:"name,omitempty"`
	MessageListVisibility  string `json:"messageListVisibility,omitempty"`
	LabelListVisibility    string `json:"labelListVisibility,omitempty"`
	Type                   string `json:"type,omitempty"`
	MessagesTotal          int    `json:"messagesTotal,omitempty"`
	MessagesUnread         int    `json:"messagesUnread,omitempty"`
	ThreadsTotal           int    `json:"threadsTotal,omitempty"`
	ThreadsUnread          int    `json:"threadsUnread,omitempty"`
}

// ListMessagesResponse is the response from listing messages.
type ListMessagesResponse struct {
	Messages           []Message `json:"messages,omitempty"`
	NextPageToken      string    `json:"nextPageToken,omitempty"`
	ResultSizeEstimate int       `json:"resultSizeEstimate,omitempty"`
}

// ListThreadsResponse is the response from listing threads.
type ListThreadsResponse struct {
	Threads            []Thread `json:"threads,omitempty"`
	NextPageToken      string   `json:"nextPageToken,omitempty"`
	ResultSizeEstimate int      `json:"resultSizeEstimate,omitempty"`
}

// ListLabelsResponse is the response from listing labels.
type ListLabelsResponse struct {
	Labels []Label `json:"labels,omitempty"`
}

// SearchOptions contains options for searching messages.
type SearchOptions struct {
	Query       string // Gmail search query (e.g., "from:someone@example.com subject:hello")
	LabelIDs    []string
	MaxResults  int
	PageToken   string
	IncludeSpam bool
}

// ListMessages lists messages in the user's mailbox.
func (c *Client) ListMessages(ctx context.Context, opts SearchOptions) (*ListMessagesResponse, error) {
	params := url.Values{}

	if opts.Query != "" {
		params.Set("q", opts.Query)
	}

	if len(opts.LabelIDs) > 0 {
		for _, label := range opts.LabelIDs {
			params.Add("labelIds", label)
		}
	}

	if opts.MaxResults > 0 {
		params.Set("maxResults", strconv.Itoa(opts.MaxResults))
	}

	if opts.PageToken != "" {
		params.Set("pageToken", opts.PageToken)
	}

	if opts.IncludeSpam {
		params.Set("includeSpamTrash", "true")
	}

	path := "/users/me/messages"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp ListMessagesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &resp, nil
}

// GetMessage retrieves a specific message by ID.
func (c *Client) GetMessage(ctx context.Context, messageID string, format string) (*Message, error) {
	params := url.Values{}

	if format != "" {
		params.Set("format", format)
	}

	path := fmt.Sprintf("/users/me/messages/%s", messageID)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("parsing message: %w", err)
	}

	return &msg, nil
}

// GetMessageFull retrieves a message with full content.
func (c *Client) GetMessageFull(ctx context.Context, messageID string) (*Message, error) {
	return c.GetMessage(ctx, messageID, "full")
}

// GetMessageMetadata retrieves only metadata for a message.
func (c *Client) GetMessageMetadata(ctx context.Context, messageID string) (*Message, error) {
	return c.GetMessage(ctx, messageID, "metadata")
}

// ListLabels lists all labels in the user's mailbox.
func (c *Client) ListLabels(ctx context.Context) (*ListLabelsResponse, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/users/me/labels", nil)
	if err != nil {
		return nil, err
	}

	var resp ListLabelsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &resp, nil
}

// ModifyMessageRequest represents a request to modify message labels.
type ModifyMessageRequest struct {
	AddLabelIDs    []string `json:"addLabelIds,omitempty"`
	RemoveLabelIDs []string `json:"removeLabelIds,omitempty"`
}

// ModifyMessage modifies the labels on a message.
func (c *Client) ModifyMessage(ctx context.Context, messageID string, req *ModifyMessageRequest) (*Message, error) {
	bodyData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	path := fmt.Sprintf("/users/me/messages/%s/modify", messageID)
	body, err := c.doRequest(ctx, http.MethodPost, path, strings.NewReader(string(bodyData)))
	if err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("parsing message: %w", err)
	}

	return &msg, nil
}

// MarkAsRead marks a message as read (removes UNREAD label).
func (c *Client) MarkAsRead(ctx context.Context, messageID string) error {
	req := &ModifyMessageRequest{
		RemoveLabelIDs: []string{"UNREAD"},
	}
	_, err := c.ModifyMessage(ctx, messageID, req)
	return err
}

// MarkAsUnread marks a message as unread (adds UNREAD label).
func (c *Client) MarkAsUnread(ctx context.Context, messageID string) error {
	req := &ModifyMessageRequest{
		AddLabelIDs: []string{"UNREAD"},
	}
	_, err := c.ModifyMessage(ctx, messageID, req)
	return err
}

// ArchiveMessage removes the INBOX label from a message.
func (c *Client) ArchiveMessage(ctx context.Context, messageID string) error {
	req := &ModifyMessageRequest{
		RemoveLabelIDs: []string{"INBOX"},
	}
	_, err := c.ModifyMessage(ctx, messageID, req)
	return err
}

// MoveToInbox adds the INBOX label to a message.
func (c *Client) MoveToInbox(ctx context.Context, messageID string) error {
	req := &ModifyMessageRequest{
		AddLabelIDs: []string{"INBOX"},
	}
	_, err := c.ModifyMessage(ctx, messageID, req)
	return err
}

// TrashMessage moves a message to trash.
func (c *Client) TrashMessage(ctx context.Context, messageID string) error {
	path := fmt.Sprintf("/users/me/messages/%s/trash", messageID)
	_, err := c.doRequest(ctx, http.MethodPost, path, nil)
	return err
}

// UntrashMessage removes a message from trash.
func (c *Client) UntrashMessage(ctx context.Context, messageID string) error {
	path := fmt.Sprintf("/users/me/messages/%s/untrash", messageID)
	_, err := c.doRequest(ctx, http.MethodPost, path, nil)
	return err
}

// DeleteMessage permanently deletes a message.
func (c *Client) DeleteMessage(ctx context.Context, messageID string) error {
	path := fmt.Sprintf("/users/me/messages/%s", messageID)
	_, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	return err
}

// GetMessageBody extracts the text body from a message.
func GetMessageBody(msg *Message, mimeType string) string {
	if msg.Payload == nil {
		return ""
	}

	return getPartBody(msg.Payload, mimeType)
}

// getPartBody recursively searches for a part with the given MIME type.
func getPartBody(part *MessagePart, mimeType string) string {
	if part.MIMEType == mimeType && part.Body != nil && part.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			// Try standard base64
			decoded, err = base64.StdEncoding.DecodeString(part.Body.Data)
			if err != nil {
				return part.Body.Data
			}
		}
		return string(decoded)
	}

	for _, child := range part.Parts {
		if body := getPartBody(child, mimeType); body != "" {
			return body
		}
	}

	return ""
}

// GetHeader extracts a header value from a message.
func GetHeader(msg *Message, name string) string {
	if msg.Payload == nil {
		return ""
	}

	nameLower := strings.ToLower(name)
	for _, h := range msg.Payload.Headers {
		if strings.ToLower(h.Name) == nameLower {
			return h.Value
		}
	}
	return ""
}

// ParseInternalDate converts internal date to time.Time.
func ParseInternalDate(internalDate int64) time.Time {
	return time.UnixMilli(internalDate)
}

// ListThreads lists threads in the user's mailbox.
func (c *Client) ListThreads(ctx context.Context, opts SearchOptions) (*ListThreadsResponse, error) {
	params := url.Values{}

	if opts.Query != "" {
		params.Set("q", opts.Query)
	}

	if len(opts.LabelIDs) > 0 {
		for _, label := range opts.LabelIDs {
			params.Add("labelIds", label)
		}
	}

	if opts.MaxResults > 0 {
		params.Set("maxResults", strconv.Itoa(opts.MaxResults))
	}

	if opts.PageToken != "" {
		params.Set("pageToken", opts.PageToken)
	}

	if opts.IncludeSpam {
		params.Set("includeSpamTrash", "true")
	}

	path := "/users/me/threads"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp ListThreadsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &resp, nil
}

// GetThread retrieves a specific thread by ID.
func (c *Client) GetThread(ctx context.Context, threadID string) (*Thread, error) {
	path := fmt.Sprintf("/users/me/threads/%s", threadID)
	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var thread Thread
	if err := json.Unmarshal(body, &thread); err != nil {
		return nil, fmt.Errorf("parsing thread: %w", err)
	}

	return &thread, nil
}
