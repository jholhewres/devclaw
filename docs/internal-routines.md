# DevClaw Internal Routines

Complete documentation of all background routines, goroutines, timers, and periodic processes that run during DevClaw operation.

---

## Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     DevClaw Background Routines                          │
├─────────────────────────────────────────────────────────────────────────┤
│  INITIALIZATION-STARTED (CONTINUOUS)                                    │
│  ├── Message Loop (event pump)          [assistant.go:933]              │
│  ├── Heartbeat Ticker (30min)           [heartbeat.go:92]               │
│  ├── Config Watcher (5s poll)           [config_watcher.go:41]          │
│  ├── Session Pruner (12h)               [session.go:454]                │
│  ├── Daemon Health Loop (30s)           [daemon_manager.go:262]         │
│  ├── Metrics Collector (1min)           [metrics_collector.go]          │
│  └── Memory Indexer (5min)              [memory_indexer.go]             │
├─────────────────────────────────────────────────────────────────────────┤
│  ON-DEMAND (PER-OPERATION)                                              │
│  ├── Message Handlers                   [assistant.go:940]              │
│  ├── Typing Indicator (8s refresh)      [assistant.go:1243]             │
│  ├── Media Enrichment                   [assistant.go:1261]             │
│  ├── Auto-Capture Facts                 [assistant.go:1292]             │
│  ├── Session Compaction                 [assistant.go:1298]             │
│  ├── Subagent Execution                 [subagent.go:537]               │
│  ├── Tool Parallel Execution            [tool_executor.go:489]          │
│  ├── Webhook Delivery                   [webhooks.go:194]               │
│  └── Canvas Servers                     [canvas_host.go:189]            │
├─────────────────────────────────────────────────────────────────────────┤
│  SCHEDULER (USER-DEFINED)                                               │
│  └── Cron Jobs                          [scheduler.go]                  │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 1. Initialization-Started Routines (Continuous)

### 1.1 Message Loop
**File**: `assistant.go:933-946`

```go
for {
    select {
    case msg := <-a.channelMgr.Messages():
        go a.handleMessage(msg)
    case <-a.ctx.Done():
        return
    }
}
```

- **Purpose**: Main event pump receiving messages from all channels
- **Trigger**: Continuous, blocking select
- **Started**: `Assistant.Start()`

---

### 1.2 Heartbeat Ticker
**File**: `heartbeat.go:92-93`

```go
ticker := time.NewTicker(interval)
for {
    select {
    case <-ticker.C:
        if !h.withinActiveHours() { continue }
        // trigger heartbeat
    }
}
```

- **Purpose**: Triggers proactive agent behavior at intervals
- **Interval**: Configurable (default: 30 min)
- **Active hours**: 9 AM - 10 PM (configurable)
- **Started**: `Heartbeat.Start(ctx)`

---

### 1.3 Config Watcher
**File**: `config_watcher.go:41-56`

```go
ticker := time.NewTicker(w.interval)
for {
    select {
    case <-ticker.C:
        if mtime changed && hash changed {
            w.reload()
        }
    }
}
```

- **Purpose**: Hot-reload of config.yaml
- **Interval**: 5 seconds
- **Verification**: mtime + SHA-256 hash
- **Started**: `ConfigWatcher.Start(ctx)`

---

### 1.4 Session Pruner
**File**: `session.go:454-468`

```go
ticker := time.NewTicker(ss.sessionTTL / 2)  // 12h
for {
    select {
    case <-ticker.C:
        ss.pruneExpired()
    }
}
```

- **Purpose**: Removes inactive sessions
- **Interval**: TTL/2 (default: 12h)
- **Default TTL**: 24h
- **Started**: `SessionStore.StartPruner(ctx)`

---

### 1.5 Daemon Health Loop
**File**: `daemon_manager.go:262-274`

```go
ticker := time.NewTicker(healthCheckFreq)  // 30s
for {
    select {
    case <-ticker.C:
        dm.healthCheck()
    }
}
```

- **Purpose**: Health checks for managed daemon processes
- **Interval**: 30 seconds
- **Action**: Cleans up dead processes, updates status
- **Started**: `NewDaemonManager()`

---

### 1.6 Metrics Collector (NEW)
**File**: `metrics_collector.go`

```go
ticker := time.NewTicker(m.interval)  // 1min default
for {
    select {
    case <-ticker.C:
        snapshot := m.collect()
        m.notifySubscribers(snapshot)
        m.sendWebhook(snapshot)
    }
}
```

- **Purpose**: Collects and aggregates system metrics
- **Interval**: Configurable (default: 1 min)
- **Metrics**: Messages, tokens, agent runs, tools, goroutines, memory, latency, DB size
- **Output**: Webhook or subscriber channel
- **Started**: `MetricsCollector.Start(ctx)`

**Configuration**:
```yaml
routines:
  metrics:
    enabled: true
    interval: 1m
    webhook: ""  # Optional webhook URL for external reporting
```

---

### 1.7 Memory Indexer (NEW)
**File**: `memory_indexer.go`

```go
ticker := time.NewTicker(m.interval)  // 5min default
for {
    select {
    case <-ticker.C:
        m.indexAll()  // Incremental hash-based reindex
    }
}
```

- **Purpose**: Incremental indexing of memory files for FTS5/vector search
- **Interval**: Configurable (default: 5 min)
- **Method**: SHA-256 hash comparison, only reindexes changed files
- **Started**: `MemoryIndexer.Start(ctx)`

**Configuration**:
```yaml
routines:
  memory_indexer:
    enabled: true
    interval: 5m
```

---

## 2. On-Demand Routines (Per-Operation)

### 2.1 Message Handlers
**File**: `assistant.go:940`

```go
go a.handleMessage(msg)
```

- **Purpose**: Processes each message asynchronously
- **Trigger**: Message arrival in message loop
- **Goroutines**: 1 per message

---

### 2.2 Typing Indicator
**File**: `assistant.go:1243-1254`

```go
go func() {
    ticker := time.NewTicker(8 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            a.channelMgr.SendTyping(chatID, true)
        case <-typingDone:
            return
        }
    }
}()
```

- **Purpose**: Keeps "typing..." indicator active
- **Interval**: 8 seconds
- **Trigger**: Start of message processing
- **Ends**: When agent finishes response

---

### 2.3 Media Enrichment
**File**: `assistant.go:1261`

```go
go a.enrichMediaAsync(ctx, msg, sessionID, logger)
```

- **Purpose**: Processes images/audio in parallel with agent
- **Trigger**: Message contains pending media
- **Result**: Injects via interrupt channel

---

### 2.4 Auto-Capture Facts
**File**: `assistant.go:1292`

```go
go a.autoCaptureFacts(userContent, response, sessionID)
```

- **Purpose**: Extracts important facts from conversation for memory
- **Trigger**: After each agent response
- **Analysis**: LLM identifies persistent information

---

### 2.5 Session Compaction
**File**: `assistant.go:1298`

```go
go a.maybeCompactSession(session)
```

- **Purpose**: Compacts history when approaching limit
- **Trigger**: After each response
- **Strategies**: summarize, truncate, sliding

---

### 2.6 Subagent Execution
**File**: `subagent.go:537-591`

```go
go func() {
    defer close(run.done)
    agent.Run(ctx, systemPrompt, task, tools)
}()
```

- **Purpose**: Runs child agent in isolated context
- **Trigger**: Tool `spawn_subagent`
- **Maximum**: 8 concurrent

---

### 2.7 Parallel Tool Execution
**File**: `tool_executor.go:489-496`

```go
for i, call := range calls {
    go func(idx int, tc ToolCall) {
        results[idx] = e.executeSingle(ctx, tc)
    }(i, call)
}
```

- **Purpose**: Executes multiple tools in parallel
- **Trigger**: Multiple tool_calls in same turn
- **Limit**: 5 concurrent (semaphore)

---

### 2.8 Webhook Delivery
**File**: `webhooks.go:194`

```go
go wm.sendWebhook(wh, payload)
```

- **Purpose**: Sends event to HTTP endpoint
- **Trigger**: Hook event fire
- **Retry**: Exponential backoff

---

### 2.9 Block Streamer Idle Timer
**File**: `block_streamer.go:190-212`

```go
time.AfterFunc(idleDuration, func() {
    if len(buffer) >= minChars {
        flush()
    }
})
```

- **Purpose**: Flush text when LLM pauses
- **Interval**: 1.5s (configurable)
- **Reset**: On each token received

---

### 2.10 Message Queue Debounce
**File**: `message_queue.go:138-143`

```go
time.AfterFunc(dur, func() {
    msgs := q.Drain(sid)
    q.onDrain(msgs)
})
```

- **Purpose**: Batches messages when session busy
- **Interval**: 500ms followup / 200ms default
- **Idle**: Immediate drain

---

### 2.11 Interrupted Run Recovery
**File**: `assistant.go:2888-2938`

```go
go func(run interruptedRun) {
    time.Sleep(2 * time.Second)
    // resume run
}(run)
```

- **Purpose**: Resumes interrupted runs (crash/shutdown)
- **Trigger**: Startup, for each interrupted run
- **Delay**: 2 seconds

---

### 2.12 Canvas Server
**File**: `canvas_host.go:189-193`

```go
go func() {
    canvas.server.Serve(ln)
}()
```

- **Purpose**: HTTP server for interactive canvas
- **Trigger**: Tool `canvas_create`
- **Lifetime**: Until `canvas_stop`

---

### 2.13 Lane Task Execution
**File**: `lanes.go:82-83, 146`

```go
go l.runTask(ctx, task)
```

- **Purpose**: Executes tasks with per-lane concurrency control
- **Lanes**: session, cron, subagent
- **Control**: Semaphore per lane

---

## 3. Scheduler Routines (User-Defined)

### Cron Jobs
**File**: `scheduler.go`

```go
// User-created scheduled jobs
schedule_add(label, schedule, command)
```

- **Purpose**: Jobs defined by user via tool/chat
- **Types**: cron, once
- **Persistence**: SQLite or JSON

---

## 4. Daemon Management Routines

### 4.1 Daemon Wait
**File**: `daemon_manager.go:95-108`

```go
go func() {
    err := cmd.Wait()
    dm.updateStatus(id, status)
}()
```

- **Purpose**: Waits for each daemon's exit
- **Trigger**: `StartDaemon()`
- **1 per daemon**

---

### 4.2 Daemon Ready Pattern Wait
**File**: `daemon_manager.go:114-129`

```go
ticker := time.NewTicker(200 * time.Millisecond)
deadline := time.After(30 * time.Second)
for {
    select {
    case <-ticker.C:
        if pattern.Match(output) { ready = true; return }
    case <-deadline:
        return // timeout
    }
}
```

- **Purpose**: Waits for "ready" pattern in stdout
- **Timeout**: 30 seconds
- **Check interval**: 200ms

---

## 5. Summary by Category

| Category | Routines | Type |
|----------|----------|------|
| **Event Pumps** | Message Loop | Continuous |
| **Periodic Tickers** | Heartbeat, Config Watcher, Session Pruner, Daemon Health, Metrics Collector, Memory Indexer | Interval |
| **Per-Message** | Handler, Typing, Media, Facts, Compaction | On-demand |
| **Per-Tool** | Subagent, Canvas, Parallel Tools | On-demand |
| **Per-Event** | Webhooks, Hook Dispatch | On-demand |
| **Per-Daemon** | Wait, Ready Pattern | On-demand |
| **Recovery** | Interrupted Run | Startup |
| **User-Defined** | Cron Jobs | Scheduled |

---

## 6. Potential Future Routines

### 6.1 Skill Updater
**Purpose**: Check for updates to installed skills

```go
go func() {
    ticker := time.NewTicker(24 * time.Hour)
    for range ticker.C {
        a.skillRegistry.CheckUpdates()
    }
}()
```

---

### 6.2 Vault Key Rotation Reminder
**Purpose**: Alert about password rotation

```go
go func() {
    ticker := time.NewTicker(7 * 24 * time.Hour)  // weekly
    for range ticker.C {
        if vault.Age() > 90*24*time.Hour {
            a.notify("Vault password > 90 days, consider rotation")
        }
    }
}()
```

---

### 6.3 Stale Lock Cleaner
**Purpose**: Clean orphaned locks from interrupted operations

```go
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        a.lockManager.CleanStale()
    }
}()
```

---

## 7. Goroutine Count by Scenario

| Scenario | Expected Goroutines |
|----------|---------------------|
| Idle (no activity) | 7-9 (continuous loops) |
| 1 message processing | +5-8 (handler, typing, media, facts, etc) |
| 1 subagent running | +1 |
| 5 parallel tools | +5 |
| 1 active canvas | +2 (server + SSE) |
| 3 running daemons | +6 (wait + ready each) |
| **Typical peak** | 25-45 goroutines |

---

## 8. Configuration Reference

```yaml
# Internal Routines Configuration
routines:
  # Metrics Collector
  metrics:
    enabled: true
    interval: 1m           # Collection interval
    webhook: ""            # Optional webhook URL for external reporting

  # Memory Indexer
  memory_indexer:
    enabled: true
    interval: 5m           # Indexing interval
```

---

## 9. Monitoring

For debugging, these endpoints are available:

```
GET /api/debug/goroutines   # List active goroutines
GET /api/metrics            # Current metrics snapshot
```

---

## Files Reference

| File | Purpose |
|------|---------|
| `metrics_collector.go` | Background metrics collection and reporting |
| `memory_indexer.go` | Incremental memory file indexing |
| `heartbeat.go` | Proactive agent behavior scheduler |
| `config_watcher.go` | Hot-reload configuration |
| `session.go` | Session management and pruning |
| `daemon_manager.go` | Background process management |
| `lanes.go` | Concurrency-controlled task execution |
