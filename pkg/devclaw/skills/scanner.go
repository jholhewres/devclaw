// Package skills – scanner.go implements a security scanner for skill files.
// It detects dangerous patterns in skill code that could indicate malicious intent:
// - Critical patterns (block): eval/exec injection, crypto mining
// - Warning patterns (warn): file read + network send, obfuscation
//
// The scanner caches results by file mtime+size to avoid re-scanning unchanged files.
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ScanSeverity indicates how severe a finding is.
type ScanSeverity string

const (
	ScanCritical ScanSeverity = "critical"
	ScanWarning  ScanSeverity = "warning"
)

// ScanFinding represents a single security finding in a skill file.
type ScanFinding struct {
	File     string       `json:"file"`
	Line     int          `json:"line"`
	Severity ScanSeverity `json:"severity"`
	Rule     string       `json:"rule"`
	Match    string       `json:"match"`
}

// ScanResult is the outcome of scanning a skill directory.
type ScanResult struct {
	Findings []ScanFinding `json:"findings"`
	Scanned  int           `json:"scanned"`
	Duration time.Duration `json:"duration"`
}

// HasCritical returns true if any finding is critical.
func (r ScanResult) HasCritical() bool {
	for _, f := range r.Findings {
		if f.Severity == ScanCritical {
			return true
		}
	}
	return false
}

// scanRule defines a pattern to match against source code lines.
type scanRule struct {
	name     string
	severity ScanSeverity
	pattern  *regexp.Regexp
}

// scannableExts are the file extensions we scan.
var scannableExts = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true,
	".sh": true, ".bash": true,
}

// lineCriticalRules are critical patterns that should block installation.
var lineCriticalRules = []scanRule{
	{name: "eval_injection", severity: ScanCritical,
		pattern: regexp.MustCompile(`(?i)\beval\s*\(`)},
	{name: "exec_child_process", severity: ScanCritical,
		pattern: regexp.MustCompile(`(?i)(child_process|subprocess\.call|os\.system|exec\.Command)`)},
	{name: "crypto_mining", severity: ScanCritical,
		pattern: regexp.MustCompile(`(?i)(stratum\+tcp|coinhive|cryptonight|xmrig)`)},
	{name: "reverse_shell", severity: ScanCritical,
		pattern: regexp.MustCompile(`(?i)(\/dev\/tcp\/|nc\s+-e|bash\s+-i\s+>&)`)},
}

// lineWarningRules are patterns that warrant a warning.
var lineWarningRules = []scanRule{
	{name: "network_exfil", severity: ScanWarning,
		pattern: regexp.MustCompile(`(?i)(http\.Post|requests\.post|fetch\(|XMLHttpRequest)`)},
	{name: "hex_obfuscation", severity: ScanWarning,
		pattern: regexp.MustCompile(`\\x[0-9a-fA-F]{2}\\x[0-9a-fA-F]{2}\\x[0-9a-fA-F]{2}`)},
	{name: "base64_decode", severity: ScanWarning,
		pattern: regexp.MustCompile(`(?i)(base64\.b64decode|atob\(|base64\.StdEncoding\.Decode)`)},
	{name: "env_access", severity: ScanWarning,
		pattern: regexp.MustCompile(`(?i)(os\.Getenv|process\.env|os\.environ)`)},
}

const scanCacheMaxEntries = 500

// scanCacheEntry caches scan results for a single file.
type scanCacheEntry struct {
	mtime    time.Time
	size     int64
	findings []ScanFinding
}

// SecurityScanner scans skill files for dangerous patterns.
type SecurityScanner struct {
	mu    sync.Mutex
	cache map[string]scanCacheEntry // keyed by file path
}

// NewSecurityScanner creates a new security scanner.
func NewSecurityScanner() *SecurityScanner {
	return &SecurityScanner{
		cache: make(map[string]scanCacheEntry),
	}
}

// ScanDirectory scans all scannable files in the given directory.
func (s *SecurityScanner) ScanDirectory(dir string) ScanResult {
	start := time.Now()
	var result ScanResult

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(info.Name())
		if !scannableExts[ext] {
			return nil
		}

		findings := s.scanFile(path, info)
		result.Findings = append(result.Findings, findings...)
		result.Scanned++
		return nil
	})

	result.Duration = time.Since(start)
	return result
}

// ScanFile scans a single file for dangerous patterns.
func (s *SecurityScanner) ScanFile(path string) []ScanFinding {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return s.scanFile(path, info)
}

func (s *SecurityScanner) scanFile(path string, info os.FileInfo) []ScanFinding {
	s.mu.Lock()
	if cached, ok := s.cache[path]; ok {
		if cached.mtime.Equal(info.ModTime()) && cached.size == info.Size() {
			s.mu.Unlock()
			return cached.findings
		}
	}
	s.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var findings []ScanFinding
	lines := strings.Split(string(data), "\n")

	allRules := append(lineCriticalRules, lineWarningRules...)
	for lineNum, line := range lines {
		for _, rule := range allRules {
			if rule.pattern.MatchString(line) {
				match := line
				if len(match) > 120 {
					match = match[:120] + "..."
				}
				findings = append(findings, ScanFinding{
					File:     path,
					Line:     lineNum + 1,
					Severity: rule.severity,
					Rule:     rule.name,
					Match:    strings.TrimSpace(match),
				})
			}
		}
	}

	// Cache result (with size cap to prevent unbounded growth).
	s.mu.Lock()
	if len(s.cache) >= scanCacheMaxEntries {
		// Evict ~25% of entries (simple strategy: clear all, let cache refill).
		for k := range s.cache {
			delete(s.cache, k)
			if len(s.cache) < scanCacheMaxEntries*3/4 {
				break
			}
		}
	}
	s.cache[path] = scanCacheEntry{
		mtime:    info.ModTime(),
		size:     info.Size(),
		findings: findings,
	}
	s.mu.Unlock()

	return findings
}

// FormatFindings returns a human-readable summary of scan findings.
func FormatFindings(findings []ScanFinding) string {
	if len(findings) == 0 {
		return "No security issues found."
	}

	var sb strings.Builder
	critCount, warnCount := 0, 0
	for _, f := range findings {
		if f.Severity == ScanCritical {
			critCount++
		} else {
			warnCount++
		}
	}

	sb.WriteString(fmt.Sprintf("Security scan: %d critical, %d warnings\n\n", critCount, warnCount))
	for _, f := range findings {
		icon := "⚠️"
		if f.Severity == ScanCritical {
			icon = "🚫"
		}
		sb.WriteString(fmt.Sprintf("%s [%s] %s:%d — %s\n  %s\n",
			icon, f.Severity, filepath.Base(f.File), f.Line, f.Rule, f.Match))
	}
	return sb.String()
}
