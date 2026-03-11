// Package copilot – link_understanding.go extracts URLs from messages
// and enriches them with readable content before passing to the agent.
// This aligns with OpenClaw's link understanding pipeline.
package copilot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/security"
)

// LinkConfig configures the link understanding pipeline.
type LinkConfig struct {
	Enabled        bool `yaml:"enabled" json:"enabled"`
	MaxLinks       int  `yaml:"max_links" json:"max_links"`
	TimeoutSeconds int  `yaml:"timeout_seconds" json:"timeout_seconds"`
	MaxCharsPerURL int  `yaml:"max_chars_per_url" json:"max_chars_per_url"`
}

// DefaultLinkConfig returns sensible defaults.
func DefaultLinkConfig() LinkConfig {
	return LinkConfig{
		Enabled:        false,
		MaxLinks:       3,
		TimeoutSeconds: 30,
		MaxCharsPerURL: 8000,
	}
}

// LinkResult holds the extracted content from a URL.
type LinkResult struct {
	URL     string
	Title   string
	Content string
	Error   error
}

// urlPattern matches HTTP(S) URLs, stripping surrounding markdown/punctuation.
var urlPattern = regexp.MustCompile(`https?://[^\s<>\[\]()'"` + "`" + `]+`)

// ExtractLinksFromMessage extracts HTTP(S) URLs from a message body.
// Strips markdown link formatting and limits to maxLinks.
func ExtractLinksFromMessage(body string, maxLinks int) []string {
	if maxLinks <= 0 {
		maxLinks = 3
	}

	matches := urlPattern.FindAllString(body, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var urls []string
	for _, raw := range matches {
		u := cleanURLSuffix(raw)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		urls = append(urls, u)
		if len(urls) >= maxLinks {
			break
		}
	}
	return urls
}

// cleanURLSuffix removes trailing punctuation that often gets captured from prose.
func cleanURLSuffix(u string) string {
	for strings.HasSuffix(u, ".") || strings.HasSuffix(u, ",") ||
		strings.HasSuffix(u, ";") || strings.HasSuffix(u, ")") ||
		strings.HasSuffix(u, "]") || strings.HasSuffix(u, ">") {
		u = u[:len(u)-1]
	}
	return u
}

// RunLinkUnderstanding fetches and extracts readable content from the given URLs.
// Uses SSRFGuard when available to validate URLs. Reuses web_fetch_readability.go
// for HTML-to-text conversion.
func RunLinkUnderstanding(ctx context.Context, urls []string, cfg LinkConfig, ssrfGuard *security.SSRFGuard, logger *slog.Logger) []LinkResult {
	if len(urls) == 0 {
		return nil
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxChars := cfg.MaxCharsPerURL
	if maxChars <= 0 {
		maxChars = 8000
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	results := make([]LinkResult, len(urls))
	for i, u := range urls {
		results[i] = fetchAndExtract(ctx, client, u, maxChars, ssrfGuard, logger)
	}
	return results
}

func fetchAndExtract(ctx context.Context, client *http.Client, url string, maxChars int, ssrfGuard *security.SSRFGuard, logger *slog.Logger) LinkResult {
	if ssrfGuard != nil {
		if err := ssrfGuard.IsAllowed(url); err != nil {
			return LinkResult{URL: url, Error: fmt.Errorf("blocked by SSRF guard: %w", err)}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LinkResult{URL: url, Error: err}
	}
	req.Header.Set("User-Agent", "DevClaw/1.0 (link-understanding)")
	req.Header.Set("Accept", "text/html, application/xhtml+xml, text/plain;q=0.9, */*;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return LinkResult{URL: url, Error: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return LinkResult{URL: url, Error: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}

	ct := resp.Header.Get("Content-Type")
	isHTML := strings.Contains(ct, "text/html") || strings.Contains(ct, "xhtml")

	var title, content string
	if isHTML {
		title, content = ExtractReadableText(io.LimitReader(resp.Body, 2*1024*1024))
	} else {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(maxChars+1024)))
		if readErr != nil {
			return LinkResult{URL: url, Error: fmt.Errorf("read body: %w", readErr)}
		}
		content = string(bodyBytes)
	}

	if len(content) > maxChars {
		content = content[:maxChars] + "\n... [truncated]"
	}

	if logger != nil {
		logger.Debug("link understanding: fetched",
			"url", url,
			"title", truncate(title, 60),
			"content_len", len(content),
		)
	}

	return LinkResult{URL: url, Title: title, Content: content}
}

// FormatLinkResults formats link results as context to prepend to the user message.
func FormatLinkResults(results []LinkResult) string {
	var b strings.Builder
	successful := 0
	for _, r := range results {
		if r.Error != nil || r.Content == "" {
			continue
		}
		successful++
		b.WriteString(fmt.Sprintf("[Link: %s", r.URL))
		if r.Title != "" {
			b.WriteString(fmt.Sprintf(" — %s", r.Title))
		}
		b.WriteString("]\n")
		b.WriteString(r.Content)
		b.WriteString("\n\n")
	}
	if successful == 0 {
		return ""
	}
	return b.String()
}
