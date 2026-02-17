// Package skills – clawhub_client.go implements a client for the ClawHub
// registry API (clawhub.ai). Supports searching, resolving, and downloading
// skills from the community hub.
//
// ClawHub API base: https://clawhub.ai/api/v1
// Endpoints:
//   GET /search?q=<query>&limit=<n>
//   GET /resolve?slug=<slug>
//   GET /skills/<slug>/file?path=SKILL.md
//   GET /download?slug=<slug>&version=<version>
package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	// DefaultClawHubURL is the default ClawHub registry API base.
	DefaultClawHubURL = "https://clawhub.ai/api/v1"
)

// ClawHubClient communicates with the ClawHub skill registry.
type ClawHubClient struct {
	baseURL string
	client  *http.Client
}

// ClawHubSkill represents a skill entry from ClawHub search results.
type ClawHubSkill struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Version     string `json:"version"`
	Downloads   int    `json:"downloads"`
	Stars       int    `json:"stars"`
	Tags        []string `json:"tags"`
	Category    string `json:"category"`
	Homepage    string `json:"homepage"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// ClawHubSearchResult holds a list of skills from a search query.
type ClawHubSearchResult struct {
	Skills []ClawHubSkill `json:"skills"`
	Total  int            `json:"total"`
}

// NewClawHubClient creates a new ClawHub API client.
func NewClawHubClient(baseURL string) *ClawHubClient {
	if baseURL == "" {
		baseURL = DefaultClawHubURL
	}
	return &ClawHubClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Search queries ClawHub for skills matching the given query.
func (c *ClawHubClient) Search(query string, limit int) (*ClawHubSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	u := fmt.Sprintf("%s/search?q=%s&limit=%d",
		c.baseURL,
		url.QueryEscape(query),
		limit,
	)

	resp, err := c.get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result ClawHubSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// ClawHub may return an array directly.
		return nil, fmt.Errorf("parsing search results: %w", err)
	}

	return &result, nil
}

// Resolve fetches full details for a skill by slug (e.g. "steipete/trello").
func (c *ClawHubClient) Resolve(slug string) (*ClawHubSkill, error) {
	u := fmt.Sprintf("%s/resolve?slug=%s", c.baseURL, url.QueryEscape(slug))

	resp, err := c.get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var skill ClawHubSkill
	if err := json.NewDecoder(resp.Body).Decode(&skill); err != nil {
		return nil, fmt.Errorf("parsing skill details: %w", err)
	}

	return &skill, nil
}

// FetchFile downloads a single file from a skill (e.g. SKILL.md).
func (c *ClawHubClient) FetchFile(slug, path string) ([]byte, error) {
	u := fmt.Sprintf("%s/skills/%s/file?path=%s",
		c.baseURL,
		url.PathEscape(slug),
		url.QueryEscape(path),
	)

	resp, err := c.get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024)) // 1MB limit
}

// Download fetches the full skill archive (zip).
func (c *ClawHubClient) Download(slug, version string) ([]byte, error) {
	u := fmt.Sprintf("%s/download?slug=%s", c.baseURL, url.QueryEscape(slug))
	if version != "" {
		u += "&version=" + url.QueryEscape(version)
	}

	resp, err := c.get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024)) // 50MB limit
}

// get performs a GET request and checks for errors.
func (c *ClawHubClient) get(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "DevClaw/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ClawHub request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("ClawHub API %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}
