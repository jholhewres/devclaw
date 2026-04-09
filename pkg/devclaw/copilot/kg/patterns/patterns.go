// Package patterns defines the YAML schema and loader for extraction rule sets.
//
// A PatternSet is a collection of regex-based extraction rules keyed by language.
// Templates use {{subject}} and {{object}} placeholders that expand to named
// capture groups for SPO triple extraction.
package patterns

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Pattern defines a single extraction rule.
type Pattern struct {
	Name       string   `yaml:"name"`
	Predicate  string   `yaml:"predicate"`
	Templates  []string `yaml:"templates"`
	Functional bool     `yaml:"functional"`
	Confidence float64  `yaml:"confidence"`
}

// PatternSet is the top-level YAML structure for a language-specific rule file.
type PatternSet struct {
	Language         string    `yaml:"language"`
	Version          int       `yaml:"version"`
	ClauseSeparators []string  `yaml:"clause_separators"`
	Patterns         []Pattern `yaml:"patterns"`
}

const (
	subjectPlaceholder = "{{subject}}"
	objectPlaceholder  = "{{object}}"
	subjectGroup       = "(?P<subject>.+?)"
	objectGroup        = "(?P<object>.+)"
)

// Load reads and validates a YAML pattern file.
func Load(data []byte) (*PatternSet, error) {
	var ps PatternSet
	if err := yaml.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("patterns: parse yaml: %w", err)
	}
	if err := ps.Validate(); err != nil {
		return nil, err
	}
	return &ps, nil
}

// Validate checks a PatternSet for correctness.
func (ps *PatternSet) Validate() error {
	if strings.TrimSpace(ps.Language) == "" {
		return fmt.Errorf("patterns: language is required")
	}
	if len(ps.Patterns) == 0 {
		return fmt.Errorf("patterns: at least one pattern is required")
	}
	for i, p := range ps.Patterns {
		if err := validatePattern(i, p); err != nil {
			return err
		}
	}
	return nil
}

func validatePattern(idx int, p Pattern) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("patterns: pattern[%d]: name is required", idx)
	}
	if strings.TrimSpace(p.Predicate) == "" {
		return fmt.Errorf("patterns: pattern[%d]: predicate is required", idx)
	}
	if len(p.Templates) == 0 {
		return fmt.Errorf("patterns: pattern[%d]: at least one template is required", idx)
	}
	for j, tmpl := range p.Templates {
		if err := validateTemplate(idx, j, tmpl); err != nil {
			return err
		}
	}
	return nil
}

func validateTemplate(patIdx, tmplIdx int, tmpl string) error {
	if !strings.Contains(tmpl, subjectPlaceholder) {
		return fmt.Errorf("patterns: pattern[%d].templates[%d]: missing {{subject}} placeholder", patIdx, tmplIdx)
	}
	if !strings.Contains(tmpl, objectPlaceholder) {
		return fmt.Errorf("patterns: pattern[%d].templates[%d]: missing {{object}} placeholder", patIdx, tmplIdx)
	}
	expanded := expandTemplate(tmpl)
	if _, err := regexp.Compile(expanded); err != nil {
		return fmt.Errorf("patterns: pattern[%d].templates[%d]: invalid regex: %w", patIdx, tmplIdx, err)
	}
	return nil
}

// ExpandTemplate converts a template with placeholders into a regex string.
func ExpandTemplate(tmpl string) string {
	return expandTemplate(tmpl)
}

func expandTemplate(tmpl string) string {
	s := strings.ReplaceAll(tmpl, subjectPlaceholder, subjectGroup)
	s = strings.ReplaceAll(s, objectPlaceholder, objectGroup)
	return s
}
