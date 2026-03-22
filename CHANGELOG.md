# Changelog

All notable changes to DevClaw are documented in this file.

## [1.16.0] — 2026-03-22

### Channels — Multi-Instance & QR Lifecycle

- **Multi-instance channel creation**: `CreateChannelInstanceFn` now calls `RegisterAndConnect()` instead of `Register()` for all 4 channel types (WhatsApp, Telegram, Discord, Slack), ensuring new instances are fully connected on creation
- **Instance-aware QR closures**: Moved `GetWhatsAppStatusByInstanceFn`, `SubscribeWhatsAppQRByInstanceFn`, `RequestWhatsAppQRByInstanceFn`, and `DisconnectWhatsAppByInstanceFn` outside the default-instance guard so they work for dynamically created instances
- **SSE named events for instances**: `channel_instance_handlers.go` now uses `writeSSE(w, flusher, evt.Type, evt)` with named events (`code`, `success`, `timeout`, `error`) matching the frontend's `addEventListener` expectations
- **WhatsApp QR lifecycle overhaul**: Removed auto-QR generation on `Connect()` (now sets `StateWaitingQR` and waits for explicit request); added `qrGuard atomic.Bool` to prevent concurrent QR generation; `RequestNewQR` goroutine uses instance context (`w.ctx`) instead of process context to stop on disconnect; `Disconnect()` notifies QR observers with `close` event before cancelling context
- **Instance lock safety**: All instance-aware closures in `serve.go` now acquire `instanceMu.RLock()` before reading `waInstances` map; error event emitted for unknown instance IDs instead of silent closed channel
- **Generic instance status**: `handleInstanceStatus` now handles non-WhatsApp channels (Telegram, Discord, Slack) via `ListChannelInstancesFn` fallback, fixing 501 errors
- **Instance handler routing**: Added 405 Method Not Allowed response and GET guard for status endpoint

### Concurrency & Stability

- **Workspace Resolve() race fix**: Changed from single `Lock()` to double-check locking pattern — `RLock` for read path, upgrade to `Lock` only when creating new session store
- **Multiuser update_role race fix**: Single locked section for user lookup + role mutation instead of separate `GetUser()` + lock
- **Context propagation**: `RequestNewQR` and default-instance QR closures use server context (`ctx`) instead of `context.Background()`, ensuring goroutines stop on shutdown
- **NeedsQR field**: `ListChannelInstancesFn` now populates `NeedsQR` via type assertion for WhatsApp instances

### Backend Cleanup

- **Removed team system**: Deleted `team_manager.go`, `team_memory.go`, `team_tools.go`, `team_types.go`, `notification_dispatcher.go` and their tests; removed `builtin/skills/teams/SKILL.md`; removed `"Team"` category from `tool_profiles.go` and corresponding test cases
- **Tool groups sync**: Removed `"group:teams"` from `GetToolGroups()`; added `"send_media"` to media group; added `"group:browser"` and `"group:skill_db"`
- **Path resolution**: Replaced 5 hardcoded paths in `serve.go` with `paths.Resolve*()` calls (skills dir, TLS dir, hub_config.json)
- **Permissions fix**: Removed `ResolveWorkspaceTemplatesDir()` from `EnsureStateDirs` (was 0755, now only created by `EnsureWorkspaceTemplates` with 0700)
- **CLI help text**: Updated `--channel` flag to list all 4 supported channels
- **Stale comment fix**: Updated "teams" reference to "agents" in `prompt_layers.go`
- **Test adjustments**: `builtin_skills_test.go` threshold updated from `< 2` to `< 10`; removed 3 `team_*` test cases from `tool_profiles_test.go`

### Workspace Templates

- **Enriched templates**: `configs/templates/` files (SOUL.md, IDENTITY.md, TOOLS.md, MEMORY.md) upgraded from skeleton placeholders to substantive content matching `configs/bootstrap/` quality — includes vault guide, boundaries, continuity instructions, and security notes

### Frontend

- **WhatsApp QR on-demand**: Removed auto `requestQR()` from useEffect; QR generation only triggers on explicit user action; proper error handling on `requestQR().catch()`
- **Telegram instance-aware**: `TelegramConnect.tsx` branches on `instanceId` for status/disconnect API calls using instance endpoints; fixed `phone_number` → `phone` type mismatch
- **CSS fixes**: Fixed 5 missing-space class concatenation bugs across `WhatsAppConnect.tsx` and `TelegramConnect.tsx` (`border-secondarypy-4` → `border-secondary py-4`, etc.)
- **Toggle knob size**: Fixed non-standard `h-4.5 w-4.5` to `h-5 w-5` in `Toggle.tsx`
- **Agents page i18n**: Full internationalization of `Agents.tsx` with `agentsPage` section in `en.json` and `pt.json`

### TLS / HTTPS

- **Self-signed TLS support**: New `pkg/devclaw/tls` package generates ECDSA P-256 certificates (10yr validity, SHA-256, 0600 permissions) using Go's `crypto/x509` stdlib — no OpenSSL dependency required
- **WebUI + Gateway HTTPS**: Both servers support `tls.enabled: true` in config with conditional `ListenAndServeTLS`, enforcing TLS 1.2 minimum
- **CLI `devclaw tls`**: New `generate` and `info` subcommands for certificate management (fingerprint, expiry)
- **Auto-generation on startup**: When `auto_generate: true` (default), certificates are generated automatically on first `devclaw serve` with SHA-256 fingerprint logged
- **Install script TLS**: Three-tier fallback (devclaw binary → openssl → skip), `--no-tls` flag to skip

### Deploy / CI

- **macOS Apple Silicon fix**: Removed Darwin from GoReleaser (was using `CGO_ENABLED=0` breaking SQLite FTS5); added native `release-macos` job in GitHub Actions with `CGO_ENABLED=1` for arm64 + amd64
- **CI macOS matrix**: Tests now run on both `ubuntu-latest` and `macos-latest`
- **Docker multi-arch**: Removed hardcoded `--platform=linux/arm64` from runtime stage in Dockerfile
- **Install script GitHub mode**: New `--github` flag downloads from GitHub Releases (GoReleaser tar.gz archives)

### Channels

- **Telegram & WhatsApp message tracking**: Added `SentMessageTracker` interface for accurate reply detection; message deduplication in WhatsApp during reconnections; Telegram message editing via `EditMessageID`; structured error types for Telegram API; latency metrics in health reporting
- **Channel token management**: Vault-based token storage for Telegram, secure environment variable injection, and configuration status in health reports
- **Discord & Slack removal**: Simplified messaging channels to WhatsApp and Telegram; removed Discord and Slack integrations from serve command, setup wizard, and UI
- **WhatsApp access control fix**: Setup wizard now mirrors global access config into `channels.whatsapp.access`; `serve.go` inherits global → channel config as fallback
- **Telegram management UI**: Complete management stack with Connection and Settings tabs, hot-reload on token changes, connect/disconnect flows, full i18n (PT/EN/ES)

### Core — Agent & Copilot

- **Subagent yield fix**: `collectPendingSubagentResults` now also collects completed/failed subagent results from the last 60 seconds, closing a race window where results were missed between the yield trigger and collection call
- **Compaction & context engine**: Content-aware compaction guard (require real conversation); session truncation after compaction; persona/language continuity in summaries; stabilized trim ordering with anti-loop detection; preemptive context overflow guard at 85% high-water mark; duplicate tool call ID deduplication
- **Failover hardening**: Cause-chain traversal via recursive `errors.Unwrap()` for accurate error classification; symbolic error code map for Google Cloud, AWS, and Anthropic providers
- **LCM enhancements**: Depth-aware summarization prompts (leaf/session/arc/durable); three-level escalation (normal → aggressive → deterministic fallback); large file interception (>25k tokens) with disk storage; heartbeat pruning; session reconciliation on bootstrap; LLM-bounded `expand_query` action; local timezone formatting; configurable summary model/provider
- **Subagent improvements**: Orphan recovery with resume (max 3 retries, exponential backoff); partial progress synthesis on timeout (max 4000 chars); delivery scope (`All`/`Parent`/`External`) for completion announcements; tool-spawned subagents default to parent-only delivery

### Skills, Memory & Gateway

- **Skills compact fallback**: When skills exceed the token budget, degrades to compact format (name + description + location) via binary search instead of hard truncation
- **Pluggable memory sections**: `MemoryPromptSectionBuilder` interface allows plugins to inject custom sections into the memory prompt layer, with panic protection per builder
- **Channel health monitor**: Background goroutine that detects stale channels and triggers restarts with per-channel cooldown, global hourly cap, and per-restart timeout
- **Session reset with preservation**: `ResetWithPreservation()` clears history/compaction/counters while preserving model, language, thinking level, fast mode, skills, and facts. Exposed via `POST /api/sessions/:id/reset`
- **WS handshake timeout**: Configurable via `DEVCLAW_WS_HANDSHAKE_TIMEOUT_MS` environment variable (default 10s, max 120s)

### UI

- **ModelCombobox**: New component for model selection with type-ahead support, integrated into ApiConfig and StepProvider pages
- **UnsavedChangesBar**: Notifies users of unsaved config changes with save/discard actions
- **Configuration refactoring**: Streamlined model selection logic, improved collapsible sections, consolidated provider management
- **Channel metadata**: Updated icon property type from `JSX.Element` to `ReactNode` for rendering flexibility

### Plugins

- **Unified YAML-first plugin system**: Replaces both legacy plugin systems (native `.so` and HTTP `PluginManager`) with a manifest-based framework supporting agents, tools, hooks, skills, channels, and services
- **Plugin agent runtime**: `SpawnWithExecutor` for plugin agents with trigger-based routing, bidirectional communication (`delegate_to_plugin_agent` + `escalate_to_main`), and auto-escalation
- **Plugin installer**: CLI commands (`devclaw plugin install/list/remove/update`) and WebUI install/remove flow with path traversal protection
- **Security hardening**: `EvalSymlinks` path validation, `PLUGIN_` prefix for script env vars, secret redaction in frontend config, bounded HTTP responses, context-aware HTTP requests

---

## [1.15.0] — 2026-03-17

### UI

- **Unified design system**: Consolidated two competing design systems into one modern visual language with `rounded-xl/2xl`, `h-11` inputs, modern focus rings. Replaced ~40 ad-hoc buttons with the shared Button component. Removed dead code (Navbar, CollapsibleSection, Dashboard, `cx.ts`)

### Core

- **User-friendly error messages**: Raw LLM errors (429, timeout, quota, auth, context overflow) mapped to friendly messages instead of exposing technical details
- **GLM-5-Turbo support**: New model entry (202K context, 16K default output, agent-optimized)
- **Subagent timeout increase**: Default timeout changed from 5 to 15 minutes to resolve timeout errors on long-running research tasks
- **Skill name normalization**: Auto-normalize skill names (hyphens → underscores, lowercase) in all `SkillDB` public methods
- **Process manager detection**: Runtime prompt layer detects systemd/pm2/docker/k8s

### Media

- **File sending across all channels**: `send_media` tool auto-resolves channel and recipient from delivery context; Web UI delivers media via SSE `media` event with inline rendering (images, audio, video, document download cards)

### Integrations

- **Claude Code OAuth**: Removed API key injection in favor of user's OAuth config; cleared `ANTHROPIC_*` env vars instead of injecting credentials

### Bug Fixes

- Demoted empty streaming fallback log from Warn to Debug
- Added startup warning when no fallback models are configured
- Documented allowed column types in `skill_init` `database_schema`

---

## [1.8.1] - 2025-02-22

### Fixed

- **Provider-specific API keys**: Changed from generic `DEVCLAW_API_KEY` to standard names (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, etc.)
- **Tool schema validation**: Fixed `team_list` tool missing `properties` field causing OpenAI API 400 errors
- **Setup wizard**: Now saves API keys with correct provider-specific names in vault
- **Vault injection**: All secrets are now injected as environment variables with their original names

### Changed

- **LLM client**: `resolveAPIKey()` now looks for provider-specific env vars first, then generic fallback
- **Key resolution priority**: Config key → Provider env var → Generic `API_KEY`
- `DEVCLAW_API_KEY` is now reserved for DevClaw gateway authentication (not LLM providers)

### Security

- Removed hardcoded API keys from example configs
- Vault now injects all secrets as env vars at runtime (no plaintext in config files)

## [1.14.0] - 2026-03-15

### Lossless Compaction Module (LCM) — DAG-Based Memory

Complete port of the lossless-claw compaction system from OpenClaw (TypeScript) to Go, integrated directly into the DevClaw agent. Instead of flat summary strings that discard detail, LCM builds a hierarchical DAG of summaries where every message is persisted verbatim and recoverable on demand.

**Architecture:**
- **DAG structure**: Leaf summaries (depth 0) compress raw messages; condensed summaries (depth 1+) compress groups of summaries, cascading until stable
- **Lossless guarantee**: Every message persisted verbatim in SQLite; summaries link back to source messages via join tables
- **Fresh tail**: Last N messages (default 32) always included raw, never summarized
- **FTS5 graceful degradation**: Full-text search when available, falls back to substring search
- **Feature flag**: `lcm_enabled` (default true) with `LCMConfig` struct for all parameters

**New `lcm` tool** (single dispatcher, 3 actions):
- `lcm grep query="search term"` — search across all messages and summaries (FTS5 + regex + substring)
- `lcm describe summary_id="tree"` — inspect the full DAG structure and metadata
- `lcm expand summary_id="sum_xxx"` — recover original messages behind any summary

**Integration:**
- `buildMessages()` assembles context from DAG (root summaries + fresh tail), falls back to legacy on error
- `managedCompaction()` ingests new messages, evaluates soft/hard triggers, runs leaf + condensed passes
- System prompt automatically includes LCM tool guidance when summaries exist
- Budget-aware assembly: reserves 20% for response, evicts oldest summaries when over budget

**New files:** `lcm.go`, `lcm_store.go`, `lcm_compaction.go`, `lcm_assembler.go`, `lcm_retrieval.go`, `lcm_tools.go`, `lcm_test.go` (22 unit tests)

**Modified files:** `db.go` (7 new tables), `compaction_safeguard.go`, `agent.go`, `assistant.go`

**Configuration:**
```yaml
compaction:
  lcm_enabled: true
  lcm:
    fresh_tail_count: 32
    leaf_chunk_max_tokens: 20000
    condensed_min_children: 4
    condensed_max_children: 8
    soft_trigger_ratio: 0.6
    hard_trigger_ratio: 0.85
    max_summary_tokens: 2000
```

### OpenClaw Feature Alignment (9 Features)

Ported 9 features from OpenClaw to align the Go agent with the TypeScript reference implementation:

- **Configurable compaction timeout**: `CompactionConfig.TimeoutSeconds` (default 900s / 15min)
- **Isolated heartbeat sessions**: `IsolateSession` option gives each heartbeat tick a unique session ID
- **Auto-compaction counter**: Persisted in session metadata for observability
- **Smart strict mode**: Disabled for non-native OpenAI-compat providers (ollama, lmstudio, vllm, huggingface)
- **Fast mode**: Anthropic `service_tier: auto`, OpenAI priority + low reasoning effort
- **Post-compaction memory sync**: Async/await/off modes for memory indexer after compaction
- **Compaction status reactions**: ✍ on start, ⏳ on stall detection (30s), cleaned up on completion
- **`sessions_yield` tool**: Cooperative turn-ending for subagent orchestration (denied for leaf subagents)
- **Skill DB error enrichment**: "table not found" errors now list available tables

### Agent Safeguards & Compaction Hardening

- **Structured compaction**: 5 mandatory sections (Decisions, Open TODOs, Constraints/Rules, Pending user asks, Exact identifiers) with quality guard audit and retry loop
- **Identifier preservation audit**: Post-summarization check that extracted identifiers appear in the summary
- **Ratio-based context pruning**: Soft trim tool results at 30% context usage, hard clear at 50%
- **Emergency compression**: Preserves goal + summary + recent turns instead of discarding all history
- **Anthropic refusal scrubbing**: Removes magic refusal strings from user messages
- **Thinking-level fallback**: Gracefully retries with extended thinking stripped on thinking errors
- **Safety constitution**: `buildSafetyLayer` now populates the safety constitution (was returning empty)
- **Deterministic output**: Sorted map iteration in `buildMinimalFallbackSummary` and `collectFileOperationsSeparated`

### Anti-Hallucination Defense

Three-layer defense against LLM fabricating data when tool calls fail:

- System prompt explicitly forbids inventing tool results
- `[System]` reminder injected into conversation after tool errors
- URL grounding check activated in output guardrail (log-only mode)

### Frontend & i18n

- Complete i18n coverage for Config, Domain, Channels, Access, Groups, Skills pages
- All hardcoded dropdown labels/placeholders replaced with `t()` calls
- Skill toggle/install failure feedback UI
- Dark mode support improvements
- Session management and dashboard error handling improvements
- `web/README.md` updated to match actual codebase (React 19, lucide-react, cn)

### Bug Fixes

- **LCM Hub schema (critical)**: LCM tables were missing from the production `GetSQLiteSchema()` in `backends/sqlite.go`, causing `no such table: lcm_conversations` on every request. Added all 7 LCM tables + indexes + FTS5 best-effort to the Hub migration path
- **LCM legacy migrations**: Added individual CREATE TABLE/INDEX migrations in `copilot/db.go` as belt-and-suspenders for the legacy `OpenDatabase()` path
- **LCM ON DELETE CASCADE**: All 7 LCM foreign keys now cascade deletes through the DAG hierarchy
- **LCM ingestion sequence reset**: Fixed `lcmIngestedSeq` reset from 0 to `len(messages)` — prevented re-ingesting assembled context after compaction
- **LCM aggressive compaction**: Added missing `UpdateLastCompactAt` call when `aggressiveCompaction` bypasses `LCMEngine.Compact()`
- **LCM summarization error**: Fixed `trySummarize` wrapping a nil error when both normal and aggressive summarization return empty responses
- **LCM grep logging**: `grepSubstring` now logs errors instead of silently swallowing them
- Fixed hook tests missing `Enabled: true` (Go bool zero value = false)
- Fixed `TestAllHookEvents` to include message pipeline hooks
- Fixed `TestBuiltinSkills_FormatForPrompt` for on-demand teams skill
- Fixed `MemoryIndexer` data race on `fsWatcher` field access
- Fixed skills backend handler logic for already-installed skills

### Team Tools — Dispatcher Pattern Refactor

Reduced team tool count from 34 to 5 to stay under OpenAI's 128 tool limit:

- **`team_manage`**: Team CRUD operations (create, list, get, update, delete)
- **`team_agent`**: Agent management + working state (create, list, get, update, start, stop, delete, working_get, working_update, working_clear)
- **`team_task`**: Task management (create, list, get, update, assign, delete)
- **`team_memory`**: Shared memory (fact_save, fact_list, fact_delete, doc_create, doc_list, doc_get, doc_update, doc_delete, standup)
- **`team_comm`**: Communication (comment, mention_check, send_message, notify, notify_list)

**Breaking Change**: Individual team tools (e.g., `team_create`, `team_list_agents`) are replaced by dispatcher tools with `action` parameter.

Example:
```
# Old: team_create(name="Engineering", description="Dev team")
# New: team_manage(action="create", name="Engineering", description="Dev team")
```

### WebUI Configuration Pages — Complete Settings UI

Full configuration management via WebUI with 7 new pages covering 100% of YAML settings:

- **API Configuration Page** (`/config`): LLM provider selection, API key management, base URL config, connection testing
- **Access Control Page** (`/access`): Owner/admin/user management, allowlist/blocklist, default policy settings
- **Budget Page** (`/budget`): Monthly spending limits, warning thresholds, usage visualization
- **Memory Page** (`/memory`): Embedding settings, search configuration, compression strategies
- **Database Hub Page** (`/database`): Backend selection (SQLite/PostgreSQL), connection status, pool metrics, vector search config
- **Groups Page** (`/groups`): Activation modes, intro messages, quiet hours, participant limits
- **MCP Servers Page** (`/mcp`): Server CRUD, start/stop controls, status monitoring

**New REST Endpoints:**
- `GET/PUT /api/config` - General configuration
- `GET/PUT /api/access` - Access control settings
- `GET/PUT /api/config/budget` - Budget configuration
- `GET/PUT /api/config/memory` - Memory configuration
- `GET/PUT /api/config/groups` - Groups configuration
- `GET /api/database/status` - Database health and pool metrics
- `GET/POST /api/mcp/servers` - List/create MCP servers
- `GET/PUT/DELETE /api/mcp/servers/{name}` - Manage specific server
- `POST /api/mcp/servers/{name}/start` - Start MCP server
- `POST /api/mcp/servers/{name}/stop` - Stop MCP server

**Reusable UI Components:**
- `ConfigPage`, `ConfigSection`, `ConfigField` - Layout components
- `ConfigInput`, `ConfigSelect`, `ConfigToggle`, `ConfigTextarea` - Form controls
- `ConfigCard`, `ConfigActions`, `ConfigTagList` - Interactive elements
- `LoadingSpinner`, `ErrorState`, `ConfigEmptyState`, `ConfigInfoBox` - State components

**Testing:**
- 49 frontend unit tests (Vitest + React Testing Library)
- All configuration pages tested for rendering, interactions, and API calls

**Internationalization:**
- Full i18n support with English and Portuguese translations
- All new pages support language switching

### Database Hub — Multi-Backend Support

Complete database abstraction layer with SQLite (default) and PostgreSQL/Supabase support:

- **Multi-Backend Architecture**: Unified interface for SQLite, PostgreSQL, and future backends (MySQL)
- **Connection Pool Management**: Full pool metrics (open/in-use/idle connections, wait count/duration)
- **Vector Search**: In-memory for SQLite, pgvector for PostgreSQL with HNSW/IVFFlat indexes
- **Schema Migrations**: Automatic schema creation and version tracking for all backends
- **Health Monitoring**: Detailed health status with pool metrics and latency tracking

**Security Improvements:**
- SQL injection prevention in `db_hub_schema` (table name validation)
- Path traversal prevention in `db_hub_backup` (path sanitization)
- Rate limiting in `db_hub_raw` (10 ops/sec per session)

**New Agent Tools:**
- `db_hub_status`: Health status with pool metrics
- `db_hub_query`: Execute SELECT queries (validated)
- `db_hub_execute`: Execute INSERT/UPDATE/DELETE
- `db_hub_schema`: View schema/tables (SQL injection protected)
- `db_hub_migrate`: Run schema migrations
- `db_hub_backup`: Create SQLite backups (path traversal protected)
- `db_hub_backends`: List available backends
- `db_hub_raw`: Execute raw SQL (rate-limited)

**Configuration:**
```yaml
database:
  hub:
    backend: "sqlite"  # sqlite | postgresql
    sqlite:
      path: "./data/devclaw.db"
      journal_mode: "WAL"
    postgresql:
      host: "localhost"
      port: 5432
      vector:
        enabled: true
        dimensions: 1536
        index_type: "hnsw"
```

**Testing:**
- Unit tests for SQLite backend (62% coverage)
- Integration tests for PostgreSQL (run with `-tags=integration`)
- Rate limiter tests with concurrent access

### Native Media Handling

Complete native media system for receiving, processing, and sending images, audio, and documents across all channels:

- **MediaService**: Channel-agnostic media handling with upload, enrichment, and send capabilities
- **MediaStore**: File-based storage with automatic cleanup of temporary files
- **Validator**: MIME type detection using magic bytes + extension heuristics
- **WebUI API**: REST endpoints for media upload/download at `/api/media`

**New Media Tools:**
- `send_image`: Send images to users via media_id, file_path, or URL
- `send_audio`: Send audio files to users
- `send_document`: Send documents (PDF, DOCX, TXT) to users

**Enrichment Features:**
- **Images**: Auto-description via Vision API (configurable model with fallback)
- **Audio**: Auto-transcription via Whisper API (configurable model with fallback)
- **Documents**: Text extraction from PDF (pdftotext), DOCX (unzip), and plain text

**Configuration:**
```yaml
native_media:
  enabled: true
  store:
    base_dir: "./data/media"
    temp_dir: "./data/media/temp"
  service:
    max_image_size: 20MB
    max_audio_size: 25MB
    max_doc_size: 50MB
  enrichment:
    auto_enrich_images: true     # requires media.vision_enabled
    auto_enrich_audio: true      # requires media.transcription_enabled
    auto_enrich_documents: true  # works independently
```

**Model Selection:**
- Vision uses `media.vision_model` (falls back to main model)
- Transcription uses `media.transcription_model`
- Set specific models for better quality or cost optimization

### Teams System — Persistent Agents & Shared Memory

Complete team coordination system with persistent agents, shared memory, and real-time collaboration:

- **Persistent Agents**: Long-lived agents with specific roles, personalities, and instructions
- **Team Memory**: Shared state accessible by all team members (tasks, messages, facts, documents)
- **Thread Subscriptions**: Auto-subscribe to threads for continuous notifications
- **Working State (WORKING.md)**: Persist work-in-progress across heartbeats
- **Active Notification Push**: Trigger agents immediately on @mentions
- **Documents Storage**: Store deliverables linked to tasks with versioning

**New Team Tools:**
- Team management: `team_create`, `team_list`, `team_create_agent`, `team_list_agents`, `team_stop_agent`, `team_delete_agent`
- Tasks: `team_create_task`, `team_list_tasks`, `team_update_task`, `team_assign_task`
- Communication: `team_comment`, `team_check_mentions`, `team_send_message`, `team_mention`
- Memory: `team_save_fact`, `team_get_facts`, `team_delete_fact`, `team_standup`
- Documents: `team_create_document`, `team_list_documents`, `team_get_document`, `team_update_document`, `team_delete_document`
- Working State: `team_get_working`, `team_update_working`, `team_clear_working`

### LLM Improvements

- **Gemini model ID normalization**: Automatic conversion of Gemini model IDs (e.g., `gemini-2.0-flash` → `gemini-2.0-flash-001`) for compatibility with Google AI API
- **Enhanced subagent spawning controls**: Better control over which models can spawn subagents

### Agent Improvements

- **Tool filtering by profile**: Filter available tools based on agent profile configuration
- **Tool count limits**: Configurable limits on number of tools exposed to agent

### Background Routines

- **Metrics Collector**: Periodic system metrics collection with webhook support
  - Message and token counts with per-minute rates
  - Agent runs (total, active, success, failed, timeout)
  - Tool calls and subagent statistics
  - System metrics (goroutines, memory, uptime)
  - Latency tracking with P50/P99 percentiles
  - Subscriber pattern for real-time metrics delivery

- **Memory Indexer**: Incremental indexing of memory files for enhanced search
  - SHA-256 hash-based change detection
  - Automatic detection of deleted files
  - Configurable interval and memory directory

- **Internal Routines Documentation**: Complete documentation of all 26+ background routines in `docs/internal-routines.md`

### Team Notification System

Complete notification system for team agents with routing, destinations, and delivery:

- **NotificationDispatcher**: Central component for routing notifications to configured destinations
- **Multiple Destinations**: Channel (WhatsApp/Discord/Telegram), Inbox, Webhook, Owner, Activity Feed
- **Rate Limiting**: Per-rule rate limits with hourly reset
- **Quiet Hours**: Configurable time windows for suppressing non-urgent notifications
- **Priority System**: 1-5 priority levels (1=urgent always delivered, 5=low priority)
- **Notification History**: Persistent storage with read/unread tracking

**New Agent Tools:**
- `team_notify`: Send notifications about completed work or important events
- `team_get_notifications`: View team notification history

**Notification Types:**
- `task_completed` / `task_failed` / `task_blocked` / `task_progress` / `agent_error`

**Configuration:**
```yaml
notifications:
  enabled: true
  defaults:
    activity_feed: true
    owner: false
  quiet_hours:
    enabled: true
    start: "22:00"
    end: "08:00"
    timezone: "America/Sao_Paulo"
  rules:
    - name: "Critical Alerts"
      events: [task_failed, agent_error]
      destinations:
        - type: channel
          channel: "whatsapp"
          chat_id: "120363XXXXXX@g.us"
```

**Files:**
- `notification_dispatcher.go`: Core dispatcher with routing logic
- `notification_dispatcher_test.go`: Comprehensive test coverage
- Updated `team_types.go`, `team_manager.go`, `team_tools.go`, `db.go`

---

## [1.7.0] — 2026-02-18

WhatsApp UX overhaul, setup improvements, tool profiles, native media, and comprehensive documentation.

### WhatsApp & Session Management

- **Enhanced session management**: Improved WhatsApp connection handling and session persistence
- **Better connection management**: Robust event handling for WhatsApp connections
- **Session persistence**: Workspace session stores now survive container restarts

### Setup & Configuration

- **New setup components**: Streamlined setup wizard with improved UX
- **Tool profiles**: Simplified permission management with predefined profiles (admin, developer, readonly, etc.)
- **System administration commands**: New `/maintenance` and system commands for admin control
- **Maintenance mode**: Enable maintenance mode to block non-admin access during updates

### Media & Vision

- **Native vision and transcription**: Vision and transcription are now native tools with auto-detection
- **Vision model selector**: UI for selecting vision model in settings
- **Synchronous media enrichment**: All media enrichment runs synchronously for better reliability
- **Audio transcription**: End-to-end audio transcription working

### Documentation

- **Agent architecture review**: Comprehensive documentation of agent routing, DM pairing, environment variables
- **Hooks system documentation**: Full documentation of lifecycle hooks
- **Group policy documentation**: Group chat policy configuration

### UI Improvements

- **Enhanced dark mode**: Better dark mode styling across all components
- **Chat message UI**: Improved chat message display and localization
- **Language switcher**: Multilingual support with language selection

### Docker & Deployment

- **Restructured Docker setup**: Simplified Dockerfile and docker-compose for easier deployment
- **Runtime tools in Docker**: All necessary tools included in Docker image
- **Workspace bind mount**: Workspace directory bind-mounted into container

---

## [1.6.1] — 2026-02-17

Starter pack system and UI fixes.

### Skills

- **Starter pack system**: Skills can now teach the LLM how to use native tools via embedded instructions

### UI Fixes

- Fixed vault nomenclature in security page
- Fixed toggle bug using canonical Tailwind classes
- Fixed PT-BR accents, gateway port 8085
- More concise vault texts in setup wizard

### Docker

- **Named volume for state**: State now persists between container restarts via named volume
- **Simplified Dockerfile**: Cleaner Dockerfile and compose for VM deployment
- Removed .env from tracking, conditional phone, improved Channels

---

## [1.6.0] — 2026-02-17

Major release with 20+ native tools, MCP server, plugin system, and redesigned UI.

### Native Tools (20+ New Tools)

- **Git tools**: status, diff, log, commit, branch, stash, blame with structured JSON output
- **Docker tools**: ps, logs, exec, images, compose (up/down/ps/logs), stop, rm
- **Database tools**: query, execute, schema, connections (PostgreSQL, MySQL, SQLite)
- **Dev utility tools**: json_format, jwt_decode, regex_test, base64, hash, uuid, url_parse, timestamp
- **System tools**: env_info, port_scan, process_list
- **Codebase tools**: index (file tree), code_search (ripgrep), symbols, cursor_rules_generate
- **Testing tools**: test_run (auto-detect framework), api_test, test_coverage
- **Ops tools**: server_health (HTTP/TCP/DNS), deploy_run, tunnel_manage, ssh_exec
- **Product tools**: sprint_report, dora_metrics, project_summary
- **Daemon manager**: start_daemon, daemon_logs, daemon_list, daemon_stop, daemon_restart

### MCP Server

- **Model Context Protocol**: Full MCP server implementation for IDE integration
- **stdio and SSE transports**: Support for both transport modes
- Works with Cursor, VSCode, Claude Code, OpenCode, Windsurf, Zed, Neovim

### Plugin System

- **Extensible plugins**: GitHub, Jira, Sentry integrations
- **Webhook dispatcher**: Event-driven webhook support

### CLI Enhancements

- **Pipe mode**: `git diff | devclaw diff` or `npm build 2>&1 | devclaw fix`
- **Quick commands**: `devclaw fix`, `devclaw explain .`, `devclaw commit`, `devclaw how "task"`
- **Shell hook**: Auto-capture failed commands with `devclaw shell-hook bash`

### Multi-User System

- **RBAC**: Role-based access control with owner/admin/user roles
- **IDE extension configuration**: Configure IDE extensions per user

### N-Provider Fallback

- **Budget tracking**: Configurable cost limits and alerts
- **Fallback chain**: Automatic model fallback with per-model cooldowns

### UI Redesign

- **v1.6.0 redesign**: Clean, professional look
- **Hooks management**: Lifecycle hooks via UI
- **Webhooks management**: Webhook configuration via UI
- **Domain/network settings**: Configuration page for domain and network

### Project Rename

- Renamed from GoClaw to DevClaw across entire project

---

## [1.5.1] — 2026-02-16

Agent loop safety improvements ported from OpenClaw: tool loop detection, skills token budget guard, heartbeat transcript pruning, compaction retry with backoff, and cron spin loop fix.

### Agent Loop Safety

- **Tool loop detection**: New `ToolLoopDetector` module tracks tool call history with a ring buffer and detects two patterns — **repeat** (same tool+args N times) and **ping-pong** (A→B→A→B). Three severity levels: warning (8x, injects hint), critical (15x, strong nudge), circuit breaker (25x, terminates run). Fully configurable via `config.yaml` under `agent.tool_loop`
- **Per-run detector isolation**: Each agent run creates its own `ToolLoopDetector` instance to avoid cross-session race conditions when multiple users interact concurrently
- **Valid message ordering**: Loop warnings are injected AFTER tool results (assistant→tool→user sequence) to avoid API rejections from providers that validate message order

### Prompt & Memory Optimization

- **Skills prompt bloat guard**: Skills layer now enforces a ~4000 token budget. When total skills text exceeds the budget, largest skills are truncated first (minimum 200 chars preserved). Prevents verbose skill prompts from consuming the entire context window
- **Heartbeat transcript pruning**: No-op heartbeat turns (HEARTBEAT_OK, NO_REPLY, empty) are no longer saved to session history. Only actionable heartbeat responses are persisted, preventing transcript bloat over time

### Reliability

- **Compaction retry with exponential backoff**: The `compactSummarize` LLM call now retries up to 3 times with backoff (2s→4s→8s) on transient errors (rate-limits, timeouts). Properly exits retry loop on context cancellation. Falls back to static summary only after all retries are exhausted
- **Cron spin loop fix**: New `minJobInterval` (2s) guard in scheduler's `executeJob` prevents rapid re-execution when cron fires at the exact same second boundary. Skips silently with debug log when a job ran too recently

### Testing

- **Tool loop detection tests**: 12 test cases covering warning/critical/breaker thresholds, ping-pong detection, reset behavior, disabled mode, ring buffer limits, threshold normalization, and hash determinism
- **Scheduler spin loop tests**: 3 test cases covering spin loop guard, duplicate execution guard, and minJobInterval value assertion
- All tests pass with `-race` detector

---

## [1.5.0] — 2026-02-16

Media processing, document enrichment, WhatsApp UX overhaul, comprehensive security hardening, agent intelligence improvements, and full unit test coverage.

### Media & Document Processing

- **Document parsing**: Automatic text extraction from PDF (`pdftotext`), DOCX (`unzip` + XML strip), and 30+ plain text formats (code files, CSV, JSON, YAML, Markdown, etc.)
- **Video enrichment**: First-frame extraction via `ffmpeg` + Vision API description — agent sees `[Video: description]` in context
- **Auto-send generated images**: New `onToolResult` hook on `AgentRun` detects `generate_image` tool output and sends the image file directly to the user's channel as media — no more "image saved to /tmp/..." text responses
- **Async media pipeline**: Document and video enrichment wired into the existing background media processing with placeholder → enriched content flow
- **System prompt documentation**: Agent is informed of media capabilities and how to install system dependencies (`poppler-utils`, `ffmpeg`, `unzip`)

### WhatsApp UX

- **Message duplication fix**: Steer mode now injects OR enqueues (never both) — each message processed exactly once; eliminates duplicate and triple responses to `/stop`
- **Message fragmentation fix**: BlockStreamer params tuned — MinChars 20→200, MaxChars 600→1500, IdleMs 200→1500 — produces coherent paragraphs instead of 4-word fragments
- **Progress flood eliminated**: `ProgressSender` now has per-channel cooldown (60s WhatsApp, 10s WebUI); removed duplicate heartbeats from tool_executor and claude-code skill
- **Queue modes operational**: All 5 modes (collect, steer, followup, interrupt, steer-backlog) fully implemented; removed hardcoded "Recebi sua mensagem..." canned response; default changed from collect→steer

### Agent Intelligence

- **Subagent delegation rewrite**: Complete rewrite of subagent section in system prompt with detailed workflow, concrete examples, and clear rules for when to delegate
- **Parallel task handling**: New "Handling New Messages During Work" prompt section — agent uses `spawn_subagent` for parallel tasks and recognizes follow-ups vs new requests
- **Media awareness**: System prompt now documents all media types the agent can receive and process (images, audio, documents, video)

### Security Hardening

- **Output sanitization**: Strip `[[reply_to_*]]`, `<final>`, `<thinking>`, `NO_REPLY`, `HEARTBEAT_OK` from all outgoing messages via `StripInternalTags` in `FormatForChannel`
- **Empty message guards**: Prevent sending blank messages after tag stripping in `sendReply`, `BlockStreamer`, and `ProgressSender`
- **File permissions hardened**: Session transcripts/facts/meta files 0644→0600 (owner-only); sessions directory 0755→0700
- **Config save safety**: YAML validation before writing + `.bak` backup creation
- **TTS duplicate prevention**: Skip TTS synthesis for `NO_REPLY`/`HEARTBEAT_OK` silent tokens
- **Centralized constants**: `TokenNoReply` and `TokenHeartbeatOK` exported as constants

### LLM Reliability

- **Rate-limit auto-recovery**: Per-model cooldown tracking on 429 responses (duration from `Retry-After` header, min 60s, max 10min)
- **Smart fallback**: Rate-limited models skipped in fallback loop; periodic probe near cooldown expiry (within 10s, throttled at 30s between probes) for automatic recovery without restart
- **Cron reliability**: `LastRunAt` persisted before job execution (not after) to prevent duplicate fires on crash recovery

### Setup & Configuration

- **Vault in setup wizard**: Dedicated vault password field in StepSecurity with toggle for separate vault vs WebUI password
- **Owner phone in setup**: Setup wizard collects owner phone number, formats as WhatsApp JID — owner gets full tool access (bash, exec, write_file, etc.)
- **Skill installation from WebUI**: New endpoints `GET /api/skills/available`, `POST /api/skills/install`, `POST /api/skills/{name}/toggle`; modal with search and categories in the Skills page

### Testing

- **259 test cases across 13 files** covering 3 phases:
  - *Pure logic*: markdown formatting, message splitting, queue modes, config loading, memory hardening, session keys
  - *Security*: SSRF protection, input/output guardrails, rate limiting, vault encryption, keyring injection
  - *Stateful components*: tool guard permissions, access control, skill registry
- **Makefile integration**: `make test` and `make test-v` targets with `-race` detector and `sqlite_fts5` tag
- All tests run with `t.Parallel()` where safe, table-driven patterns, `t.TempDir()` for isolation

### Dependencies

- System: `poppler-utils` (PDF), `ffmpeg` (video), `unzip` (DOCX) — optional, graceful fallback when missing

---

## [1.4.0] — 2026-02-16

Performance improvements, new concurrency architecture, advanced agent capabilities, comprehensive security hardening, and new tools.

### Performance

- **Adaptive debounce**: reduced default from 1000ms to 200ms; followup debounce at 500ms; idle sessions drain immediately
- **Unified send+stream endpoint**: new `POST /api/chat/{sessionId}/stream` combines send and stream in a single HTTP request, eliminating a round-trip
- **Asynchronous media enrichment**: vision/transcription runs in background while the agent starts responding; results are injected via interrupt channel
- **Background compaction**: `maybeCompactSession` now runs in a goroutine, no longer blocking the response path
- **Faster block streaming**: `MinChars` reduced from 50→20, `IdleMs` from 400→200ms; eliminated double idle check for immediate flushing
- **Lazy prompt composition**: memory and skills layers cached (60s TTL) with background refresh; critical layers (bootstrap, history) still loaded synchronously

### Architecture

- **Lane-based concurrency** (`lanes.go`): configurable lanes (session, cron, subagent) with per-lane queue and concurrency limits — prevents work-type contention
- **Event bus** (`events.go`): in-memory pub/sub for agent lifecycle events (delta, tool_use, done, error) — supports multi-consumer streams
- **WebSocket JSON-RPC** (`gateway/websocket.go`): bidirectional real-time communication as alternative to HTTP/SSE; supports client requests (chat.abort) and server events
- **Fast abort** (`tool_executor.go`): abort channel with `ResetAbort`/`Abort`/`IsAborted` — tools detect and respond to external abort signals during execution

### Agent Capabilities

- **Queue modes** (`queue_modes.go`): configurable strategies for incoming messages when session is busy — collect, steer, followup, interrupt, steer-backlog
- **Model failover** (`model_failover.go`): automatic LLM failover with reason classification (billing, rate limit, auth, timeout, format) and per-model cooldowns
- **Proactive prompts** (`prompt_layers.go`): reply tags, silent reply tokens, heartbeat guidance, reasoning format, memory recall, subagent orchestration, and messaging directives
- **Agent steering**: interrupt during tool execution; tools check `interruptCh` for incoming messages to steer behavior mid-run
- **Context pruning** (`agent.go`): proactive soft/hard trim of old tool results based on turn age — prevents context bloat without waiting for LLM overflow
- **Memory flush pre-compaction**: explicit "pre-compaction memory flush turn" saves durable memories to disk before compaction using append-only strategy
- **Expanded directives**: new `/verbose`, `/reasoning` (alias for `/think`), `/queue`, `/usage`, `/activation` commands with thread-safe config access

### Security

- **Workspace containment** (`workspace_containment.go`): sandbox path validation and symlink escape protection for all file operations
- **Memory injection hardening** (`memory_hardening.go`): treats memories as untrusted data; escapes HTML entities, strips dangerous tags, wraps in `<relevant-memories>` tags, detects injection patterns
- **Request body limiter**: 2MB limit on `POST /v1/chat/completions` to prevent OOM from oversized payloads
- **Partial output on abort**: `handleChatAbort` now includes a `partial` flag indicating preserved partial output before cancellation

### New Tools & Features

- **Browser automation** (`browser_tool.go`): Chrome DevTools Protocol (CDP) integration with `browser_navigate`, `browser_screenshot`, `browser_content`, `browser_click`, `browser_fill`
- **Interactive canvas** (`canvas_host.go`): HTML/JS canvas with live-reload via temporary HTTP server; `canvas_create`, `canvas_update`, `canvas_list`, `canvas_stop`
- **Session management tools**: `sessions_list` (all workspaces), `sessions_send` (inter-agent messaging), `sessions_delete`, `sessions_export` (full history + metadata as JSON)
- **Lifecycle hooks** (`hooks.go`): 16+ event types (SessionStart, PreToolUse, AgentStop, etc.) with priority-ordered dispatch, sync blocking, async non-blocking, and panic recovery
- **Group chat** (`group_chat.go`): activation modes, intro messages, context injection, participant tracking, quiet hours, ignore patterns
- **Tailscale integration** (`tailscale.go`): Tailscale Serve/Funnel for secure remote access without manual port forwarding or DNS configuration
- **Advanced cron** (`scheduler.go`): isolated sessions per job, announce handler for broadcasting results, subagent spawn option, per-job timeouts, job labels

### Session Management

- **Structured session keys**: `SessionKey` struct (Channel, ChatID, Branch) enables multi-agent routing
- **Session CRUD**: `DeleteByID`, `Export`, `RenameSession` on SessionStore
- **Bounded followup queue**: FIFO eviction at 20 items maximum

### Bug Fixes

- Fixed data race in `ListSessions` when reading `lastActiveAt`/`CreatedAt` without lock
- Fixed `splitMax` panic on empty separator
- Fixed restored sessions not persisting (missing `persistence` field)
- Fixed duplicate hook event registration in `HookManager.Register`
- Fixed panic in hook dispatch — individual handler panics no longer crash dispatch goroutine
- Fixed race condition in canvas `Update` — hold mutex during channel send to prevent panic from concurrent `Stop`
- Fixed infinite loop in `findFreePort` when `MaxCanvases` exceeds available port range
- Fixed `tailscale.go` hostname parsing — replaced manual JSON parsing with `json.Unmarshal`
- Fixed data race on `Job.LastRunAt`/`RunCount`/`LastError` in scheduler `executeJob`
- Fixed scheduler persisting removed jobs — added existence check before `Save`
- Fixed `commands.go` reading config without mutex — added `configMu.RLock()` to `/queue` and `/activation`

### Frontend (WebUI)

- New `createPOSTSSEConnection` for POST-based SSE in unified send+stream flow
- Updated `useChat` hook to eliminate separate `api.chat.send` + `createSSEConnection` calls
- Added `run_start` event handler for unified stream protocol

### Dependencies

- Added `github.com/gorilla/websocket` (WebSocket support)
- Added `github.com/robfig/cron/v3` (advanced cron scheduling)

---

## [1.1.0] — 2026-02-12

### Performance

- Increased default agent turn timeout from 90s to 300s to handle slow model cold starts (e.g. GLM-5 ~30-60s first turn)
- Added transient error retry (1x, 2.5s delay) for streaming LLM calls before falling back to non-streaming
- Bootstrap file loading now uses in-memory SHA-256 cache to avoid redundant disk reads

### Progressive Message Delivery (Block Streaming)

- New `BlockStreamer` module: accumulates LLM output tokens and sends them progressively to channels (WhatsApp) as partial messages
- Configurable: `block_stream.enabled`, `block_stream.min_chars` (default: 80), `block_stream.idle_ms` (default: 1200), `block_stream.max_chars` (default: 3000)
- Natural break detection: splits at paragraph boundaries, sentence endings, or list items
- Avoids duplicate final messages when blocks have already been sent

### Advanced Memory System

- **SQLite-backed memory store** with FTS5 (BM25 ranking) and in-process vector search (cosine similarity)
- **Embedding provider**: OpenAI `text-embedding-3-small` (1536 dims) with SQLite-backed embedding cache to reduce API calls
- **Markdown chunker**: intelligent splitting by headings, paragraphs, and sentences with configurable overlap and max tokens
- **Delta sync**: hash-based change detection — only re-embeds chunks that actually changed
- **Hybrid search** (RRF): combines vector similarity and BM25 keyword scores with configurable weights (default: 0.7 vector / 0.3 BM25)
- **Session memory hook**: on `/new` command, summarizes the conversation via LLM and saves to `memory/YYYY-MM-DD-slug.md`
- New `memory_index` tool for manual re-indexing of memory files
- `memory_search` upgraded to use hybrid search when SQLite store is available (falls back to substring search)
- Configurable via `memory.embedding`, `memory.search`, `memory.index`, `memory.session_memory` in config.yaml

### Bootstrap Improvements

- `BOOT.md` support: agent executes instructions from `BOOT.md` after startup (proactive initialization)
- `BootstrapMaxChars` limit prevents oversized bootstrap from consuming the token budget
- Subagent-specific filtering: subagents only load `AGENTS.md` and `TOOLS.md` (not the full bootstrap set)

### Session Persistence

- `SessionPersistence` properly wired to session store — conversations now survive restarts
- JSONL history, facts, and metadata are loaded on startup and saved on changes

### Bug Fixes

- Fixed race condition in `/new` command where session history could be cleared before the summary goroutine reads it (now captures a snapshot first)
- Fixed hybrid search merge key collision: different chunks from the same file that share a prefix are no longer collapsed
- Fixed FTS5 query injection: user input is now sanitized and wrapped in double quotes for phrase matching
- Fixed potential SQL row leak in `IndexChunks` error path

### New Config Options

```yaml
block_stream:
  enabled: false
  min_chars: 80
  idle_ms: 1200
  max_chars: 3000

memory:
  embedding:
    provider: openai
    model: text-embedding-3-small
    dimensions: 1536
    batch_size: 20
  search:
    hybrid_weight_vector: 0.7
    hybrid_weight_bm25: 0.3
    max_results: 6
    min_score: 0.1
  index:
    auto: true
    chunk_max_tokens: 500
  session_memory:
    enabled: false
    messages: 15
```

### Dependencies

- Added `github.com/mattn/go-sqlite3` (SQLite driver with FTS5 support)

## [1.0.0] — 2026-02-12

First stable release.

### Core

- Agent loop with multi-turn tool calling, auto-continue (up to 2 continuations), and reflection nudges every 8 turns
- 8-layer prompt composer (Core, Safety, Identity, Thinking, Bootstrap, Business, Skills, Memory, Temporal, Conversation, Runtime) with priority-based token budget trimming
- Session isolation per chat/group with JSONL persistence and auto-pruning
- Three compression strategies for session compaction: `summarize` (LLM), `truncate`, `sliding` — preventive trigger at 80% capacity
- Token-aware sliding window for conversation history (backwards construction, per-message truncation)
- Subagent system: spawn, track, wait, stop child agents with filtered tool sets
- Message queue with per-session debounce (configurable ms), deduplication (5s window), and burst handling
- Config hot-reload via file watcher (mtime + SHA256 hash)
- Token/cost tracking per session and global with model-specific pricing

### LLM Client

- OpenAI-compatible HTTP client with provider auto-detection
- Providers: OpenAI, Z.AI (API, Coding, Anthropic proxy), Anthropic, any OpenAI-compatible
- Model-specific defaults for temperature and max_tokens (GPT-5, Claude Opus 4.6, GLM-5, etc.)
- Model fallback chain with exponential backoff, `Retry-After` header support, and error classification
- SSE streaming with `[DONE]` terminator handling
- Prompt caching (`cache_control: ephemeral`) for Anthropic and Z.AI Anthropic proxy
- Context overflow handling: auto-compaction, tool result truncation, retry

### Security

- Encrypted vault (AES-256-GCM + Argon2id key derivation: 64 MB, 3 iterations, 4 threads)
- Secret resolution chain: vault → OS keyring → env vars → .env → config.yaml
- Tool Guard: per-tool role permissions (owner/admin/user), dangerous command regex blocking, protected paths, SSH host allowlist, audit logging
- Interactive execution approval via chat (`/approve`, `/deny`)
- SSRF protection for `web_fetch`: blocks private IPs, loopback, link-local, cloud metadata
- Script sandbox: none / Linux namespaces (PID, mount, net, user) / Docker container
- Pre-execution content scanning: eval, reverse shells, crypto mining, shell injection, obfuscation
- Input guardrails: rate limiting (sliding window), prompt injection detection, max input length
- Output guardrails: system prompt leak detection, empty response fallback

### Tools (35+)

- File I/O: `read_file`, `write_file`, `edit_file`, `list_files`, `search_files`, `glob_files` — full filesystem access
- Shell: `bash` (persistent cwd/env), `set_env`, `ssh`, `scp`
- Web: `web_search` (DuckDuckGo HTML parsing), `web_fetch` (SSRF-protected)
- Memory: `memory_save`, `memory_search`, `memory_list`
- Scheduler: `schedule_add`, `schedule_list`, `schedule_remove`
- Media: `describe_image` (LLM vision), `transcribe_audio` (Whisper API)
- Skills: `init_skill`, `edit_skill`, `add_script`, `list_skills`, `test_skill`, `install_skill`, `search_skills`, `remove_skill`
- Subagents: `spawn_subagent`, `list_subagents`, `wait_subagent`, `stop_subagent`
- Parallel tool execution with configurable semaphore (max 5 concurrent)

### Channels

- WhatsApp (native Go via whatsmeow): text, images, audio, video, documents, stickers, voice notes, locations, contacts, reactions, reply/quoting, typing indicators, read receipts, group messages
- Automatic media enrichment: vision (image description) and audio transcription (Whisper) on incoming media
- WhatsApp markdown formatting (bold, italic, strikethrough, code, code blocks)
- Message splitting for long responses (preserves code blocks, prefers paragraph/sentence boundaries)
- Plugin loader for additional channels (Go native `.so`)

### Access Control & Workspaces

- Per-user/group allowlist and blocklist with deny-by-default policy
- Roles: owner > admin > user > blocked
- Chat commands: `/allow`, `/block`, `/admin`, `/users`, `/group allow`
- Multi-tenant workspaces with isolated system prompts, skills, models, languages, and memory
- Workspace management via chat: `/ws create`, `/ws assign`, `/ws list`

### Skills

- Native Go skills (compiled, direct execution) and SKILL.md format (ClawHub compatible)
- Skill installation from ClawHub, GitHub, HTTP URLs, and local paths
- Built-in skills: weather, calculator, web-search, web-fetch, summarize, github, gog, calendar
- Skill creation via chat (agent can author its own skills)
- Hot-reload on install/remove

### HTTP API Gateway

- OpenAI-compatible `POST /v1/chat/completions` with SSE streaming
- Session management: `GET/DELETE /api/sessions`
- Usage tracking: `GET /api/usage`
- System status: `GET /api/status`
- Webhook registration: `POST /api/webhooks`
- Bearer token authentication and CORS

### CLI

- Interactive setup wizard with arrow-key navigation (`charmbracelet/huh`), model auto-detection, vault creation
- CLI chat REPL with `readline` support: arrow-key history, reverse search (Ctrl+R), tab completion, persistent history
- Commands: `setup`, `serve`, `chat`, `config` (init, show, validate, vault-*, set-key, key-status), `skill` (list, search, install, create), `schedule` (list, add), `health`
- Chat commands: `/help`, `/model`, `/usage`, `/compact`, `/think`, `/new`, `/reset`, `/stop`, `/approve`, `/deny`

### Bootstrap System

- Template files in `configs/bootstrap/`: SOUL.md, AGENTS.md, IDENTITY.md, USER.md, TOOLS.md, HEARTBEAT.md
- Loaded at runtime into the prompt layer system
- Agent can read and update its own bootstrap files

### Scheduler & Heartbeat

- Cron-based task scheduler with file persistence
- Heartbeat: proactive agent behavior on configurable interval with active hours
- Agent reads HEARTBEAT.md for pending tasks

### Deployment

- Single binary, zero runtime dependencies
- Docker and Docker Compose support
- systemd service unit with hardening (ProtectSystem, PrivateTmp, MemoryMax)
- Makefile: build, run, setup, chat, test, lint, clean, docker-build, docker-up
