// Package calendar provides tools for interacting with the Google Calendar API.
package calendar

import (
	"context"
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
	calendarAPIBaseURL = "https://www.googleapis.com/calendar/v3"
)

// Client provides access to the Google Calendar API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Calendar API client.
func NewClient(accessToken string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &authTransport{
				accessToken: accessToken,
				base:        http.DefaultTransport,
			},
		},
		baseURL: calendarAPIBaseURL,
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

// Calendar represents a Google Calendar.
type Calendar struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
	TimeZone    string `json:"timeZone,omitempty"`
	AccessRole  string `json:"accessRole,omitempty"`
	Primary     bool   `json:"primary,omitempty"`
	ColorID     string `json:"colorId,omitempty"`
	BackgroundColor string `json:"backgroundColor,omitempty"`
	ForegroundColor string `json:"foregroundColor,omitempty"`
	Selected    bool   `json:"selected,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
}

// Event represents a calendar event.
type Event struct {
	ID          string         `json:"id"`
	Summary     string         `json:"summary,omitempty"`
	Description string         `json:"description,omitempty"`
	Location    string         `json:"location,omitempty"`
	ColorID     string         `json:"colorId,omitempty"`
	Start       *EventDateTime `json:"start,omitempty"`
	End         *EventDateTime `json:"end,omitempty"`
	Status      string         `json:"status,omitempty"`
	Created     time.Time      `json:"created,omitempty"`
	Updated     time.Time      `json:"updated,omitempty"`
	Creator     *EventPerson   `json:"creator,omitempty"`
	Organizer   *EventPerson   `json:"organizer,omitempty"`
	Attendees   []EventAttendee `json:"attendees,omitempty"`
	RecurringEventID string    `json:"recurringEventId,omitempty"`
	OriginalStartTime *EventDateTime `json:"originalStartTime,omitempty"`
	Transparency string          `json:"transparency,omitempty"`
	Visibility   string          `json:"visibility,omitempty"`
	ICalUID      string          `json:"iCalUID,omitempty"`
	Sequence     int            `json:"sequence,omitempty"`
	AttendeesOmitted bool        `json:"attendeesOmitted,omitempty"`
	ExtendedProperties *ExtendedProperties `json:"extendedProperties,omitempty"`
	HangoutLink  string         `json:"hangoutLink,omitempty"`
	ConferenceData *ConferenceData `json:"conferenceData,omitempty"`
	Gadget       *Gadget         `json:"gadget,omitempty"`
	AnyoneCanAddSelf bool        `json:"anyoneCanAddSelf,omitempty"`
	GuestsCanInviteOthers bool   `json:"guestsCanInviteOthers,omitempty"`
	GuestsCanModify bool        `json:"guestsCanModify,omitempty"`
	GuestsCanSeeOtherGuests bool `json:"guestsCanSeeOtherGuests,omitempty"`
	PrivateCopy  bool           `json:"privateCopy,omitempty"`
	Locked       bool           `json:"locked,omitempty"`
	Reminders    *Reminders     `json:"reminders,omitempty"`
	Source       *Source        `json:"source,omitempty"`
}

// EventDateTime represents the start or end time of an event.
type EventDateTime struct {
	Date     string `json:"date,omitempty"`     // For all-day events
	DateTime string `json:"dateTime,omitempty"` // For timed events
	TimeZone string `json:"timeZone,omitempty"`
}

// ToTime converts the EventDateTime to a time.Time.
func (e *EventDateTime) ToTime() (time.Time, error) {
	if e.Date != "" {
		return time.Parse("2006-01-02", e.Date)
	}
	if e.DateTime != "" {
		// Try RFC3339
		t, err := time.Parse(time.RFC3339, e.DateTime)
		if err != nil {
			// Try without timezone
			t, err = time.Parse("2006-01-02T15:04:05", e.DateTime)
		}
		return t, err
	}
	return time.Time{}, fmt.Errorf("no date or datetime set")
}

// EventPerson represents a person (creator or organizer).
type EventPerson struct {
	ID             string `json:"id,omitempty"`
	Email          string `json:"email,omitempty"`
	DisplayName    string `json:"displayName,omitempty"`
	Self           bool   `json:"self,omitempty"`
}

// EventAttendee represents an attendee of an event.
type EventAttendee struct {
	ID             string `json:"id,omitempty"`
	Email          string `json:"email"`
	DisplayName    string `json:"displayName,omitempty"`
	Organizer      bool   `json:"organizer,omitempty"`
	Self           bool   `json:"self,omitempty"`
	ResponseStatus string `json:"responseStatus,omitempty"`
	Comment        string `json:"comment,omitempty"`
	AdditionalGuests int  `json:"additionalGuests,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
}

// ExtendedProperties represents extended properties of an event.
type ExtendedProperties struct {
	Private map[string]string `json:"private,omitempty"`
	Shared  map[string]string `json:"shared,omitempty"`
}

// ConferenceData represents conference data for an event.
type ConferenceData struct {
	CreateRequest *CreateConferenceRequest `json:"createRequest,omitempty"`
	EntryPoints   []EntryPoint             `json:"entryPoints,omitempty"`
	ConferenceSolution ConferenceSolution `json:"conferenceSolution,omitempty"`
	ConferenceID  string                   `json:"conferenceId,omitempty"`
	Signature     string                   `json:"signature,omitempty"`
}

// CreateConferenceRequest represents a request to create a conference.
type CreateConferenceRequest struct {
	RequestID        string           `json:"requestId"`
	ConferenceSolutionKey ConferenceSolutionKey `json:"conferenceSolutionKey"`
	Status           ConferenceRequestStatus `json:"status,omitempty"`
}

// ConferenceSolutionKey identifies the conference solution.
type ConferenceSolutionKey struct {
	Type string `json:"type"`
}

// ConferenceRequestStatus represents the status of a conference request.
type ConferenceRequestStatus struct {
	StatusCode string `json:"statusCode,omitempty"`
}

// ConferenceSolution describes a conference solution.
type ConferenceSolution struct {
	Key     ConferenceSolutionKey `json:"key"`
	Name    string                `json:"name,omitempty"`
	IconURI string                `json:"iconUri,omitempty"`
}

// EntryPoint represents an entry point for a conference.
type EntryPoint struct {
	EntryPointType string `json:"entryPointType,omitempty"`
	Uri            string `json:"uri,omitempty"`
	Label          string `json:"label,omitempty"`
	Pin            string `json:"pin,omitempty"`
	AccessCode     string `json:"accessCode,omitempty"`
	MeetingCode    string `json:"meetingCode,omitempty"`
	Passcode       string `json:"passcode,omitempty"`
	Password       string `json:"password,omitempty"`
}

// Gadget represents a gadget for an event.
type Gadget struct {
	Type      string            `json:"type,omitempty"`
	Title     string            `json:"title,omitempty"`
	Link      string            `json:"link,omitempty"`
	IconLink  string            `json:"iconLink,omitempty"`
	Width     int               `json:"width,omitempty"`
	Height    int               `json:"height,omitempty"`
	Display   string            `json:"display,omitempty"`
	Preferences map[string]string `json:"preferences,omitempty"`
}

// Reminders represents reminders for an event.
type Reminders struct {
	UseDefault      bool       `json:"useDefault,omitempty"`
	Overrides       []Reminder `json:"overrides,omitempty"`
}

// Reminder represents a single reminder.
type Reminder struct {
	Method  string `json:"method"`
	Minutes int    `json:"minutes"`
}

// Source represents the source of an event.
type Source struct {
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
}

// ListCalendarsResponse is the response from listing calendars.
type ListCalendarsResponse struct {
	Items         []Calendar `json:"items,omitempty"`
	NextPageToken string     `json:"nextPageToken,omitempty"`
	NextSyncToken string     `json:"nextSyncToken,omitempty"`
}

// ListEventsResponse is the response from listing events.
type ListEventsResponse struct {
	Items         []Event `json:"items,omitempty"`
	NextPageToken string  `json:"nextPageToken,omitempty"`
	NextSyncToken string  `json:"nextSyncToken,omitempty"`
}

// FreeBusyResponse is the response from a free/busy query.
type FreeBusyResponse struct {
	Calendars map[string]*FreeBusyCalendar `json:"calendars,omitempty"`
	Groups    map[string]*FreeBusyGroup    `json:"groups,omitempty"`
	TimeMin   string                       `json:"timeMin,omitempty"`
	TimeMax   string                       `json:"timeMax,omitempty"`
}

// FreeBusyCalendar contains free/busy information for a calendar.
type FreeBusyCalendar struct {
	Errors []Error `json:"errors,omitempty"`
	Busy   []TimePeriod `json:"busy,omitempty"`
}

// FreeBusyGroup contains free/busy information for a group.
type FreeBusyGroup struct {
	Errors    []Error  `json:"errors,omitempty"`
	Calendars []string `json:"calendars,omitempty"`
}

// Error represents an error in the API response.
type Error struct {
	Domain  string `json:"domain,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// TimePeriod represents a period of time.
type TimePeriod struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// ListCalendars lists all calendars for the authenticated user.
func (c *Client) ListCalendars(ctx context.Context) (*ListCalendarsResponse, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/users/me/calendarList", nil)
	if err != nil {
		return nil, err
	}

	var resp ListCalendarsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &resp, nil
}

// GetCalendar retrieves a specific calendar by ID.
func (c *Client) GetCalendar(ctx context.Context, calendarID string) (*Calendar, error) {
	// URL encode calendar ID
	encodedID := url.QueryEscape(calendarID)
	path := fmt.Sprintf("/calendars/%s", encodedID)

	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var cal Calendar
	if err := json.Unmarshal(body, &cal); err != nil {
		return nil, fmt.Errorf("parsing calendar: %w", err)
	}

	return &cal, nil
}

// GetPrimaryCalendar retrieves the user's primary calendar.
func (c *Client) GetPrimaryCalendar(ctx context.Context) (*Calendar, error) {
	return c.GetCalendar(ctx, "primary")
}

// ListEvents lists events in a calendar.
func (c *Client) ListEvents(ctx context.Context, calendarID string, opts ListEventsOptions) (*ListEventsResponse, error) {
	params := url.Values{}

	if !opts.TimeMin.IsZero() {
		params.Set("timeMin", opts.TimeMin.Format(time.RFC3339))
	}

	if !opts.TimeMax.IsZero() {
		params.Set("timeMax", opts.TimeMax.Format(time.RFC3339))
	}

	if opts.MaxResults > 0 {
		params.Set("maxResults", strconv.Itoa(opts.MaxResults))
	}

	if opts.PageToken != "" {
		params.Set("pageToken", opts.PageToken)
	}

	if opts.Query != "" {
		params.Set("q", opts.Query)
	}

	if opts.ShowDeleted {
		params.Set("showDeleted", "true")
	}

	if opts.SingleEvents {
		params.Set("singleEvents", "true")
	}

	if opts.OrderBy != "" {
		params.Set("orderBy", opts.OrderBy)
	}

	// URL encode calendar ID
	encodedID := url.QueryEscape(calendarID)
	path := fmt.Sprintf("/calendars/%s/events", encodedID)

	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var resp ListEventsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &resp, nil
}

// ListEventsOptions contains options for listing events.
type ListEventsOptions struct {
	TimeMin      time.Time
	TimeMax      time.Time
	MaxResults   int
	PageToken    string
	Query        string
	ShowDeleted  bool
	SingleEvents bool
	OrderBy      string // "startTime" or "updated"
}

// GetEvent retrieves a specific event.
func (c *Client) GetEvent(ctx context.Context, calendarID, eventID string) (*Event, error) {
	encodedCalendarID := url.QueryEscape(calendarID)
	encodedEventID := url.QueryEscape(eventID)
	path := fmt.Sprintf("/calendars/%s/events/%s", encodedCalendarID, encodedEventID)

	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var event Event
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("parsing event: %w", err)
	}

	return &event, nil
}

// CreateEventRequest contains data for creating an event.
type CreateEventRequest struct {
	Summary     string         `json:"summary,omitempty"`
	Description string         `json:"description,omitempty"`
	Location    string         `json:"location,omitempty"`
	Start       *EventDateTime `json:"start,omitempty"`
	End         *EventDateTime `json:"end,omitempty"`
	Attendees   []EventAttendee `json:"attendees,omitempty"`
	Reminders   *Reminders     `json:"reminders,omitempty"`
}

// CreateEvent creates a new event in a calendar.
func (c *Client) CreateEvent(ctx context.Context, calendarID string, req *CreateEventRequest) (*Event, error) {
	bodyData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	encodedID := url.QueryEscape(calendarID)
	path := fmt.Sprintf("/calendars/%s/events", encodedID)

	body, err := c.doRequest(ctx, http.MethodPost, path, strings.NewReader(string(bodyData)))
	if err != nil {
		return nil, err
	}

	var event Event
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("parsing event: %w", err)
	}

	return &event, nil
}

// UpdateEvent updates an existing event.
func (c *Client) UpdateEvent(ctx context.Context, calendarID, eventID string, req *CreateEventRequest) (*Event, error) {
	bodyData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	encodedCalendarID := url.QueryEscape(calendarID)
	encodedEventID := url.QueryEscape(eventID)
	path := fmt.Sprintf("/calendars/%s/events/%s", encodedCalendarID, encodedEventID)

	body, err := c.doRequest(ctx, http.MethodPut, path, strings.NewReader(string(bodyData)))
	if err != nil {
		return nil, err
	}

	var event Event
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("parsing event: %w", err)
	}

	return &event, nil
}

// PatchEvent partially updates an event.
func (c *Client) PatchEvent(ctx context.Context, calendarID, eventID string, updates map[string]interface{}) (*Event, error) {
	bodyData, err := json.Marshal(updates)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	encodedCalendarID := url.QueryEscape(calendarID)
	encodedEventID := url.QueryEscape(eventID)
	path := fmt.Sprintf("/calendars/%s/events/%s", encodedCalendarID, encodedEventID)

	body, err := c.doRequest(ctx, http.MethodPatch, path, strings.NewReader(string(bodyData)))
	if err != nil {
		return nil, err
	}

	var event Event
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("parsing event: %w", err)
	}

	return &event, nil
}

// DeleteEvent deletes an event.
func (c *Client) DeleteEvent(ctx context.Context, calendarID, eventID string) error {
	encodedCalendarID := url.QueryEscape(calendarID)
	encodedEventID := url.QueryEscape(eventID)
	path := fmt.Sprintf("/calendars/%s/events/%s", encodedCalendarID, encodedEventID)

	_, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	return err
}
