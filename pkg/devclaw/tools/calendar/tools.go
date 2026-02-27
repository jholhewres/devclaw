// Package calendar provides agent tools for Google Calendar operations.
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/auth/profiles"
)

// ToolContext provides access to auth profiles for Calendar tools.
type ToolContext struct {
	ProfileManager profiles.ProfileManager
	ProfileID      string // Optional: specific profile to use
}

// resolveToken resolves a Calendar OAuth token from profiles.
func resolveToken(ctx context.Context, tc *ToolContext) (string, *profiles.AuthProfile, error) {
	if tc.ProfileManager == nil {
		return "", nil, fmt.Errorf("profile manager not available")
	}

	opts := profiles.ResolutionOptions{
		Provider:     "google-calendar",
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
			return "", nil, fmt.Errorf("no valid Calendar profile found: %w", result.Error)
		}
	}

	if result.Profile == nil {
		return "", nil, fmt.Errorf("no Calendar profile found")
	}

	return result.Credential, result.Profile, nil
}

// ListCalendarsArgs contains arguments for listing calendars.
type ListCalendarsArgs struct{}

// CalendarInfo contains information about a calendar.
type CalendarInfo struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description,omitempty"`
	TimeZone    string `json:"time_zone,omitempty"`
	AccessRole  string `json:"access_role,omitempty"`
	Primary     bool   `json:"primary,omitempty"`
	Color       string `json:"color,omitempty"`
}

// ListCalendarsResult contains the result of listing calendars.
type ListCalendarsResult struct {
	Calendars    []CalendarInfo `json:"calendars"`
	Primary      *CalendarInfo  `json:"primary,omitempty"`
	ProfileUsed  string         `json:"profile_used,omitempty"`
	ProfileEmail string         `json:"profile_email,omitempty"`
	Error        string         `json:"error,omitempty"`
}

// ListCalendars lists all calendars for the user.
func ListCalendars(ctx context.Context, tc *ToolContext, args ListCalendarsArgs) (*ListCalendarsResult, error) {
	token, profile, err := resolveToken(ctx, tc)
	if err != nil {
		return &ListCalendarsResult{
			Error: err.Error(),
		}, nil
	}

	client := NewClient(token)

	resp, err := client.ListCalendars(ctx)
	if err != nil {
		return &ListCalendarsResult{
			Error: fmt.Sprintf("failed to list calendars: %v", err),
		}, nil
	}

	result := &ListCalendarsResult{
		ProfileUsed:  string(profile.ID),
		ProfileEmail: profile.OAuth.Email,
	}

	for _, cal := range resp.Items {
		info := CalendarInfo{
			ID:          cal.ID,
			Summary:     cal.Summary,
			Description: cal.Description,
			TimeZone:    cal.TimeZone,
			AccessRole:  cal.AccessRole,
			Primary:     cal.Primary,
			Color:       cal.BackgroundColor,
		}

		result.Calendars = append(result.Calendars, info)

		if cal.Primary {
			result.Primary = &info
		}
	}

	return result, nil
}

// ListEventsArgs contains arguments for listing events.
type ListEventsArgs struct {
	CalendarID string `json:"calendar_id,omitempty"` // "primary" for main calendar
	StartDate  string `json:"start_date,omitempty"`    // ISO date (YYYY-MM-DD) or "today", "tomorrow"
	EndDate    string `json:"end_date,omitempty"`      // ISO date or duration like "1week"
	MaxResults int    `json:"max_results,omitempty"`   // Default 10, max 50
	Query      string `json:"query,omitempty"`         // Search query
}

// EventSummary contains a summary of an event.
type EventSummary struct {
	ID           string    `json:"id"`
	Summary      string    `json:"summary"`
	Description  string    `json:"description,omitempty"`
	Location     string    `json:"location,omitempty"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	AllDay       bool      `json:"all_day"`
	Creator      string    `json:"creator,omitempty"`
	Organizer    string    `json:"organizer,omitempty"`
	Attendees    int       `json:"attendees,omitempty"`
	Status       string    `json:"status,omitempty"`
	HangoutLink  string    `json:"hangout_link,omitempty"`
}

// ListEventsResult contains the result of listing events.
type ListEventsResult struct {
	Events        []EventSummary `json:"events"`
	Total         int            `json:"total"`
	HasMore       bool           `json:"has_more"`
	NextPageToken string         `json:"next_page_token,omitempty"`
	CalendarID    string         `json:"calendar_id"`
	ProfileUsed   string         `json:"profile_used,omitempty"`
	ProfileEmail  string         `json:"profile_email,omitempty"`
	Error         string         `json:"error,omitempty"`
}

// ListEvents lists events from a calendar.
func ListEvents(ctx context.Context, tc *ToolContext, args ListEventsArgs) (*ListEventsResult, error) {
	token, profile, err := resolveToken(ctx, tc)
	if err != nil {
		return &ListEventsResult{
			Error: err.Error(),
		}, nil
	}

	calendarID := args.CalendarID
	if calendarID == "" {
		calendarID = "primary"
	}

	maxResults := args.MaxResults
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 10
	}

	// Parse dates
	var timeMin, timeMax time.Time
	now := time.Now()

	if args.StartDate == "" || args.StartDate == "today" {
		timeMin = now
	} else if args.StartDate == "tomorrow" {
		timeMin = now.AddDate(0, 0, 1)
	} else {
		timeMin, _ = time.Parse("2006-01-02", args.StartDate)
		if timeMin.IsZero() {
			timeMin = now
		}
	}

	if args.EndDate == "" {
		// Default to 1 week
		timeMax = timeMin.AddDate(0, 0, 7)
	} else if strings.HasSuffix(args.EndDate, "week") {
		weeks := 1
		fmt.Sscanf(args.EndDate, "%d", &weeks)
		timeMax = timeMin.AddDate(0, 0, weeks*7)
	} else if strings.HasSuffix(args.EndDate, "day") {
		days := 1
		fmt.Sscanf(args.EndDate, "%d", &days)
		timeMax = timeMin.AddDate(0, 0, days)
	} else {
		timeMax, _ = time.Parse("2006-01-02", args.EndDate)
		if timeMax.IsZero() {
			timeMax = timeMin.AddDate(0, 0, 7)
		}
	}

	opts := ListEventsOptions{
		TimeMin:      timeMin,
		TimeMax:      timeMax,
		MaxResults:   maxResults,
		Query:        args.Query,
		SingleEvents: true, // Expand recurring events
		OrderBy:      "startTime",
	}

	client := NewClient(token)

	resp, err := client.ListEvents(ctx, calendarID, opts)
	if err != nil {
		return &ListEventsResult{
			Error:      fmt.Sprintf("failed to list events: %v", err),
			CalendarID: calendarID,
		}, nil
	}

	result := &ListEventsResult{
		Total:         len(resp.Items),
		HasMore:       resp.NextPageToken != "",
		NextPageToken: resp.NextPageToken,
		CalendarID:    calendarID,
		ProfileUsed:   string(profile.ID),
		ProfileEmail:  profile.OAuth.Email,
	}

	for _, event := range resp.Items {
		summary := EventSummary{
			ID:          event.ID,
			Summary:     event.Summary,
			Description: event.Description,
			Location:    event.Location,
			Status:      event.Status,
			HangoutLink: event.HangoutLink,
		}

		if event.Creator != nil {
			summary.Creator = event.Creator.Email
		}
		if event.Organizer != nil {
			summary.Organizer = event.Organizer.Email
		}

		summary.Attendees = len(event.Attendees)

		// Parse times
		if event.Start != nil {
			if event.Start.Date != "" {
				// All-day event
				summary.AllDay = true
				summary.Start, _ = time.Parse("2006-01-02", event.Start.Date)
			} else if event.Start.DateTime != "" {
				summary.Start, _ = time.Parse(time.RFC3339, event.Start.DateTime)
			}
		}

		if event.End != nil {
			if event.End.Date != "" {
				summary.End, _ = time.Parse("2006-01-02", event.End.Date)
			} else if event.End.DateTime != "" {
				summary.End, _ = time.Parse(time.RFC3339, event.End.DateTime)
			}
		}

		result.Events = append(result.Events, summary)
	}

	return result, nil
}

// CreateEventArgs contains arguments for creating an event.
type CreateEventArgs struct {
	CalendarID  string `json:"calendar_id,omitempty"` // "primary" for main calendar
	Summary     string `json:"summary"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
	StartTime   string `json:"start_time"`  // ISO 8601 or natural language
	EndTime     string `json:"end_time"`    // ISO 8601 or natural language
	AllDay      bool   `json:"all_day,omitempty"`
	Attendees   string `json:"attendees,omitempty"` // Comma-separated emails
}

// CreateEventResult contains the result of creating an event.
type CreateEventResult struct {
	Success      bool   `json:"success"`
	EventID      string `json:"event_id,omitempty"`
	HTMLLink     string `json:"html_link,omitempty"`
	CalendarID   string `json:"calendar_id"`
	Error        string `json:"error,omitempty"`
	ProfileUsed  string `json:"profile_used,omitempty"`
	ProfileEmail string `json:"profile_email,omitempty"`
}

// CreateEvent creates a new calendar event.
func CreateEvent(ctx context.Context, tc *ToolContext, args CreateEventArgs) (*CreateEventResult, error) {
	if args.Summary == "" {
		return &CreateEventResult{
			Success: false,
			Error:   "summary is required",
		}, nil
	}

	token, profile, err := resolveToken(ctx, tc)
	if err != nil {
		return &CreateEventResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	calendarID := args.CalendarID
	if calendarID == "" {
		calendarID = "primary"
	}

	// Parse start/end times
	var start, end *EventDateTime

	if args.AllDay {
		// Parse as dates
		startDate := args.StartTime
		endDate := args.EndTime

		// Try to parse various formats
		if t, err := time.Parse("2006-01-02", startDate); err == nil {
			startDate = t.Format("2006-01-02")
		}
		if endDate == "" {
			// Default to same day
			endDate = startDate
		} else if t, err := time.Parse("2006-01-02", endDate); err == nil {
			endDate = t.Format("2006-01-02")
		}

		start = &EventDateTime{Date: startDate}
		end = &EventDateTime{Date: endDate}
	} else {
		// Parse as datetimes
		startTime := parseDateTime(args.StartTime)
		endTime := parseDateTime(args.EndTime)

		if endTime.IsZero() {
			// Default to 1 hour later
			endTime = startTime.Add(time.Hour)
		}

		start = &EventDateTime{
			DateTime: startTime.Format(time.RFC3339),
			TimeZone: time.Local.String(),
		}
		end = &EventDateTime{
			DateTime: endTime.Format(time.RFC3339),
			TimeZone: time.Local.String(),
		}
	}

	req := &CreateEventRequest{
		Summary:     args.Summary,
		Description: args.Description,
		Location:    args.Location,
		Start:       start,
		End:         end,
	}

	// Add attendees
	if args.Attendees != "" {
		emails := strings.Split(args.Attendees, ",")
		for _, email := range emails {
			email = strings.TrimSpace(email)
			if email != "" {
				req.Attendees = append(req.Attendees, EventAttendee{
					Email: email,
				})
			}
		}
	}

	client := NewClient(token)

	event, err := client.CreateEvent(ctx, calendarID, req)
	if err != nil {
		return &CreateEventResult{
			Success:    false,
			Error:      fmt.Sprintf("failed to create event: %v", err),
			CalendarID: calendarID,
		}, nil
	}

	return &CreateEventResult{
		Success:      true,
		EventID:      event.ID,
		HTMLLink:     fmt.Sprintf("https://calendar.google.com/calendar/event?eid=%s", event.ID),
		CalendarID:   calendarID,
		ProfileUsed:  string(profile.ID),
		ProfileEmail: profile.OAuth.Email,
	}, nil
}

// parseDateTime attempts to parse a datetime string in various formats.
func parseDateTime(s string) time.Time {
	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}

	// Try common formats
	formats := []string{
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
		"01/02/2006 15:04",
		"Jan 2, 2006 15:04",
		"January 2, 2006 15:04",
		"15:04", // Time only - assume today
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, s, time.Local); err == nil {
			// If it's time only, add today's date
			if format == "15:04" {
				now := time.Now()
				return time.Date(now.Year(), now.Month(), now.Day(),
					t.Hour(), t.Minute(), 0, 0, time.Local)
			}
			return t
		}
	}

	// Default to now
	return time.Now()
}

// UpdateEventArgs contains arguments for updating an event.
type UpdateEventArgs struct {
	CalendarID  string `json:"calendar_id,omitempty"`
	EventID     string `json:"event_id"`
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
	StartTime   string `json:"start_time,omitempty"`
	EndTime     string `json:"end_time,omitempty"`
}

// UpdateEventResult contains the result of updating an event.
type UpdateEventResult struct {
	Success      bool   `json:"success"`
	EventID      string `json:"event_id"`
	HTMLLink     string `json:"html_link,omitempty"`
	CalendarID   string `json:"calendar_id"`
	Error        string `json:"error,omitempty"`
}

// UpdateEvent updates an existing calendar event.
func UpdateEvent(ctx context.Context, tc *ToolContext, args UpdateEventArgs) (*UpdateEventResult, error) {
	if args.EventID == "" {
		return &UpdateEventResult{
			Success: false,
			Error:   "event_id is required",
		}, nil
	}

	token, _, err := resolveToken(ctx, tc)
	if err != nil {
		return &UpdateEventResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	calendarID := args.CalendarID
	if calendarID == "" {
		calendarID = "primary"
	}

	// Get existing event first
	client := NewClient(token)
	existing, err := client.GetEvent(ctx, calendarID, args.EventID)
	if err != nil {
		return &UpdateEventResult{
			Success:    false,
			Error:      fmt.Sprintf("failed to get existing event: %v", err),
			EventID:    args.EventID,
			CalendarID: calendarID,
		}, nil
	}

	// Build update request
	req := &CreateEventRequest{
		Summary:     existing.Summary,
		Description: existing.Description,
		Location:    existing.Location,
		Start:       existing.Start,
		End:         existing.End,
	}

	// Apply updates
	if args.Summary != "" {
		req.Summary = args.Summary
	}
	if args.Description != "" {
		req.Description = args.Description
	}
	if args.Location != "" {
		req.Location = args.Location
	}
	if args.StartTime != "" {
		req.Start = &EventDateTime{
			DateTime: parseDateTime(args.StartTime).Format(time.RFC3339),
			TimeZone: time.Local.String(),
		}
	}
	if args.EndTime != "" {
		req.End = &EventDateTime{
			DateTime: parseDateTime(args.EndTime).Format(time.RFC3339),
			TimeZone: time.Local.String(),
		}
	}

	event, err := client.UpdateEvent(ctx, calendarID, args.EventID, req)
	if err != nil {
		return &UpdateEventResult{
			Success:    false,
			Error:      fmt.Sprintf("failed to update event: %v", err),
			EventID:    args.EventID,
			CalendarID: calendarID,
		}, nil
	}

	return &UpdateEventResult{
		Success:    true,
		EventID:    event.ID,
		HTMLLink:   fmt.Sprintf("https://calendar.google.com/calendar/event?eid=%s", event.ID),
		CalendarID: calendarID,
	}, nil
}

// DeleteEventArgs contains arguments for deleting an event.
type DeleteEventArgs struct {
	CalendarID string `json:"calendar_id,omitempty"`
	EventID    string `json:"event_id"`
}

// DeleteEventResult contains the result of deleting an event.
type DeleteEventResult struct {
	Success    bool   `json:"success"`
	EventID    string `json:"event_id"`
	CalendarID string `json:"calendar_id"`
	Error      string `json:"error,omitempty"`
}

// DeleteEvent deletes a calendar event.
func DeleteEvent(ctx context.Context, tc *ToolContext, args DeleteEventArgs) (*DeleteEventResult, error) {
	if args.EventID == "" {
		return &DeleteEventResult{
			Success: false,
			Error:   "event_id is required",
		}, nil
	}

	token, _, err := resolveToken(ctx, tc)
	if err != nil {
		return &DeleteEventResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	calendarID := args.CalendarID
	if calendarID == "" {
		calendarID = "primary"
	}

	client := NewClient(token)

	if err := client.DeleteEvent(ctx, calendarID, args.EventID); err != nil {
		return &DeleteEventResult{
			Success:    false,
			Error:      fmt.Sprintf("failed to delete event: %v", err),
			EventID:    args.EventID,
			CalendarID: calendarID,
		}, nil
	}

	return &DeleteEventResult{
		Success:    true,
		EventID:    args.EventID,
		CalendarID: calendarID,
	}, nil
}

// FormatCalendarList formats a calendar list for display.
func FormatCalendarList(result *ListCalendarsResult) string {
	if len(result.Calendars) == 0 {
		return "No calendars found."
	}

	var lines []string
	lines = append(lines, "Calendars:\n")

	for _, cal := range result.Calendars {
		marker := "  "
		if cal.Primary {
			marker = "★ "
		}

		access := ""
		if cal.AccessRole != "owner" {
			access = fmt.Sprintf("[%s]", cal.AccessRole)
		}

		lines = append(lines, fmt.Sprintf("%s%s %s", marker, cal.Summary, access))
		if cal.Description != "" {
			lines = append(lines, fmt.Sprintf("     %s", cal.Description))
		}
	}

	return strings.Join(lines, "\n")
}

// FormatEventList formats an event list for display.
func FormatEventList(result *ListEventsResult) string {
	if len(result.Events) == 0 {
		return "No events found."
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Events (%d total):\n", result.Total))

	var currentDate string

	for _, event := range result.Events {
		// Group by date
		dateStr := event.Start.Format("Monday, January 2")
		if dateStr != currentDate {
			currentDate = dateStr
			lines = append(lines, fmt.Sprintf("\n%s:", dateStr))
		}

		timeStr := "All day"
		if !event.AllDay {
			timeStr = fmt.Sprintf("%s - %s",
				event.Start.Format("3:04 PM"),
				event.End.Format("3:04 PM"))
		}

		status := ""
		if event.Status != "confirmed" {
			status = fmt.Sprintf("[%s]", event.Status)
		}

		summary := event.Summary
		if summary == "" {
			summary = "(no title)"
		}

		lines = append(lines, fmt.Sprintf("  %s %s %s", timeStr, summary, status))

		if event.Location != "" {
			lines = append(lines, fmt.Sprintf("    📍 %s", event.Location))
		}

		if len(event.HangoutLink) > 0 {
			lines = append(lines, fmt.Sprintf("    🔗 Video call available"))
		}
	}

	if result.HasMore {
		lines = append(lines, "\n(More events available...)")
	}

	return strings.Join(lines, "\n")
}

// FormatCreateEventResult formats a create event result for display.
func FormatCreateEventResult(result *CreateEventResult) string {
	if result.Error != "" {
		return fmt.Sprintf("Error: %s", result.Error)
	}

	if result.Success {
		return fmt.Sprintf("✓ Event created successfully!\nView: %s", result.HTMLLink)
	}

	return "Failed to create event."
}

// JSON helpers
func (r *ListCalendarsResult) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

func (r *ListEventsResult) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

func (r *CreateEventResult) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}
