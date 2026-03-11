// Package copilot – context_engine.go defines the pluggable ContextEngine
// interface. A ContextEngine provides additional context layers that are
// injected into the system prompt alongside the core prompt composition.
//
// The LegacyContextEngine wraps the existing PromptComposer behavior,
// making it possible to swap in alternative engines (RAG, vector search,
// code indexing, etc.) without modifying the prompt pipeline.
package copilot

import (
	"context"
	"sync"
)

// ContextEngine produces extra context to inject into the system prompt.
// Implementations can source context from any backend: memory stores,
// vector databases, code indices, knowledge graphs, etc.
type ContextEngine interface {
	// Name returns a unique identifier for this engine.
	Name() string

	// Gather returns context text to inject into the prompt for a given
	// session and user input. Returns "" if no relevant context is found.
	// The maxTokens hint tells the engine its approximate token budget.
	Gather(ctx context.Context, session *Session, input string, maxTokens int) string
}

// ContextEngineRegistry manages multiple context engines and merges their
// output during prompt composition.
type ContextEngineRegistry struct {
	mu      sync.RWMutex
	engines []ContextEngine
}

// NewContextEngineRegistry creates a new registry.
func NewContextEngineRegistry() *ContextEngineRegistry {
	return &ContextEngineRegistry{}
}

// Register adds a context engine to the registry.
func (r *ContextEngineRegistry) Register(engine ContextEngine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engines = append(r.engines, engine)
}

// GatherAll calls Gather on every registered engine and returns the
// concatenated results, separated by newlines. Empty results are skipped.
func (r *ContextEngineRegistry) GatherAll(ctx context.Context, session *Session, input string, maxTokensPerEngine int) string {
	r.mu.RLock()
	engines := make([]ContextEngine, len(r.engines))
	copy(engines, r.engines)
	r.mu.RUnlock()

	if len(engines) == 0 {
		return ""
	}

	type result struct {
		name    string
		content string
	}
	results := make([]result, len(engines))

	var wg sync.WaitGroup
	for i, engine := range engines {
		wg.Add(1)
		go func(idx int, e ContextEngine) {
			defer wg.Done()
			defer func() {
				if rv := recover(); rv != nil {
					results[idx] = result{name: e.Name()}
				}
			}()
			content := e.Gather(ctx, session, input, maxTokensPerEngine)
			results[idx] = result{name: e.Name(), content: content}
		}(i, engine)
	}
	wg.Wait()

	var combined string
	for _, r := range results {
		if r.content == "" {
			continue
		}
		if combined != "" {
			combined += "\n\n"
		}
		combined += r.content
	}
	return combined
}

// Engines returns the names of all registered engines.
func (r *ContextEngineRegistry) Engines() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, len(r.engines))
	for i, e := range r.engines {
		names[i] = e.Name()
	}
	return names
}

// LegacyContextEngine is a no-op engine that serves as a reference
// implementation. The existing PromptComposer already builds project context,
// memory, and skills layers directly. Register custom ContextEngine
// implementations to inject additional context from external sources
// (RAG pipelines, code indices, knowledge graphs, etc.).
type LegacyContextEngine struct{}

// NewLegacyContextEngine creates the default no-op context engine.
func NewLegacyContextEngine(_ *PromptComposer) *LegacyContextEngine {
	return &LegacyContextEngine{}
}

// Name returns the engine identifier.
func (e *LegacyContextEngine) Name() string { return "legacy" }

// Gather is a no-op — all legacy context is already built by PromptComposer.
func (e *LegacyContextEngine) Gather(_ context.Context, _ *Session, _ string, _ int) string {
	return ""
}
