// Package copilot – directives.go parses inline directives embedded in message bodies.
// Unlike standalone commands (e.g., "/think high"), inline directives can appear
// alongside regular text: "explain this /model gpt-4o /think high".
// This aligns with OpenClaw's inline directive system.
package copilot

import (
	"strings"
)

// InlineDirectives holds directives extracted from a message body.
type InlineDirectives struct {
	Think    string // "off", "low", "medium", "high"
	Model    string // model override for this message
	Verbose  *bool  // nil = not set, true/false = override
	Queue    string // queue mode override
	Language string // response language hint
}

// HasAny returns true if any directive was set.
func (d InlineDirectives) HasAny() bool {
	return d.Think != "" || d.Model != "" || d.Verbose != nil || d.Queue != "" || d.Language != ""
}

// directiveSpec defines a single directive pattern.
type directiveSpec struct {
	prefix  string
	handler func(value string, d *InlineDirectives)
}

var directiveSpecs = []directiveSpec{
	{
		prefix: "/think",
		handler: func(value string, d *InlineDirectives) {
			v := strings.ToLower(value)
			switch v {
			case "off", "low", "medium", "high":
				d.Think = v
			}
		},
	},
	{
		prefix: "/reasoning",
		handler: func(value string, d *InlineDirectives) {
			v := strings.ToLower(value)
			switch v {
			case "off", "low", "medium", "high":
				d.Think = v
			}
		},
	},
	{
		prefix: "/model",
		handler: func(value string, d *InlineDirectives) {
			if value != "" {
				d.Model = value
			}
		},
	},
	{
		prefix: "/verbose",
		handler: func(value string, d *InlineDirectives) {
			switch strings.ToLower(value) {
			case "on", "true", "1":
				v := true
				d.Verbose = &v
			case "off", "false", "0":
				v := false
				d.Verbose = &v
			}
		},
	},
	{
		prefix: "/queue",
		handler: func(value string, d *InlineDirectives) {
			v := strings.ToLower(value)
			switch v {
			case "collect", "steer", "followup", "interrupt", "steer-backlog":
				d.Queue = v
			}
		},
	},
	{
		prefix: "/lang",
		handler: func(value string, d *InlineDirectives) {
			if value != "" {
				d.Language = strings.ToLower(value)
			}
		},
	},
}

// ParseInlineDirectives extracts inline directives from a message body.
// Returns the directives found and the cleaned body with directives removed.
// Directives can appear at the beginning or end of the message.
//
// Examples:
//
//	"/think high explain quantum physics" → Think="high", body="explain quantum physics"
//	"explain this /model gpt-4o" → Model="gpt-4o", body="explain this"
//	"/think high /verbose on" → Think="high", Verbose=true, body=""
func ParseInlineDirectives(body string) (InlineDirectives, string) {
	var d InlineDirectives
	cleaned := body

	// Multiple passes: extract directives from both ends until no more found.
	for pass := 0; pass < 3; pass++ {
		found := false
		for _, spec := range directiveSpecs {
			if newCleaned, ok := extractDirective(cleaned, spec.prefix, spec.handler, &d); ok {
				cleaned = newCleaned
				found = true
			}
		}
		if !found {
			break
		}
	}

	return d, strings.TrimSpace(cleaned)
}

// extractDirective looks for a directive at the start or end of text.
// Returns the text with the directive removed and true if found.
func extractDirective(text string, prefix string, handler func(string, *InlineDirectives), d *InlineDirectives) (string, bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)

	// Check at start of text.
	if strings.HasPrefix(lower, prefix+" ") || strings.HasPrefix(lower, prefix+"\t") {
		rest := trimmed[len(prefix):]
		rest = strings.TrimLeft(rest, " \t")
		value, remainder := splitFirstWord(rest)
		handler(value, d)
		return strings.TrimSpace(remainder), true
	}

	// Exact match (directive with no body).
	if lower == prefix {
		handler("", d)
		return "", true
	}

	// Check at end of text: find last occurrence of " /directive value"
	idx := strings.LastIndex(lower, " "+prefix+" ")
	if idx >= 0 {
		before := trimmed[:idx]
		after := trimmed[idx+1+len(prefix):]
		after = strings.TrimLeft(after, " \t")
		value, trailing := splitFirstWord(after)
		handler(value, d)
		result := before
		if trailing != "" {
			result += " " + trailing
		}
		return strings.TrimSpace(result), true
	}

	// End of text with no value after: "some text /directive"
	if strings.HasSuffix(lower, " "+prefix) {
		before := trimmed[:len(trimmed)-len(prefix)-1]
		handler("", d)
		return strings.TrimSpace(before), true
	}

	return text, false
}

// splitFirstWord splits text into the first whitespace-delimited word and the rest.
func splitFirstWord(s string) (word, rest string) {
	s = strings.TrimLeft(s, " \t")
	idx := strings.IndexAny(s, " \t")
	if idx < 0 {
		return s, ""
	}
	return s[:idx], strings.TrimLeft(s[idx:], " \t")
}
