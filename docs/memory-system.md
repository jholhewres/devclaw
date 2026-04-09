# Memory System

DevClaw's memory system provides long-term storage and retrieval of information across conversations. It uses a hybrid search approach combining vector embeddings and BM25 full-text search.

---

## Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Memory Tool (Dispatcher)                 │
│  memory(action="save|search|list|index", ...)               │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        ▼                  ▼                  ▼
┌───────────────┐  ┌───────────────┐  ┌───────────────┐
│  File Store   │  │ SQLite Store  │  │ Hybrid Search │
│  (Markdown)   │  │ (FTS5+Vec)    │  │ (Vec + BM25)  │
└───────────────┘  └───────────────┘  └───────────────┘
```

---

## Storage Backends

### 1. File Store (`memory/`)

Markdown files for human-readable storage:

```
data/memory/
├── fact/
│   ├── 2024-01-15-api-key.md
│   └── 2024-01-16-preferences.md
├── preference/
│   └── 2024-01-10-language.md
├── event/
│   └── 2024-01-20-deployment.md
└── summary/
    └── 2024-01-25-sprint-review.md
```

**Format:**
```markdown
---
category: fact
created_at: 2024-01-15T10:30:00Z
keywords: [api, authentication, stripe]
---

The Stripe API key is stored in the vault under `stripe_api_key`.
```

### 2. SQLite Store

Database for fast querying with FTS5 and vector search:

```sql
CREATE TABLE memories (
    id TEXT PRIMARY KEY,
    category TEXT NOT NULL,
    content TEXT NOT NULL,
    keywords TEXT,  -- JSON array
    embedding BLOB, -- Float32 vector
    created_at DATETIME,
    metadata TEXT   -- JSON object
);

-- Full-text search
CREATE VIRTUAL TABLE memories_fts USING fts5(
    content,
    keywords,
    content='memories',
    content_rowid='rowid'
);
```

---

## Memory Tool (Dispatcher)

### Actions

| Action | Description | Parameters |
|--------|-------------|------------|
| `save` | Save new memory | content, category, keywords, metadata |
| `search` | Search memories | query, category, limit |
| `list` | List memories | category, limit, offset |
| `index` | Rebuild search index | - |

### Usage Examples

```bash
# Save a fact
memory(
  action="save",
  content="The API uses JWT tokens with 24h expiration",
  category="fact",
  keywords=["api", "jwt", "authentication"]
)

# Search memories
memory(
  action="search",
  query="authentication token",
  limit=10
)

# List by category
memory(
  action="list",
  category="preference",
  limit=20
)

# Rebuild index
memory(action="index")
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | Yes | `save`, `search`, `list`, or `index` |
| `content` | string | For save | Memory content to store |
| `category` | string | For save/list | `fact`, `preference`, `event`, `summary` |
| `keywords` | []string | No | Tags for categorization |
| `metadata` | object | No | Additional structured data |
| `query` | string | For search | Search query text |
| `limit` | int | No | Max results (default: 20, max: 100) |
| `offset` | int | For list | Pagination offset |

---

## Categories

| Category | Description | Example |
|----------|-------------|---------|
| `fact` | Objective information | "Database runs on port 5432" |
| `preference` | User preferences | "User prefers dark theme" |
| `event` | Timestamped events | "Deployed v2.1 on 2024-01-15" |
| `summary` | Summarized knowledge | "Sprint review conclusions" |

---

## Hybrid Search

Memory uses a hybrid search combining two approaches:

### 1. Vector Search (Semantic)

- **Embedding**: Text converted to vector via configured embedding model
- **Similarity**: Cosine similarity between query and memory vectors
- **Use case**: Finding conceptually similar content

```
Query: "how to authenticate"
Matches: "API uses JWT tokens" (semantic similarity)
```

### 2. BM25 Search (Lexical)

- **Engine**: SQLite FTS5
- **Algorithm**: Okapi BM25
- **Use case**: Exact keyword matching

```
Query: "JWT expiration"
Matches: "JWT tokens with 24h expiration" (keyword match)
```

### 3. Score Fusion

Results from both methods are combined using reciprocal rank fusion:

```
final_score = (k / (k + rank_vector)) + (k / (k + rank_bm25))
```

Where `k = 60` (standard RRF constant).

---

## Prompt Layer Integration

Memory is integrated into the prompt via **LayerMemory** (priority 50).

### Layer Content

```go
func (p *PromptComposer) buildMemoryLayer() string {
    // Hybrid search for relevant memories
    memories := p.memoryStore.HybridSearch(input, 20)

    // Format for prompt
    var sb strings.Builder
    sb.WriteString("## Relevant Memories\n\n")
    for _, m := range memories {
        sb.WriteString(fmt.Sprintf("- [%s] %s\n", m.Category, m.Content))
    }
    return sb.String()
}
```

### Token Budget

```go
LayerMemory: config.TokenBudget.Memory, // Default: 2000 tokens
```

If memories exceed the budget, they are truncated with an indicator:

```
## Relevant Memories

- [fact] The API uses JWT tokens with 24h expiration
- [preference] User prefers dark theme
- [event] Deployed v2.1 on 2024-01-15
...
[truncated - 8 more memories available]
```

---

## Caching

Memory layer uses session-level caching:

| Setting | Value |
|---------|-------|
| TTL | 30 seconds |
| Strategy | Lazy refresh |
| Storage | In-memory map |

### Cache Flow

1. Check cache for fresh result (< 30s old)
2. If fresh: return cached memories
3. If stale: return cached, trigger background refresh
4. Next prompt: use refreshed results

This prevents memory search from blocking agent startup.

---

## Memory Lifecycle

### Automatic Save (Pre-Compaction)

Before context compaction, DevClaw flushes durable memories:

```go
// In assistant.go
if shouldCompact {
    // Trigger pre-compaction memory flush
    p.flushMemoryTurn(ctx, session)
    // Then compact
    p.compactContext(session)
}
```

The agent is given a special turn to save important information before it's lost to compaction.

### Manual Save

Users or agents can explicitly save memories:

```bash
memory(action="save", content="...", category="fact")
```

### Expiration

Memories don't expire automatically, but can be pruned:

```bash
# List old memories
memory(action="list", category="event", limit=100)

# Delete specific memory (manual curation)
# (Future: memory action="delete")
```

---

## Configuration

### YAML Config

```yaml
memory:
  enabled: true
  storage_dir: "data/memory"
  database_path: "data/memory.db"
  embedding_model: "text-embedding-3-small"
  max_results: 20
  cache_ttl: 30s

token_budget:
  memory: 2000
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DEVCLAW_MEMORY_DIR` | `data/memory` | Memory storage directory |
| `DEVCLAW_MEMORY_DB` | `data/memory.db` | SQLite database path |

---

## Builtin Skill

Memory has a builtin skill embedded in the binary:

**Location**: `pkg/devclaw/copilot/builtin/skills/memory/SKILL.md`

**Trigger**: `automatic` (always included in prompt)

**Purpose**: Provides usage guidance for the memory tool

```yaml
---
name: memory
description: "Long-term memory management"
trigger: automatic
---
```

---

## Implementation Files

| File | Description |
|------|-------------|
| `memory_tools.go` | Dispatcher tool implementation |
| `memory/store.go` | File store implementation |
| `memory/sqlite.go` | SQLite store with FTS5 |
| `memory/hybrid.go` | Hybrid search implementation |
| `prompt_layers.go` | LayerMemory integration |
| `builtin/skills/memory/SKILL.md` | Usage documentation |

---

## Best Practices

### For Agents

1. **Save proactively** - Don't wait for user to ask
2. **Use keywords** - Add relevant tags for better retrieval
3. **Choose right category** - Facts vs preferences vs events
4. **Keep concise** - Memory budget is limited
5. **Search before save** - Avoid duplicates

### For Configuration

1. **Tune embedding model** - Balance quality vs cost
2. **Adjust cache TTL** - Higher for stable content, lower for dynamic
3. **Monitor token usage** - Increase budget if memories are truncated

### For Searches

1. **Use natural language** - Vector search handles synonyms
2. **Include keywords** - BM25 benefits from exact terms
3. **Filter by category** - Narrow search space when possible

---

## Troubleshooting

### Memories Not Found

1. Check if memory is enabled: `memory(action="list")`
2. Rebuild index: `memory(action="index")`
3. Check storage directory exists
4. Verify database path in config

### Slow Searches

1. Check database size: `SELECT COUNT(*) FROM memories`
2. Rebuild FTS index: `memory(action="index")`
3. Reduce `max_results` in config

### Token Budget Exceeded

1. Increase `token_budget.memory` in config
2. Use more specific category filters
3. Reduce `max_results` in search calls

---

## Summary

| Aspect | Details |
|--------|---------|
| Storage | Markdown files + SQLite with FTS5 |
| Search | Hybrid (vector + BM25) |
| Tool | `memory(action="...")` dispatcher |
| Categories | fact, preference, event, summary |
| Prompt Layer | LayerMemory (priority 50) |
| Token Budget | ~2000 tokens |
| Cache TTL | 30 seconds |

---

## Layered Memory Stack (Sprint 2, v1.19.0+)

### What it is

Starting with v1.19.0, the memory prompt is assembled by a **MemoryStack** that
composes four layers in priority order:

```
L0  Identity      — user-curated persona anchor (~/.devclaw/identity.md)
L1  Essential     — per-wing narrative summary, cached 6h in SQLite
L2  On-demand     — per-turn entity-driven retrieval from the active wing
L3  Legacy        — the v1.18.0 hybrid-search block (always present)
```

L0–L2 form a prefix that is prepended to the L3 legacy output.
When all three Sprint 2 layers render empty, the output is **byte-identical to
v1.18.0** — the retrocompat gate enforced by the golden fixture test.

### Default behavior

The stack is **on by default** when `memory.hierarchy.enabled: true` (which is
also the default since v1.18.0). No config changes are required on upgrade.

### Layer priority for budget enforcement

| Priority | Layer | Behavior |
|----------|-------|----------|
| 1 (never trimmed) | L0 Identity | Always included in full, even if it exceeds the total budget. A WARN is logged when L0 alone is over-budget. |
| 2 (trimmed second) | L1 Essential | Truncated at a word boundary when L0 + L1 would exceed the budget. |
| 3 (trimmed first) | L2 On-demand | Trimmed to zero before L1 is touched. Ephemeral per-turn context. |

Default combined budget: 3600 bytes (~900 tokens). Controlled by the internal
`defaultStackBudget` constant in `memory_stack.go`.

### How to opt out

Set the following in your `devclaw.yaml` to bypass the layered stack entirely
and fall back to v1.18.0 prompt composition:

```yaml
memory:
  stack:
    force_legacy: true
```

No migration, no restart beyond a config reload. The `StackConfig.ForceLegacy`
flag short-circuits the `Build()` call to an empty string, causing
`buildMemoryLayer` to produce byte-identical output to v1.18.0.

### Identity file

Your L0 identity content lives at `~/.devclaw/identity.md`. Edit it with:

```
devclaw identity edit
```

This opens `$EDITOR` on the file, creating a default template on first run.
The file is hot-reloaded via `fsnotify` — changes take effect on the next turn
without restarting the daemon.

**Example identity.md snippet:**

```markdown
# Identity

I am a software engineer who prefers concise answers and working code over
long explanations. I value correctness over speed.
```

### How L1 caches essential stories

The L1 EssentialLayer renders a deterministic Markdown summary of the most
recently touched rooms and the lead sentences of their top files within the
active wing. The result is cached in the `essential_stories` SQLite table.

- **Cache key:** wing name (normalized to lowercase, accents stripped)
- **TTL:** 6 hours (configurable via `memory.hierarchy.essential_story_stale_after`)
- **Invalidation:** automatic on the next Render call after TTL expires; can be
  forced manually via the `EssentialLayer.Invalidate` API or by the dream cycle
- **Zero LLM calls** — template-only, fully deterministic

### How L2 detects entities

The L2 OnDemandLayer runs a per-turn regex tokenization over the user's message
to extract candidate entities (names, room keywords, topics). These are then
matched against the `rooms` and `wings` tables in a direct SQL lookup — no LLM
call, no embedding inference.

- **Latency contract:** p95 < 10ms warm
- **Result cap:** configurable via `memory.hierarchy.on_demand_max_results` (default: 5)
- **Wing-scoped by default:** queries are biased toward the active wing

### Cross-wing fallback

When a search against the active wing returns no results, the OnDemandLayer
includes **one cross-wing result** as a fallback. This preserves relevance for
queries that naturally span wings (e.g. a question about a shared contact).
The fallback is controlled by `memory.hierarchy.on_demand_cross_wing` (default: `true`).

### Example YAML (all defaults shown)

```yaml
memory:
  hierarchy:
    enabled: true                     # ON by default since v1.18.0
    l1_budget_tokens: 400
    l2_budget_tokens: 300
    on_demand_max_results: 5
    on_demand_cross_wing: true
    essential_story_stale_after: 6h
    essential_story_rooms_per_wing: 4
    wing_boost_match: 1.3
    wing_boost_penalty: 0.4

  # Escape hatch — uncomment to revert to v1.18.0 behavior:
  # stack:
  #   force_legacy: true
```

### Retrocompat guarantee

When all three Sprint 2 layers (L0, L1, L2) render empty strings — which
happens when `hierarchy.enabled: false`, or when the identity file is absent
and no wing is active and no entities are detected — the `MemoryStack.Build()`
method returns `""`. In that case `buildMemoryLayer` produces output that is
**byte-identical to v1.18.0**. This invariant is enforced by the golden fixture
test in `prompt_layers_golden_test.go`.

Existing v1.18.0 databases require no migration. The `essential_stories` table
is created with `CREATE TABLE IF NOT EXISTS` on first use. Users with
`memory.hierarchy.enabled: false` see zero behavior change.
