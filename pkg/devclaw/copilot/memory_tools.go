// Package copilot – memory_tools.go implements individual memory tools.
// Each tool has a focused schema with only the parameters it needs,
// eliminating the ambiguity of the dispatcher pattern.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// MemoryDispatcherConfig holds configuration for memory tools.
type MemoryDispatcherConfig struct {
	Store       *memory.FileStore
	SQLiteStore *memory.SQLiteStore
	Config      MemoryConfig
}

// RegisterMemoryTools registers individual memory tools.
// Replaces the old dispatcher pattern with focused tools:
// memory_save, memory_search, memory_list, memory_index.
func RegisterMemoryTools(executor *ToolExecutor, cfg MemoryDispatcherConfig) {
	store := cfg.Store
	sqliteStore := cfg.SQLiteStore
	memCfg := cfg.Config

	// ── memory_save ──
	executor.RegisterHidden(
		MakeToolDefinition("memory_save",
			"Save a fact, preference, event, or summary to long-term memory. "+
				"Use this to remember important information from conversations.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":        "string",
						"description": "The content to remember (fact, preference, event, or summary)",
					},
					"category": map[string]any{
						"type":        "string",
						"description": "Category: fact, preference, event, or summary",
						"enum":        []string{"fact", "preference", "event", "summary"},
					},
				},
				"required": []string{"content"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleMemorySave(ctx, store, sqliteStore, memCfg, args)
		},
	)

	// ── memory_search ──
	searchDesc := "Search long-term memory for relevant facts, preferences, or past events. " +
		"Use this before answering questions about prior work, decisions, dates, people, or preferences."
	if sqliteStore != nil {
		searchDesc += " Supports semantic search (vector + keyword hybrid)."
	}
	executor.RegisterHidden(
		MakeToolDefinition("memory_search", searchDesc,
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query describing what to find in memory",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum results to return (default: 10, max: 100)",
					},
				},
				"required": []string{"query"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleMemorySearch(ctx, store, sqliteStore, memCfg, args)
		},
	)

	// ── memory_list ──
	executor.RegisterHidden(
		MakeToolDefinition("memory_list",
			"List recent entries from long-term memory, ordered by date.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum entries to return (default: 20, max: 100)",
					},
				},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleMemoryList(ctx, store, args)
		},
	)

	// ── memory_index ──
	executor.RegisterHidden(
		MakeToolDefinition("memory_index",
			"Rebuild the memory search index. Use this after manually editing memory files.",
			map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		func(ctx context.Context, _ map[string]any) (any, error) {
			return handleMemoryIndex(ctx, sqliteStore, memCfg)
		},
	)
}

// handleMemorySave saves content to long-term memory.
func handleMemorySave(_ context.Context, store *memory.FileStore, sqliteStore *memory.SQLiteStore, cfg MemoryConfig, args map[string]any) (any, error) {
	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	validCategories := map[string]bool{"fact": true, "preference": true, "event": true, "summary": true}
	category, _ := args["category"].(string)
	if category == "" {
		category = "fact"
	} else if !validCategories[category] {
		return nil, fmt.Errorf("invalid category: %s (valid: fact, preference, event, summary)", category)
	}

	err := store.Save(memory.Entry{
		Content:   content,
		Source:    "agent",
		Category:  category,
		Timestamp: time.Now(),
	})
	if err != nil {
		return nil, err
	}

	// Re-index the MEMORY.md file if SQLite memory is available.
	// Run synchronously so searches immediately after save find the new entry.
	if sqliteStore != nil && cfg.Index.Auto {
		memDir := filepath.Join(filepath.Dir(cfg.Path), "memory")
		chunkCfg := memory.ChunkConfig{MaxTokens: cfg.Index.ChunkMaxTokens, Overlap: 100}
		if chunkCfg.MaxTokens <= 0 {
			chunkCfg.MaxTokens = 500
		}
		indexCtx, indexCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer indexCancel()
		if err := sqliteStore.IndexMemoryDir(indexCtx, memDir, chunkCfg); err != nil {
			slog.Warn("memory index update after save failed", "error", err)
		}
	}

	return fmt.Sprintf("Saved to memory [%s]: %s", category, content), nil
}

// handleMemorySearch searches long-term memory.
func handleMemorySearch(ctx context.Context, store *memory.FileStore, sqliteStore *memory.SQLiteStore, cfg MemoryConfig, args map[string]any) (any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	maxLimit := 100
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > maxLimit {
			limit = maxLimit
		}
	}

	// Try hybrid search first if SQLite is available.
	if sqliteStore != nil {
		decayCfg := memory.TemporalDecayConfig{
			Enabled:      cfg.Search.TemporalDecay.Enabled,
			HalfLifeDays: cfg.Search.TemporalDecay.HalfLifeDays,
		}
		mmrCfg := memory.MMRConfig{
			Enabled: cfg.Search.MMR.Enabled,
			Lambda:  cfg.Search.MMR.Lambda,
		}

		results, err := sqliteStore.HybridSearchWithOptions(
			ctx, query, limit, cfg.Search.MinScore,
			cfg.Search.HybridWeightVector, cfg.Search.HybridWeightBM25,
			decayCfg, mmrCfg,
		)
		if err == nil && len(results) > 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d memories (semantic search):\n\n", len(results)))
			for _, r := range results {
				text := r.Text
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				sb.WriteString(fmt.Sprintf("- [%s] (score: %.2f) %s\n", r.FileID, r.Score, text))
			}
			return sb.String(), nil
		}
	}

	// Fallback to substring search.
	entries, err := store.Search(query, limit)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return "No memories found matching the query.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories:\n\n", len(entries)))
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", e.Category, e.Content))
	}
	return sb.String(), nil
}

// handleMemoryList lists recent memories.
func handleMemoryList(_ context.Context, store *memory.FileStore, args map[string]any) (any, error) {
	maxLimit := 100
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > maxLimit {
			limit = maxLimit
		}
	}

	entries, err := store.GetRecent(limit)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return "No memories stored yet.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Recent memories (%d):\n\n", len(entries)))
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("- [%s] [%s] %s\n",
			e.Timestamp.Format("2006-01-02"),
			e.Category,
			e.Content))
	}
	return sb.String(), nil
}

// handleMemoryIndex manually triggers re-indexing.
func handleMemoryIndex(ctx context.Context, sqliteStore *memory.SQLiteStore, cfg MemoryConfig) (any, error) {
	if sqliteStore == nil {
		return "Memory indexing not available (SQLite store not configured).", nil
	}

	memDir := filepath.Join(filepath.Dir(cfg.Path), "memory")
	chunkCfg := memory.ChunkConfig{MaxTokens: cfg.Index.ChunkMaxTokens, Overlap: 100}
	if chunkCfg.MaxTokens <= 0 {
		chunkCfg.MaxTokens = 500
	}

	if err := sqliteStore.IndexMemoryDir(ctx, memDir, chunkCfg); err != nil {
		return nil, fmt.Errorf("indexing failed: %w", err)
	}

	return fmt.Sprintf("Memory index updated: %d files, %d chunks.",
		sqliteStore.FileCount(), sqliteStore.ChunkCount()), nil
}
