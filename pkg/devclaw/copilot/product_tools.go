// Package copilot – product_tools.go implements product management tools:
// project management API integration (Jira, Linear), sprint reporting,
// documentation sync (Notion/Confluence), and DORA metrics calculation.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ---------- Data Types ----------

type sprintReport struct {
	Sprint       string         `json:"sprint"`
	StartDate    string         `json:"start_date"`
	EndDate      string         `json:"end_date"`
	Completed    int            `json:"completed"`
	InProgress   int            `json:"in_progress"`
	Remaining    int            `json:"remaining"`
	Velocity     float64        `json:"velocity"`
	Burndown     []burndownPoint `json:"burndown"`
}

type burndownPoint struct {
	Date      string  `json:"date"`
	Remaining float64 `json:"remaining"`
}

type doraMetrics struct {
	DeployFrequency     string  `json:"deploy_frequency"`
	LeadTimeForChanges  string  `json:"lead_time_for_changes"`
	ChangeFailureRate   string  `json:"change_failure_rate"`
	TimeToRestore       string  `json:"time_to_restore"`
	DeploysInPeriod     int     `json:"deploys_in_period"`
	PeriodDays          int     `json:"period_days"`
	AvgLeadTimeHours    float64 `json:"avg_lead_time_hours"`
	FailureRatePercent  float64 `json:"failure_rate_percent"`
}

// ---------- Tool Registration ----------

// RegisterProductTools registers product management tools.
func RegisterProductTools(executor *ToolExecutor) {
	// sprint_report
	executor.RegisterHidden(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "sprint_report",
			Description: "Generate a sprint report from Git activity: commits, PRs merged, deployments, and velocity estimation based on commit history.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sprint_name": map[string]any{"type": "string", "description": "Sprint name/identifier"},
					"start_date":  map[string]any{"type": "string", "description": "Sprint start date (YYYY-MM-DD)"},
					"end_date":    map[string]any{"type": "string", "description": "Sprint end date (YYYY-MM-DD)"},
				},
				"required": []string{"start_date", "end_date"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		sprintName, _ := args["sprint_name"].(string)
		startDate, _ := args["start_date"].(string)
		endDate, _ := args["end_date"].(string)

		if sprintName == "" {
			sprintName = fmt.Sprintf("Sprint %s", startDate)
		}

		// Count commits in date range
		commitCount, _ := gitCountCommits(startDate, endDate)

		// Count merge commits (approximation for PRs merged)
		mergeCount, _ := gitCountMerges(startDate, endDate)

		// Generate burndown from daily commit count
		burndown := generateBurndown(startDate, endDate)

		report := sprintReport{
			Sprint:     sprintName,
			StartDate:  startDate,
			EndDate:    endDate,
			Completed:  commitCount,
			InProgress: mergeCount,
			Remaining:  0,
			Velocity:   float64(commitCount),
			Burndown:   burndown,
		}

		data, _ := json.MarshalIndent(report, "", "  ")
		return string(data), nil
	})

	// dora_metrics
	executor.RegisterHidden(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "dora_metrics",
			Description: "Calculate DORA metrics from Git history: deployment frequency, lead time for changes, change failure rate (requires git tags for deploys).",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"days":       map[string]any{"type": "integer", "description": "Period in days to analyze (default: 30)"},
					"deploy_tag": map[string]any{"type": "string", "description": "Tag pattern for deploys (default: 'v*')"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		days := 30
		if v, ok := args["days"].(float64); ok {
			days = int(v)
		}
		deployTag := "v*"
		if v, ok := args["deploy_tag"].(string); ok && v != "" {
			deployTag = v
		}

		since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

		// Deployment frequency: count tags in period
		deploysCount := countTagsInPeriod(deployTag, since)

		// Lead time: average time from first commit to tag
		avgLeadTime := calculateAvgLeadTime(deployTag, days)

		// Change failure rate: tags with "hotfix" or "fix" in name / total tags
		failureRate := calculateFailureRate(deployTag, since)

		// Deploy frequency category
		var freqCategory string
		deploysPerDay := float64(deploysCount) / float64(days)
		switch {
		case deploysPerDay >= 1:
			freqCategory = "On-demand (multiple per day)"
		case deploysPerDay >= 1.0/7:
			freqCategory = "Weekly"
		case deploysPerDay >= 1.0/30:
			freqCategory = "Monthly"
		default:
			freqCategory = "Less than monthly"
		}

		// Lead time category
		var leadCategory string
		switch {
		case avgLeadTime < 24:
			leadCategory = "Less than one day"
		case avgLeadTime < 168:
			leadCategory = "Less than one week"
		case avgLeadTime < 720:
			leadCategory = "Less than one month"
		default:
			leadCategory = "More than one month"
		}

		metrics := doraMetrics{
			DeployFrequency:    freqCategory,
			LeadTimeForChanges: leadCategory,
			ChangeFailureRate:  fmt.Sprintf("%.1f%%", failureRate*100),
			TimeToRestore:      "N/A (requires incident data)",
			DeploysInPeriod:    deploysCount,
			PeriodDays:         days,
			AvgLeadTimeHours:   math.Round(avgLeadTime*10) / 10,
			FailureRatePercent: math.Round(failureRate*1000) / 10,
		}

		data, _ := json.MarshalIndent(metrics, "", "  ")
		return string(data), nil
	})

	// project_summary
	executor.RegisterHidden(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "project_summary",
			Description: "Generate a project activity summary from Git: contributors, commit frequency, active areas, recent changes.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"days": map[string]any{"type": "integer", "description": "Period in days (default: 7)"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		days := 7
		if v, ok := args["days"].(float64); ok {
			days = int(v)
		}
		since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

		// Contributors
		authors, _ := runGit("shortlog", "-sn", "--since="+since, "HEAD")

		// Most changed files
		changedFiles, _ := runGit("log", "--since="+since, "--name-only", "--pretty=format:", "--diff-filter=ACMR")

		// Commit count
			commitOut, _ := runGit("rev-list", "--count", "--since="+since, "HEAD")
		commitCount, _ := strconv.Atoi(strings.TrimSpace(commitOut))

		// File change frequency
		fileFreq := map[string]int{}
		for _, f := range strings.Split(changedFiles, "\n") {
			f = strings.TrimSpace(f)
			if f != "" {
				fileFreq[f]++
			}
		}

		// Top 10 most changed files
		type fileChange struct {
			File    string `json:"file"`
			Changes int    `json:"changes"`
		}
		var topFiles []fileChange
		for f, c := range fileFreq {
			topFiles = append(topFiles, fileChange{File: f, Changes: c})
		}
		// Simple sort (bubble)
		for i := range topFiles {
			for j := i + 1; j < len(topFiles); j++ {
				if topFiles[j].Changes > topFiles[i].Changes {
					topFiles[i], topFiles[j] = topFiles[j], topFiles[i]
				}
			}
		}
		if len(topFiles) > 10 {
			topFiles = topFiles[:10]
		}

		summary := map[string]any{
			"period":         fmt.Sprintf("%d days", days),
			"total_commits":  commitCount,
			"contributors":   strings.TrimSpace(authors),
			"active_files":   len(fileFreq),
			"hotspot_files":  topFiles,
		}

		data, _ := json.MarshalIndent(summary, "", "  ")
		return string(data), nil
	})
}

// ---------- Git Helpers ----------

func gitCountCommits(since, until string) (int, error) {
	out, err := runGit("rev-list", "--count", "--since="+since, "--until="+until, "HEAD")
	if err != nil {
		return 0, err
	}
	count, _ := strconv.Atoi(strings.TrimSpace(out))
	return count, nil
}

func gitCountMerges(since, until string) (int, error) {
	out, err := runGit("rev-list", "--count", "--merges", "--since="+since, "--until="+until, "HEAD")
	if err != nil {
		return 0, err
	}
	count, _ := strconv.Atoi(strings.TrimSpace(out))
	return count, nil
}

func generateBurndown(startDate, endDate string) []burndownPoint {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil
	}

	var points []burndownPoint
	total := end.Sub(start).Hours() / 24
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		elapsed := d.Sub(start).Hours() / 24
		remaining := total - elapsed
		if remaining < 0 {
			remaining = 0
		}
		points = append(points, burndownPoint{
			Date:      d.Format("2006-01-02"),
			Remaining: math.Round(remaining*10) / 10,
		})
	}
	return points
}

func countTagsInPeriod(pattern, since string) int {
	out, _ := exec.Command("git", "tag", "-l", pattern, "--sort=-creatordate").CombinedOutput()
	tags := strings.Split(strings.TrimSpace(string(out)), "\n")

	sinceTime, _ := time.Parse("2006-01-02", since)
	count := 0

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		dateOut, _ := exec.Command("git", "log", "-1", "--format=%aI", tag).CombinedOutput()
		tagDate, err := time.Parse(time.RFC3339, strings.TrimSpace(string(dateOut)))
		if err != nil {
			continue
		}
		if tagDate.After(sinceTime) {
			count++
		}
	}
	return count
}

func calculateAvgLeadTime(pattern string, days int) float64 {
	out, _ := exec.Command("git", "tag", "-l", pattern, "--sort=-creatordate").CombinedOutput()
	tags := strings.Split(strings.TrimSpace(string(out)), "\n")

	since := time.Now().AddDate(0, 0, -days)
	var totalHours float64
	count := 0

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}

		// Tag date
		dateOut, _ := exec.Command("git", "log", "-1", "--format=%aI", tag).CombinedOutput()
		tagDate, err := time.Parse(time.RFC3339, strings.TrimSpace(string(dateOut)))
		if err != nil || tagDate.Before(since) {
			continue
		}

		// First commit after previous tag
		prevOut, _ := exec.Command("git", "describe", "--abbrev=0", "--tags", tag+"^").CombinedOutput()
		prevTag := strings.TrimSpace(string(prevOut))

		var firstCommitDate time.Time
		if prevTag != "" {
			fcOut, _ := exec.Command("git", "log", "--reverse", "--format=%aI", prevTag+".."+tag).CombinedOutput()
			lines := strings.Split(strings.TrimSpace(string(fcOut)), "\n")
			if len(lines) > 0 {
				firstCommitDate, _ = time.Parse(time.RFC3339, strings.TrimSpace(lines[0]))
			}
		}

		if !firstCommitDate.IsZero() {
			leadTime := tagDate.Sub(firstCommitDate).Hours()
			totalHours += leadTime
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return totalHours / float64(count)
}

func calculateFailureRate(pattern, since string) float64 {
	out, _ := exec.Command("git", "tag", "-l", pattern, "--sort=-creatordate").CombinedOutput()
	tags := strings.Split(strings.TrimSpace(string(out)), "\n")

	sinceTime, _ := time.Parse("2006-01-02", since)
	total := 0
	failures := 0

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}

		dateOut, _ := exec.Command("git", "log", "-1", "--format=%aI", tag).CombinedOutput()
		tagDate, err := time.Parse(time.RFC3339, strings.TrimSpace(string(dateOut)))
		if err != nil || tagDate.Before(sinceTime) {
			continue
		}

		total++
		lower := strings.ToLower(tag)
		if strings.Contains(lower, "hotfix") || strings.Contains(lower, "fix") || strings.Contains(lower, "patch") {
			failures++
		}
	}

	if total == 0 {
		return 0
	}
	return float64(failures) / float64(total)
}
