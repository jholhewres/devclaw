# DevClaw Teams — Persistent Agents & Shared Memory

Documentation for the Teams system that adds persistent agent management and team coordination capabilities to DevClaw.

---

## Overview

DevClaw Teams extends the existing subagent architecture with:

- **Persistent Agents**: Long-lived agents with specific roles, personalities, and instructions
- **Team Memory**: Shared state accessible by all team members (tasks, messages, facts, documents)
- **Agent Communication**: Inter-agent messaging via @mentions and direct messages
- **Heartbeat Integration**: Periodic wake-ups for proactive behavior
- **Thread Subscriptions**: Auto-subscribe to threads for continuous notifications
- **Working State**: Persist work-in-progress across heartbeats (WORKING.md pattern)
- **Active Notifications**: Trigger agents immediately on @mentions

```
┌─────────────────────────────────────────────────────────────┐
│                      TeamManager                             │
│  - CreateTeam / CreateAgent                                 │
│  - Heartbeat scheduling (via Scheduler)                     │
│  - Agent lifecycle (start, stop, delete)                    │
│  - @mention parsing and routing                             │
│  - Working state persistence (WORKING.md)                   │
│  - Active notification push (spawn callback)                │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│                      TeamMemory                              │
│  - Tasks (CRUD, status workflow)                            │
│  - Messages (@mentions, mailbox)                            │
│  - Facts (shared key-value store)                           │
│  - Activities (audit trail)                                 │
│  - Documents (deliverables, research, protocols)            │
│  - Thread Subscriptions (auto, mentioned, assigned)         │
│  - Standup generation                                       │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│               Existing Architecture                          │
│  - SQLite DB (new tables added)                             │
│  - Scheduler (for heartbeats)                               │
│  - SubagentManager (unchanged, works in parallel)           │
└─────────────────────────────────────────────────────────────┘
```

---

## Core Concepts

### Teams

A team is a collection of persistent agents working together:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique 8-char identifier |
| `name` | string | Team name |
| `description` | string | Team purpose |
| `owner_jid` | string | Owner's JID (user identifier) |
| `default_model` | string | Default LLM model for agents |
| `workspace_path` | string | Optional workspace directory |
| `enabled` | bool | Team active status |

### Persistent Agents

Unlike subagents (ephemeral), persistent agents have:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Derived from name (lowercase, alphanumeric) |
| `name` | string | Display name (e.g., "Jarvis", "Loki") |
| `role` | string | Role description (e.g., "Squad Lead") |
| `team_id` | string | Parent team ID |
| `level` | enum | `junior`, `mid`, `senior`, `lead` |
| `status` | enum | `idle`, `working`, `paused`, `stopped` |
| `personality` | string | Custom personality traits |
| `instructions` | string | Specific instructions for this agent |
| `model` | string | LLM model override |
| `skills` | []string | List of skill names |
| `heartbeat_schedule` | string | Cron expression for wake-ups |

### Agent Levels

| Level | Description |
|-------|-------------|
| `junior` | Entry-level agent, simple tasks |
| `mid` | Standard agent, moderate complexity |
| `senior` | Advanced agent, complex tasks |
| `lead` | Leadership role, coordination |

### Agent Status

| Status | Description |
|--------|-------------|
| `idle` | Available for work |
| `working` | Currently executing a task |
| `paused` | Temporarily inactive |
| `stopped` | Disabled (no heartbeats) |

---

## Team Memory

### Tasks

Tasks represent work items tracked in shared memory:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique 8-char identifier |
| `title` | string | Task title |
| `description` | string | Detailed description |
| `status` | enum | Current status |
| `assignees` | []string | Assigned agent IDs |
| `priority` | int | 1-5 priority level |
| `labels` | []string | Tags for categorization |
| `created_by` | string | Creator identifier |
| `completed_at` | time | Completion timestamp |

#### Task Status Workflow

```
inbox → assigned → in_progress → review → done
                     │
                     └──→ blocked
                           │
                           └──→ cancelled
```

| Status | Description |
|--------|-------------|
| `inbox` | New task, not yet assigned |
| `assigned` | Assigned to agent(s), not started |
| `in_progress` | Work in progress |
| `review` | Needs review before completion |
| `done` | Completed successfully |
| `blocked` | Blocked by external dependency |
| `cancelled` | Cancelled, will not complete |

### Messages & @Mentions

Agents communicate via messages with @mentions:

```
PostMessage(thread_id, from_agent, content, mentions)
    │
    ├── Stores message in team_messages
    ├── Creates pending messages for mentioned agents
    └── Logs activity
```

When an agent is mentioned (`@jarvis check this`), a pending message is added to their mailbox. The agent can retrieve these on their next heartbeat or when triggered.

### Facts (Shared Memory)

Key-value store for team-wide knowledge:

```yaml
# Example facts
project_name: "DevClaw Teams"
api_endpoint: "https://api.example.com"
sprint_goal: "Complete authentication flow"
```

- Facts are unique by key within a team
- Updates overwrite existing values
- Authors are tracked for audit

### Activities

Audit trail of team actions:

| Type | Description |
|------|-------------|
| `task_created` | New task created |
| `task_updated` | Task status changed |
| `task_assigned` | Task assigned to agent |
| `message_sent` | Message posted |
| `mention` | Agent mentioned |
| `fact_created` | Fact saved |
| `document_created` | Document created |
| `document_updated` | Document updated |
| `subscribed` | Agent subscribed to thread |

### Documents

Store deliverables and research linked to tasks:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique 8-char identifier |
| `title` | string | Document title |
| `doc_type` | enum | `deliverable`, `research`, `protocol`, `notes` |
| `content` | string | Document content |
| `format` | string | `markdown`, `code`, `json`, `image` |
| `task_id` | string | Linked task ID (optional) |
| `version` | int | Version number (auto-incremented) |
| `author` | string | Creator agent ID |

### Thread Subscriptions

Agents automatically subscribe to threads for continuous notifications:

| Reason | Trigger |
|--------|---------|
| `auto` | Posted a message in thread |
| `mentioned` | Was @mentioned in thread |
| `assigned` | Assigned to linked task |

When a new message is posted, ALL subscribers receive a pending message notification—not just explicitly @mentioned agents.

### Working State (WORKING.md)

Each agent can persist their current work state:

| Field | Type | Description |
|-------|------|-------------|
| `agent_id` | string | Agent identifier |
| `current_task_id` | string | Task being worked on |
| `status` | string | `idle`, `working`, `blocked`, `waiting` |
| `next_steps` | string | Markdown checklist of next steps |
| `context` | string | Context for resuming work |

This allows agents to resume work across heartbeats without losing context.

---

## Heartbeat System

Persistent agents can be woken up periodically via the scheduler:

```yaml
# Agent configuration
heartbeat_schedule: "*/15 * * * *"  # Every 15 minutes
```

### Heartbeat Checklist

When triggered, the agent:

1. Checks `WORKING.md` for ongoing tasks (via `team_get_working`)
2. Resumes any in-progress work (using saved context and next steps)
3. Checks TeamMemory for @mentions and subscribed thread notifications
4. Reviews assigned tasks
5. Scans activity feed for relevant updates

### Working State Persistence

The agent's working state is automatically included in the heartbeat prompt:

```
## Current Work State (WORKING.md)
- Status: working
- Current Task: xyz78901
- Next Steps:
1. Complete authentication
2. Add tests
3. Review with team
```

This allows agents to seamlessly resume work across heartbeats.

### Response Protocol

- **Has work**: Execute the work, update working state with `team_update_working`
- **No work**: Respond with exactly `HEARTBEAT_OK`

### Active Notification Push

When an agent is @mentioned or a subscribed thread has activity, the agent is triggered immediately—without waiting for the scheduled heartbeat. This enables real-time collaboration between agents.

---

## Tools Reference

### Team Management

| Tool | Description | Permission |
|------|-------------|------------|
| `team_create` | Create a new team | admin |
| `team_list` | List all teams | user |
| `team_create_agent` | Create a persistent agent | admin |
| `team_list_agents` | List agents in a team | user |
| `team_stop_agent` | Stop an agent (disable heartbeats) | admin |
| `team_delete_agent` | Permanently delete an agent | admin |

### Task Management

| Tool | Description | Permission |
|------|-------------|------------|
| `team_create_task` | Create a task | user |
| `team_list_tasks` | List tasks (filterable) | user |
| `team_update_task` | Update task status | user |
| `team_assign_task` | Assign agents to task | admin |

### Communication

| Tool | Description | Permission |
|------|-------------|------------|
| `team_comment` | Add comment to task thread | user |
| `team_check_mentions` | Check pending @mentions | user |
| `team_send_message` | Send direct message to agent | user |
| `team_mention` | Mention another agent | user |

### Shared Memory

| Tool | Description | Permission |
|------|-------------|------------|
| `team_save_fact` | Save a fact | user |
| `team_get_facts` | Get all facts | user |
| `team_delete_fact` | Delete a fact | admin |
| `team_standup` | Generate daily standup | user |

### Documents

| Tool | Description | Permission |
|------|-------------|------------|
| `team_create_document` | Create a document | user |
| `team_list_documents` | List documents (filterable) | user |
| `team_get_document` | Get document by ID | user |
| `team_update_document` | Update document content | user |
| `team_delete_document` | Delete a document | admin |

### Working State

| Tool | Description | Permission |
|------|-------------|------------|
| `team_get_working` | Get agent's working state | user |
| `team_update_working` | Update working state | user |
| `team_clear_working` | Clear working state (task done) | user |

---

## Usage Examples

### Creating a Team

```bash
# Via conversation
"Create a team called Engineering with description 'Main development team'"

# Via tool call
team_create(
  name="Engineering",
  description="Main development team",
  owner_jid="user@example.com"
)
```

### Creating Agents

```bash
# Create a squad lead
team_create_agent(
  team_id="abc12345",
  name="Jarvis",
  role="Squad Lead",
  personality="Professional, proactive, detail-oriented",
  instructions="Coordinate team activities, assign tasks, generate reports",
  model="claude-sonnet",
  skills=["planning", "coordination"],
  level="lead",
  heartbeat_schedule="*/15 * * * *"
)

# Create a writer
team_create_agent(
  team_id="abc12345",
  name="Loki",
  role="Technical Writer",
  personality="Creative, thorough, articulate",
  instructions="Write documentation, blog posts, and technical guides",
  model="claude-sonnet",
  skills=["writing", "documentation"],
  level="mid",
  heartbeat_schedule="*/30 * * * *"
)
```

### Task Workflow

```bash
# Create a task
team_create_task(
  title="Implement authentication",
  description="Add OAuth2 support with Google and GitHub providers",
  assignees=["jarvis", "loki"]
)

# Update status
team_update_task(
  task_id="xyz78901",
  status="in_progress",
  comment="Starting implementation"
)

# Complete
team_update_task(
  task_id="xyz78901",
  status="done",
  comment="OAuth2 implemented and tested"
)
```

### Agent Communication

```bash
# Mention another agent
team_comment(
  thread_id="xyz78901",
  content="@loki can you write the docs for this endpoint?",
  mentions=["loki"]
)

# Check mentions (agent calls this on heartbeat)
team_check_mentions(mark_delivered=true)

# Send direct message
team_send_message(
  to_agent="jarvis",
  message="The deployment is complete"
)
```

### Shared Facts

```bash
# Save facts
team_save_fact(key="api_version", value="v2.1.0")
team_save_fact(key="deploy_endpoint", value="https://deploy.example.com")

# Retrieve facts
team_get_facts()

# Generate standup
team_standup()
```

### Documents

```bash
# Create a deliverable document
team_create_document(
  title="API Design Document",
  doc_type="deliverable",
  content="# API Design\n\n## Endpoints\n...",
  format="markdown",
  task_id="xyz78901"
)

# Create research notes
team_create_document(
  title="Performance Analysis",
  doc_type="research",
  content="## Results\n...",
  format="markdown"
)

# List documents for a task
team_list_documents(task_id="xyz78901")

# Update document (version auto-incremented)
team_update_document(
  doc_id="abc12345",
  content="# API Design v2\n..."
)
```

### Working State (WORKING.md)

```bash
# Save work in progress
team_update_working(
  status="working",
  current_task_id="xyz78901",
  next_steps="1. Complete authentication\n2. Add tests\n3. Review with team",
  context="Implementing OAuth2 with Google provider"
)

# Check current working state
team_get_working()

# Clear when task is done
team_clear_working()
```

### Thread Subscriptions

```bash
# Agents auto-subscribe when:
# 1. They post a message in a thread
# 2. They are @mentioned in a thread
# 3. They are assigned to a linked task

# Example: Post message (auto-subscribes sender and mentioned)
team_comment(
  thread_id="task-xyz78901",
  content="@loki can you review the docs?",
  mentions=["loki"]
)

# Both jarvis (sender) and loki (mentioned) are now subscribed
# Any future messages in this thread will notify both
```

---

## Database Schema

### New Tables

```sql
-- Teams
CREATE TABLE teams (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    owner_jid TEXT NOT NULL,
    default_model TEXT DEFAULT '',
    workspace_path TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    enabled INTEGER DEFAULT 1
);

-- Persistent Agents
CREATE TABLE persistent_agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    team_id TEXT NOT NULL,
    level TEXT DEFAULT 'mid',
    status TEXT DEFAULT 'idle',
    personality TEXT DEFAULT '',
    instructions TEXT DEFAULT '',
    model TEXT DEFAULT '',
    skills TEXT DEFAULT '[]',
    heartbeat_schedule TEXT DEFAULT '*/15 * * * *',
    current_task_id TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    last_active_at TEXT DEFAULT '',
    last_heartbeat_at TEXT DEFAULT ''
);

-- Team Tasks
CREATE TABLE team_tasks (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    status TEXT DEFAULT 'inbox',
    assignees TEXT DEFAULT '[]',
    priority INTEGER DEFAULT 3,
    labels TEXT DEFAULT '[]',
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT DEFAULT '',
    blocked_reason TEXT DEFAULT ''
);

-- Team Messages
CREATE TABLE team_messages (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    thread_id TEXT DEFAULT '',
    from_agent TEXT DEFAULT '',
    from_user TEXT DEFAULT '',
    content TEXT NOT NULL,
    mentions TEXT DEFAULT '[]',
    created_at TEXT NOT NULL,
    delivered INTEGER DEFAULT 0
);

-- Pending Messages (Mailbox)
CREATE TABLE team_pending_messages (
    id TEXT PRIMARY KEY,
    to_agent TEXT NOT NULL,
    from_agent TEXT DEFAULT '',
    from_user TEXT DEFAULT '',
    content TEXT NOT NULL,
    thread_id TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    delivered INTEGER DEFAULT 0,
    delivered_at TEXT DEFAULT ''
);

-- Team Facts
CREATE TABLE team_facts (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    author TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(team_id, key)
);

-- Team Activities
CREATE TABLE team_activities (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    type TEXT NOT NULL,
    agent_id TEXT DEFAULT '',
    message TEXT NOT NULL,
    related_id TEXT DEFAULT '',
    created_at TEXT NOT NULL
);

-- Team Documents
CREATE TABLE team_documents (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    task_id TEXT DEFAULT '',
    title TEXT NOT NULL,
    doc_type TEXT DEFAULT 'deliverable',
    content TEXT NOT NULL,
    format TEXT DEFAULT 'markdown',
    file_path TEXT DEFAULT '',
    version INTEGER DEFAULT 1,
    author TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Thread Subscriptions
CREATE TABLE team_thread_subscriptions (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    thread_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    subscribed_at TEXT NOT NULL,
    reason TEXT DEFAULT 'auto',
    UNIQUE(team_id, thread_id, agent_id)
);

-- Agent Working State
CREATE TABLE agent_working_state (
    agent_id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    current_task_id TEXT DEFAULT '',
    status TEXT DEFAULT 'idle',
    next_steps TEXT DEFAULT '',
    context TEXT DEFAULT '',
    updated_at TEXT NOT NULL
);
```

---

## Integration with Existing Systems

### SubagentManager

Teams work alongside the existing subagent system:

- **Subagents**: Ephemeral, spawned for parallel work, discarded after completion
- **Persistent Agents**: Long-lived, maintain state, managed via TeamManager

Both can be used together:
```
Main Agent → spawn_subagent → Subagent (parallel task)
         ↘ TeamManager → PersistentAgent (long-running role)
```

### Scheduler

Persistent agent heartbeats use the existing scheduler:

```go
// TeamManager creates heartbeat jobs automatically
func (tm *TeamManager) createHeartbeatJob(agent *PersistentAgent) {
    job := &scheduler.Job{
        ID:       fmt.Sprintf("heartbeat-%s", agent.ID),
        Schedule: agent.HeartbeatSchedule,
        Type:     "cron",
        Command:  tm.buildHeartbeatPrompt(agent),
        Enabled:  true,
    }
    tm.scheduler.Add(job)
}
```

### Session Isolation

Each agent run uses session isolation:
- Separate session ID per agent
- Independent conversation history
- Isolated tool permissions

---

## Standup Generation

The `team_standup` tool generates a daily standup summary:

```
📊 DAILY STANDUP — Feb 21, 2026

✅ COMPLETED TODAY
• Implement authentication (by jarvis)
• Write API docs (by loki)

🔄 IN PROGRESS
• Add unit tests (jarvis)

🚫 BLOCKED
• Deploy to staging — Waiting for CI credentials

👀 NEEDS REVIEW
• OAuth callback handler
```

---

## Best Practices

### Agent Design

1. **Clear Roles**: Each agent should have a well-defined role
2. **Specific Instructions**: Provide detailed instructions for expected behavior
3. **Appropriate Heartbeat**: Set heartbeat interval based on task frequency
4. **Skill Alignment**: Assign skills that match the agent's role

### Task Management

1. **Descriptive Titles**: Use clear, actionable task titles
2. **Proper Assignment**: Assign tasks to appropriate agents
3. **Status Updates**: Keep task status current
4. **Use Comments**: Add context via thread comments

### Communication

1. **Use @Mentions**: Direct messages to specific agents
2. **Thread Context**: Keep related messages in the same thread
3. **Facts for Knowledge**: Store persistent knowledge as facts
4. **Activity Awareness**: Monitor activity feed for team updates

---

## Troubleshooting

### Agent Not Responding to Heartbeats

1. Check agent status (`team_list_agents`)
2. Verify scheduler job exists
3. Check heartbeat schedule expression
4. Review agent logs

### Missing @Mentions

1. Verify agent ID matches the mention
2. Check pending messages (`team_check_mentions`)
3. Ensure agent is in the same team

### Task Not Showing

1. Verify team ID matches
2. Check task status filter
3. Ensure proper assignment

---

## Files Reference

| File | Purpose |
|------|---------|
| `team_types.go` | Data structures (Team, PersistentAgent, TeamTask, etc.) |
| `team_memory.go` | TeamMemory implementation (tasks, messages, facts) |
| `team_manager.go` | TeamManager implementation (lifecycle, heartbeats) |
| `team_tools.go` | Tool registration and handlers |
| `team_memory_test.go` | Unit tests for TeamMemory |
| `team_manager_test.go` | Unit tests for TeamManager |
