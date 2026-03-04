// Package skills – catalog.go provides an embedded skill catalog parsed from
// catalog.yaml. This makes the full catalog available offline without needing
// to fetch from ClawHub at runtime.
package skills

import (
	_ "embed"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed catalog.yaml
var catalogYAML []byte

// CatalogEntry represents a single skill in the embedded catalog.
type CatalogEntry struct {
	Name          string   `yaml:"-"`
	Label         string   `yaml:"label"`
	LabelPT       string   `yaml:"label_pt"`
	Path          string   `yaml:"path"`
	Version       string   `yaml:"version"`
	Category      string   `yaml:"category"`
	Tags          []string `yaml:"tags"`
	Description   string   `yaml:"description"`
	DescriptionPT string   `yaml:"description_pt"`
	ClawHubURL    string   `yaml:"clawhub_url"`
}

// catalogFile represents the top-level structure of the catalog YAML.
type catalogFile struct {
	Skills map[string]CatalogEntry `yaml:"skills"`
}

var (
	catalogOnce    sync.Once
	catalogEntries []CatalogEntry
)

// CatalogSkills returns all skills from the embedded catalog, sorted alphabetically.
func CatalogSkills() []CatalogEntry {
	catalogOnce.Do(func() {
		var cf catalogFile
		if err := yaml.Unmarshal(catalogYAML, &cf); err != nil {
			catalogEntries = []CatalogEntry{}
			return
		}

		catalogEntries = make([]CatalogEntry, 0, len(cf.Skills))
		for name, entry := range cf.Skills {
			entry.Name = name
			catalogEntries = append(catalogEntries, entry)
		}

		sort.Slice(catalogEntries, func(i, j int) bool {
			return catalogEntries[i].Name < catalogEntries[j].Name
		})
	})

	return catalogEntries
}

// GetCatalogSkill returns a catalog entry by name, or nil if not found.
func GetCatalogSkill(name string) *CatalogEntry {
	for _, e := range CatalogSkills() {
		if e.Name == name {
			return &e
		}
	}
	return nil
}

// CatalogSkillNames returns the names of all skills in the embedded catalog.
func CatalogSkillNames() []string {
	entries := CatalogSkills()
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}
