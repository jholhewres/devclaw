// Package copilot – message_split.go provides splitting of long messages
// for channels with character limits (e.g. WhatsApp 4096).
package copilot

import (
	"fmt"
	"strings"
)

const (
	// MaxMessageWhatsApp is WhatsApp's character limit.
	MaxMessageWhatsApp = 4096

	// MaxMessageDefault is the default max length for general channels.
	MaxMessageDefault = 4000
)

// extractCodeBlocks replaces ```...``` blocks with placeholders. Returns the
// modified text and a list of (placeholder, original) pairs.
func extractCodeBlocks(text string) (string, []struct{ placeholder, content string }) {
	var blocks []struct{ placeholder, content string }
	var result strings.Builder
	i := 0
	for i < len(text) {
		if strings.HasPrefix(text[i:], "```") {
			// Find start of code block
			start := i
			i += 3
			// Optional language tag
			for i < len(text) && (text[i] >= 'a' && text[i] <= 'z' || text[i] >= 'A' && text[i] <= 'Z' || text[i] >= '0' && text[i] <= '9') {
				i++
			}
			// Optional newline
			if i < len(text) && text[i] == '\n' {
				i++
			}
			// Find closing ```
			closeIdx := strings.Index(text[i:], "```")
			var blockContent string
			if closeIdx < 0 {
				// No closing, treat rest as code
				blockContent = text[start:]
				i = len(text)
			} else {
				blockContent = text[start : i+closeIdx+3]
				i += closeIdx + 3
			}

			ph := fmt.Sprintf("<<<DEVCLAW_CODE_%d>>>", len(blocks))
			blocks = append(blocks, struct{ placeholder, content string }{ph, blockContent})
			result.WriteString(ph)
			continue
		}
		result.WriteByte(text[i])
		i++
	}
	return result.String(), blocks
}

// SplitMessage splits a long message into chunks respecting maxLen.
// Tries to break on paragraph boundaries, then sentence boundaries, then word boundaries.
// Does not split inside ``` code blocks.
func SplitMessage(text string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = MaxMessageDefault
	}
	if len(text) <= maxLen {
		if text == "" {
			return nil
		}
		return []string{text}
	}

	// Extract code blocks and replace with placeholders to avoid splitting inside them.
	textWithPlaceholders, blocks := extractCodeBlocks(text)

	var chunks []string
	remain := textWithPlaceholders

	for len(remain) > maxLen {
		segment := remain[:maxLen]

		// Find the best split point (priority: paragraph > line > sentence > word > hard)
		splitAt := -1
		searchIn := segment

		// 1. Paragraph boundary (\n\n)
		if idx := strings.LastIndex(searchIn, "\n\n"); idx >= 0 && idx > maxLen/2 {
			splitAt = idx + 2 // include \n\n in first chunk
		}

		// 2. Line boundary (\n)
		if splitAt < 0 {
			if idx := strings.LastIndex(searchIn, "\n"); idx >= 0 && idx > maxLen/2 {
				splitAt = idx + 1
			}
		}

		// 3. Sentence boundary (. )
		if splitAt < 0 {
			if idx := strings.LastIndex(searchIn, ". "); idx >= 0 && idx > maxLen/2 {
				splitAt = idx + 2
			}
		}

		// 4. Word boundary (space)
		if splitAt < 0 {
			if idx := strings.LastIndex(searchIn, " "); idx >= 0 && idx > maxLen/2 {
				splitAt = idx + 1
			}
		}

		// 5. Avoid splitting in the middle of a placeholder
		if splitAt >= 0 {
			for _, b := range blocks {
				idx := strings.Index(segment, b.placeholder)
				if idx >= 0 {
					end := idx + len(b.placeholder)
					if splitAt > idx && splitAt < end {
						splitAt = idx
						break
					}
				}
			}
		}

		if splitAt > 0 {
			chunk := strings.TrimSpace(remain[:splitAt])
			remain = strings.TrimLeft(remain[splitAt:], " \n")
			if chunk != "" {
				chunks = append(chunks, restoreCodeBlocks(chunk, blocks))
			}
		} else {
			// Hard split - avoid cutting a placeholder
			cut := maxLen
			for _, b := range blocks {
				idx := strings.Index(remain, b.placeholder)
				if idx >= 0 {
					end := idx + len(b.placeholder)
					if idx < maxLen && end > maxLen {
						// Placeholder spans the cut - include it entirely in this chunk
						cut = end
						break
					}
				}
			}
			chunk := strings.TrimSpace(remain[:cut])
			remain = remain[cut:]
			if chunk != "" {
				chunks = append(chunks, restoreCodeBlocks(chunk, blocks))
			}
		}
	}

	if remain != "" {
		chunks = append(chunks, restoreCodeBlocks(strings.TrimSpace(remain), blocks))
	}

	return chunks
}

// restoreCodeBlocks replaces placeholders with original code block content.
func restoreCodeBlocks(s string, blocks []struct{ placeholder, content string }) string {
	for _, b := range blocks {
		s = strings.ReplaceAll(s, b.placeholder, b.content)
	}
	return s
}
