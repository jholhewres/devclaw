// Package copilot – web_fetch_readability.go provides HTML-to-text conversion
// and an in-memory LRU cache for web_fetch results.
package copilot

import (
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// ---------- HTML to Markdown/Text Extraction ----------

// skipTags are HTML elements whose content should be omitted entirely.
var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "svg": true,
	"nav": true, "footer": true, "header": true, "aside": true,
	"iframe": true, "object": true, "embed": true,
}

// blockTags are elements that should produce line breaks.
var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true,
	"main": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "li": true, "tr": true, "br": true,
	"blockquote": true, "pre": true, "table": true,
}

// headingTags map heading elements to their markdown prefix.
var headingTags = map[string]string{
	"h1": "# ", "h2": "## ", "h3": "### ",
	"h4": "#### ", "h5": "##### ", "h6": "###### ",
}

// ExtractReadableText converts raw HTML into clean readable text with
// minimal markdown formatting (headings, links, lists). It prioritizes
// <main>, <article>, and <body> content while skipping navigation,
// footers, scripts, and styles.
func ExtractReadableText(r io.Reader) (title string, text string) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", ""
	}

	// Extract title.
	title = extractTitle(doc)

	// Try to find main content areas first.
	var contentNode *html.Node
	for _, tag := range []string{"main", "article"} {
		if n := findFirstElement(doc, tag); n != nil {
			contentNode = n
			break
		}
	}
	if contentNode == nil {
		contentNode = findFirstElement(doc, "body")
	}
	if contentNode == nil {
		contentNode = doc
	}

	var b strings.Builder
	extractText(contentNode, &b)
	text = cleanupText(b.String())

	return title, text
}

// extractTitle finds the <title> element text.
func extractTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		return collectText(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if t := extractTitle(c); t != "" {
			return t
		}
	}
	return ""
}

// findFirstElement finds the first element with the given tag name.
func findFirstElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirstElement(c, tag); found != nil {
			return found
		}
	}
	return nil
}

// collectText collects all text content from a node and its children.
func collectText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(collectText(c))
	}
	return b.String()
}

// extractText recursively extracts readable text from an HTML node.
func extractText(n *html.Node, b *strings.Builder) {
	if n.Type == html.ElementNode {
		if skipTags[n.Data] {
			return
		}

		// Add heading prefix.
		if prefix, ok := headingTags[n.Data]; ok {
			b.WriteString("\n\n")
			b.WriteString(prefix)
		} else if blockTags[n.Data] {
			b.WriteString("\n")
		}

		// List items.
		if n.Data == "li" {
			b.WriteString("- ")
		}

		// Links: extract href for non-trivial links.
		if n.Data == "a" {
			href := getAttr(n, "href")
			linkText := strings.TrimSpace(collectText(n))
			if href != "" && linkText != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") {
				b.WriteString("[")
				b.WriteString(linkText)
				b.WriteString("](")
				b.WriteString(href)
				b.WriteString(")")
				return // Don't recurse into link children.
			}
		}

		// <br> produces a newline.
		if n.Data == "br" {
			b.WriteString("\n")
		}
	}

	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			b.WriteString(text)
			b.WriteString(" ")
		}
		return
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractText(c, b)
	}

	if n.Type == html.ElementNode && blockTags[n.Data] {
		b.WriteString("\n")
	}
}

// getAttr returns the value of an attribute on an HTML node.
func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// cleanupText normalizes whitespace in extracted text.
func cleanupText(s string) string {
	// Collapse multiple newlines to at most 2.
	lines := strings.Split(s, "\n")
	var result []string
	emptyCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			emptyCount++
			if emptyCount <= 2 {
				result = append(result, "")
			}
		} else {
			emptyCount = 0
			result = append(result, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

// ---------- Web Fetch Cache ----------

// webFetchCacheEntry is a single cached response.
type webFetchCacheEntry struct {
	content   string
	fetchedAt time.Time
}

// WebFetchCache is a simple in-memory cache with TTL for web fetch results.
type WebFetchCache struct {
	mu      sync.Mutex
	entries map[string]*webFetchCacheEntry
	ttl     time.Duration
	maxSize int
}

// NewWebFetchCache creates a cache with the given TTL and max entries.
func NewWebFetchCache(ttl time.Duration, maxSize int) *WebFetchCache {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if maxSize <= 0 {
		maxSize = 100
	}
	return &WebFetchCache{
		entries: make(map[string]*webFetchCacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Get returns a cached entry if it exists and is not expired.
func (c *WebFetchCache) Get(url string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[url]
	if !ok {
		return "", false
	}
	if time.Since(entry.fetchedAt) > c.ttl {
		delete(c.entries, url)
		return "", false
	}
	return entry.content, true
}

// Set stores a result in the cache, evicting the oldest entry if full.
func (c *WebFetchCache) Set(url, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest if at capacity.
	if len(c.entries) >= c.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.entries {
			if oldestKey == "" || v.fetchedAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.fetchedAt
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}

	c.entries[url] = &webFetchCacheEntry{
		content:   content,
		fetchedAt: time.Now(),
	}
}
