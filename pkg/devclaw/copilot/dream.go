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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg"
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

	// DisableContradictionResolution is an escape hatch. When true, the dream
	// cycle still detects and logs contradictions but never supersedes (deletes)
	// the older side. Default false (resolution enabled). Negative flag so that a
	// config that omits it keeps the safe, on-by-default behavior.
	DisableContradictionResolution bool `yaml:"disable_contradiction_resolution"`
}

// DefaultDreamConfig returns sensible defaults.
func DefaultDreamConfig() DreamConfig {
	return DreamConfig{
		Enabled:              true,
		MinHoursBetween:      1,
		MinSessionsBetween:   2,
		MaxMemoriesToProcess: 100,
		IdleMinutes:          10,
	}
}

// DreamState persists the dream system's state between restarts.
type DreamState struct {
	LastDreamAt      time.Time `json:"last_dream_at"`
	SessionsSince    int       `json:"sessions_since"`
	CompactionsSince int       `json:"compactions_since"` // proxy for sessions in persistent channels (WhatsApp)
	TotalDreams      int       `json:"total_dreams"`
	LastResult       string    `json:"last_result"` // "success", "error", "no_changes"
}

// DreamResult holds the outcome of a dream consolidation run.
type DreamResult struct {
	MemoriesAnalyzed       int           `json:"memories_analyzed"`
	Duplicates             int           `json:"duplicates_merged"`
	Contradictions         int           `json:"contradictions_found"`
	ContradictionsResolved int           `json:"contradictions_resolved"`
	Consolidated           int           `json:"consolidated"`
	Duration               time.Duration `json:"duration"`
	Error                  error         `json:"-"`
}

// dreamClassifierBatchSize is the number of legacy files the classifier
// pass inspects per dream cycle. Bounded to prevent a single pass from
// hogging the DB. Configurable in Sprint 3.
const dreamClassifierBatchSize = 20

// DreamConsolidator manages background memory consolidation.
type DreamConsolidator struct {
	config   DreamConfig
	store    memory.Store
	stateDir string
	logger   *slog.Logger

	// sqliteStore is optional. When set (via WithSQLiteStore), the dream
	// cycle runs the legacy classifier phase. When nil, the phase is a no-op.
	sqliteStore *memory.SQLiteStore
	// hierarchyCfg gates the classifier phase. Zero value = disabled.
	hierarchyCfg HierarchyConfig

	state DreamState
	mu    sync.Mutex

	// running guards against concurrent Run() invocations. A scheduled
	// trigger and a ForceRun (SIGUSR1) can race; the CAS ensures only
	// one dream cycle executes at a time.
	running atomic.Bool

	// stopCh signals the background goroutine to stop.
	stopCh chan struct{}
	// done is closed when the background goroutine exits.
	done chan struct{}
}

// WithSQLiteStore wires an optional *SQLiteStore into the consolidator so
// the classifier phase can run during dream cycles. Keeps the memory.Store
// interface unbroken (Option A).
func (d *DreamConsolidator) WithSQLiteStore(s *memory.SQLiteStore) *DreamConsolidator {
	d.sqliteStore = s
	return d
}

// WithHierarchyConfig provides the hierarchy feature flag and keyword map
// needed to gate and drive the legacy classifier phase.
func (d *DreamConsolidator) WithHierarchyConfig(cfg HierarchyConfig) *DreamConsolidator {
	d.hierarchyCfg = cfg
	return d
}

// NewDreamConsolidator creates a new dream consolidator.
func NewDreamConsolidator(config DreamConfig, store memory.Store, stateDir string, logger *slog.Logger) *DreamConsolidator {
	if logger == nil {
		logger = slog.Default()
	}
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
		close(d.done) // Prevent Stop() deadlock.
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

// RecordCompaction increments the compaction counter.
// For persistent sessions (WhatsApp), compactions serve as the activity
// signal since sessions never formally end.
func (d *DreamConsolidator) RecordCompaction() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.CompactionsSince++
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

	// Gate 2: Activity since last dream (sessions OR compactions).
	// For persistent sessions (WhatsApp), compactions serve as the activity
	// signal since sessions never formally end.
	minSessions := d.config.MinSessionsBetween
	if minSessions <= 0 {
		minSessions = 2
	}
	activityCount := d.state.SessionsSince + d.state.CompactionsSince
	if activityCount < minSessions {
		return false
	}

	// Gate 3: File lock (prevent concurrent dreams).
	// Check if lock exists and is still valid (not stale from a crash).
	lockPath := filepath.Join(d.stateDir, "dream.lock")
	if info, err := os.Stat(lockPath); err == nil {
		// Lock file exists — check if it's stale (older than 30 minutes).
		if time.Since(info.ModTime()) < 30*time.Minute {
			return false
		}
		// Stale lock — remove it and proceed.
		d.logger.Warn("removing stale dream lock file", "age", time.Since(info.ModTime()))
		os.Remove(lockPath)
	}

	return true
}

// Run executes a single dream consolidation cycle.
// Phases: Orient → Gather → Consolidate → Apply
func (d *DreamConsolidator) Run(ctx context.Context) DreamResult {
	start := time.Now()
	result := DreamResult{}

	// In-process reentry guard. The file lock below is the cross-process
	// authority; this CAS is a fast-path rejection for same-process races
	// (scheduled ticker firing while a SIGUSR1-triggered ForceRun is active).
	if !d.running.CompareAndSwap(false, true) {
		d.logger.Warn("dream: run skipped — already running in this process")
		result.Error = fmt.Errorf("dream already running")
		return result
	}
	defer d.running.Store(false)

	// Ensure state directory exists before acquiring lock.
	if err := os.MkdirAll(d.stateDir, 0o700); err != nil {
		result.Error = fmt.Errorf("create dream state dir: %w", err)
		return result
	}

	// Acquire lock atomically using O_CREATE|O_EXCL to prevent TOCTOU races.
	lockPath := filepath.Join(d.stateDir, "dream.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			result.Error = fmt.Errorf("dream already running (lock exists)")
		} else {
			result.Error = fmt.Errorf("acquire dream lock: %w", err)
		}
		return result
	}
	fmt.Fprintf(lockFile, "pid=%d,time=%s", os.Getpid(), start.Format(time.RFC3339))
	lockFile.Close()
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

	generalContradictions := d.findContradictions(entries)

	// Evidence-based contradictions: detect negative facts that are contradicted
	// by newer positive facts about the same entity.
	evidenceContradictions := d.findEvidenceContradictions(entries)

	result.Contradictions = len(generalContradictions) + len(evidenceContradictions)

	// Phase 3: Consolidate — RESOLVE contradictions by superseding the older
	// side, and merge duplicates. The previous implementation only appended
	// "[Contradiction] A vs B" report entries, which grew MEMORY.md unboundedly
	// and left the stale fact retrievable — the root cause of hundreds of
	// unresolved contradictions accumulating in production while the agent kept
	// recalling outdated information.
	d.logger.Info("dream phase: consolidate",
		"duplicates", len(duplicates),
		"contradictions", result.Contradictions,
	)

	// Pinned memories are never superseded automatically.
	pinned := make(map[string]bool)
	for _, e := range entries {
		if e.IsPinned() {
			pinned[strings.TrimSpace(e.Content)] = true
		}
	}

	// Build the set to supersede. Resolution is on by default; the
	// disable_contradiction_resolution escape hatch turns it off entirely
	// (detect + log only) if a deployment sees a false positive.
	resolvable := make([]contradiction, 0, result.Contradictions)
	if !d.config.DisableContradictionResolution {
		// Evidence-based contradictions are high-confidence: a newer positive
		// fact about the same entity supersedes the older negative one.
		for _, c := range evidenceContradictions {
			if !pinned[strings.TrimSpace(c.entryA)] {
				resolvable = append(resolvable, c)
			}
		}
		// General negation-heuristic contradictions are a weaker signal, so only
		// supersede when the pair is also a near-duplicate restatement (high
		// token overlap). This prevents deleting a distinct fact that merely
		// shares a topic and an opposing keyword.
		for _, c := range generalContradictions {
			if pinned[strings.TrimSpace(c.entryA)] {
				continue
			}
			if contentsNearDuplicate(c.entryA, c.entryB) {
				resolvable = append(resolvable, c)
			}
		}
	}

	consolidated := 0
	resolved := 0
	if fileStore, ok := d.store.(*memory.FileStore); ok {
		// Supersede the older side of each contradiction with a soft [stale]
		// marker. parseMemoryFile skips stale entries (so searches stop
		// returning them) and Compact then drops them from disk. Soft and
		// reversible — the raw content stays in the file's history until compact.
		if len(resolvable) > 0 {
			resolved = d.expireStaleEntries(fileStore, resolvable)
		}
		// Compact: remove stale/expired entries and exact duplicates.
		removed, err := fileStore.Compact()
		if err != nil {
			d.logger.Warn("dream: compact failed", "error", err)
		} else if removed > 0 {
			consolidated = removed
		}
		d.logger.Info("dream: compacted memory file",
			"removed", consolidated,
			"duplicates_detected", len(duplicates),
			"contradictions_detected", result.Contradictions,
			"contradictions_resolved", resolved,
		)
	}

	result.Consolidated = consolidated
	result.ContradictionsResolved = resolved

	// Phase 3b: Classify — opportunistically label legacy files (wing IS NULL).
	// Error-isolated: a classifier panic or error MUST NOT abort the dream cycle.
	d.runClassifierPhase(ctx)
	IncClassifierPass()

	// Phase 3c: KG extraction — extract structured facts from memories.
	// Pattern-based only (no LLM budget consumed during idle consolidation).
	// Error-isolated: same as classifier phase.
	d.runKGExtractionPhase(ctx)

	// Phase 4: Apply — record results and update state.
	d.logger.Info("dream phase: apply", "consolidated", consolidated)
	d.recordResult("success")

	result.Duration = time.Since(start)
	return result
}

// runClassifierPhase runs the legacy classifier pass during a dream cycle.
// It is fully error-isolated: any panic or error is logged and swallowed so
// the rest of the dream cycle continues unaffected.
func (d *DreamConsolidator) runClassifierPhase(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			d.logger.Warn("legacy classifier phase panicked — isolated from dream cycle", "panic", r)
		}
	}()

	if !d.hierarchyCfg.Enabled || d.sqliteStore == nil {
		return
	}

	passCfg := memory.LegacyClassificationConfig{
		BatchSize: dreamClassifierBatchSize,
		Keywords:  d.hierarchyCfg.LegacyKeywords,
	}
	stats, err := d.sqliteStore.RunLegacyClassificationPass(ctx, passCfg)
	if err != nil {
		d.logger.Warn("legacy classifier phase error — continuing dream cycle", "error", err)
		return
	}
	if stats.Classified > 0 {
		d.logger.Info("legacy classifier classified files", "count", stats.Classified)
	}
}

// runKGExtractionPhase extracts KG triples from indexed memories during
// the dream cycle. Pattern-based only (no LLM budget consumed during idle
// consolidation). Capped at 100 extractions per cycle to bound runtime.
// Fully error-isolated: any panic or error is logged and swallowed.
func (d *DreamConsolidator) runKGExtractionPhase(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			d.logger.Warn("kg extraction phase panicked — isolated from dream cycle", "panic", r)
		}
	}()

	if d.sqliteStore == nil {
		return
	}
	kgStore := d.sqliteStore.KG()
	if kgStore == nil {
		return
	}
	if d.hierarchyCfg.KG.AutoExtract == "off" || d.hierarchyCfg.KG.AutoExtract == "" {
		return
	}

	patternSets := kg.DefaultPatternSets()
	if patternSets == nil {
		return
	}
	extractor, err := kg.NewExtractor(patternSets, d.logger)
	if err != nil {
		d.logger.Warn("kg extraction phase: failed to create extractor", "error", err)
		return
	}

	// Read indexed memories from the store for extraction.
	entries, err := d.store.GetAll()
	if err != nil {
		d.logger.Warn("kg extraction phase: failed to get memories", "error", err)
		return
	}

	extracted := 0
	const maxPerCycle = 100
	for _, entry := range entries {
		if ctx.Err() != nil || extracted >= maxPerCycle {
			break
		}
		n, err := extractor.ExtractAndStore(ctx, kgStore, entry.Content, "", "dream-cycle")
		if err != nil {
			d.logger.Warn("kg extraction phase: extraction error", "error", err)
			continue
		}
		extracted += n
	}

	if extracted > 0 {
		d.logger.Info("dream kg extraction complete", "triples", extracted)
	}
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
		" not ", " don't ", " doesn't ", " shouldn't ", " never ",
		" avoid ", " instead of ", " replaced ", " deprecated ",
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
				aHasNeg := strings.Contains(a, neg)
				bHasNeg := strings.Contains(b, neg)
				if aHasNeg != bHasNeg {
					// entryA = older entry (the one to supersede); entryB = newer.
					older, newer := entries[i], entries[j]
					if newer.Timestamp.Before(older.Timestamp) {
						older, newer = newer, older
					}
					found = append(found, contradiction{
						entryA: older.Content,
						entryB: newer.Content,
					})
					break
				}
			}
		}
	}

	return found
}

// findEvidenceContradictions detects negative memory facts that are contradicted
// by newer positive facts about the same entity. Generic — works for any topic
// (servers, APIs, services, tools), not just SSH or specific protocols.
func (d *DreamConsolidator) findEvidenceContradictions(entries []memory.Entry) []contradiction {
	// Generic negative sentiment indicators (PT + EN).
	negativeKeywords := []string{
		"bloqueado", "blocked", "unavailable", "indisponível",
		"denied", "refused", "failed", "erro", "error",
		"não consegue", "can't", "cannot", "unable",
		"não funciona", "doesn't work", "broken", "down",
		"deprecated", "removido", "removed", "desativado", "disabled",
	}

	// Generic positive sentiment indicators (PT + EN).
	positiveKeywords := []string{
		"access:", "acesso:", "funciona", "works", "working",
		"succeeded", "connected", "conectou", "accessible",
		"acessível", "deployed", "running", "active", "ativo",
		"fixed", "corrigido", "resolved", "resolvido",
		"enabled", "habilitado", "available", "disponível",
	}

	type taggedEntry struct {
		entry    memory.Entry
		negative bool
		topic    string // normalized topic for matching
	}

	// Extract a topic identifier from the entry: any IP, hostname, URL,
	// or the first significant noun phrase (lowercased first 60 chars).
	extractTopic := func(s string) string {
		lower := strings.ToLower(s)
		// Try IP address first.
		for i := 0; i < len(lower)-6; i++ {
			if lower[i] >= '0' && lower[i] <= '9' {
				end := i
				for end < len(lower) && (lower[end] >= '0' && lower[end] <= '9' || lower[end] == '.') {
					end++
				}
				candidate := lower[i:end]
				if strings.Count(candidate, ".") >= 2 && len(candidate) >= 7 {
					return candidate
				}
			}
		}
		// Fallback: use first 60 chars as topic fingerprint for overlap check.
		if len(lower) > 60 {
			lower = lower[:60]
		}
		return lower
	}

	var negatives, positives []taggedEntry

	for _, e := range entries {
		lower := strings.ToLower(e.Content)
		topic := extractTopic(e.Content)

		for _, kw := range negativeKeywords {
			if strings.Contains(lower, kw) {
				negatives = append(negatives, taggedEntry{entry: e, negative: true, topic: topic})
				break
			}
		}
		for _, kw := range positiveKeywords {
			if strings.Contains(lower, kw) {
				positives = append(positives, taggedEntry{entry: e, negative: false, topic: topic})
				break
			}
		}
	}

	// Cross-reference: if a positive fact targets the same topic as a negative,
	// and is newer, the negative is contradicted by evidence.
	var found []contradiction
	for _, neg := range negatives {
		for _, pos := range positives {
			sameEntity := neg.topic == pos.topic ||
				(len(neg.topic) > 10 && strings.Contains(pos.topic, neg.topic)) ||
				(len(pos.topic) > 10 && strings.Contains(neg.topic, pos.topic))

			if sameEntity && pos.entry.Timestamp.After(neg.entry.Timestamp) {
				found = append(found, contradiction{
					entryA: neg.entry.Content,
					entryB: fmt.Sprintf("[Evidence] %s (%s)",
						pos.entry.Content, pos.entry.Timestamp.Format("2006-01-02")),
				})
				break
			}
		}
	}

	return found
}

// expireStaleEntries scans all memory .md files and marks contradicted entries
// with "[stale]" prefix. Stale entries are skipped by memory search filtering
// and removed by the next Compact() pass. Returns the number of entries marked.
func (d *DreamConsolidator) expireStaleEntries(fileStore *memory.FileStore, contradictions []contradiction) int {
	if len(contradictions) == 0 {
		return 0
	}

	// Build a set of stale contents to match.
	stale := make(map[string]bool, len(contradictions))
	for _, c := range contradictions {
		stale[strings.TrimSpace(c.entryA)] = true
	}

	// Read all .md files in the memory directory.
	memDir := fileStore.BaseDir()
	entries, err := os.ReadDir(memDir)
	if err != nil {
		d.logger.Warn("dream: failed to read memory dir for expiration", "error", err)
		return 0
	}

	expired := 0
	for _, fi := range entries {
		if fi.IsDir() || !strings.HasSuffix(fi.Name(), ".md") {
			continue
		}
		path := filepath.Join(memDir, fi.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		modified := false
		for i, line := range lines {
			// Parse the line as a memory entry and compare by exact trimmed
			// content. This avoids the previous bug where a short stale string
			// could substring-match an unrelated entry, and works for any
			// leading whitespace / prefix variation (the memory format reserves
			// "- " as the bullet prefix).
			bulletIdx := indexBullet(line)
			if bulletIdx < 0 {
				continue
			}
			inner := strings.TrimSpace(line[bulletIdx+2:])
			if strings.HasPrefix(inner, "[stale]") {
				continue // already marked
			}
			parsedContent := stripEntryBrackets(inner)
			if parsedContent == "" {
				continue
			}
			if !stale[parsedContent] {
				continue
			}
			// Insert [stale] marker right after the "- " bullet, preserving
			// any indentation and the original entry body verbatim.
			lines[i] = line[:bulletIdx+2] + "[stale] " + line[bulletIdx+2:]
			modified = true
			expired++
		}

		if modified {
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
				d.logger.Warn("dream: failed to write expired entries", "file", path, "error", err)
			}
		}
	}

	return expired
}

// ForceRun bypasses all trigger gates and runs a dream cycle immediately.
// Used by the memory(action="dream_force") tool for on-demand consolidation.
func (d *DreamConsolidator) ForceRun(ctx context.Context) DreamResult {
	return d.Run(ctx)
}

// indexBullet returns the byte index of the "- " markdown bullet in line,
// allowing leading whitespace. Returns -1 if not found or if the bullet is
// not the first non-whitespace content on the line.
func indexBullet(line string) int {
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == ' ' || c == '\t' {
			continue
		}
		if c == '-' && i+1 < len(line) && line[i+1] == ' ' {
			return i
		}
		return -1
	}
	return -1
}

// stripEntryBrackets removes the leading `[timestamp]`, `[category]`, and
// optional `[expires:YYYY-MM-DD]` brackets that the memory format writes,
// returning the trimmed entry content. Mirrors the parsing logic in
// memory.parseMemoryFile so the stale comparison uses the same definition
// of "content" as reads.
func stripEntryBrackets(s string) string {
	s = strings.TrimSpace(s)
	// [timestamp]
	s = skipBracket(s)
	// [category]
	s = skipBracket(s)
	// [expires:...] optional
	if strings.HasPrefix(s, "[expires:") {
		s = skipBracket(s)
	}
	// [meta:...] optional v2 lifecycle metadata — must be peeled so content
	// comparison (e.g. stale-matching during supersede) sees the bare content.
	if strings.HasPrefix(s, "[meta:") {
		s = skipBracket(s)
	}
	return strings.TrimSpace(s)
}

// skipBracket drops a leading `[...]` token and trims surrounding space.
// Returns the input unchanged if there's no leading bracket group.
func skipBracket(s string) string {
	if !strings.HasPrefix(s, "[") {
		return s
	}
	close := strings.Index(s, "]")
	if close < 0 {
		return s
	}
	return strings.TrimSpace(s[close+1:])
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
// contentsNearDuplicate reports whether two memory contents are restatements of
// the same fact (high token overlap), as opposed to merely sharing a topic. It
// uses Jaccard similarity over words longer than 3 chars and is robust to word
// order and an inserted negation. Used to gate destructive supersede on the weak
// general-negation contradiction signal.
func contentsNearDuplicate(a, b string) bool {
	setA := significantWordSet(a)
	setB := significantWordSet(b)
	if len(setA) == 0 || len(setB) == 0 {
		return false
	}
	inter := 0
	for w := range setA {
		if setB[w] {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return false
	}
	return float64(inter)/float64(union) >= 0.6
}

// significantWordSet returns the set of normalized words longer than 3 chars.
func significantWordSet(s string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range splitWords(normalizeForComparison(s)) {
		if len(w) > 3 {
			set[w] = true
		}
	}
	return set
}

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
	_ = os.MkdirAll(d.stateDir, 0o700) // defensive: ensure dir exists
	path := filepath.Join(d.stateDir, "dream_state.json")
	data, _ := json.MarshalIndent(d.state, "", "  ")
	_ = os.WriteFile(path, data, 0o600)
}

func (d *DreamConsolidator) recordResult(result string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.LastDreamAt = time.Now()
	d.state.SessionsSince = 0
	d.state.CompactionsSince = 0
	d.state.TotalDreams++
	d.state.LastResult = result
	d.saveState()
}
