// Package copilot – memory_tools.go implements individual memory tools.
// Each tool has a focused schema with only the parameters it needs,
// eliminating the ambiguity of the dispatcher pattern.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/kg"
	"github.com/jholhewres/devclaw/pkg/devclaw/copilot/memory"
)

// MemoryDispatcherConfig holds configuration for memory tools.
type MemoryDispatcherConfig struct {
	Store         *memory.FileStore
	SQLiteStore   *memory.SQLiteStore
	Config        MemoryConfig
	ContextRouter *ContextRouter // optional; nil disables wing routing
}

// RegisterMemoryTools registers individual memory tools.
// Replaces the old dispatcher pattern with focused tools:
// memory_save, memory_search, memory_list, memory_index.
func RegisterMemoryTools(executor *ToolExecutor, cfg MemoryDispatcherConfig) {
	store := cfg.Store
	sqliteStore := cfg.SQLiteStore
	memCfg := cfg.Config
	router := cfg.ContextRouter

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
					"wing": map[string]any{
						"type":        "string",
						"description": "Optional palace wing to file this memory under. If omitted, falls back to session context routing. Leave empty for legacy behavior.",
					},
				},
				"required": []string{"content"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleMemorySave(ctx, store, sqliteStore, memCfg, router, args)
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
					"wing": map[string]any{
						"type":        "string",
						"description": "Optional palace wing to bias the search toward. Files in this wing rank higher; files in other wings are demoted. Files with no wing (legacy) are unaffected. If omitted, falls back to session context routing; leave empty for legacy behavior.",
					},
				},
				"required": []string{"query"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleMemorySearch(ctx, store, sqliteStore, memCfg, router, args)
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
// After indexing, it resolves the palace wing from args["wing"] (explicit LLM
// override) or via the ContextRouter using the session's delivery target from
// ctx. If no wing can be determined, files.wing stays NULL (legacy behavior).
func handleMemorySave(ctx context.Context, store *memory.FileStore, sqliteStore *memory.SQLiteStore, cfg MemoryConfig, router *ContextRouter, args map[string]any) (any, error) {
	content, _ := args["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	// Block credentials/secrets from being saved in semantic memory.
	// These should be stored in the vault instead.
	if looksLikeCredential(content) {
		return nil, fmt.Errorf("this looks like a credential or password — use the vault tool instead of memory_save to store secrets securely")
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

	// ── Wing assignment (Sprint 2 Room 2.0b) ──
	//
	// Priority:
	//  1. Explicit wing from LLM args (user or agent override).
	//  2. Session-context routing via ContextRouter using the DeliveryTarget
	//     that the agent runtime injects into ctx for every real session.
	//  3. Leave files.wing as NULL (legacy first-class citizen per ADR-006).
	//
	// NormalizeWing validates, strips accents, rejects reserved prefixes, and
	// returns "" for any input that can't form a valid wing name — so an empty
	// result always means "no wing", and the early-return preserves NULL.
	if sqliteStore != nil && cfg.Hierarchy.Enabled {
		wingArg, _ := args["wing"].(string)
		wing := memory.NormalizeWing(wingArg)

		if wing == "" && router != nil {
			// No explicit wing from LLM — ask the router based on session context.
			dt := DeliveryTargetFromContext(ctx)
			res := router.Resolve(ctx, dt.Channel, dt.ChatID, content)
			wing = res.Wing // already normalized by router
		}

		if wing != "" {
			// FileStore.Save always writes to memory.MemoryFileName; IndexDirectory
			// keys it as the bare filename. Reference the const so a future rename
			// of the long-term facts file cannot silently break wing assignment.
			if err := sqliteStore.AssignWingToFile(ctx, memory.MemoryFileName, wing); err != nil {
				// Log but do NOT fail the save — wing is advisory, file is persisted.
				slog.Warn("failed to assign wing to file after save",
					"file_id", memory.MemoryFileName,
					"wing", wing,
					"error", err,
				)
			} else {
				IncSaveWingRouted()
			}
		}
	}

	return fmt.Sprintf("Saved to memory [%s]: %s", category, content), nil
}

// handleMemorySearch searches long-term memory.
//
// Sprint 2 Room 2.0c: when hierarchy is enabled, the query wing is resolved
// from args["wing"] (explicit LLM override) → ContextRouter via ctx delivery
// target → empty (legacy). The resolved wing is passed into
// HybridSearchWithOptsAndPostFilters as opts.QueryWing, which biases the
// fusion score so that wing-matching files rank higher and wing-mismatched
// files are demoted. Files with wing IS NULL stay neutral.
//
// When the hierarchy is disabled, QueryWing is left empty so the search
// takes the byte-identical legacy code path.
func handleMemorySearch(ctx context.Context, store *memory.FileStore, sqliteStore *memory.SQLiteStore, cfg MemoryConfig, router *ContextRouter, args map[string]any) (any, error) {
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

		// Resolve the query wing for fusion biasing. The priority mirrors
		// handleMemorySave's wing assignment logic so saves and searches
		// converge on the same wing for the same session context.
		var queryWing string
		if cfg.Hierarchy.Enabled {
			wingArg, _ := args["wing"].(string)
			queryWing = memory.NormalizeWing(wingArg)
			if queryWing == "" && router != nil {
				dt := DeliveryTargetFromContext(ctx)
				res := router.Resolve(ctx, dt.Channel, dt.ChatID, query)
				queryWing = res.Wing // already normalized by router
			}
		}

		opts := memory.HybridSearchOptions{
			MaxResults:       limit,
			MinScore:         cfg.Search.MinScore,
			VectorWeight:     cfg.Search.HybridWeightVector,
			BM25Weight:       cfg.Search.HybridWeightBM25,
			QueryWing:        queryWing,
			WingBoostMatch:   cfg.Hierarchy.WingBoostMatch,
			WingBoostPenalty: cfg.Hierarchy.WingBoostPenalty,
		}
		results, err := sqliteStore.HybridSearchWithOptsAndPostFilters(
			ctx, query, opts, decayCfg, mmrCfg,
		)
		if err == nil && len(results) > 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d memories (semantic search):\n\n", len(results)))
			for _, r := range results {
				text := r.Text
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				sb.WriteString(fmt.Sprintf("- [%s] (score: %.2f) %s\n", r.FileID, r.Score, redactCredentials(text)))
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
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", e.Category, redactCredentials(e.Content)))
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
			redactCredentials(e.Content)))
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

// ── Knowledge Graph Tools (Sprint 3 Room 3.6) ──

// RegisterKGTools registers the 6 knowledge-graph tools.
func RegisterKGTools(executor *ToolExecutor, sqliteStore *memory.SQLiteStore) {
	k := sqliteStore.KG()
	if k == nil {
		return
	}

	executor.RegisterHidden(
		MakeToolDefinition("kg_query",
			"Query the knowledge graph for facts about an entity. Returns triples where the entity is subject, object, or both.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entity": map[string]any{
						"type":        "string",
						"description": "Entity name to query",
					},
					"direction": map[string]any{
						"type":        "string",
						"description": "Direction: out (subject→), in (→object), or both. Default: out",
						"enum":        []string{"out", "in", "both"},
					},
					"wing": map[string]any{
						"type":        "string",
						"description": "Optional wing filter to narrow results",
					},
				},
				"required": []string{"entity"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleKGQuery(ctx, k, args)
		},
	)

	executor.RegisterHidden(
		MakeToolDefinition("kg_add",
			"Add a triple (subject, predicate, object) to the knowledge graph.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"subject": map[string]any{
						"type":        "string",
						"description": "Subject entity name",
					},
					"predicate": map[string]any{
						"type":        "string",
						"description": "Predicate name (e.g. works_at, likes, located_in)",
					},
					"object": map[string]any{
						"type":        "string",
						"description": "Object text or entity name",
					},
					"wing": map[string]any{
						"type":        "string",
						"description": "Optional palace wing for this triple",
					},
					"confidence": map[string]any{
						"type":        "number",
						"description": "Confidence score 0-1 (default: 0.5)",
					},
				},
				"required": []string{"subject", "predicate", "object"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleKGAdd(ctx, k, args)
		},
	)

	executor.RegisterHidden(
		MakeToolDefinition("kg_invalidate",
			"Invalidate (soft-delete) a triple by ID. Requires confirm=true to proceed.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"triple_id": map[string]any{
						"type":        "integer",
						"description": "ID of the triple to invalidate",
					},
					"confirm": map[string]any{
						"type":        "boolean",
						"description": "Set to true to confirm invalidation (default: false)",
					},
				},
				"required": []string{"triple_id"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleKGInvalidate(ctx, k, args)
		},
	)

	executor.RegisterHidden(
		MakeToolDefinition("kg_timeline",
			"Query the knowledge graph timeline for an entity within a date range.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entity": map[string]any{
						"type":        "string",
						"description": "Entity name",
					},
					"from": map[string]any{
						"type":        "string",
						"description": "Start date (RFC3339, optional)",
					},
					"until": map[string]any{
						"type":        "string",
						"description": "End date (RFC3339, optional)",
					},
					"direction": map[string]any{
						"type":        "string",
						"description": "Direction: out, in, or both (default: out)",
						"enum":        []string{"out", "in", "both"},
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Max results (default: 100, max: 100)",
					},
				},
				"required": []string{"entity"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleKGTimeline(ctx, k, args)
		},
	)

	executor.RegisterHidden(
		MakeToolDefinition("kg_stats",
			"Return counts of entities, predicates, active triples, and total triples in the knowledge graph.",
			map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleKGStats(ctx, k)
		},
	)

	executor.RegisterHidden(
		MakeToolDefinition("kg_merge_entities",
			"Merge source entity into target entity. All triples are reassigned and source becomes an alias. Requires confirm=true.",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source_entity": map[string]any{
						"type":        "string",
						"description": "Entity to merge (will be deleted)",
					},
					"target_entity": map[string]any{
						"type":        "string",
						"description": "Entity to merge into (kept)",
					},
					"confirm": map[string]any{
						"type":        "boolean",
						"description": "Set to true to confirm merge (default: false)",
					},
				},
				"required": []string{"source_entity", "target_entity"},
			}),
		func(ctx context.Context, args map[string]any) (any, error) {
			return handleKGMergeEntities(ctx, k, args)
		},
	)
}

func parseDirection(dir string) kg.Direction {
	switch strings.ToLower(dir) {
	case "in":
		return kg.In
	case "both":
		return kg.Both
	default:
		return kg.Out
	}
}

func handleKGQuery(ctx context.Context, k *kg.KG, args map[string]any) (any, error) {
	entity, _ := args["entity"].(string)
	if entity == "" {
		return nil, fmt.Errorf("entity is required")
	}

	dirStr, _ := args["direction"].(string)
	dir := parseDirection(dirStr)

	wing := memory.NormalizeWing("")
	if w, ok := args["wing"].(string); ok {
		wing = memory.NormalizeWing(w)
	}

	triples, err := k.QueryEntity(ctx, entity, dir)
	if err != nil {
		return nil, fmt.Errorf("kg query: %w", err)
	}

	if len(triples) == 0 {
		return fmt.Sprintf("No triples found for entity %q.", entity), nil
	}

	filtered := triples
	if wing != "" {
		var kept []kg.Triple
		for _, tr := range triples {
			if tr.Wing == wing {
				kept = append(kept, tr)
			}
		}
		filtered = kept
	}

	var sb strings.Builder
	for _, tr := range filtered {
		obj := redactCredentials(tr.ObjectText)
		if tr.ObjectName != "" {
			obj = redactCredentials(tr.ObjectName)
		}
		sb.WriteString(fmt.Sprintf("%s (%s) %s [confidence=%.1f", tr.SubjectName, tr.PredicateName, obj, tr.Confidence))
		if tr.Wing != "" {
			sb.WriteString(fmt.Sprintf(", wing=%s", tr.Wing))
		}
		sb.WriteString("]\n")
	}
	return sb.String(), nil
}

func handleKGAdd(ctx context.Context, k *kg.KG, args map[string]any) (any, error) {
	subject, _ := args["subject"].(string)
	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	predicate, _ := args["predicate"].(string)
	if predicate == "" {
		return nil, fmt.Errorf("predicate is required")
	}
	object, _ := args["object"].(string)
	if object == "" {
		return nil, fmt.Errorf("object is required")
	}

	// Block credentials from being stored in the knowledge graph.
	if looksLikeCredential(object) {
		return nil, fmt.Errorf("object looks like a credential — use the vault tool instead of kg_add to store secrets securely")
	}

	confidence := 0.5
	if c, ok := args["confidence"].(float64); ok && c > 0 {
		confidence = c
	}

	wing := ""
	if w, ok := args["wing"].(string); ok {
		wing = memory.NormalizeWing(w)
	}

	tid, err := k.AddTriple(ctx, subject, predicate, object, kg.TripleOpts{
		Confidence: confidence,
		Wing:       wing,
	})
	if err != nil {
		return nil, fmt.Errorf("kg add: %w", err)
	}

	return fmt.Sprintf("Added triple #%d: %s (%s) %s [confidence=%.1f]", tid, subject, predicate, object, confidence), nil
}

func handleKGInvalidate(ctx context.Context, k *kg.KG, args map[string]any) (any, error) {
	tidFloat, ok := args["triple_id"].(float64)
	if !ok {
		return nil, fmt.Errorf("triple_id is required and must be an integer")
	}
	tid := int64(tidFloat)

	confirm, _ := args["confirm"].(bool)
	if !confirm {
		return fmt.Sprintf("This will invalidate triple #%d. Set confirm=true to proceed.", tid), nil
	}

	if err := k.InvalidateTriple(ctx, tid); err != nil {
		return nil, fmt.Errorf("kg invalidate: %w", err)
	}

	return fmt.Sprintf("Triple #%d invalidated.", tid), nil
}

func handleKGTimeline(ctx context.Context, k *kg.KG, args map[string]any) (any, error) {
	entity, _ := args["entity"].(string)
	if entity == "" {
		return nil, fmt.Errorf("entity is required")
	}

	dirStr, _ := args["direction"].(string)
	dir := parseDirection(dirStr)

	from, _ := args["from"].(string)
	if from == "" {
		from = "0001-01-01T00:00:00Z"
	}
	until, _ := args["until"].(string)
	if until == "" {
		until = "9999-12-31T23:59:59Z"
	}

	maxCap := 100
	limit := 100
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	requestedLimit := limit
	if limit > maxCap {
		limit = maxCap
	}

	triples, err := k.Timeline(ctx, kg.TimelineOpts{
		Subject:   entity,
		From:      from,
		Until:     until,
		Direction: dir,
		Limit:     limit + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("kg timeline: %w", err)
	}

	hasMore := false
	if len(triples) > limit {
		hasMore = true
		triples = triples[:limit]
	}

	var sb strings.Builder
	for _, tr := range triples {
		obj := redactCredentials(tr.ObjectText)
		if tr.ObjectName != "" {
			obj = redactCredentials(tr.ObjectName)
		}
		sb.WriteString(fmt.Sprintf("[%s] %s (%s) %s [confidence=%.1f",
			tr.ValidFrom, tr.SubjectName, tr.PredicateName, obj, tr.Confidence))
		if tr.Wing != "" {
			sb.WriteString(fmt.Sprintf(", wing=%s", tr.Wing))
		}
		if tr.ValidUntil != "" {
			sb.WriteString(fmt.Sprintf(", until=%s", tr.ValidUntil))
		}
		sb.WriteString("]\n")
	}

	if hasMore {
		sb.WriteString(fmt.Sprintf("has_more: true (showing %d of %d requested)\n", limit, requestedLimit))
	}

	return sb.String(), nil
}

func handleKGStats(ctx context.Context, k *kg.KG) (any, error) {
	db := k.DB()

	var entities, predicates, activeTriples, totalTriples int

	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM kg_entities").Scan(&entities); err != nil {
		return nil, fmt.Errorf("kg stats entities: %w", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM kg_predicates").Scan(&predicates); err != nil {
		return nil, fmt.Errorf("kg stats predicates: %w", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM kg_triples WHERE valid_until IS NULL AND txn_until IS NULL").Scan(&activeTriples); err != nil {
		return nil, fmt.Errorf("kg stats active triples: %w", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM kg_triples").Scan(&totalTriples); err != nil {
		return nil, fmt.Errorf("kg stats total triples: %w", err)
	}

	return fmt.Sprintf("KG Stats: entities=%d, predicates=%d, active_triples=%d, total_triples=%d",
		entities, predicates, activeTriples, totalTriples), nil
}

func handleKGMergeEntities(ctx context.Context, k *kg.KG, args map[string]any) (any, error) {
	source, _ := args["source_entity"].(string)
	if source == "" {
		return nil, fmt.Errorf("source_entity is required")
	}
	target, _ := args["target_entity"].(string)
	if target == "" {
		return nil, fmt.Errorf("target_entity is required")
	}

	confirm, _ := args["confirm"].(bool)
	if !confirm {
		return fmt.Sprintf("This will merge %q into %q. All triples will be reassigned. Set confirm=true to proceed.", source, target), nil
	}

	sourceID, err := k.EnsureEntity(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("resolve source entity: %w", err)
	}
	targetID, err := k.EnsureEntity(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("resolve target entity: %w", err)
	}
	if sourceID == targetID {
		return fmt.Sprintf("Source and target are the same entity (id=%d). Nothing to merge.", sourceID), nil
	}

	db := k.DB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("merge begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		"UPDATE kg_triples SET subject_entity_id = ? WHERE subject_entity_id = ?",
		targetID, sourceID,
	); err != nil {
		return nil, fmt.Errorf("merge reassign subject: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE kg_triples SET object_entity_id = ? WHERE object_entity_id = ?",
		targetID, sourceID,
	); err != nil {
		return nil, fmt.Errorf("merge reassign object: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM kg_entity_aliases WHERE entity_id = ?",
		sourceID,
	); err != nil {
		return nil, fmt.Errorf("merge delete source aliases: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT OR IGNORE INTO kg_entity_aliases (entity_id, alias_name) VALUES (?, ?)",
		targetID, strings.TrimSpace(strings.ToLower(source)),
	); err != nil {
		return nil, fmt.Errorf("merge register alias: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM kg_entities WHERE entity_id = ?",
		sourceID,
	); err != nil {
		return nil, fmt.Errorf("merge delete source entity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("merge commit: %w", err)
	}

	return fmt.Sprintf("Merged %q into %q. Source entity deleted and registered as alias.", source, target), nil
}

// ---------- Credential Detection ----------

// credentialPatterns detects content that looks like passwords, API keys,
// tokens, or other secrets that should be stored in the vault instead of
// semantic memory.
var credentialPatterns = []string{
	`(?i)senha[:\s]+\S+`,
	`(?i)password[:\s]+\S+`,
	`(?i)api[_-]?key[:\s]+\S+`,
	`(?i)secret[_-]?key[:\s]+\S+`,
	`(?i)access[_-]?token[:\s]+\S+`,
	`(?i)bearer\s+[a-zA-Z0-9\-_.]+`,
	`(?i)token[:\s]+[a-zA-Z0-9\-_.]{20,}`,
	`(?i)(ssh|pgp|gpg)[_-]?(key|private)[:\s]`,
	`(?i)private[_-]?key[:\s]`,
	`-----BEGIN\s+(RSA|EC|OPENSSH|PGP)\s+PRIVATE\s+KEY-----`,
	`(?i)(aws|gcp|azure)[_-]?(secret|key|token)[:\s]+\S+`,
	`ghp_[a-zA-Z0-9]{36}`,          // GitHub PAT
	`sk-[a-zA-Z0-9]{32,}`,          // OpenAI API key
	`AIza[a-zA-Z0-9\-_]{35}`,       // Google API key
	`xox[bpas]-[a-zA-Z0-9\-]{10,}`, // Slack token
}

var compiledCredentialPatterns []*regexp.Regexp

func init() {
	for _, p := range credentialPatterns {
		compiledCredentialPatterns = append(compiledCredentialPatterns, regexp.MustCompile(p))
	}
}

// looksLikeCredential checks whether content contains patterns that
// indicate passwords, API keys, tokens, or other secrets.
func looksLikeCredential(content string) bool {
	for _, re := range compiledCredentialPatterns {
		if re.MatchString(content) {
			return true
		}
	}
	return false
}

// LooksLikeCredential is the exported version for use by the security package.
func LooksLikeCredential(content string) bool {
	return looksLikeCredential(content)
}

// redactCredentials replaces detected credential patterns with a redaction marker.
// Keeps the label (e.g. "senha") but replaces the value with [REDACTED — use vault].
func redactCredentials(content string) string {
	for _, re := range compiledCredentialPatterns {
		content = re.ReplaceAllStringFunc(content, func(match string) string {
			if idx := strings.IndexByte(match, ':'); idx >= 0 {
				return match[:idx] + ": [REDACTED — use vault]"
			}
			return "[REDACTED — use vault]"
		})
	}
	return content
}

// RedactCredentials is the exported version for use by the security package.
func RedactCredentials(content string) string {
	return redactCredentials(content)
}
