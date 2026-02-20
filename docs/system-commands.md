# System Commands

DevClaw provides system administration commands that allow operators to manage the system remotely via chat channels (WhatsApp, Discord, Telegram, Slack) without requiring SSH access to the server.

## Overview

All system commands require **admin** or **owner** level access. Regular users cannot execute these commands.

| Command | Description |
|---------|-------------|
| `/reload` | Reload configuration sections |
| `/status` | Display comprehensive system status |
| `/diagnostics` | Run system diagnostics |
| `/exec` | Manage execution approvals |
| `/channels` | Manage communication channels |
| `/maintenance` | Control maintenance mode |
| `/logs` | View audit logs |
| `/health` | Run health checks |
| `/metrics` | Display usage metrics |

---

## Configuration Reload

### `/reload [section]`

Reloads configuration from disk without restarting the service.

**Arguments:**
- `section` (optional): Specific section to reload. If omitted, reloads all safe sections.

**Available Sections:**
| Section | Description |
|---------|-------------|
| `access` | Access control policies (allowlist, blocklist) |
| `instructions` | System prompt instructions |
| `tools` | Tool guard configuration (permissions, dangerous commands) |
| `heartbeat` | Heartbeat/proactive check configuration |
| `budget` | Token budget limits |
| `all` | Reload all above sections (default) |

**Examples:**
```
/reload                  # Reload all safe sections
/reload access           # Reload only access control
/reload instructions     # Reload only instructions
/reload tools            # Reload tool permissions
```

**Response:**
```
✅ Reloaded: access, instructions, tool_guard, heartbeat, token_budget
```

---

## System Status

### `/status [--json]`

Displays comprehensive system status including version, uptime, memory, channels, sessions, scheduler, and skills.

**Arguments:**
- `--json`: Output in JSON format (useful for scripts)

**Example Output:**
```
*DevClaw Status*

📊 *System*
  Version: 1.6.1
  Uptime: 2d 5h 30m
  Memory: 45.2 MB
  Goroutines: 25

📡 *Channels*
  ✅ whatsapp (errors: 0)
  ✅ telegram (errors: 0)
  ❌ discord (errors: 3)

💬 *Sessions*
  Active: 3 | Total: 15

⏰ *Scheduler*
  Jobs: 5 | Next: 2024-01-16 08:00

🎯 *Skills*: 12
```

---

## Diagnostics

### `/diagnostics [--full]`

Runs comprehensive system diagnostics including database health, configuration validation, channel connectivity, memory usage, and disk space.

**Arguments:**
- `--full`: Include recent errors from the last 24 hours

**Example Output:**
```
*Diagnostics Report*

📦 *Database*
  Status: ✅ Connected
  Size: 12.3 MB
  session_entries: 1523 rows
  audit_log: 456 rows
  jobs: 5 rows

⚙️ *Config*
  Status: ✅ Valid
  Path: ./config.yaml

📡 *Channels*
  ✅ whatsapp: OK
  ❌ discord: Errors: 3

💾 *Memory*
  Alloc: 32.1 MB
  Sys: 64.5 MB
  GC cycles: 15

💿 *Disk*
  Total: 50.0 GB
  Free: 25.3 GB (49.4% used)
```

---

## Execution Queue

### `/exec queue`

Lists all pending tool execution approvals waiting for user response.

**Example Output:**
```
📋 Pending approvals: Use /approve <id> or /deny <id> to resolve
```

### `/exec approve <id>`

Approves a pending tool execution.

**Arguments:**
- `id`: The approval ID (can be omitted if only one pending)

**Example:**
```
/exec approve abc-123
✅ Approved.
```

### `/exec deny <id> [reason]`

Denies a pending tool execution.

**Arguments:**
- `id`: The approval ID
- `reason` (optional): Reason for denial

**Example:**
```
/exec deny abc-123 Potentially dangerous command
❌ Denied.
```

---

## Channel Management

### `/channels`

Lists all registered channels with their connection status.

**Example Output:**
```
*Channels*

whatsapp
  Status: ✅ Connected
  Errors: 0
  Last msg: 5m ago

telegram
  Status: ✅ Connected
  Errors: 0
  Last msg: 1h ago

discord
  Status: ❌ Disconnected
  Errors: 3
```

### `/channels connect <name>`

Connects a specific channel.

**Arguments:**
- `name`: Channel name (e.g., `whatsapp`, `discord`, `telegram`, `slack`)

**Example:**
```
/channels connect discord
✅ Channel discord connected
```

### `/channels disconnect <name>`

Disconnects a specific channel.

**Arguments:**
- `name`: Channel name

**Example:**
```
/channels disconnect discord
✅ Channel discord disconnected
```

---

## Maintenance Mode

Maintenance mode allows you to temporarily block regular messages while still allowing admin commands. Useful during system updates or troubleshooting.

### `/maintenance`

Shows current maintenance mode status.

**Example Output:**
```
🔧 Maintenance mode: OFF
```

### `/maintenance on [message]`

Enables maintenance mode with an optional custom message.

**Arguments:**
- `message` (optional): Custom message to show users

**Example:**
```
/maintenance on System update in progress
✅ Maintenance mode enabled
```

When maintenance mode is active and a user sends a regular message:
```
System update in progress
```

### `/maintenance off`

Disables maintenance mode.

**Example:**
```
/maintenance off
✅ Maintenance mode disabled
```

**Behavior:**
- Blocks all regular messages (returns custom message)
- Allows all admin commands to pass through
- Persists across restarts (stored in database)

---

## Audit Logs

### `/logs [level] [lines]`

Views recent entries from the audit log.

**Arguments:**
- `level` (optional): Filter by level - `audit` (default), `error`, `all`
- `lines` (optional): Number of lines to show (default: 20)

**Example Output:**
```
*Audit Log* (last 10)

✅ [1234] bash
   ls -la
❌ [1233] ssh
   host: production-server
✅ [1232] read_file
   path: config.yaml
```

---

## Health Check

### `/health`

Runs active health checks on all system components.

**Example Output:**
```
*Health Check*

✅ Database: PASS [2ms]
✅ Channel/whatsapp: PASS (connected) [15ms]
❌ Channel/discord: FAIL (disconnected)

Overall: ❌ Some systems degraded
```

**Components Checked:**
- Database connectivity (SQLite)
- Each channel connection status

---

## Usage Metrics

### `/metrics [period]`

Displays usage metrics and cost estimates.

**Arguments:**
- `period` (optional): Time period - `hour`, `day` (default), `week`

**Example Output:**
```
*Usage Metrics* (day)

📊 *Global*
  Requests: 1250
  Prompt tokens: 1,250,000
  Completion tokens: 625,000
  Total tokens: 1,875,000
  Est. cost: $12.50
  Since: 2024-01-15 00:00
```

---

## Help

### `/help`

Displays all available commands organized by category.

**Example Output (Admin View):**
```
*DevClaw Commands*

*Access Control:*
/allow <phone> - Grant user access
/block <phone> - Block a user
...

*System:*
/reload [section] - Reload configuration
/status [--json] - System status
/diagnostics [--full] - System diagnostics
/channels [connect|disconnect] - Channel management
/maintenance [on|off] [msg] - Maintenance mode
/logs [level] [lines] - View audit logs
/health - Health check
/metrics [period] - Usage metrics
...
```

---

## Permission Requirements

| Command | Owner | Admin | User |
|---------|-------|-------|------|
| `/reload` | ✅ | ✅ | ❌ |
| `/status` | ✅ | ✅ | ❌ |
| `/diagnostics` | ✅ | ✅ | ❌ |
| `/exec` | ✅ | ✅ | ❌ |
| `/channels` | ✅ | ✅ | ❌ |
| `/maintenance` | ✅ | ✅ | ❌ |
| `/logs` | ✅ | ✅ | ❌ |
| `/health` | ✅ | ✅ | ❌ |
| `/metrics` | ✅ | ✅ | ❌ |

---

## Use Cases

### Remote Troubleshooting

When you don't have SSH access but need to diagnose issues:

```
/health
/diagnostics --full
/logs error 50
```

### Scheduled Maintenance

Preparing for a system update:

```
/maintenance on Scheduled maintenance: 2024-01-16 02:00-04:00 UTC
/channels disconnect discord
/reload access
/maintenance off
/channels connect discord
```

### Configuration Updates

After editing `config.yaml`:

```
/reload access
/reload tools
/status
```

### Monitoring

Check system health remotely:

```
/status
/metrics day
/health
```

---

## Error Handling

All commands return clear error messages:

- **Permission denied**: User lacks required access level
- **Unknown section**: Invalid section name for `/reload`
- **Channel not found**: Channel name doesn't exist
- **Already connected/disconnected**: Channel state doesn't match expected

---

## Implementation Details

### Maintenance Mode Persistence

Maintenance mode state is stored in the `system_state` table:

```sql
CREATE TABLE IF NOT EXISTS system_state (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

The value is JSON-encoded:
```json
{
  "enabled": true,
  "message": "Maintenance message",
  "set_by": "admin@example.com",
  "set_at": "2024-01-15T10:30:00Z"
}
```

### Configuration Reload

Only specific sections can be hot-reloaded. The following require service restart:

- API configuration (base URL, key)
- Database path
- Gateway address/port
- WebUI address/port

### Channel Control

Channel connect/disconnect operations:
- Validate channel exists
- Check current connection state
- Handle connection errors gracefully
- Log all operations
