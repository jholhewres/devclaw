// ---------- Google API Tool ----------

package copilot

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

	"github.com/jholhewres/devclaw/pkg/devclaw/auth/profiles"
	"github.com/jholhewres/devclaw/pkg/devclaw/tools/calendar"
	"github.com/jholhewres/devclaw/pkg/devclaw/tools/gmail"
)

// registerGoogleAPITool registers the google_api tool for accessing Google services
// (Gmail, Calendar, Drive, Sheets, Docs, Slides, Contacts, Tasks) via OAuth.
// This generic tool requires the user to have configured an OAuth profile via auth_profile_add before use.
func registerGoogleAPITool(executor *ToolExecutor) {
	executor.Register(
		MakeToolDefinition("google_api",
			"Make authenticated API calls to Google Workspace services (Gmail, Calendar, Drive, Sheets, Docs, Slides, Contacts, Tasks). "+
				"Requires an OAuth profile to be configured first with 'auth_profile_add'. "+
				"Supports all standard Google REST API endpoints. Results are returned as JSON. "+
				"Common services: 'gmail' (messages), 'calendar' (events), 'drive' (files), 'sheets' (spreadsheets), "+
				"'docs' (documents), 'slides' (presentations), 'contacts' (people), 'tasks' (task lists).",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"service": map[string]any{
						"type":        "string",
						"enum":        []string{"gmail", "calendar", "drive", "sheets", "docs", "slides", "contacts", "tasks", "people"},
						"description": "Google service to access: gmail, calendar, drive, sheets, docs, slides, contacts, tasks, or people",
					},
					"endpoint": map[string]any{
						"type":        "string",
						"description": "API endpoint path (e.g., '/users/me/messages', '/calendars/primary/events')",
					},
					"method": map[string]any{
						"type":        "string",
						"enum":        []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
						"default":     "GET",
						"description": "HTTP method for the request",
					},
					"params": map[string]any{
						"type":        "object",
						"description": "Query parameters for the request (as key-value pairs)",
					},
					"body": map[string]any{
						"type":        "object",
						"description": "Request body for POST/PUT/PATCH requests (as JSON object)",
					},
					"profile": map[string]any{
						"type":        "string",
						"description": "Optional: specific auth profile to use (e.g., 'work', 'personal'). If not provided, uses the default or first available profile for the service.",
					},
				},
				"required": []string{"service", "endpoint"},
			},
		),
		func(ctx context.Context, args map[string]any) (any, error) {
			service, _ := args["service"].(string)
			endpoint, _ := args["endpoint"].(string)
			method, _ := args["method"].(string)
			if method == "" {
				method = "GET"
			}

			// Extract optional params and body
			var params map[string]string
			if p, ok := args["params"].(map[string]any); ok {
				params = make(map[string]string)
				for k, v := range p {
					params[k] = fmt.Sprintf("%v", v)
				}
			}

			var body map[string]any
			if b, ok := args["body"].(map[string]any); ok {
				body = b
			}

			profileName, _ := args["profile"].(string)

			// Map service to provider
			provider := "google"
			switch service {
			case "gmail":
				provider = "google-gmail"
			case "calendar":
				provider = "google-calendar"
			case "drive":
				provider = "google-drive"
			case "sheets":
				provider = "google-sheets"
			case "docs":
				provider = "google-docs"
			case "slides":
				provider = "google-slides"
			case "contacts":
				provider = "google-contacts"
			case "tasks":
				provider = "google-tasks"
			case "people":
				provider = "google-people"
			}

			// Check if profile manager is available
			pm := executor.ProfileManager()
			if pm == nil {
				return map[string]any{
					"success":  false,
					"error":    "Profile manager not initialized. Please configure OAuth profiles using auth_profile_add.",
					"service":  service,
					"endpoint": endpoint,
					"method":   method,
				}, nil
			}

			// Resolve the OAuth token for the service
			resolutionOpts := profiles.ResolutionOptions{
				Provider:         provider,
				PreferredProfile: profileName,
				Preference:       profiles.PreferValid,
				RequireValid:     true,
				Mode:             profiles.ModeOAuth,
			}

			result := pm.Resolve(resolutionOpts)
			if result.Error != nil {
				// Try generic google provider as fallback
				resolutionOpts.Provider = "google"
				result = pm.Resolve(resolutionOpts)
				if result.Error != nil {
					return map[string]any{
						"success":   false,
						"error":     fmt.Sprintf("No valid OAuth profile found for %s. Configure with auth_profile_add(provider='%s', name='default', mode='oauth')", service, provider),
						"service":   service,
						"endpoint":  endpoint,
						"method":    method,
						"hint":      "Use auth_profile_list to see configured profiles",
					}, nil
				}
			}

			// Mark profile as used
			pm.MarkUsed(result.Profile.ID)

			// Make the API call based on service
			switch service {
			case "gmail":
				return executeGmailAPI(ctx, result.Credential, endpoint, method, params, body)
			case "calendar":
				return executeCalendarAPI(ctx, result.Credential, endpoint, method, params, body)
			case "drive":
				return executeDriveAPI(ctx, result.Credential, endpoint, method, params, body)
			case "sheets":
				return executeSheetsAPI(ctx, result.Credential, endpoint, method, params, body)
			case "docs":
				return executeDocsAPI(ctx, result.Credential, endpoint, method, params, body)
			case "slides":
				return executeSlidesAPI(ctx, result.Credential, endpoint, method, params, body)
			case "contacts":
				return executeContactsAPI(ctx, result.Credential, endpoint, method, params, body)
			case "tasks":
				return executeTasksAPI(ctx, result.Credential, endpoint, method, params, body)
			case "people":
				return executePeopleAPI(ctx, result.Credential, endpoint, method, params, body)
			default:
				return map[string]any{
					"success":  false,
					"error":    fmt.Sprintf("Unknown service: %s", service),
					"service":  service,
					"endpoint": endpoint,
				}, nil
			}
		},
	)
}

// executeGmailAPI executes Gmail API calls.
func executeGmailAPI(ctx context.Context, token, endpoint, method string, params map[string]string, body map[string]any) (any, error) {
	client := gmail.NewClient(token)

	// Parse endpoint and route to appropriate client method
	switch endpoint {
	case "/users/me/messages":
		if method == "GET" {
			// List/search messages
			opts := gmail.SearchOptions{
				MaxResults: 10,
			}
			if q, ok := params["q"]; ok {
				opts.Query = q
			}
			if max, ok := params["maxResults"]; ok {
				if n, err := strconv.Atoi(max); err == nil {
					opts.MaxResults = n
				}
			}
			if pageToken, ok := params["pageToken"]; ok {
				opts.PageToken = pageToken
			}

			resp, err := client.ListMessages(ctx, opts)
			if err != nil {
				return map[string]any{"success": false, "error": err.Error()}, nil
			}

			return map[string]any{
				"success":            true,
				"messages":           resp.Messages,
				"nextPageToken":      resp.NextPageToken,
				"resultSizeEstimate": resp.ResultSizeEstimate,
			}, nil
		}

	case "/users/me/labels":
		if method == "GET" {
			resp, err := client.ListLabels(ctx)
			if err != nil {
				return map[string]any{"success": false, "error": err.Error()}, nil
			}
			return map[string]any{"success": true, "labels": resp.Labels}, nil
		}

	default:
		// Handle dynamic endpoints like /users/me/messages/{id}
		if strings.HasPrefix(endpoint, "/users/me/messages/") {
			parts := strings.Split(endpoint, "/")
			if len(parts) >= 5 {
				messageID := parts[4]

				if method == "GET" {
					msg, err := client.GetMessageFull(ctx, messageID)
					if err != nil {
						return map[string]any{"success": false, "error": err.Error()}, nil
					}
					return map[string]any{"success": true, "message": msg}, nil
				}

				if method == "POST" && strings.HasSuffix(endpoint, "/modify") {
					// Modify labels (mark as read, archive, etc.)
					req := &gmail.ModifyMessageRequest{}
					if addLabels, ok := body["addLabelIds"].([]interface{}); ok {
						for _, l := range addLabels {
							if s, ok := l.(string); ok {
								req.AddLabelIDs = append(req.AddLabelIDs, s)
							}
						}
					}
					if removeLabels, ok := body["removeLabelIds"].([]interface{}); ok {
						for _, l := range removeLabels {
							if s, ok := l.(string); ok {
								req.RemoveLabelIDs = append(req.RemoveLabelIDs, s)
							}
						}
					}

					msg, err := client.ModifyMessage(ctx, messageID, req)
					if err != nil {
						return map[string]any{"success": false, "error": err.Error()}, nil
					}
					return map[string]any{"success": true, "message": msg}, nil
				}
			}
		}
	}

	return map[string]any{
		"success":  false,
		"error":    "Gmail endpoint not implemented: " + endpoint + " with method " + method,
		"endpoint": endpoint,
		"method":   method,
	}, nil
}

// executeCalendarAPI executes Calendar API calls.
func executeCalendarAPI(ctx context.Context, token, endpoint, method string, params map[string]string, body map[string]any) (any, error) {
	client := calendar.NewClient(token)

	switch endpoint {
	case "/users/me/calendarList":
		if method == "GET" {
			resp, err := client.ListCalendars(ctx)
			if err != nil {
				return map[string]any{"success": false, "error": err.Error()}, nil
			}
			return map[string]any{"success": true, "calendars": resp.Items}, nil
		}

	default:
		// Handle /calendars/{calendarId}/events
		if strings.Contains(endpoint, "/events") {
			parts := strings.Split(endpoint, "/")
			if len(parts) >= 3 {
				calendarID := parts[2]
				if calendarID == "primary" || calendarID == "" {
					calendarID = "primary"
				}

				// Check if this is a specific event operation
				if len(parts) >= 5 && parts[4] != "" {
					// Handle update/delete for specific events
					eventID := parts[4]

					if method == "GET" {
						event, err := client.GetEvent(ctx, calendarID, eventID)
						if err != nil {
							return map[string]any{"success": false, "error": err.Error()}, nil
						}
						return map[string]any{"success": true, "event": event}, nil
					}

					if method == "PUT" || method == "PATCH" {
						// Build update request from body
						req := &calendar.CreateEventRequest{}
						if summary, ok := body["summary"].(string); ok {
							req.Summary = summary
						}
						if description, ok := body["description"].(string); ok {
							req.Description = description
						}
						if location, ok := body["location"].(string); ok {
							req.Location = location
						}

						if start, ok := body["start"].(map[string]any); ok {
							req.Start = &calendar.EventDateTime{
								DateTime: getString(start, "dateTime"),
								Date:     getString(start, "date"),
								TimeZone: getString(start, "timeZone"),
							}
						}
						if end, ok := body["end"].(map[string]any); ok {
							req.End = &calendar.EventDateTime{
								DateTime: getString(end, "dateTime"),
								Date:     getString(end, "date"),
								TimeZone: getString(end, "timeZone"),
							}
						}

						var event *calendar.Event
						var err error
						if method == "PUT" {
							event, err = client.UpdateEvent(ctx, calendarID, eventID, req)
						} else {
							updates := make(map[string]interface{})
							for k, v := range body {
								updates[k] = v
							}
							event, err = client.PatchEvent(ctx, calendarID, eventID, updates)
						}

						if err != nil {
							return map[string]any{"success": false, "error": err.Error()}, nil
						}
						return map[string]any{"success": true, "event": event}, nil
					}

					if method == "DELETE" {
						err := client.DeleteEvent(ctx, calendarID, eventID)
						if err != nil {
							return map[string]any{"success": false, "error": err.Error()}, nil
						}
						return map[string]any{"success": true, "deleted": true, "eventId": eventID}, nil
					}
				} else {
					// List or create events
					if method == "GET" {
						opts := calendar.ListEventsOptions{
							SingleEvents: true,
							OrderBy:      "startTime",
						}

						// Parse params
						if timeMin, ok := params["timeMin"]; ok {
							t, _ := time.Parse(time.RFC3339, timeMin)
							opts.TimeMin = t
						}
						if timeMax, ok := params["timeMax"]; ok {
							t, _ := time.Parse(time.RFC3339, timeMax)
							opts.TimeMax = t
						}
						if q, ok := params["q"]; ok {
							opts.Query = q
						}
						if maxResults, ok := params["maxResults"]; ok {
							if n, err := strconv.Atoi(maxResults); err == nil {
								opts.MaxResults = n
							}
						}

						resp, err := client.ListEvents(ctx, calendarID, opts)
						if err != nil {
							return map[string]any{"success": false, "error": err.Error()}, nil
						}
						return map[string]any{
							"success":       true,
							"events":        resp.Items,
							"nextPageToken": resp.NextPageToken,
						}, nil
					}

					if method == "POST" {
						// Create event
						req := &calendar.CreateEventRequest{
							Summary:     getString(body, "summary"),
							Description: getString(body, "description"),
							Location:    getString(body, "location"),
						}

						// Parse start/end times
						if start, ok := body["start"].(map[string]any); ok {
							req.Start = &calendar.EventDateTime{
								DateTime: getString(start, "dateTime"),
								Date:     getString(start, "date"),
								TimeZone: getString(start, "timeZone"),
							}
						}
						if end, ok := body["end"].(map[string]any); ok {
							req.End = &calendar.EventDateTime{
								DateTime: getString(end, "dateTime"),
								Date:     getString(end, "date"),
								TimeZone: getString(end, "timeZone"),
							}
						}

						// Parse attendees
						if attendees, ok := body["attendees"].([]interface{}); ok {
							for _, a := range attendees {
								if email, ok := a.(string); ok {
									req.Attendees = append(req.Attendees, calendar.EventAttendee{Email: email})
								} else if attendeeMap, ok := a.(map[string]any); ok {
									req.Attendees = append(req.Attendees, calendar.EventAttendee{
										Email: getString(attendeeMap, "email"),
									})
								}
							}
						}

						event, err := client.CreateEvent(ctx, calendarID, req)
						if err != nil {
							return map[string]any{"success": false, "error": err.Error()}, nil
						}
						return map[string]any{"success": true, "event": event}, nil
					}
				}
			}
		}
	}

	return map[string]any{
		"success":  false,
		"error":    "Calendar endpoint not implemented: " + endpoint + " with method " + method,
		"endpoint": endpoint,
		"method":   method,
	}, nil
}

// executeDriveAPI executes Drive API calls via REST.
func executeDriveAPI(ctx context.Context, token, endpoint, method string, params map[string]string, body map[string]any) (any, error) {
	return executeGoogleAPI(ctx, token, "https://www.googleapis.com/drive/v3", endpoint, method, params, body)
}

// executeSheetsAPI executes Google Sheets API calls via REST.
func executeSheetsAPI(ctx context.Context, token, endpoint, method string, params map[string]string, body map[string]any) (any, error) {
	return executeGoogleAPI(ctx, token, "https://sheets.googleapis.com/v4", endpoint, method, params, body)
}

// executeDocsAPI executes Google Docs API calls via REST.
func executeDocsAPI(ctx context.Context, token, endpoint, method string, params map[string]string, body map[string]any) (any, error) {
	return executeGoogleAPI(ctx, token, "https://docs.googleapis.com/v1", endpoint, method, params, body)
}

// executeSlidesAPI executes Google Slides API calls via REST.
func executeSlidesAPI(ctx context.Context, token, endpoint, method string, params map[string]string, body map[string]any) (any, error) {
	return executeGoogleAPI(ctx, token, "https://slides.googleapis.com/v1", endpoint, method, params, body)
}

// executeContactsAPI executes Google Contacts API calls via REST.
func executeContactsAPI(ctx context.Context, token, endpoint, method string, params map[string]string, body map[string]any) (any, error) {
	return executeGoogleAPI(ctx, token, "https://people.googleapis.com/v1", endpoint, method, params, body)
}

// executeTasksAPI executes Google Tasks API calls via REST.
func executeTasksAPI(ctx context.Context, token, endpoint, method string, params map[string]string, body map[string]any) (any, error) {
	return executeGoogleAPI(ctx, token, "https://tasks.googleapis.com/tasks/v1", endpoint, method, params, body)
}

// executePeopleAPI executes Google People API calls via REST.
func executePeopleAPI(ctx context.Context, token, endpoint, method string, params map[string]string, body map[string]any) (any, error) {
	return executeGoogleAPI(ctx, token, "https://people.googleapis.com/v1", endpoint, method, params, body)
}

// executeGoogleAPI performs a generic Google API call.
func executeGoogleAPI(ctx context.Context, token, baseURL, endpoint, method string, params map[string]string, body map[string]any) (any, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Build URL with query params
	u, err := url.Parse(baseURL + endpoint)
	if err != nil {
		return map[string]any{"success": false, "error": "invalid endpoint: " + err.Error()}, nil
	}

	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	// Build request body
	var bodyReader io.Reader
	if body != nil && (method == "POST" || method == "PUT" || method == "PATCH") {
		bodyJSON, err := json.Marshal(body)
		if err != nil {
			return map[string]any{"success": false, "error": "invalid body: " + err.Error()}, nil
		}
		bodyReader = strings.NewReader(string(bodyJSON))
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to create request: " + err.Error()}, nil
	}

	// Add headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"success": false, "error": "request failed: " + err.Error()}, nil
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to read response: " + err.Error()}, nil
	}

	// Parse response
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Return raw response if not JSON
		return map[string]any{
			"success":  resp.StatusCode >= 200 && resp.StatusCode < 300,
			"status":   resp.StatusCode,
			"body":     string(respBody),
			"endpoint": endpoint,
		}, nil
	}

	// Add success flag based on status code
	result["success"] = resp.StatusCode >= 200 && resp.StatusCode < 300
	result["status"] = resp.StatusCode
	result["endpoint"] = endpoint

	return result, nil
}

// getString safely extracts a string from a map.
func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
