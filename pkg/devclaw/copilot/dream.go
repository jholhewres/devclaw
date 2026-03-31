// Package copilot – dream.go implements the Dream System, a background
// consolidation process that runs when the daemon is idle. It analyzes
// accumulated memories, detects contradictions, merges duplicates, and
// produces consolidated summaries — similar to how sleep consolidates
// learning in biological systems.
//
// The Dream System uses a 3-gate trigger:
//   - Gate 1: Minimum time since last dream (default: 6 hours)
//   - Gate 2: Minimum sessions since last dream (default: 2)
//   - Gate 3: File lock to prevent concurrent dreams
//
// Phases: Orient → Gather → Consolidate → Apply
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// DreamConfig configures the background memory consolidation system.
type DreamConfig struct {
	// Enabled controls whether the dream system is active. Default: true.
	Enabled bool `yaml:"enabled"`

	// MinHoursBetween is the minimum hours between dream runs (Gate 1). Default: 6.
	MinHoursBetween int `yaml:"min_hours_between"`

	// MinSessionsBetween is the minimum sessions between dream runs (Gate 2). Default: 2.
	MinSessionsBetween int `yaml:"min_sessions_between"`

	// MaxMemoriesToProcess limits how many memories to analyze per dream. Default: 100.
	MaxMemoriesToProcess int `yaml:"max_memories_to_process"`

	// IdleMinutes is how long the daemon must be idle before triggering. Default: 10.
	IdleMinutes int `yaml:"idle_minutes"`
}

// DefaultDreamConfig returns sensible defaults.
func DefaultDreamConfig() DreamConfig {
	return DreamConfig{
		Enabled:              true,
		MinHoursBetween:      6,
		MinSessionsBetween:   2,
		MaxMemoriesToProcess: 100,
		IdleMinutes:          10,
	}
}

// DreamState persists the dream system's state between restarts.
type DreamState struct {
	LastDreamAt    time.Time `json:"last_dream_at"`
	SessionsSince  int       `json:"sessions_since"`
	TotalDreams    int       `json:"total_dreams"`
	LastResult     string    `json:"last_result"` // "success", "error", "no_changes"
}

// DreamResult holds the outcome of a dream consolidation run.
type DreamResult struct {
	MemoriesAnalyzed int           `json:"memories_analyzed"`
	Duplicates       int           `json:"duplicates_merged"`
	Contradictions   int           `json:"contradictions_found"`
	Consolidated     int           `json:"consolidated"`
	Duration         time.Duration `json:"duration"`
	Error            error         `json:"-"`
}

// DreamConsolidator manages background memory consolidation.
type DreamConsolidator struct {
	config   DreamConfig
	store    memory.Store
	stateDir string
	logger   *slog.Logger

	state DreamState
	mu    sync.Mutex

	// stopCh signals the background goroutine to stop.
	stopCh chan struct{}
	// done is closed when the background goroutine exits.
	done chan struct{}
}

// NewDreamConsolidator creates a new dream consolidator.
func NewDreamConsolidator(config DreamConfig, store memory.Store, stateDir string, logger *slog.Logger) *DreamConsolidator {
	d := &DreamConsolidator{
		config:   config,
		store:    store,
		stateDir: stateDir,
		logger:   logger.With("component", "dream"),
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
	d.loadState()
	return d
}

// Start begins the background dream loop. It checks gates periodically
// and runs consolidation when conditions are met.
func (d *DreamConsolidator) Start(ctx context.Context) {
	if !d.config.Enabled {
		d.logger.Info("dream system disabled")
		return
	}

	idleInterval := time.Duration(d.config.IdleMinutes) * time.Minute
	if idleInterval < time.Minute {
		idleInterval = 10 * time.Minute
	}

	go func() {
		defer close(d.done)
		ticker := time.NewTicker(idleInterval)
		defer ticker.Stop()

		d.logger.Info("dream system started",
			"idle_interval", idleInterval,
			"min_hours_between", d.config.MinHoursBetween,
			"min_sessions_between", d.config.MinSessionsBetween,
		)

		for {
			select {
			case <-ctx.Done():
				return
			case <-d.stopCh:
				return
			case <-ticker.C:
				if d.shouldDream() {
					result := d.Run(ctx)
					if result.Error != nil {
						d.logger.Warn("dream run failed", "error", result.Error)
					} else {
						d.logger.Info("dream run completed",
							"analyzed", result.MemoriesAnalyzed,
							"duplicates", result.Duplicates,
							"contradictions", result.Contradictions,
							"consolidated", result.Consolidated,
							"duration_ms", result.Duration.Milliseconds(),
						)
					}
				}
			}
		}
	}()
}

// Stop halts the background dream loop.
func (d *DreamConsolidator) Stop() {
	select {
	case <-d.stopCh:
		return // Already stopped.
	default:
		close(d.stopCh)
	}
	<-d.done
}

// RecordSession increments the session counter (Gate 2).
// Called by the assistant when a session ends.
func (d *DreamConsolidator) RecordSession() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.SessionsSince++
	d.saveState()
}

// shouldDream checks all three gates.
func (d *DreamConsolidator) shouldDream() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Gate 1: Time since last dream.
	minHours := d.config.MinHoursBetween
	if minHours <= 0 {
		minHours = 6
	}
	if !d.state.LastDreamAt.IsZero() && time.Since(d.state.LastDreamAt).Hours() < float64(minHours) {
		return false
	}

	// Gate 2: Sessions since last dream.
	minSessions := d.config.MinSessionsBetween
	if minSessions <= 0 {
		minSessions = 2
	}
	if d.state.SessionsSince < minSessions {
		return false
	}

	// Gate 3: File lock (prevent concurrent dreams).
	lockPath := filepath.Join(d.stateDir, "dream.lock")
	if _, err := os.Stat(lockPath); err == nil {
		// Lock file exists — another instance is dreaming.
		return false
	}

	return true
}

// Run executes a single dream consolidation cycle.
// Phases: Orient → Gather → Consolidate → Apply
func (d *DreamConsolidator) Run(ctx context.Context) DreamResult {
	start := time.Now()
	result := DreamResult{}

	// Acquire lock.
	lockPath := filepath.Join(d.stateDir, "dream.lock")
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("pid=%d,time=%s", os.Getpid(), start.Format(time.RFC3339))), 0o600); err != nil {
		result.Error = fmt.Errorf("acquire dream lock: %w", err)
		return result
	}
	defer os.Remove(lockPath)

	// Phase 1: Orient — gather all memories.
	d.logger.Info("dream phase: orient")
	entries, err := d.store.GetAll()
	if err != nil {
		result.Error = fmt.Errorf("orient: get all memories: %w", err)
		d.recordResult("error")
		return result
	}

	maxProcess := d.config.MaxMemoriesToProcess
	if maxProcess <= 0 {
		maxProcess = 100
	}
	if len(entries) > maxProcess {
		entries = entries[len(entries)-maxProcess:]
	}
	result.MemoriesAnalyzed = len(entries)

	if len(entries) < 3 {
		d.logger.Debug("dream: too few memories to consolidate", "count", len(entries))
		d.recordResult("no_changes")
		result.Duration = time.Since(start)
		return result
	}

	// Phase 2: Gather — detect duplicates and contradictions.
	d.logger.Info("dream phase: gather", "memories", len(entries))
	duplicates := d.findDuplicates(entries)
	result.Duplicates = len(duplicates)

	contradictions := d.findContradictions(entries)
	result.Contradictions = len(contradictions)

	// Phase 3: Consolidate — merge duplicates, flag contradictions.
	d.logger.Info("dream phase: consolidate",
		"duplicates", len(duplicates),
		"contradictions", len(contradictions),
	)

	consolidated := 0

	// Save consolidated entries (merge duplicates into single entries).
	for _, dup := range duplicates {
		merged := memory.Entry{
			Content:   fmt.Sprintf("[Consolidated] %s (merged from %d similar entries)", dup.canonical, dup.count),
			Source:    "dream",
			Category: "summary",
			Timestamp: time.Now(),
		}
		if err := d.store.Save(merged); err != nil {
			d.logger.Warn("dream: failed to save consolidated memory", "error", err)
			continue
		}
		consolidated++
	}

	// Save contradiction reports.
	for _, c := range contradictions {
		report := memory.Entry{
			Content:   fmt.Sprintf("[Contradiction] %s vs %s", c.entryA, c.entryB),
			Source:    "dream",
			Category: "summary",
			Timestamp: time.Now(),
		}
		if err := d.store.Save(report); err != nil {
			d.logger.Warn("dream: failed to save contradiction report", "error", err)
		}
	}

	result.Consolidated = consolidated

	// Phase 4: Apply — record results and update state.
	d.logger.Info("dream phase: apply", "consolidated", consolidated)
	d.recordResult("success")

	result.Duration = time.Since(start)
	return result
}

// State returns the current dream state (for observability).
func (d *DreamConsolidator) State() DreamState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

// ── Duplicate Detection ──

type duplicateGroup struct {
	canonical string
	count     int
}

// findDuplicates groups memories with similar content.
// Uses simple substring/prefix matching for efficiency.
func (d *DreamConsolidator) findDuplicates(entries []memory.Entry) []duplicateGroup {
	seen := make(map[string]*duplicateGroup)

	for _, e := range entries {
		content := normalizeForComparison(e.Content)
		if len(content) < 20 {
			continue
		}

		// Use first 40 chars as the grouping key to catch entries
		// that share the same topic but diverge in details.
		key := content
		if len(key) > 40 {
			key = key[:40]
		}

		if group, ok := seen[key]; ok {
			group.count++
		} else {
			seen[key] = &duplicateGroup{
				canonical: e.Content,
				count:     1,
			}
		}
	}

	var groups []duplicateGroup
	for _, g := range seen {
		if g.count > 1 {
			groups = append(groups, *g)
		}
	}
	return groups
}

// ── Contradiction Detection ──

type contradiction struct {
	entryA string
	entryB string
}

// findContradictions detects potentially contradicting memories.
// Uses simple heuristic: entries about the same topic with opposing sentiment.
func (d *DreamConsolidator) findContradictions(entries []memory.Entry) []contradiction {
	negators := []string{
		"not ", "don't ", "doesn't ", "shouldn't ", "never ",
		"avoid ", "instead of ", "replaced ", "deprecated ",
	}

	var found []contradiction
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			a := normalizeForComparison(entries[i].Content)
			b := normalizeForComparison(entries[j].Content)

			// Skip if they're about different topics (no overlap).
			if !hasSignificantOverlap(a, b) {
				continue
			}

			// Check if one negates the other.
			for _, neg := range negators {
				aHasNeg := containsWord(a, neg)
				bHasNeg := containsWord(b, neg)
				if aHasNeg != bHasNeg {
					found = append(found, contradiction{
						entryA: entries[i].Content,
						entryB: entries[j].Content,
					})
					break
				}
			}
		}
	}

	return found
}

// ── Helpers ──

func normalizeForComparison(s string) string {
	s = fmt.Sprintf("%s", s) // Ensure string type.
	// Lowercase and collapse whitespace.
	var result []byte
	lastSpace := false
	for _, c := range []byte(s) {
		if c >= 'A' && c <= 'Z' {
			c = c + 32 // toLower
		}
		if c == ' ' || c == '\t' || c == '\n' {
			if !lastSpace {
				result = append(result, ' ')
				lastSpace = true
			}
			continue
		}
		lastSpace = false
		result = append(result, c)
	}
	return string(result)
}

func containsWord(text, word string) bool {
	return len(text) > 0 && len(word) > 0 && (len(text) >= len(word)) &&
		(text == word || // exact match
			(len(text) > len(word) && text[:len(word)] == word) || // starts with
			(len(text) > len(word) && text[len(text)-len(word):] == word) || // ends with
			containsSubstr(text, " "+word) || containsSubstr(text, word+" "))
}

func containsSubstr(s, sub string) bool {
	return len(sub) <= len(s) && findSubstr(s, sub) >= 0
}

func findSubstr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// hasSignificantOverlap checks if two strings share enough words in common.
func hasSignificantOverlap(a, b string) bool {
	wordsA := splitWords(a)
	wordsB := make(map[string]bool)
	for _, w := range splitWords(b) {
		if len(w) > 3 { // Skip short words.
			wordsB[w] = true
		}
	}

	overlap := 0
	for _, w := range wordsA {
		if len(w) > 3 && wordsB[w] {
			overlap++
		}
	}
	return overlap >= 3
}

func splitWords(s string) []string {
	var words []string
	var current []byte
	for _, c := range []byte(s) {
		if c == ' ' || c == '\t' || c == '\n' || c == ',' || c == '.' {
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
		} else {
			current = append(current, c)
		}
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}

// ── State Persistence ──

func (d *DreamConsolidator) loadState() {
	path := filepath.Join(d.stateDir, "dream_state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return // No state file — first run.
	}
	_ = json.Unmarshal(data, &d.state)
}

func (d *DreamConsolidator) saveState() {
	path := filepath.Join(d.stateDir, "dream_state.json")
	data, _ := json.MarshalIndent(d.state, "", "  ")
	_ = os.WriteFile(path, data, 0o600)
}

func (d *DreamConsolidator) recordResult(result string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.LastDreamAt = time.Now()
	d.state.SessionsSince = 0
	d.state.TotalDreams++
	d.state.LastResult = result
	d.saveState()
}
