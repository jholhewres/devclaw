package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractClawHubSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "owner/slug format",
			input:    "https://clawhub.ai/steipete/slack",
			expected: "slack",
		},
		{
			name:     "simple slug",
			input:    "https://clawhub.ai/slack",
			expected: "slack",
		},
		{
			name:     "with trailing slash",
			input:    "https://clawhub.ai/steipete/trello/",
			expected: "trello",
		},
		{
			name:     "with query params",
			input:    "https://clawhub.ai/steipete/slack?foo=bar",
			expected: "slack",
		},
		{
			name:     "clawhub.com domain",
			input:    "https://clawhub.com/owner/skill",
			expected: "skill",
		},
		{
			name:     "not a clawhub URL",
			input:    "https://github.com/user/repo",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractClawHubSlug(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSkillNameFromSlug(t *testing.T) {
	tests := []struct {
		name     string
		slug     string
		expected string
	}{
		{
			name:     "simple slug",
			slug:     "slack",
			expected: "slack",
		},
		{
			name:     "owner/slug format",
			slug:     "steipete/slack",
			expected: "slack",
		},
		{
			name:     "hyphenated slug",
			slug:     "my-awesome-skill",
			expected: "my-awesome-skill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := skillNameFromSlug(tt.slug)
			assert.Equal(t, tt.expected, result)
		})
	}
}
