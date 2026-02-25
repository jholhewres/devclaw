package skills

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClawHubSearch(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		wantCount  int
		wantSlug   string
		wantName   string
		wantScore  float64
	}{
		{
			name: "single result",
			response: `{
				"results": [
					{
						"slug": "test/skill",
						"displayName": "Test Skill",
						"summary": "A test skill for unit testing",
						"version": "1.0.0",
						"score": 0.95
					}
				]
			}`,
			wantCount: 1,
			wantSlug:  "test/skill",
			wantName:  "Test Skill",
			wantScore: 0.95,
		},
		{
			name: "multiple results",
			response: `{
				"results": [
					{
						"slug": "user/skill1",
						"displayName": "Skill One",
						"summary": "First skill",
						"score": 0.9
					},
					{
						"slug": "user/skill2",
						"displayName": "Skill Two",
						"summary": "Second skill",
						"score": 0.8
					}
				]
			}`,
			wantCount: 2,
			wantSlug:  "user/skill1",
			wantName:  "Skill One",
			wantScore: 0.9,
		},
		{
			name:      "empty results",
			response:  `{"results": []}`,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/search", r.URL.Path)
				assert.NotEmpty(t, r.URL.Query().Get("q"))
				assert.Equal(t, "10", r.URL.Query().Get("limit"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client := NewClawHubClient(server.URL)
			result, err := client.Search("test", 10)
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Len(t, result.Results, tt.wantCount)

			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantSlug, result.Results[0].Slug)
				assert.Equal(t, tt.wantName, result.Results[0].DisplayName)
				assert.Equal(t, tt.wantScore, result.Results[0].Score)
			}
		})
	}
}

func TestClawHubGetSkillMeta(t *testing.T) {
	tests := []struct {
		name         string
		slug         string
		response     string
		wantName     string
		wantSummary  string
		wantMalware  bool
	}{
		{
			name: "basic skill",
			slug: "user/my-skill",
			response: `{
				"slug": "user/my-skill",
				"displayName": "My Skill",
				"summary": "A useful skill",
				"latestVersion": {
					"version": "1.2.0"
				},
				"moderation": {
					"isMalwareBlocked": false,
					"isSuspicious": false
				}
			}`,
			wantName:    "My Skill",
			wantSummary: "A useful skill",
			wantMalware: false,
		},
		{
			name: "malware blocked skill",
			slug: "bad/malware",
			response: `{
				"slug": "bad/malware",
				"displayName": "Malware Skill",
				"summary": "This is malware",
				"moderation": {
					"isMalwareBlocked": true,
					"isSuspicious": true
				}
			}`,
			wantName:    "Malware Skill",
			wantSummary: "This is malware",
			wantMalware: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Contains(t, r.URL.Path, "/skills/")
				assert.Contains(t, r.URL.Path, tt.slug)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client := NewClawHubClient(server.URL)
			meta, err := client.GetSkillMeta(tt.slug)
			require.NoError(t, err)
			require.NotNil(t, meta)

			assert.Equal(t, tt.wantName, meta.DisplayName)
			assert.Equal(t, tt.wantSummary, meta.Summary)

			if meta.Moderation != nil {
				assert.Equal(t, tt.wantMalware, meta.Moderation.IsMalwareBlocked)
			}
		})
	}
}

func TestClawHubFetchFile(t *testing.T) {
	skillContent := `---
name: test-skill
description: "A test skill"
---
# Test Skill
This is a test skill.
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/skills/")
		assert.Contains(t, r.URL.Path, "/file")
		assert.Equal(t, "SKILL.md", r.URL.Query().Get("path"))

		w.Header().Set("Content-Type", "text/markdown")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(skillContent))
	}))
	defer server.Close()

	client := NewClawHubClient(server.URL)
	content, err := client.FetchFile("test/skill", "SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, skillContent, string(content))
}

func TestClawHubDownload(t *testing.T) {
	// Create a minimal valid ZIP file
	zipContent := []byte{
		0x50, 0x4B, 0x03, 0x04, // ZIP signature
		0x14, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x0A, 0x00, 0x00, 0x00,
		'S', 'K', 'I', 'L', 'L', '.', 'm', 'd',
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/download", r.URL.Path)
		assert.NotEmpty(t, r.URL.Query().Get("slug"))

		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		w.Write(zipContent)
	}))
	defer server.Close()

	client := NewClawHubClient(server.URL)
	content, err := client.Download("test/skill", "")
	require.NoError(t, err)
	assert.NotEmpty(t, content)
	// Verify ZIP signature
	assert.Equal(t, []byte{0x50, 0x4B}, content[:2])
}

func TestClawHubError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "not found",
			statusCode: http.StatusNotFound,
			body:       `{"error": "skill not found"}`,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			body:       `{"error": "internal server error"}`,
		},
		{
			name:       "rate limited",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error": "rate limit exceeded"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := NewClawHubClient(server.URL)
			_, err := client.Search("test", 10)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ClawHub API")
		})
	}
}

func TestClawHubResolve(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond to /skills/ endpoint (new API)
		if r.URL.Path == "/skills/user/skill" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"slug": "user/skill",
				"displayName": "Resolved Skill",
				"summary": "A resolved skill",
				"author": "user",
				"downloads": 100,
				"stars": 50
			}`))
			return
		}

		// Fallback to old /resolve endpoint
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"slug": "user/skill",
			"name": "Legacy Skill",
			"description": "Legacy endpoint"
		}`))
	}))
	defer server.Close()

	client := NewClawHubClient(server.URL)
	skill, err := client.Resolve("user/skill")
	require.NoError(t, err)
	assert.Equal(t, "Resolved Skill", skill.Name)
	assert.Equal(t, "A resolved skill", skill.Description)
}

func TestClawHubResolveWithVersion(t *testing.T) {
	t.Run("with LatestVersion", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"slug": "user/skill",
				"displayName": "Versioned Skill",
				"summary": "A skill with version",
				"latestVersion": {
					"version": "2.0.0"
				}
			}`))
		}))
		defer server.Close()

		client := NewClawHubClient(server.URL)
		skill, err := client.Resolve("user/skill")
		require.NoError(t, err)
		assert.Equal(t, "2.0.0", skill.Version)
	})

	t.Run("without LatestVersion (nil)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"slug": "user/skill",
				"displayName": "Unversioned Skill",
				"summary": "A skill without version"
			}`))
		}))
		defer server.Close()

		client := NewClawHubClient(server.URL)
		skill, err := client.Resolve("user/skill")
		require.NoError(t, err)
		assert.Equal(t, "", skill.Version, "version should be empty when LatestVersion is nil")
	})
}

func TestNewClawHubClientDefaultURL(t *testing.T) {
	client := NewClawHubClient("")
	assert.Equal(t, DefaultClawHubURL, client.baseURL)
}
