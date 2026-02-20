# Hooks System

Hooks allow you to react to events in the DevClaw system. You can use them for logging, auditing, notifications, or custom behavior.

## Overview

| Feature | Description |
|---------|-------------|
| 20+ hook events | Lifecycle, tools, sessions, channels |
| Priority-based execution | Control the order of hook execution |
| Blocking hooks | PreToolUse, UserPromptSubmit can block operations |
| Argument modification | PreToolUse can modify tool arguments |
| External webhooks | Send events to external HTTP endpoints |
| Hot enable/disable | Toggle hooks at runtime |

## Hook Events

### Session Lifecycle

| Event | Description |
|-------|-------------|
| `session_start` | Session created or restored |
| `session_end` | Session ended or removed |

### Message Processing

| Event | Description |
|-------|-------------|
| `user_prompt_submit` | User message received (before processing) |
| `notification` | Outbound notification/message being sent |

### Tool Execution

| Event | Description |
|-------|-------------|
| `pre_tool_use` | Before a tool is called (can block/modify) |
| `post_tool_use` | After a tool returns |

### Agent Lifecycle

| Event | Description |
|-------|-------------|
| `agent_start` | Agent loop is starting |
| `agent_stop` | Agent loop finished |
| `subagent_start` | A subagent has been spawned |
| `subagent_stop` | A subagent has finished |

### Memory

| Event | Description |
|-------|-------------|
| `pre_compact` | Before session compaction |
| `post_compact` | After session compaction |
| `memory_save` | A memory was saved |
| `memory_recall` | Memories were recalled for prompt |

### System

| Event | Description |
|-------|-------------|
| `heartbeat` | Periodic heartbeat tick |
| `error` | An unrecoverable error occurred |
| `user_join` | User approved/added |
| `user_leave` | User removed/blocked |
| `channel_connect` | Channel connected |
| `channel_disconnect` | Channel disconnected |

## Event Payload Reference

Each hook event includes specific payload fields. Here's a complete reference:

| Event | SessionID | Channel | ToolName | ToolArgs | ToolResult | Message | Error | Extra Fields |
|-------|-----------|---------|----------|----------|------------|---------|-------|--------------|
| `session_start` | ✓ | ✓ | - | - | - | ✓ | - | workspace_id, user_jid |
| `session_end` | ✓ | ✓ | - | - | - | ✓ | - | reason, duration_ms |
| `user_prompt_submit` | ✓ | ✓ | - | - | - | ✓ | - | user_jid, is_group |
| `pre_tool_use` | ✓ | ✓ | ✓ | ✓ | - | ✓ | - | - |
| `post_tool_use` | ✓ | ✓ | ✓ | - | ✓ | ✓ | - | duration_ms, success |
| `agent_start` | ✓ | ✓ | - | - | - | ✓ | - | model, max_turns |
| `agent_stop` | ✓ | ✓ | - | - | - | ✓ | - | turns_used, reason |
| `subagent_start` | ✓ | ✓ | - | - | - | ✓ | - | parent_session, agent_type |
| `subagent_stop` | ✓ | ✓ | - | - | - | ✓ | - | result_summary |
| `pre_compact` | ✓ | ✓ | - | - | - | ✓ | - | message_count |
| `post_compact` | ✓ | ✓ | - | - | - | ✓ | - | messages_before, messages_after |
| `memory_save` | ✓ | ✓ | - | - | - | ✓ | - | memory_type, content_preview |
| `memory_recall` | ✓ | ✓ | - | - | - | ✓ | - | results_count, query |
| `notification` | ✓ | ✓ | - | - | - | ✓ | - | recipient, message_type |
| `heartbeat` | - | - | - | - | - | ✓ | - | uptime_seconds, status |
| `error` | ✓ | ✓ | - | - | - | ✓ | ✓ | stack_trace, recoverable |
| `user_join` | - | ✓ | - | - | - | ✓ | - | user_jid, user_name, role |
| `user_leave` | - | ✓ | - | - | - | ✓ | - | user_jid, reason |
| `channel_connect` | - | ✓ | - | - | - | ✓ | - | channel_type |
| `channel_disconnect` | - | ✓ | - | - | - | ✓ | ✓ | reason |

## Blocking Events

Only these events can block operations:

| Event | Can Block | Can Modify |
|-------|-----------|------------|
| `pre_tool_use` | ✓ | ToolArgs |
| `user_prompt_submit` | ✓ | Message |

When a hook returns `Block: true`, the operation is cancelled and the `Reason` is logged.

## Configuration

```yaml
# config.yaml
hooks:
  enabled: true

  # External webhooks
  webhooks:
    - name: "Slack Alerts"
      url: "${SLACK_WEBHOOK_URL}"
      events: [error, channel_disconnect]
      secret: "${SLACK_WEBHOOK_SECRET}"

    - name: "Analytics"
      url: "https://analytics.example.com/hook"
      events: [user_prompt_submit, notification]
      headers:
        Authorization: "Bearer ${ANALYTICS_TOKEN}"

  # Internal handlers
  handlers:
    - event: error
      action: notify_admins
      template: "Error: {{.Error.Message}}"

    - event: user_join
      action: send_message
      template: |
        Welcome {{.User.Name}}!
        Use /help for commands.
```

## Webhook Configuration

| Field | Description | Example |
|-------|-------------|---------|
| `name` | Webhook identifier | `"Slack Alerts"` |
| `url` | Endpoint URL | `"https://hooks.slack.com/..."` |
| `events` | Events to send | `[error, channel_disconnect]` |
| `secret` | HMAC signing key | `"my-secret"` |
| `headers` | Custom HTTP headers | `Authorization: "Bearer token"` |
| `timeout` | Request timeout (seconds) | `10` |
| `enabled` | Enable/disable | `true` |
| `retry_count` | Retry attempts | `3` |
| `retry_delay_ms` | Delay between retries | `1000` |

## Webhook Payload

```json
{
  "event": "error",
  "timestamp": "2025-01-15T10:30:00Z",
  "session_id": "whatsapp:120363xxx@g.us",
  "channel": "whatsapp",
  "message": "Connection lost",
  "error": "dial tcp: connection refused",
  "extra": {
    "retry_count": 3
  }
}
```

## Webhook Security

When a `secret` is configured, DevClaw signs payloads with HMAC-SHA256:

```
X-Webhook-Signature: sha256=<hex-signature>
X-Webhook-Event: <webhook-name>
```

To verify in your endpoint:

```python
import hmac
import hashlib

def verify_signature(payload, signature, secret):
    expected = "sha256=" + hmac.new(
        secret.encode(),
        payload,
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(signature, expected)
```

## Commands

| Command | Description |
|---------|-------------|
| `/hooks list` | List all registered hooks |
| `/hooks events` | List available hook events |
| `/hooks enable <name>` | Enable a hook |
| `/hooks disable <name>` | Disable a hook |

## Examples

### Error Alerting to Slack

```yaml
hooks:
  enabled: true
  webhooks:
    - name: "Slack Errors"
      url: "${SLACK_WEBHOOK_URL}"
      events: [error]
      secret: "${SLACK_WEBHOOK_SECRET}"
```

### Analytics Tracking

```yaml
hooks:
  enabled: true
  webhooks:
    - name: "Analytics"
      url: "https://analytics.example.com/hook"
      events: [user_prompt_submit, notification]
      headers:
        Authorization: "Bearer ${ANALYTICS_TOKEN}"
```

### Multi-Event Webhook

```yaml
hooks:
  enabled: true
  webhooks:
    - name: "Monitoring"
      url: "https://monitor.example.com/devclaw/events"
      events:
        - error
        - channel_connect
        - channel_disconnect
        - user_join
        - user_leave
      secret: "${MONITORING_SECRET}"
      retry_count: 5
      retry_delay_ms: 2000
```

## Programmatic Usage

### Register a Hook from Go Code

```go
hookMgr.Register(&RegisteredHook{
    Name:        "my-custom-hook",
    Description: "Does something custom",
    Source:      "plugin:my-plugin",
    Events:      []HookEvent{HookPostToolUse},
    Priority:    100,
    Enabled:     true,
    Handler: func(ctx context.Context, payload HookPayload) HookAction {
        // Log the tool usage
        log.Info("tool used", "tool", payload.ToolName)
        return HookAction{}
    },
})
```

### Blocking Hook

```go
hookMgr.Register(&RegisteredHook{
    Name:     "block-dangerous-commands",
    Events:   []HookEvent{HookPreToolUse},
    Priority: 10, // Run early
    Handler: func(ctx context.Context, payload HookPayload) HookAction {
        if payload.ToolName == "bash" {
            args := payload.ToolArgs
            cmd, _ := args["command"].(string)
            if strings.Contains(cmd, "rm -rf") {
                return HookAction{
                    Block:  true,
                    Reason: "dangerous command blocked",
                }
            }
        }
        return HookAction{}
    },
})
```

### Modifying Arguments

```go
hookMgr.Register(&RegisteredHook{
    Name:   "add-default-timeout",
    Events: []HookEvent{HookPreToolUse},
    Handler: func(ctx context.Context, payload HookPayload) HookAction {
        if payload.ToolName == "bash" {
            args := payload.ToolArgs
            if _, ok := args["timeout"]; !ok {
                modifiedArgs := make(map[string]any)
                for k, v := range args {
                    modifiedArgs[k] = v
                }
                modifiedArgs["timeout"] = 30
                return HookAction{ModifiedArgs: modifiedArgs}
            }
        }
        return HookAction{}
    },
})
```

## Best Practices

1. **Keep handlers fast** - Hooks should not block the main event loop
2. **Handle panics** - Panics in hooks are caught but logged as errors
3. **Use async for external calls** - Webhooks are sent asynchronously
4. **Set appropriate priorities** - Lower numbers run first
5. **Use blocking hooks sparingly** - Only PreToolUse and UserPromptSubmit can block
6. **Sign webhooks** - Always use secrets for production webhooks

## Architecture

```
Event occurs
      ↓
HookManager.Dispatch(payload)
      ↓
Sort hooks by priority
      ↓
For each hook (in order):
  - Skip if disabled
  - Call handler
  - If Block=true → stop and return
  - Merge modifications
      ↓
Return combined action
```

### Async Events

For non-blocking events (PostToolUse, Notification, etc.), use `DispatchAsync`:

```
Event occurs
      ↓
HookManager.DispatchAsync(payload)
      ↓
Spawn goroutine
      ↓
Fire all hooks (no blocking, no modifications)
```
