// registry.go implementa o registro central de skills, responsável por
// descobrir, carregar, buscar e gerenciar o ciclo de vida das skills.
package skills

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Registry é o registro central que gerencia todas as skills disponíveis.
type Registry struct {
	// skills armazena as skills registradas, indexadas por nome.
	skills map[string]Skill

	// disabled tracks skills that have been disabled by the user.
	disabled map[string]bool

	// loaders contém os carregadores de skills de diferentes fontes.
	loaders []SkillLoader

	// index mantém índices para busca eficiente.
	index *Index

	logger *slog.Logger
	mu     sync.RWMutex
}

// Index mantém índices para busca eficiente de skills por categoria, tag e autor.
type Index struct {
	ByCategory map[string][]string
	ByTag      map[string][]string
	ByAuthor   map[string][]string
}

// NewRegistry cria um novo registro de skills.
func NewRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}

	return &Registry{
		skills:   make(map[string]Skill),
		disabled: make(map[string]bool),
		loaders:  make([]SkillLoader, 0),
		index: &Index{
			ByCategory: make(map[string][]string),
			ByTag:      make(map[string][]string),
			ByAuthor:   make(map[string][]string),
		},
		logger: logger,
	}
}

// AddLoader adiciona um carregador de skills ao registry.
func (r *Registry) AddLoader(loader SkillLoader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loaders = append(r.loaders, loader)
}

// LoadAll carrega skills de todos os loaders registrados.
func (r *Registry) LoadAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, loader := range r.loaders {
		skills, err := loader.Load(ctx)
		if err != nil {
			r.logger.Error("falha ao carregar skills",
				"source", loader.Source(),
				"error", err,
			)
			continue
		}

		for _, skill := range skills {
			meta := skill.Metadata()
			// Só indexa se a skill ainda não existia para evitar duplicatas no índice.
			if _, existed := r.skills[meta.Name]; !existed {
				r.indexSkill(meta)
			}
			r.skills[meta.Name] = skill

			r.logger.Info("skill carregada",
				"name", meta.Name,
				"version", meta.Version,
				"source", loader.Source(),
			)
		}
	}

	return nil
}

// Register registra uma skill diretamente no registry.
func (r *Registry) Register(skill Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	meta := skill.Metadata()
	if _, exists := r.skills[meta.Name]; exists {
		return fmt.Errorf("skill %q já registrada", meta.Name)
	}

	r.skills[meta.Name] = skill
	r.indexSkill(meta)
	return nil
}

// Get retorna uma skill pelo nome.
func (r *Registry) Get(name string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

// List retorna os metadados de todas as skills registradas.
func (r *Registry) List() []Metadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Metadata, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s.Metadata())
	}
	return result
}

// Search busca skills por query textual, comparando com nome, descrição e tags.
func (r *Registry) Search(query string) []Metadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query = strings.ToLower(query)
	var results []Metadata

	for _, s := range r.skills {
		meta := s.Metadata()
		if r.matchesQuery(meta, query) {
			results = append(results, meta)
		}
	}

	return results
}

// ByCategory retorna todas as skills de uma categoria específica.
func (r *Registry) ByCategory(category string) []Metadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names, ok := r.index.ByCategory[category]
	if !ok {
		return nil
	}

	results := make([]Metadata, 0, len(names))
	for _, name := range names {
		if s, exists := r.skills[name]; exists {
			results = append(results, s.Metadata())
		}
	}
	return results
}

// Reload reloads all skills from all loaders, refreshing the catalog.
// New skills are added, existing ones are updated. Returns count of skills loaded.
func (r *Registry) Reload(ctx context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	loaded := 0
	for _, loader := range r.loaders {
		skills, err := loader.Load(ctx)
		if err != nil {
			r.logger.Error("failed to reload skills",
				"source", loader.Source(),
				"error", err,
			)
			continue
		}

		for _, skill := range skills {
			meta := skill.Metadata()
			if _, existed := r.skills[meta.Name]; !existed {
				r.indexSkill(meta)
			}
			r.skills[meta.Name] = skill
			loaded++

			r.logger.Debug("skill reloaded",
				"name", meta.Name,
				"source", loader.Source(),
			)
		}
	}

	r.logger.Info("skills reloaded", "count", loaded)
	return loaded, nil
}

// Enable re-enables a previously disabled skill.
func (r *Registry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.skills[name]; !exists {
		return fmt.Errorf("skill %q not found", name)
	}
	delete(r.disabled, name)
	return nil
}

// Disable marks a skill as disabled without removing it.
func (r *Registry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.skills[name]; !exists {
		return fmt.Errorf("skill %q not found", name)
	}
	r.disabled[name] = true
	return nil
}

// IsEnabled returns whether a skill is currently enabled.
func (r *Registry) IsEnabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return !r.disabled[name]
}

// Remove removes a skill from the registry by name.
func (r *Registry) Remove(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[name]; !exists {
		return false
	}

	delete(r.skills, name)
	return true
}

// ShutdownAll encerra todas as skills de forma graciosa.
func (r *Registry) ShutdownAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, s := range r.skills {
		if err := s.Shutdown(); err != nil {
			r.logger.Error("erro ao encerrar skill",
				"name", name,
				"error", err,
			)
		}
	}
}

// indexSkill adiciona uma skill aos índices de busca.
func (r *Registry) indexSkill(meta Metadata) {
	r.index.ByCategory[meta.Category] = append(r.index.ByCategory[meta.Category], meta.Name)
	r.index.ByAuthor[meta.Author] = append(r.index.ByAuthor[meta.Author], meta.Name)
	for _, tag := range meta.Tags {
		r.index.ByTag[tag] = append(r.index.ByTag[tag], meta.Name)
	}
}

// matchesQuery verifica se uma skill corresponde à query de busca.
func (r *Registry) matchesQuery(meta Metadata, query string) bool {
	if strings.Contains(strings.ToLower(meta.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(meta.Description), query) {
		return true
	}
	for _, tag := range meta.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}
