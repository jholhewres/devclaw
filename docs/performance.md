# GoClaw — Performance and Optimizations

Documentation of the performance strategies implemented in GoClaw, including concurrency, caching, memory management, and tuning.

---

## Overview

GoClaw is designed for high throughput with low latency, leveraging Go's concurrency model (goroutines + channels) to the fullest. The optimizations cover:

1. **Adaptive message debounce** — reduced latency with smart drain.
2. **Unified send+stream** — single HTTP request eliminates round-trips.
3. **Asynchronous media enrichment** — non-blocking vision/transcription.
4. **Parallel tool execution** — semaphore for controlled concurrency.
5. **Concurrent subagents** — isolated goroutines for parallel work.
6. **Lazy prompt composition** — cached layers with background refresh.
7. **Progressive streaming** — partial delivery without waiting for full response.
8. **Context pruning** — proactive trimming prevents context bloat.
9. **Prompt caching** — cost and latency reduction on compatible providers.
10. **Incremental memory indexing** — delta sync for efficient re-indexing.
11. **Lane-based concurrency** — work-type isolation prevents contention.
12. **Config hot-reload** — zero downtime for configuration changes.

---

## 1. Adaptive Message Debounce (`message_queue.go`)

### Problem

Users send multiple messages in quick succession ("message burst"). Fixed debounce adds unnecessary delay for single messages.

### Solution

Adaptive debounce with three behaviors:

| Scenario | Debounce | Behavior |
|----------|----------|----------|
| Session idle | 0ms | Message drained immediately |
| Session busy (new message) | 200ms | Short debounce for batching |
| Session busy (followup) | 500ms | Longer debounce for follow-ups |

```yaml
queue:
  debounce_ms: 200       # Default (was 1000ms)
  max_pending: 20
```

### Impact

| Scenario | Before (v1.x) | After (v2.0) | Improvement |
|----------|---------------|--------------|-------------|
| Single message, idle session | 1000ms wait | 0ms | **1s faster** |
| Burst of 5 messages | 1000ms wait | 200ms wait | **800ms faster** |

---

## 2. Unified Send+Stream (`handlers.go`, `sse.ts`)

### Problem

The WebUI previously made two HTTP requests: (1) POST to send message, (2) GET to start SSE stream. This added a round-trip of latency.

### Solution

New `POST /api/chat/{sessionId}/stream` endpoint that accepts the message body and returns SSE stream in a single request.

```
Before:
  Client ──POST /api/chat/send──▶ Server (200 OK)
  Client ──GET /api/chat/stream──▶ Server (SSE events...)

After:
  Client ──POST /api/chat/{id}/stream──▶ Server (SSE events...)
```

### Impact

Eliminates one full HTTP round-trip (50-200ms depending on connection).

---

## 3. Asynchronous Media Enrichment (`assistant.go`)

### Problem

Vision (image description) and audio transcription blocked the agent from starting, adding 2-10s of delay.

### Solution

Two-phase enrichment:

1. **Fast phase** (synchronous): extracts text and metadata instantly.
2. **Async phase** (goroutine): runs vision/transcription in background; results injected via interrupt channel.

```
Message with image arrives
       │
       ▼
  enrichMessageContentFast()  →  Text extracted immediately
       │                              │
       ▼                              ▼
  Agent starts responding        enrichMediaAsync() (goroutine)
       │                              │
       ▼                              ▼
  ...processing...              Vision API returns description
       │                              │
       ▼                              ▼
  Agent receives interrupt  ◀─  Result injected via interruptCh
```

### Impact

Agent starts responding immediately instead of waiting for vision/transcription.

---

## 4. Parallel Tool Execution (`tool_executor.go`)

### Mechanism

When the LLM returns multiple tool calls, the ToolExecutor identifies which can run in parallel and which must be sequential.

```
LLM Tool Calls: [read_file, search_files, bash, web_search, glob_files]
                       │                       │
                       ▼                       ▼
            ┌─ Parallel ─────────┐    ┌─ Sequential ──┐
            │ read_file           │    │ bash           │
            │ search_files        │    └────────────────┘
            │ web_search          │
            │ glob_files          │
            └─────────────────────┘
```

### Sequential Tools (state-sharing)

These tools **always** run one at a time, as they share mutable state:

| Tool | Reason for Sequentiality |
|------|--------------------------|
| `bash` | Shared CWD and environment |
| `write_file` | May conflict with simultaneous writes |
| `edit_file` | Same file editing |
| `ssh` | Shared SSH session |
| `scp` | File transfer |
| `exec` | Process execution |
| `set_env` | Modifies global environment |

### Fast Abort

Tools check an abort channel during execution, allowing cancellation of long-running operations:

```go
select {
case <-te.AbortCh():
    return "aborted", nil
default:
    // continue execution
}
```

### Configuration

```yaml
security:
  tool_executor:
    parallel: true       # Enable parallel execution
    max_parallel: 5      # Maximum concurrent tools
```

### Expected Benchmarks

| Scenario | Sequential | Parallel (5) | Speedup |
|----------|-----------|--------------|---------|
| 5x `read_file` (local) | ~5ms | ~1ms | 5x |
| 3x `web_search` + 2x `read_file` | ~3.5s | ~1.2s | ~3x |
| 2x `bash` (forced sequential) | ~2s | ~2s | 1x |

---

## 5. Concurrent Subagents (`subagent.go`)

### Architecture

Each subagent runs in its own goroutine with an isolated context:

```
Main Agent
    ├── spawn_subagent("search docs")    ──▶ goroutine #1 (AgentRun)
    ├── spawn_subagent("analyze code")   ──▶ goroutine #2 (AgentRun)
    └── spawn_subagent("create tests")   ──▶ goroutine #3 (AgentRun)
         │
         ▼
    wait_subagent / poll status
```

### Concurrency Control

```yaml
subagents:
  max_concurrent: 4       # Global semaphore
  max_turns: 15            # Turns per subagent (vs 25 for main)
  timeout_seconds: 300     # 5 min per subagent
```

- **Semaphore**: limits concurrent subagents globally.
- **Cancellable context**: each subagent has `context.WithTimeout`.
- **No recursion**: subagents cannot spawn other subagents.
- **Tool filtering**: deny list removes spawning tools.

---

## 6. Lazy Prompt Composition (`prompt_layers.go`)

### Problem

Loading memory and skills layers involves disk I/O and embedding queries, blocking agent startup.

### Solution

Layer caching with background refresh:

```
Agent starts
    │
    ▼
  Critical layers loaded synchronously:
    - Bootstrap (SOUL.md, AGENTS.md)
    - History (conversation)
    │
    ▼
  Cached layers used immediately:
    - Memory (60s TTL cache)
    - Skills (60s TTL cache)
    │
    ▼
  Agent starts responding
    │
    ▼
  Background goroutine refreshes stale caches
  (ready for next prompt)
```

### Impact

| Phase | Before | After |
|-------|--------|-------|
| Memory layer | ~200-500ms (blocking) | ~0ms (cached) |
| Skills layer | ~100-300ms (blocking) | ~0ms (cached) |
| First token latency | Higher | Lower |

---

## 7. Block Streaming (`block_streamer.go`)

### Motivation

Long LLM responses can take 10-30s to complete. Without streaming, the user waits without feedback. The Block Streamer delivers progressive blocks.

### How It Works

```
LLM Response (tokens arriving)
    │
    ├──20 chars──▶ Block 1 sent to channel
    ├──idle 200ms──▶ Block 2 sent
    ├──3000 chars──▶ Block 3 (force flush)
    └──end──▶ Final block (or skip if already delivered)
```

### Configuration

```yaml
block_stream:
  enabled: false        # Disabled by default
  min_chars: 20         # Minimum chars before first block (was 80)
  idle_ms: 200          # Idle timeout before flush (was 1200)
  max_chars: 3000       # Force flush at this limit
```

### Smart Splitting

Blocks are split at natural boundaries (in order of preference):
1. Paragraphs (double line break)
2. Sentence endings (`.`, `!`, `?`)
3. List items (`- `, `* `, `1. `)
4. Character limit (force flush)

---

## 8. Context Pruning (`agent.go`)

### Problem

Long agent runs accumulate large tool results that consume context window space, eventually triggering overflow errors.

### Solution

Proactive pruning based on turn age:

| Trim Type | Turn Age | Action |
|-----------|----------|--------|
| Soft trim | Medium | Truncate result to summary |
| Hard trim | Old | Remove result entirely |

Pruning runs on every agent turn, preventing gradual context bloat without waiting for LLM overflow errors.

---

## 9. Prompt Caching

### Anthropic Prompt Caching

For Anthropic and Z.AI Anthropic proxy providers, GoClaw automatically adds `cache_control` to the system message and the second-to-last user message:

```json
{
  "cache_control": {"type": "ephemeral"}
}
```

### Impact

| Metric | Without Cache | With Cache | Savings |
|--------|--------------|------------|---------|
| Prompt token cost | 100% | ~10% | **90%** |
| Latency (TTFT) | ~2s | ~500ms | **75%** |

### Model Failover

On persistent API failure, the model failover manager classifies the error (billing, rate limit, auth, timeout, format) and rotates to fallback models with per-model cooldowns:

```yaml
fallback:
  models: [gpt-4o, glm-4.7-flash, claude-sonnet-4.5]
  max_retries: 2
  initial_backoff_ms: 1000
  max_backoff_ms: 30000
  retry_on_status_codes: [429, 500, 502, 503, 529]
```

---

## 10. Incremental Memory Indexing (`memory/sqlite_store.go`)

### Delta Sync

Instead of re-indexing all memory on every startup, the system performs delta sync:

1. Computes SHA-256 of each chunk of each `.md` file.
2. Compares with hashes stored in SQLite.
3. Only re-embeds chunks whose hash changed.
4. Removes chunks from deleted files.

### Impact

| Scenario | Full Index | Delta Sync | Savings |
|----------|-----------|------------|---------|
| 100 files, 2 changed | ~50s + API calls | ~1s + 2 API calls | **98%** |
| 500 chunks, 10 new | ~25 API calls | ~1 API call (batch) | **96%** |

### Hybrid Search

Search combines two strategies in parallel:

```
Query ──▶ ┌─── BM25 (FTS5) ──▶ keyword scores
          │
          └─── Cosine Similarity ──▶ vector scores
                        │
                        ▼
              Reciprocal Rank Fusion (RRF)
                        │
                        ▼
                 Results ranked
```

### Embedding Cache

Embeddings are cached in SQLite by chunk hash. If the text hasn't changed, the existing embedding is reused — zero unnecessary API calls.

---

## 11. Lane-Based Concurrency (`lanes.go`)

### Problem

Different work types (user messages, cron jobs, subagents) compete for the same resources, causing contention.

### Solution

Isolated lanes with per-type concurrency limits:

| Lane | Concurrency | Purpose |
|------|-------------|---------|
| `session` | 10 | User message processing |
| `cron` | 3 | Scheduled task execution |
| `subagent` | 4 | Child agent execution |

Each lane has its own queue and goroutine pool. Work submitted to a lane never blocks work in other lanes.

---

## 12. Config Hot-Reload (`config_watcher.go`)

### Mechanism

```
config.yaml changed
       │
       ▼
  mtime changed? ──No──▶ (skip)
       │
      Yes
       │
       ▼
  SHA-256 changed? ──No──▶ (skip, mtime glitch)
       │
      Yes
       │
       ▼
  Reload hot-reloadable fields
  (no process restart)
```

### Hot-Reloadable Fields

| Field | Restart Required |
|-------|-----------------|
| Access control (users, groups) | No |
| Instructions | No |
| Tool guard rules | No |
| Heartbeat config | No |
| Token budgets | No |
| Queue modes | No |
| LLM provider/model | **Yes** |
| Channel config | **Yes** |
| Gateway config | **Yes** |

---

## Tuning Guide

### High Throughput (Server)

```yaml
security:
  tool_executor:
    parallel: true
    max_parallel: 8        # More concurrent tools

subagents:
  max_concurrent: 6        # More subagents

queue:
  debounce_ms: 100         # Minimal debounce
  default_mode: steer      # Steer instead of queue
  max_pending: 50          # Larger queue

block_stream:
  enabled: true
  idle_ms: 150             # Faster flush

memory:
  embedding:
    batch_size: 50         # Larger embedding batches
```

### Low Cost (Personal Use)

```yaml
security:
  tool_executor:
    parallel: true
    max_parallel: 3

subagents:
  max_concurrent: 2
  model: "glm-4.7-flash"  # Cheap model for subagents

memory:
  compression_strategy: truncate   # No summarization cost
  session_memory:
    enabled: false

block_stream:
  enabled: false
```

### Low Latency (Real-time)

```yaml
agent:
  max_turns: 10            # Limit turns for fast response
  turn_timeout_seconds: 60

security:
  tool_executor:
    parallel: true
    max_parallel: 10

queue:
  debounce_ms: 50          # Near-instant drain
  default_mode: interrupt  # Cancel stale runs

block_stream:
  enabled: true
  min_chars: 20
  idle_ms: 150
```
