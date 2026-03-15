// Package copilot – lcm_retrieval.go provides the retrieval operations for the
// LCM tool: grep (FTS + regex), describe (inspect DAG), expand (recover messages).
package copilot

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// maxGrepMessages caps the number of messages loaded for grep searches
// to prevent excessive memory usage on long-lived conversations.
const maxGrepMessages = 10000

// LCMRetrieval provides search and inspection over the LCM DAG.
type LCMRetrieval struct {
	store  *LCMStore
	logger *slog.Logger
}

// NewLCMRetrieval creates a new retrieval engine.
func NewLCMRetrieval(store *LCMStore, logger *slog.Logger) *LCMRetrieval {
	return &LCMRetrieval{store: store, logger: logger}
}

// Grep searches messages and summaries by text (FTS) or regex.
func (r *LCMRetrieval) Grep(convID, query string, isRegex bool, limit int) (string, error) {
	if limit <= 0 {
		limit = 20
	}

	if isRegex {
		return r.grepRegex(convID, query, limit)
	}
	return r.grepFTS(convID, query, limit)
}

// grepFTS uses SQLite FTS5 for full-text search. Falls back to plain substring
// search if FTS5 is unavailable.
func (r *LCMRetrieval) grepFTS(convID, query string, limit int) (string, error) {
	results, err := r.store.SearchFTS(convID, query, limit)
	if err != nil {
		return "", fmt.Errorf("lcm grep fts: %w", err)
	}

	// Fallback: if FTS5 returned nothing (possibly unavailable), try substring search.
	if len(results) == 0 {
		return r.grepSubstring(convID, query, limit)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[%d results for %q]\n\n", len(results), query)
	for _, res := range results {
		snippet := res.Content
		if len(snippet) > 300 {
			snippet = snippet[:300] + "..."
		}
		switch res.EntityType {
		case "message":
			fmt.Fprintf(&b, "--- Message #%s ---\n%s\n\n", res.EntityID, snippet)
		case "summary":
			fmt.Fprintf(&b, "--- Summary %s ---\n%s\n\n", res.EntityID, snippet)
		}
	}
	return b.String(), nil
}

// grepSubstring performs plain substring search (fallback when FTS5 unavailable).
func (r *LCMRetrieval) grepSubstring(convID, query string, limit int) (string, error) {
	lowerQuery := strings.ToLower(query)
	var b strings.Builder
	matchCount := 0

	msgs, _ := r.store.GetMessageRange(convID, 0, maxGrepMessages)
	for _, m := range msgs {
		if matchCount >= limit {
			break
		}
		if strings.Contains(strings.ToLower(m.Content), lowerQuery) {
			matchCount++
			snippet := m.Content
			if len(snippet) > 300 {
				snippet = snippet[:300] + "..."
			}
			fmt.Fprintf(&b, "--- Message #%d (%s, seq %d) ---\n%s\n\n",
				m.ID, m.Role, m.Seq, snippet)
		}
	}

	sums, _ := r.store.GetAllSummaries(convID)
	for _, s := range sums {
		if matchCount >= limit {
			break
		}
		if strings.Contains(strings.ToLower(s.Content), lowerQuery) {
			matchCount++
			snippet := s.Content
			if len(snippet) > 300 {
				snippet = snippet[:300] + "..."
			}
			fmt.Fprintf(&b, "--- Summary %s (%s, depth %d) ---\n%s\n\n",
				s.ID, s.Kind, s.Depth, snippet)
		}
	}

	if matchCount == 0 {
		return fmt.Sprintf("No results for %q.", query), nil
	}
	header := fmt.Sprintf("[%d results for %q]\n\n", matchCount, query)
	return header + b.String(), nil
}

// grepRegex performs regex search over all messages and summaries.
func (r *LCMRetrieval) grepRegex(convID, pattern string, limit int) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex: %w", err)
	}

	var b strings.Builder
	matchCount := 0

	// Search messages.
	msgs, err := r.store.GetMessageRange(convID, 0, maxGrepMessages)
	if err == nil {
		for _, m := range msgs {
			if matchCount >= limit {
				break
			}
			if re.MatchString(m.Content) {
				matchCount++
				snippet := m.Content
				if len(snippet) > 300 {
					snippet = snippet[:300] + "..."
				}
				fmt.Fprintf(&b, "--- Message #%d (%s, seq %d) ---\n%s\n\n",
					m.ID, m.Role, m.Seq, snippet)
			}
		}
	}

	// Search summaries.
	if matchCount < limit {
		sums, err := r.store.GetAllSummaries(convID)
		if err == nil {
			for _, s := range sums {
				if matchCount >= limit {
					break
				}
				if re.MatchString(s.Content) {
					matchCount++
					snippet := s.Content
					if len(snippet) > 300 {
						snippet = snippet[:300] + "..."
					}
					fmt.Fprintf(&b, "--- Summary %s (%s, depth %d) ---\n%s\n\n",
						s.ID, s.Kind, s.Depth, snippet)
				}
			}
		}
	}

	if matchCount == 0 {
		return fmt.Sprintf("No regex matches for %q.", pattern), nil
	}

	header := fmt.Sprintf("[%d regex matches for %q]\n\n", matchCount, pattern)
	return header + b.String(), nil
}

// Describe returns metadata about a summary or the full DAG tree.
func (r *LCMRetrieval) Describe(convID, summaryID string) (string, error) {
	if summaryID == "tree" {
		return r.DescribeTree(convID)
	}

	sum, err := r.store.GetSummary(summaryID)
	if err != nil {
		return "", fmt.Errorf("lcm describe: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Summary: %s\n", sum.ID)
	fmt.Fprintf(&b, "Kind: %s | Depth: %d | Tokens: %d\n", sum.Kind, sum.Depth, sum.TokenCount)
	fmt.Fprintf(&b, "Source message tokens: %d\n", sum.SourceMessageTokenCount)
	fmt.Fprintf(&b, "Time range: %s → %s\n",
		sum.EarliestAt.Format("2006-01-02 15:04"),
		sum.LatestAt.Format("2006-01-02 15:04"))

	// Parents.
	parents, _ := r.store.GetSummaryParents(summaryID)
	if len(parents) > 0 {
		var ids []string
		for _, p := range parents {
			ids = append(ids, p.ID)
		}
		fmt.Fprintf(&b, "Parent(s): %s\n", strings.Join(ids, ", "))
	} else {
		b.WriteString("Parent: none (root)\n")
	}

	// Children.
	if sum.Kind == "condensed" {
		children, _ := r.store.GetSummaryChildren(summaryID)
		if len(children) > 0 {
			var ids []string
			for _, c := range children {
				ids = append(ids, fmt.Sprintf("%s (%s d%d)", c.ID, c.Kind, c.Depth))
			}
			fmt.Fprintf(&b, "Children: %s\n", strings.Join(ids, ", "))
		}
		fmt.Fprintf(&b, "Descendant count: %d | Descendant tokens: %d\n",
			sum.DescendantCount, sum.DescendantTokenCount)
	} else {
		msgs, _ := r.store.GetSummaryMessages(summaryID)
		fmt.Fprintf(&b, "Source messages: %d\n", len(msgs))
	}

	fmt.Fprintf(&b, "\nContent:\n%s\n", sum.Content)
	return b.String(), nil
}

// DescribeTree returns an overview of the full LCM DAG.
func (r *LCMRetrieval) DescribeTree(convID string) (string, error) {
	totalMsgs, err := r.store.MessageCount(convID)
	if err != nil {
		return "", fmt.Errorf("lcm describe tree: %w", err)
	}

	leafCount, condensedCount, err := r.store.SummaryCount(convID)
	if err != nil {
		return "", fmt.Errorf("lcm describe tree: %w", err)
	}
	totalSummaries := leafCount + condensedCount

	maxDepth, _ := r.store.GetMaxDepth(convID)

	roots, _ := r.store.GetRootSummaries(convID)

	// Count all unsummarized tokens (includes fresh tail + pending compaction).
	unsummarizedTokens, _ := r.store.CountUnsummarizedTokens(convID, 0)

	var b strings.Builder
	fmt.Fprintf(&b, "LCM DAG Overview:\n")
	fmt.Fprintf(&b, "Total messages: %d (in LCM store)\n", totalMsgs)
	fmt.Fprintf(&b, "Total summaries: %d (%d leaf, %d condensed)\n",
		totalSummaries, leafCount, condensedCount)
	fmt.Fprintf(&b, "Max depth: %d\n", maxDepth)
	fmt.Fprintf(&b, "Unsummarized tokens: ~%d\n\n", unsummarizedTokens)

	if len(roots) > 0 {
		b.WriteString("Root summaries (what the model sees):\n")
		for _, root := range roots {
			msgInfo := ""
			if root.Kind == "leaf" {
				msgs, _ := r.store.GetSummaryMessages(root.ID)
				msgInfo = fmt.Sprintf("covers %d msgs", len(msgs))
			} else {
				msgInfo = fmt.Sprintf("covers %d descendants", root.DescendantCount)
			}
			fmt.Fprintf(&b, "  %s (%s d%d, %d tokens, %s, %s→%s)\n",
				root.ID, root.Kind, root.Depth, root.TokenCount, msgInfo,
				root.EarliestAt.Format("2006-01-02 15:04"),
				root.LatestAt.Format("2006-01-02 15:04"))
		}
	} else {
		b.WriteString("No summaries yet — all messages are in the fresh tail.\n")
	}

	return b.String(), nil
}

// Expand recovers the original messages behind a summary.
// depth=0 expands to messages (for leaf) or child summaries (for condensed).
// depth>0 recursively expands children.
func (r *LCMRetrieval) Expand(convID, summaryID string, depth int) (string, error) {
	sum, err := r.store.GetSummary(summaryID)
	if err != nil {
		return "", fmt.Errorf("lcm expand: %w", err)
	}

	result := r.expandRecursive(sum, depth, 0)

	// Truncate if too large.
	const maxChars = 50000
	if len(result) > maxChars {
		result = result[:maxChars] + "\n...(truncated at 50,000 chars)"
	}
	return result, nil
}

func (r *LCMRetrieval) expandRecursive(sum *LCMSummary, maxDepth, currentDepth int) string {
	var b strings.Builder

	indent := strings.Repeat("  ", currentDepth)

	if sum.Kind == "leaf" {
		msgs, err := r.store.GetSummaryMessages(sum.ID)
		if err != nil {
			return fmt.Sprintf("%s[Error loading messages for %s: %v]\n", indent, sum.ID, err)
		}
		fmt.Fprintf(&b, "%s=== Leaf %s (%d messages, %s→%s) ===\n",
			indent, sum.ID, len(msgs),
			sum.EarliestAt.Format("15:04"),
			sum.LatestAt.Format("15:04"))
		for _, m := range msgs {
			fmt.Fprintf(&b, "%s[#%d %s %s] %s\n",
				indent, m.Seq, m.Role, m.CreatedAt.Format("15:04"), m.Content)
		}
		return b.String()
	}

	// Condensed summary.
	children, err := r.store.GetSummaryChildren(sum.ID)
	if err != nil {
		return fmt.Sprintf("%s[Error loading children for %s: %v]\n", indent, sum.ID, err)
	}

	fmt.Fprintf(&b, "%s=== Condensed %s (depth %d, %d children, %s→%s) ===\n",
		indent, sum.ID, sum.Depth, len(children),
		sum.EarliestAt.Format("15:04"),
		sum.LatestAt.Format("15:04"))

	for _, child := range children {
		if currentDepth < maxDepth {
			b.WriteString(r.expandRecursive(child, maxDepth, currentDepth+1))
		} else {
			// Show child summary content without further expansion.
			preview := child.Content
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			fmt.Fprintf(&b, "%s  [%s %s d%d %d tokens] %s\n",
				indent, child.ID, child.Kind, child.Depth, child.TokenCount, preview)
		}
	}

	return b.String()
}
