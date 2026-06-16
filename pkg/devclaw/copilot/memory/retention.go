package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MemoryArchiveFileName is where retention sweeps move aged entries instead of
// hard-deleting them, so removal is reversible.
const MemoryArchiveFileName = "MEMORY.archive.md"

// RetentionOptions configures a retention sweep. The zero value is not useful;
// prefer DefaultRetentionOptions.
type RetentionOptions struct {
	DryRun         bool          // when true, report only — never modify files
	OperationalTTL time.Duration // age after which operational memories are archived
	EpisodicTTL    time.Duration // age after which episodic memories are archived
}

// DefaultRetentionOptions returns a conservative, dry-run-by-default policy.
func DefaultRetentionOptions() RetentionOptions {
	return RetentionOptions{
		DryRun:         true,
		OperationalTTL: 14 * 24 * time.Hour,
		EpisodicTTL:    90 * 24 * time.Hour,
	}
}

// RetentionCandidate describes an entry a sweep would archive.
type RetentionCandidate struct {
	Content string
	Reason  string
}

// RetentionReport summarizes a sweep.
type RetentionReport struct {
	DryRun     bool
	Scanned    int
	Candidates []RetentionCandidate
	Archived   int // 0 on dry-run
}

// memoryTypeOf returns the entry's lifecycle class, inferring from category when
// the explicit MemoryType is unset (matches the v2 default mapping: event ->
// episodic, everything else -> semantic).
func memoryTypeOf(e Entry) string {
	if e.MemoryType != "" {
		return e.MemoryType
	}
	if e.Category == "event" {
		return "episodic"
	}
	return "semantic"
}

// RetentionSweep archives aged operational/episodic entries and entries already
// past their TTL. Semantic and pinned entries are always kept. It is dry-run by
// default (reports candidates without touching disk); when applied, candidates
// are appended to MEMORY.archive.md before being removed from MEMORY.md, so the
// operation is reversible (soft-delete, never a hard delete of user data).
func (fs *FileStore) RetentionSweep(now time.Time, opts RetentionOptions) (RetentionReport, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	memFile := filepath.Join(fs.baseDir, MemoryFileName)
	content, err := os.ReadFile(memFile)
	if err != nil {
		if os.IsNotExist(err) {
			return RetentionReport{DryRun: opts.DryRun}, nil
		}
		return RetentionReport{}, fmt.Errorf("reading memory file: %w", err)
	}

	entries := parseMemoryFile(string(content), "memory")
	report := RetentionReport{DryRun: opts.DryRun, Scanned: len(entries)}

	keep := make([]Entry, 0, len(entries))
	archive := make([]Entry, 0)
	for _, e := range entries {
		if e.IsPinned() {
			keep = append(keep, e)
			continue
		}
		reason := ""
		switch {
		case e.IsExpired(now):
			reason = "expired"
		case memoryTypeOf(e) == "operational" && opts.OperationalTTL > 0 && now.Sub(e.Timestamp) > opts.OperationalTTL:
			reason = "operational beyond TTL"
		case memoryTypeOf(e) == "episodic" && opts.EpisodicTTL > 0 && now.Sub(e.Timestamp) > opts.EpisodicTTL:
			reason = "episodic beyond TTL"
		}
		if reason != "" {
			report.Candidates = append(report.Candidates, RetentionCandidate{Content: e.Content, Reason: reason})
			archive = append(archive, e)
			continue
		}
		keep = append(keep, e)
	}

	if opts.DryRun || len(archive) == 0 {
		return report, nil
	}

	// Append candidates to the archive (soft-delete) before rewriting MEMORY.md.
	if err := fs.appendArchive(archive); err != nil {
		return report, err
	}

	var buf strings.Builder
	buf.WriteString("# DevClaw Memory\n\nLong-term facts and preferences.\n\n")
	for _, e := range keep {
		buf.WriteString(formatEntryLine(e))
	}
	tmp := memFile + ".retention.tmp"
	if err := os.WriteFile(tmp, []byte(buf.String()), 0o600); err != nil {
		return report, fmt.Errorf("writing retention temp file: %w", err)
	}
	if err := os.Rename(tmp, memFile); err != nil {
		os.Remove(tmp)
		return report, fmt.Errorf("atomic rename: %w", err)
	}
	report.Archived = len(archive)
	return report, nil
}

// appendArchive appends entries to MEMORY.archive.md (creating it if needed).
func (fs *FileStore) appendArchive(entries []Entry) error {
	archiveFile := filepath.Join(fs.baseDir, MemoryArchiveFileName)
	f, err := os.OpenFile(archiveFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening archive file: %w", err)
	}
	defer f.Close()

	info, _ := f.Stat()
	if info != nil && info.Size() == 0 {
		if _, err := f.WriteString("# DevClaw Memory Archive\n\nEntries removed by retention sweeps (recoverable).\n\n"); err != nil {
			return err
		}
	}
	for _, e := range entries {
		if _, err := f.WriteString(formatEntryLine(e)); err != nil {
			return err
		}
	}
	return nil
}
