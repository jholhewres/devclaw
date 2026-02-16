# GoClaw — Features and Capabilities

Detailed documentation of all features available in GoClaw.

---

## Autonomous Agent Loop

The core of GoClaw is an agentic loop that allows the assistant to execute complex tasks autonomously, iterating between LLM calls and tool execution.

### Execution Cycle

```
User Message
       │
       ▼
  ┌─────────┐     ┌─────────────┐     ┌──────────┐
  │  LLM    │────▶│ Tool Calls? │────▶│ Execute  │
  │  Call   │     │  (parse)    │ Yes │ Tools    │
  └─────────┘     └──────┬──────┘     └────┬─────┘
       ▲                 │ No               │
       │                 ▼                  │
       │          Final Response            │
       └────────────────────────────────────┘
                  (append results)
```

### Auto-Continue

When the agent reaches the turn limit but is still executing tools, it automatically starts a new execution cycle. This enables long tasks without manual intervention.

- **Maximum continuations**: 2 (configurable)
- **Trigger**: last turn contains tool_calls (agent still working)
- **Context**: preserved between continuations

### Reflection (Self-Awareness)

Every 8 turns, the system injects a `[System: N turns used of M]` message, allowing the agent to consciously manage its budget — prioritizing remaining tasks or summarizing progress.

### Context Pruning

Old tool results are proactively trimmed based on turn age:

| Trim Type | Turn Age | Action |
|-----------|----------|--------|
| Soft trim | Medium | Truncate result to summary |
| Hard trim | Old | Remove result entirely |

This prevents context bloat without waiting for LLM overflow errors.

### Agent Steering

During tool execution, the agent monitors an interrupt channel for incoming messages. Users can redirect the agent mid-run, and the agent adjusts its behavior accordingly.

### Context Compaction

Three strategies to keep the context within limits:

| Strategy | Method | LLM Cost | Speed |
|----------|--------|----------|-------|
| `summarize` | Memory flush → LLM summarization → keep 25% recent | High | Slow |
| `truncate` | Drop oldest entries, keep 50% recent | Zero | Instant |
| `sliding` | Fixed window of most recent entries | Zero | Instant |

**Memory flush pre-compaction**: before compaction, the agent performs a dedicated turn to save durable memories to disk using an append-only strategy, ensuring important context survives.

**Preventive compaction**: triggers automatically at 80% of the `max_messages` threshold, avoiding overflow during conversation.

---

## Tool System

### Built-in Tools (45+)

#### Filesystem

| Tool | Description | Permission |
|------|-------------|------------|
| `read_file` | Read any file. Supports line ranges (offset + limit) | user |
| `write_file` | Create or overwrite files. Auto-creates parent directories | owner |
| `edit_file` | Precise line-based edits (search & replace) | owner |
| `list_files` | List directory contents with metadata (size, permissions, type) | user |
| `search_files` | Regex search across files (ripgrep-style). Supports include/exclude | user |
| `glob_files` | Find files by recursive glob pattern | user |

#### Shell and SSH

| Tool | Description | Permission |
|------|-------------|------------|
| `bash` | Execute shell commands. Persistent CWD and env across calls | owner |
| `set_env` | Set persistent environment variables for bash | owner |
| `ssh` | Execute commands on remote machines via SSH | owner |
| `scp` | Copy files to/from remote machines | admin |

#### Web

| Tool | Description | Permission |
|------|-------------|------------|
| `web_search` | DuckDuckGo search (HTML parsing). SSRF-protected | user |
| `web_fetch` | Fetch URL content with SSRF validation. Returns clean content | user |

#### Memory

| Tool | Description | Permission |
|------|-------------|------------|
| `memory_save` | Save facts to long-term memory. Triggers re-index | user |
| `memory_search` | Hybrid semantic + keyword search (BM25 + cosine) | user |
| `memory_list` | List recent memory entries | user |
| `memory_index` | Manually re-index all memory files | admin |

#### Scheduler

| Tool | Description | Permission |
|------|-------------|------------|
| `schedule_add` | Add cron task (cron expression + command/message) | admin |
| `schedule_list` | List scheduled tasks | user |
| `schedule_remove` | Remove a scheduled task | admin |

#### Media

| Tool | Description | Permission |
|------|-------------|------------|
| `describe_image` | Describe image content via LLM vision API | user |
| `transcribe_audio` | Audio transcription via Whisper API | user |

#### Subagents

| Tool | Description | Permission |
|------|-------------|------------|
| `spawn_subagent` | Create child agent for parallel work | admin |
| `list_subagents` | List active subagents and their status | admin |
| `wait_subagent` | Wait for subagent completion | admin |
| `stop_subagent` | Terminate a running subagent | admin |

#### Browser Automation

| Tool | Description | Permission |
|------|-------------|------------|
| `browser_navigate` | Navigate browser to URL via CDP | admin |
| `browser_screenshot` | Take screenshot of current page | admin |
| `browser_content` | Extract page text content | admin |
| `browser_click` | Click element by CSS selector | admin |
| `browser_fill` | Fill form input field | admin |

#### Interactive Canvas

| Tool | Description | Permission |
|------|-------------|------------|
| `canvas_create` | Create HTML/JS canvas with live-reload | admin |
| `canvas_update` | Update canvas content (triggers live-reload) | admin |
| `canvas_list` | List active canvases | user |
| `canvas_stop` | Stop a canvas server | admin |

#### Session Management

| Tool | Description | Permission |
|------|-------------|------------|
| `sessions_list` | List active sessions across all workspaces | admin |
| `sessions_send` | Send message to another session (inter-agent) | admin |
| `sessions_delete` | Delete a session by ID | admin |
| `sessions_export` | Export session history and metadata as JSON | admin |

#### Skills Management

| Tool | Description | Permission |
|------|-------------|------------|
| `init_skill` | Create new skill scaffold (SKILL.md + structure) | admin |
| `edit_skill` | Modify files of an existing skill | admin |
| `add_script` | Add script to a skill | admin |
| `list_skills` | List installed skills | user |
| `test_skill` | Test skill execution | admin |
| `install_skill` | Install from ClawHub, GitHub, URL, or local path | admin |
| `search_skills` | Search the ClawHub catalog | user |
| `remove_skill` | Uninstall a skill | admin |

---

## Lifecycle Hooks

16+ lifecycle events that external code can subscribe to for observing and modifying agent behavior.

### Events

| Event | Phase | Blocking |
|-------|-------|----------|
| `SessionStart` | Session creation | No |
| `SessionEnd` | Session destruction | No |
| `PreToolUse` | Before tool execution | Yes (can block) |
| `PostToolUse` | After tool execution | No |
| `AgentStart` | Agent loop begins | No |
| `AgentStop` | Agent loop ends | No |
| `PreCompact` | Before session compaction | No |
| `PostCompact` | After session compaction | No |
| `MessageReceived` | New message arrives | No |
| `MessageSent` | Response sent to channel | No |

### Features

- **Priority-ordered dispatch**: hooks execute in priority order.
- **Sync/async modes**: blocking hooks for pre-execution validation, non-blocking for logging/monitoring.
- **Panic recovery**: individual handler panics don't crash the dispatch or goroutine.
- **Deduplication**: prevents duplicate event subscriptions.

---

## Queue Modes

Configurable strategies for handling incoming messages when the agent is busy:

| Mode | Behavior |
|------|----------|
| `collect` | Queue messages, process as batch when agent is free |
| `steer` | Inject new message into running agent's context |
| `followup` | Queue as followup to current run |
| `interrupt` | Cancel current run, process new message |
| `steer-backlog` | Steer if possible, queue remainder |

```yaml
queue:
  default_mode: collect
  by_channel:
    whatsapp: collect
    webui: steer
  drop_policy: oldest
```

---

## Memory System

### File-Based Memory

- **MEMORY.md**: long-term facts curated by the agent.
- **Daily notes** (`memory/YYYY-MM-DD.md`): daily logs.
- **Session facts** (`facts.json`): per-session extracted facts.

### Advanced Memory (SQLite + Vectors)

SQLite store with FTS5 (keyword) and vector search (embeddings):

1. **Indexing**: `.md` files in the memory directory are chunked by heading/paragraph/sentence.
2. **Embeddings**: chunks embedded via `text-embedding-3-small` (1536 dims), cached in SQLite.
3. **Delta Sync**: only re-indexes/re-embeds chunks whose SHA-256 hash changed.
4. **Hybrid search**: combines BM25 (FTS5) and cosine similarity via Reciprocal Rank Fusion (RRF).

### Memory Security

Memory content injected into prompts is treated as untrusted data — HTML entities are escaped, dangerous tags stripped, content wrapped in `<relevant-memories>` tags, and injection patterns detected.

### Session Memory

When enabled, the `/new` command summarizes the conversation via LLM before clearing history. The summary is saved to `memory/YYYY-MM-DD-slug.md` and indexed for future recall.

---

## Skill System

Skills extend the agent's capabilities with custom tools and behaviors.

### Supported Formats

| Format | Structure | Execution |
|--------|-----------|-----------|
| **Native Go** | `skill.yaml` + `skill.go` | Compiled into binary |
| **ClawHub** | `SKILL.md` + `scripts/` | Python, Node.js, Shell — sandboxed |

### Installation Sources

| Source | Example |
|--------|---------|
| ClawHub | `copilot skill install brave-search` |
| GitHub | `copilot skill install github.com/user/repo` |
| URL | `copilot skill install https://example.com/skill.tar.gz` |
| Local | `copilot skill install ./my-local-skill` |
| Chat | Ask the agent to create one |

### Creation via Chat

The agent can create skills interactively:
1. `init_skill` creates the scaffold (SKILL.md + directory structure).
2. `edit_skill` and `add_script` allow modifying/adding code.
3. `test_skill` validates execution.

---

## Communication Channels

### WhatsApp

Native Go implementation via [whatsmeow](https://go.mau.fi/whatsmeow):

- **Messages**: text, images, audio, video, documents, stickers, voice notes, locations, contacts.
- **Interactions**: reactions, reply/quoting, typing indicators, read receipts.
- **Groups**: full group message support with access control.
- **Device name**: "GoClaw". LID resolution for phone number normalization.

### Group Chat (Enhanced)

| Feature | Description |
|---------|-------------|
| Activation modes | Always respond, mention-only, or keyword-based |
| Intro messages | Automatic welcome message for new groups |
| Context injection | Inject group context into agent prompts |
| Participant tracking | Track active participants |
| Quiet hours | Suppress responses during configured hours |
| Ignore patterns | Skip messages matching regex patterns |

### Media Processing

| Type | Processing | API |
|------|-----------|-----|
| Image | Automatic description via LLM vision | OpenAI Vision |
| Audio | Automatic transcription | Whisper |
| Video | Thumbnail extraction → vision | OpenAI Vision |

Media enrichment runs **asynchronously** — the agent starts responding immediately while vision/transcription happens in background.

### Block Streaming

Progressive delivery of long responses:

```yaml
block_stream:
  enabled: false
  min_chars: 20
  idle_ms: 200
  max_chars: 3000
```

---

## HTTP Gateway

OpenAI-compatible REST API with WebSocket support.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/v1/chat/completions` | Chat completions (SSE streaming) |
| GET | `/api/sessions` | List all sessions |
| GET | `/api/sessions/:id` | Session details |
| DELETE | `/api/sessions/:id` | Delete session |
| GET | `/api/usage` | Global token statistics |
| GET | `/api/usage/:session` | Per-session usage |
| GET | `/api/status` | System status |
| POST | `/api/webhooks` | Register webhook |
| POST | `/api/chat/{id}/stream` | Unified send+stream (SSE) |
| WS | `/ws` | WebSocket JSON-RPC (bidirectional) |

### WebSocket JSON-RPC

Bidirectional communication supporting:
- **Client requests**: `chat.send`, `chat.abort`, `session.list`
- **Server events**: `delta`, `tool_use`, `done`, `error`

### Authentication

```yaml
gateway:
  enabled: true
  address: ":8080"
  auth_token: "your-secret-token"
  cors_origins: ["http://localhost:3000"]
```

---

## Scheduler (Advanced Cron)

Task scheduling system with advanced job features:

| Feature | Description |
|---------|-------------|
| Cron expressions | Standard 5-field or predefined (`@hourly`, `@daily`) |
| Isolated sessions | Each job runs in its own session |
| Announce | Broadcast results to target channels |
| Subagent spawn | Run job as a subagent |
| Per-job timeouts | Custom timeout per task |
| Labels | Categorize and filter jobs |
| Persistence | Jobs survive restarts |

---

## Remote Access (Tailscale)

Secure remote access via Tailscale Serve/Funnel:

```yaml
tailscale:
  enabled: true
  serve_port: 8080
  funnel: false     # expose to internet
```

No manual port forwarding or DNS configuration required.

---

## Heartbeat

Proactive agent behavior at configurable intervals:

```yaml
heartbeat:
  enabled: true
  interval: 30m
  active_start: 9
  active_end: 22
  channel: whatsapp
  chat_id: "5511999999999"
```

The agent reads `HEARTBEAT.md` for pending tasks and acts on them. Replies with `HEARTBEAT_OK` if nothing needs attention.

---

## Workspaces

Multi-tenant isolation with independent configurations per workspace. Each workspace has: independent system prompt, skills, model, language, and conversation memory.

---

## Session Management

### Structured Session Keys

Sessions are identified by `SessionKey` (Channel, ChatID, Branch), enabling multi-agent routing and session branching.

### CRUD Operations

| Operation | Description |
|-----------|-------------|
| Create | Automatic on first message |
| Get | By session ID or structured key |
| Delete | Via tool or API |
| Export | Full history + metadata as JSON |
| Rename | Change session display name |

### Bounded Followup Queue

When the agent is busy, incoming messages are queued with FIFO eviction at 20 items maximum.

---

## Token Usage Tracking

Per-session and global tracking of consumed tokens. Accessible via `/usage` command or `GET /api/usage`.

---

## Config Hot-Reload

`ConfigWatcher` monitors `config.yaml` for changes. Hot-reloadable: access control, instructions, tool guard, heartbeat, token budgets, queue modes. No restart required.

---

## CLI

### Main Commands

| Command | Description |
|---------|-------------|
| `copilot setup` | Interactive wizard (TUI) |
| `copilot serve` | Start daemon |
| `copilot chat [msg]` | Interactive REPL or single message |
| `copilot config init/show/validate` | Config management |
| `copilot config vault-*` | Vault management |
| `copilot skill list/search/install` | Skills management |
| `copilot schedule list/add` | Cron management |
| `copilot health` | Health check |
| `copilot changelog` | Version changelog |

### Chat Commands (via messaging or CLI REPL)

| Command | Description |
|---------|-------------|
| `/help` | List all commands |
| `/allow`, `/block`, `/admin` | Access management |
| `/users` | List authorized users |
| `/model [name]` | Show/change model |
| `/usage [global\|reset]` | Token statistics |
| `/compact` | Manually compact session |
| `/think [off\|low\|medium\|high]` | Extended thinking level |
| `/verbose [on\|off]` | Toggle verbose output |
| `/reasoning [level]` | Set reasoning format (alias for /think) |
| `/queue [mode]` | Show/change queue mode |
| `/activation [mode]` | Show/change group activation mode |
| `/new` | Clear history (with summarization if enabled) |
| `/reset` | Full session reset |
| `/stop` | Cancel active execution |
| `/approve`, `/deny` | Approve/reject tool execution |
| `/ws create/assign/list` | Workspace management |

---

## Deployment

### Docker

```bash
docker compose up -d
docker compose logs -f copilot
```

### systemd

```bash
sudo cp copilot.service /etc/systemd/system/
sudo systemctl enable --now copilot
```

### Binary

```bash
make build && ./bin/copilot serve
```
