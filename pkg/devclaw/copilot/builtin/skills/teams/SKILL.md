---
name: teams
description: "Manage persistent agents and team coordination with shared memory"
trigger: automatic
---

# Agent Teams

Teams system for persistent agents with shared memory, tasks, and communication.

## Architecture
```
┌─────────────────────────────────────────────────────────────────┐
│                         Team                                     │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    Shared Memory                         │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │    │
│  │  │   Facts     │  │  Documents  │  │   Standup   │      │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘      │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │   Agent A   │  │   Agent B   │  │   Agent C   │              │
│  │  ┌───────┐  │  │  ┌───────┐  │  │  ┌───────┐  │              │
│  │  │Task 1 │  │  │  │Task 2 │  │  │  │Task 3 │  │              │
│  │  └───────┘  │  │  └───────┘  │  │  └───────┘  │              │
│  └─────────────┘  └─────────────┘  └─────────────┘              │
│         │                │                │                      │
│         └────────────────┼────────────────┘                      │
│                          │                                       │
│                   ┌──────┴──────┐                               │
│                   │ Communication│                               │
│                   └─────────────┘                               │
└─────────────────────────────────────────────────────────────────┘
```

## Tools Overview
| Tool | Actions |
|------|---------|
| `team_manage` | create, list, get, update, delete |
| `team_agent` | create, list, get, update, start, stop, delete, working_get, working_update, working_clear |
| `team_task` | create, list, get, update, assign, delete |
| `team_memory` | fact_save, fact_list, fact_delete, doc_create, doc_list, doc_get, doc_update, doc_delete, standup |
| `team_comm` | comment, mention_check, send_message, notify, notify_list |

## Task Status Workflow
```
inbox → assigned → in_progress → review → done
                     │
                     └──→ blocked
                           │
                           └──→ cancelled
```

## Agent Heartbeat Pattern
```
1. CHECK INCOMING → team_comm(action="mention_check", agent_id="X")
2. CHECK WORKING  → team_agent(action="working_get", agent_id="X")
3. CHECK TASKS    → team_task(action="list", assignee_filter="X")
4. DO WORK        → team_agent(action="working_update", ...)
5. NOTIFY         → team_comm(action="notify", type="...", ...)
6. CLEAR STATE    → team_agent(action="working_clear", ...)
```

## Notification Types
| Type | When | Priority |
|------|------|----------|
| `task_completed` | Task finished | 3 |
| `task_failed` | Task failed | 2 |
| `task_blocked` | Waiting for input | 3 |
| `task_progress` | Progress update | 4 |
| `agent_error` | Internal error | 2 |

## Working State Management
```bash
# Get current state
team_agent(action="working_get", agent_id="siri")

# Update working state
team_agent(
  action="working_update",
  agent_id="siri",
  current_task_id="abc12345",
  status="working",
  next_steps="1. Process request\n2. Send response",
  context="Handling user query"
)

# Clear working state
team_agent(action="working_clear", agent_id="siri")
```

## Shared Memory

### Facts
```bash
team_memory(action="fact_save", key="api_version", value="v2.1.0")
team_memory(action="fact_list")
team_memory(action="fact_delete", key="old_config")
```

### Documents
```bash
team_memory(
  action="doc_create",
  title="API Design Document",
  doc_type="deliverable",
  content="# API Design\n...",
  task_id="abc12345"
)
team_memory(action="doc_list", doc_type="deliverable")
```

## Communication
```bash
# Comment on task
team_comm(action="comment", task_id="abc12345", content="@loki can you review?")

# Check mentions
team_comm(action="mention_check", agent_id="loki")

# Send direct message
team_comm(action="send_message", to_agent="jarvis", content="Deployment complete")

# Send notification
team_comm(action="notify", type="task_completed", message="Done!", task_id="abc12345", priority=3)
```

## Complete Workflow Example

### Heartbeat Cycle
```bash
# 1. Check for mentions
team_comm(action="mention_check", agent_id="siri")

# 2. Check working state
team_agent(action="working_get", agent_id="siri")

# 3. Check assigned tasks
team_task(action="list", assignee_filter="siri")

# 4. Start working
team_agent(action="working_update", agent_id="siri", current_task_id="abc12345", status="working")

# 5. Do work...

# 6. Complete and notify
team_task(action="update", task_id="abc12345", status="done")
team_comm(action="notify", type="task_completed", message="Done!", task_id="abc12345")

# 7. Clear state
team_agent(action="working_clear", agent_id="siri")
```

## Tips
- Always update working state before/after tasks
- Notify on completion/errors
- Use facts for shared knowledge
- Clear working state when done

## Common Mistakes
| Mistake | Correct Approach |
|---------|------------------|
| Not updating working state | Update before/after each task |
| Forgetting to notify | Always notify on task completion |
| Leaving working state stale | Clear when task complete |
