# DevClaw — Technical Architecture

Technical documentation of DevClaw's internal architecture, covering components, data flows, and design decisions.

## Overview

DevClaw is an AI agent for tech teams, written in Go. Single binary, zero runtime dependencies. Supports interactive CLI, WebUI, MCP server (for IDE integration), and messaging channels (WhatsApp, Discord, Telegram, Slack).

```
┌─────────────────────────────────────────────────────────┐
│                      cmd/devclaw                        │
│              CLI (Cobra) — setup, serve, chat            │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                   pkg/devclaw/copilot                     │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐              │
│  │ Assistant │──│  Agent   │──│ LLMClient │              │
│  │ (message  │  │ (loop +  │  │ (OpenAI-  │              │
│  │  flow)    │  │  tools)  │  │  compat)  │              │
│  └────┬─────┘  └────┬─────┘  └───────────┘              │
│       │              │                                    │
│  ┌────▼─────┐  ┌────▼──────────────┐                     │
│  │ Session  │  │  ToolExecutor     │                     │
│  │ Manager  │  │  ├─ SystemTools   │                     │
│  │ (per-chat)│  │  ├─ SkillTools   │                     │
│  │          │  │  ├─ PluginTools   │                     │
│  └──────────┘  │  ├─ ToolGuard    │                     │
│                │  └─ HookManager  │                     │
│                └───────────────────┘                     │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐              │
│  │  Vault   │  │  Memory  │  │ Subagent  │              │
│  │ (AES-256)│  │ (SQLite+ │  │  Manager  │              │
│  │          │  │  FTS5)   │  │           │              │
│  └──────────┘  └──────────┘  └───────────┘              │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐              │
│  │   Team   │  │   Team   │  │ Heartbeat │              │
│  │ Manager  │──│  Memory  │  │ Scheduler │              │
│  │(persist. │  │ (shared) │  │ (cron)    │              │
│  │ agents)  │  │          │  │           │              │
│  └──────────┘  └──────────┘  └───────────┘              │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐              │
│  │ EventBus │  │  Lanes   │  │  Browser  │              │
│  │ (pub/sub)│  │ (conc.)  │  │  Manager  │              │
│  │          │  │          │  │  (CDP)    │              │
│  └──────────┘  └──────────┘  └───────────┘              │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐              │
│  │ Canvas   │  │  Group   │  │ Failover  │              │
│  │  Host    │  │  Chat    │  │  Manager  │              │
│  └──────────┘  └──────────┘  └───────────┘              │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐              │
│  │ Daemon   │  │ Plugin   │  │   User    │              │
│  │ Manager  │  │ Manager  │  │  Manager  │              │
│  └──────────┘  └──────────┘  └───────────┘              │
└──────────────────────────────────────────────────────────┘
         │                          │                │
┌────────▼──────────┐    ┌─────────▼──────────┐  ┌▼────────────┐
│  pkg/devclaw/      │    │  pkg/devclaw/       │  │ pkg/devclaw/ │
│  channels/        │    │  gateway/           │  │ mcp/        │
│  ├─ whatsapp/     │    │  (HTTP API,         │  │ (MCP Server,│
│  ├─ discord/      │    │   WebSocket,        │  │  stdio+SSE) │
│  ├─ telegram/     │    │   OpenAI-compat)    │  └─────────────┘
│  └─ slack/        │    └────────────────────┘
└───────────────────┘
         │                          │
┌────────▼──────────┐    ┌─────────▼──────────┐
│  pkg/devclaw/      │    │  pkg/devclaw/       │
│  sandbox/         │    │  scheduler/         │
│  (namespaces,     │    │  (cron + advanced   │
│   Docker)         │    │   job features)     │
└───────────────────┘    └────────────────────┘
                                    │
                         ┌──────────▼──────────┐
                         │  pkg/devclaw/        │
                         │  webui/             │
                         │  (React SPA +       │
                         │   SSE streaming)    │
                         └─────────────────────┘
```

## Core Components

### 1. Assistant (`assistant.go`)

Entry point for message processing. Responsible for:

- **Message routing**: receives messages from channels, resolves session, dispatches to the agent loop.
- **Asynchronous media enrichment**: images/audio processing starts in background while the agent begins responding; results injected via interrupt channel.
- **Context compaction**: when the context exceeds the limit, applies one of three strategies (`summarize`, `truncate`, `sliding`). Runs in background goroutine.
- **Memory flush**: before compaction, triggers a pre-compaction memory flush turn to save durable memories.
- **Subagent dispatch**: creates child agents for parallel tasks.
- **Bounded followup queue**: FIFO eviction at 20 items when the agent is busy.
- **Hook integration**: dispatches lifecycle events via `HookManager`.

### 2. Agent Loop (`agent.go`)

Agentic loop that orchestrates LLM calls with tool execution:

```
LLM Call → tool_calls? → Execute Tools → Append Results → LLM Call (repeat)
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `max_turns` | 25 | Maximum LLM round-trips per execution |
| `turn_timeout_seconds` | 300 | Timeout per LLM call |
| `max_continuations` | 2 | Auto-continuations when the budget is exhausted |
| `reflection_enabled` | true | Periodic budget nudges (every 8 turns) |
| `max_compaction_attempts` | 3 | Retries after context overflow |

**Auto-continue flow**: when the agent exhausts its turn budget while still calling tools, it automatically starts a continuation (up to `max_continuations` times).

**Context overflow**: if the LLM returns `context_length_exceeded`, the agent compacts messages (keeps system + recent history), truncates tool results to 2000 chars, and retries.

**Context pruning**: proactively trims old tool results based on turn age (soft trim: truncate to summary, hard trim: remove entirely). Prevents context bloat before overflow errors occur.

**Agent steering**: the agent loop monitors the interrupt channel during tool execution. Incoming messages can steer behavior mid-run, allowing the user to redirect the agent without waiting for completion.

### 3. LLM Client (`llm.go`)

HTTP client for OpenAI-compatible APIs. Supports multiple providers:

| Provider | Base URL | Key |
|----------|----------|-----|
| OpenAI | `api.openai.com/v1` | `openai` |
| Z.AI (API) | `api.z.ai/api/paas/v4` | `zai` |
| Z.AI (Coding) | `api.z.ai/api/coding/paas/v4` | `zai-coding` |
| Z.AI (Anthropic) | `api.z.ai/api/anthropic` | `zai-anthropic` |
| Anthropic | `api.anthropic.com/v1` | `anthropic` |

**Features**:
- Streaming via SSE (`CompleteWithToolsStream`)
- Prompt caching for Anthropic (`cache_control: {"type": "ephemeral"}`)
- Fallback chain with exponential backoff
- Model failover with reason classification and per-model cooldowns
- Automatic provider detection from URL
- Per-model defaults (temperature, max tokens, tool support)

### 4. Prompt Composer (`prompt_layers.go`)

8-layer system prompt with priority-based token budget trimming and lazy caching:

| Layer | Priority | Content |
|-------|----------|---------|
| Core | 0 | Base identity, tooling guidance, proactive prompts |
| Safety | 5 | Guardrails, boundaries |
| Identity | 10 | Custom instructions |
| Thinking | 12 | Extended thinking hints |
| Bootstrap | 15 | SOUL.md, AGENTS.md, etc. |
| Business | 20 | Workspace context |
| Skills | 40 | Active skill instructions |
| Memory | 50 | Long-term facts |
| Temporal | 60 | Date/time/timezone |
| Conversation | 70 | History (sliding window) |
| Runtime | 80 | System info |

**Lazy caching**: Memory and Skills layers are cached with a 60s TTL and refreshed in background, ensuring agent starts aren't blocked by slow layer loading.

**Proactive prompts**: Core layer includes directives for reply tags, silent reply tokens, heartbeats, reasoning format, memory recall, subagent orchestration, and messaging.

**Trimming rules**:
- System prompt uses at most 40% of the total token budget.
- Layers with priority < 20 (Core, Safety, Identity, Thinking) are never trimmed.
- Layers with priority >= 50 can be dropped entirely if over budget.

### 5. Tool Executor (`tool_executor.go`)

Tool registry and dispatcher with parallel execution and fast abort:

- **Dynamic registration**: system, skill, and plugin tools are registered in the same registry.
- **Name sanitization**: invalid characters are replaced with `_` via regex.
- **Parallel execution**: independent tools run concurrently (configurable semaphore, default 5).
- **Sequential tools**: `bash`, `write_file`, `edit_file`, `ssh`, `scp`, `exec`, `set_env` always run sequentially.
- **Timeout**: 30s per tool execution (configurable).
- **Fast abort**: abort channel allows cancellation of running tools during execution.
- **Session context**: session ID propagated via `context.Value` for goroutine-safe isolation.

### 6. Session Manager (`session.go`, `session_persistence.go`)

Per-chat/group isolation with disk persistence:

```
data/sessions/
├── whatsapp_5511999999999/
│   ├── history.jsonl     # Conversation entries (JSONL)
│   ├── facts.json        # Extracted facts
│   └── meta.json         # Session metadata
```

- **Structured keys**: `SessionKey` (Channel, ChatID, Branch) for multi-agent routing.
- **Thread-safety**: `sync.RWMutex` per session.
- **File locks**: file-level locks for persistence.
- **CRUD**: Create, Get, Delete, Export, Rename operations.
- **Preventive compaction**: triggers at 80% of the threshold (not 100%).

### 7. Subagent System (`subagent.go`)

Child agents for parallel work:

```
Main Agent ──spawn_subagent──▶ SubagentManager ──goroutine──▶ Child AgentRun
                                    │                              │
                                    ▼                              ▼
                             SubagentRegistry           (isolated session,
                             tracks runs + results       filtered tools,
                                                         separate prompt)
```

- **No recursion**: subagents cannot spawn subagents.
- **Semaphore**: maximum 4 concurrent subagents (default).
- **Timeout**: 300s per subagent.
- **Filtered tools**: deny list removes spawning tools.

### 8. Event Bus (`events.go`)

In-memory pub/sub for agent lifecycle events:

- **Event types**: `delta`, `tool_use`, `done`, `error`, and custom events.
- **Multi-consumer**: supports multiple subscribers (UI, logs, other agents).
- **Buffered channels**: prevents slow consumers from blocking producers.

### 9. Lane System (`lanes.go`)

Lane-based concurrency for work-type isolation:

| Lane | Purpose | Default Concurrency |
|------|---------|-------------------|
| `session` | User message processing | 10 |
| `cron` | Scheduled task execution | 3 |
| `subagent` | Child agent execution | 4 |

Each lane has its own queue and concurrency limit, preventing contention between different work types.

### 10. Hook Manager (`hooks.go`)

Lifecycle hook system with 16+ event types:

- **Priority-ordered dispatch**: hooks execute in priority order.
- **Sync/async**: blocking hooks for pre-execution validation, non-blocking for logging/monitoring.
- **Panic recovery**: individual handler panics don't crash the dispatch.
- **Deduplication**: prevents duplicate event subscriptions.

### 11. Browser Manager (`browser_tool.go`)

Browser automation via Chrome DevTools Protocol:

- **Tools**: navigate, screenshot, content extraction, click, fill.
- **WebSocket**: connects to Chrome's DevTools WebSocket.
- **Session management**: maintains browser state across tool calls.

### 12. Canvas Host (`canvas_host.go`)

Interactive HTML/JS canvas with live-reload:

- **Temporary HTTP server**: each canvas runs on a free port.
- **WebSocket live-reload**: content updates trigger immediate browser refresh.
- **Concurrency-safe**: mutex-protected updates with race condition prevention.

### 13. Model Failover (`model_failover.go`)

Automatic LLM failover:

- **Reason classification**: billing, rate limit, auth, timeout, format.
- **Per-model cooldowns**: failed models are cooled down before retry.
- **Rotation**: cycles through configured fallback models.

## Channels and Gateway

### Channels (`channels/`)

Abstract interface that each channel implements:

- **WhatsApp** (`channels/whatsapp/`): native Go implementation via whatsmeow. Supports text, images, audio, video, documents, stickers, locations, contacts, reactions, typing indicators, read receipts.
- **Discord** (`channels/discord/`): via discordgo. Text, embeds, reactions.
- **Telegram** (`channels/telegram/`): via telebot. Text, images, audio, documents.
- **Slack** (`channels/slack/`): via slack-go. Text, threads, reactions.

### HTTP Gateway (`gateway/`)

OpenAI-compatible REST API with WebSocket support:

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/chat/completions` | Chat (supports SSE streaming) |
| GET | `/api/sessions` | List sessions |
| GET/DELETE | `/api/sessions/:id` | Specific session |
| GET | `/api/usage` | Usage statistics |
| GET | `/api/status` | System status |
| POST | `/api/webhooks` | Register webhook |
| POST | `/api/chat/{id}/stream` | Unified send+stream (SSE) |
| WS | `/ws` | WebSocket JSON-RPC (bidirectional) |
| GET | `/health` | Health check |

### WebUI (`webui/`)

React-based single page application with SSE streaming, session management, and real-time updates.

## Message Flow

```
1. Channel receives message (WhatsApp / Gateway / WebUI / CLI)
2. Channel Manager routes to Assistant
3. Message Queue applies adaptive debounce (200-500ms) and dedup (5s window)
4. Assistant resolves session (creates or reuses via structured SessionKey)
5. Queue mode determines handling (collect, steer, followup, interrupt)
6. Media enrichment starts asynchronously (vision/whisper if applicable)
7. Prompt Composer builds system prompt (8 layers + lazy caching + trimming)
8. Agent Loop starts:
   a. Calls LLM with context
   b. Context pruning trims old tool results if needed
   c. If tool_calls: ToolExecutor dispatches (parallel/sequential)
   d. HookManager fires PreToolUse/PostToolUse events
   e. ToolGuard validates permissions and blocks dangerous commands
   f. Agent checks interrupt channel for steering messages
   g. Results are appended to context
   h. Repeats until final response or max_turns
9. Response is formatted (WhatsApp markdown if needed)
10. Message Splitter divides into channel-compatible chunks
11. Block Streamer delivers progressively (if enabled)
12. EventBus publishes done/delta events
13. Session is persisted to disk
14. HookManager fires AgentStop event
```

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.24+ |
| CLI | Cobra + readline |
| Setup | charmbracelet/huh (TUI forms) |
| WhatsApp | whatsmeow (native Go) |
| Database | SQLite (go-sqlite3) with FTS5 |
| Encryption | AES-256-GCM + Argon2id (stdlib + x/crypto) |
| Scheduler | robfig/cron v3 |
| Config | YAML (gopkg.in/yaml.v3) |
| Keyring | go-keyring (GNOME/macOS/Windows) |
| QR Code | mdp/qrterminal |
| Sandbox | Linux namespaces (syscall) / Docker CLI |
| WebSocket | gorilla/websocket |
| Frontend | React + Vite + TypeScript |

## New Components (v2.x+)

### 14. MCP Server (`mcp/`)

Model Context Protocol server enabling IDE integration:

- **Transports**: stdio (standard) and SSE (HTTP-based)
- **JSON-RPC 2.0**: handles `initialize`, `tools/list`, `tools/call`, `resources/list`, `prompts/list`, `ping`
- **IDE Support**: Cursor, VSCode, Claude Code, Windsurf, Zed, Neovim — any MCP-compatible client
- **CLI**: `devclaw mcp serve` starts the stdio transport

### 15. Daemon Manager (`daemon_manager.go`)

Background process lifecycle management:

- **Start/stop/restart**: dev servers, watchers, databases, build tools
- **Ring buffer output**: last 1000 lines per daemon, tail-friendly
- **Health monitoring**: periodic health checks with auto-restart on failure
- **Tools**: `start_daemon`, `daemon_logs`, `daemon_list`, `daemon_stop`, `daemon_restart`

### 16. Plugin System (`plugin_system.go`)

Plugin architecture with built-in integrations:

- **Built-in plugins**: GitHub (issues, PRs, CI/CD), Jira (issues, boards), Sentry (events, error tracking)
- **Plugin Manager**: load, enable/disable, call plugin tools
- **Webhook Dispatcher**: event routing to configured endpoints
- **Tools**: `plugin_list`, `plugin_install`, `plugin_call`

### 17. User Manager (`multiuser.go`)

Multi-user support with RBAC:

- **Roles**: owner, admin, editor, viewer
- **Shared Memory**: team-wide persistent context
- **Tools**: `team_users`, `shared_memory`

### 18. Extended Tool Categories

| Category | File | Tools |
|----------|------|-------|
| Git | `git_tools.go` | status, diff, log, commit, branch, stash, blame |
| Docker | `docker_tools.go` | ps, logs, exec, images, compose, stop, rm |
| Database | `db_tools.go` | query, execute, schema, connections |
| Dev Utilities | `dev_utils.go` | json_format, jwt_decode, regex_test, base64, hash, uuid, url_parse, timestamp |
| System/Env | `env_tools.go` | env_info, port_scan, process_list |
| Codebase | `codebase_tools.go` | codebase_index, code_search, code_symbols, cursor_rules_generate |
| Testing | `testing_tools.go` | test_run, api_test, test_coverage |
| Operations | `ops_tools.go` | server_health, deploy_run, tunnel_manage, ssh_exec |
| Product | `product_tools.go` | sprint_report, dora_metrics, project_summary |
| IDE | `ide_extensions.go` | ide_configure |
| Teams | `team_tools.go` | team_create, team_list, team_create_agent, team_list_agents, team_stop_agent, team_delete_agent, team_create_task, team_list_tasks, team_update_task, team_assign_task, team_comment, team_check_mentions, team_send_message, team_save_fact, team_get_facts, team_delete_fact, team_standup |

### 19. Teams System (`team_manager.go`, `team_memory.go`, `team_tools.go`)

Persistent agents and shared team memory:

- **TeamManager**: lifecycle management for teams and persistent agents
- **TeamMemory**: shared state (tasks, messages, facts, activities)
- **Heartbeat integration**: periodic wake-ups via scheduler
- **@mentions**: inter-agent communication with mailbox delivery

Key differences from subagents:
- **Persistent agents**: long-lived, maintain state, managed via TeamManager
- **Subagents**: ephemeral, spawned for parallel work, discarded after completion

Both systems work together — persistent agents can spawn subagents for parallel tasks.
