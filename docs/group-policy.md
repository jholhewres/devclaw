# Group Policy

Group Policy allows you to configure how the bot behaves in group chats, including activation modes, quiet hours, and access control.

## Overview

| Feature | Description |
|---------|-------------|
| Activation modes | Control when the bot responds in groups |
| Quiet hours | Define time ranges when the bot stays silent |
| Access policies | Open, disabled, or allowlist per group |
| Workspace override | Route group messages to specific workspaces |
| Blocked groups | Globally block specific groups |

## Configuration

```yaml
# config.yaml
groups:
  # Default policy for unconfigured groups
  default_policy: open

  # Group-specific configurations
  groups:
    - id: "120363xxx@g.us"
      name: "Dev Team"
      policy: open
      activation: always
      workspace: dev-workspace

    - id: "120363yyy@g.us"
      name: "VIP Support"
      policy: allowlist
      activation: mention
      allowed_users:
        - "5511999999999@s.whatsapp.net"
        - "5511888888888@s.whatsapp.net"

    - id: "120363zzz@g.us"
      name: "Night Shift"
      policy: open
      activation: keyword
      keywords: ["urgent", "emergency", "help"]
      quiet_hours:
        start: "08:00"
        end: "22:00"
        timezone: "America/Sao_Paulo"

  # Globally blocked groups
  blocked:
    - "120363bad@g.us"
```

## Group Policies

| Policy | Description |
|--------|-------------|
| `open` | All group members can use the bot (default) |
| `disabled` | Bot does not respond in this group |
| `allowlist` | Only specified users can use the bot |

## Activation Modes

| Mode | Description |
|------|-------------|
| `always` | Respond to all messages |
| `mention` | Respond only when mentioned or triggered |
| `reply` | Respond only when replying to bot's messages |
| `keyword` | Respond when keywords are detected or when mentioned |

## Quiet Hours

Quiet hours define when the bot should be silent in a group.

```yaml
quiet_hours:
  start: "22:00"    # Start time (HH:MM)
  end: "08:00"      # End time (HH:MM)
  timezone: "UTC"   # Timezone (optional, default: UTC)
```

### Overnight Quiet Hours

Quiet hours can span midnight. For example, `22:00` to `08:00` means the bot is silent from 10 PM to 8 AM.

## Examples

### Development Team Group

Always respond in the dev team group with access to development workspace:

```yaml
groups:
  groups:
    - id: "120363dev@g.us"
      name: "Development Team"
      policy: open
      activation: always
      workspace: dev-workspace
```

### Customer Support with VIP Access

Allow only specific users to use the bot in the VIP group:

```yaml
groups:
  groups:
    - id: "120363vip@g.us"
      name: "VIP Support"
      policy: allowlist
      activation: mention
      allowed_users:
        - "5511999999999@s.whatsapp.net"
        - "5511888888888@s.whatsapp.net"
```

### Emergency Response Group

Use keyword activation for emergency situations, with quiet hours for non-emergencies:

```yaml
groups:
  groups:
    - id: "120363ops@g.us"
      name: "Operations"
      policy: open
      activation: keyword
      keywords: ["urgent", "emergency", "down", "outage", "critical"]
      quiet_hours:
        start: "22:00"
        end: "06:00"
        timezone: "America/Sao_Paulo"
```

### Multiple Groups with Blocked List

```yaml
groups:
  default_policy: open

  groups:
    - id: "120363sales@g.us"
      name: "Sales Team"
      activation: mention

    - id: "120363marketing@g.us"
      name: "Marketing Team"
      activation: mention

  blocked:
    - "120363spam@g.us"
    - "120363competitor@g.us"
```

## Integration with Agent Routing

Group policy works alongside [Agent Routing](agent-routing.md):

1. Group policy determines if the bot should respond
2. Agent routing determines which agent profile to use
3. Both can specify workspace overrides

If both group policy and agent routing specify a workspace, the agent routing takes precedence.

## Hot Reload

Group policy configuration can be reloaded without restart:

```
/reload all
```

This reloads the entire configuration including group policies.

## Best Practices

1. **Set a default policy** - Use `open` or `mention` as default
2. **Use mention mode for large groups** - Reduces noise
3. **Configure quiet hours** - Respect team working hours
4. **Use keywords for emergency groups** - Only respond to urgent matters
5. **Block spam groups** - Keep the blocked list updated

## Logging

Group policy decisions are logged:

```
level=DEBUG msg="group is blocked" group=120363bad@g.us
level=DEBUG msg="user not in allowlist" group=120363vip@g.us user=5511xxx@s.whatsapp.net
level=DEBUG msg="quiet hours active" group=120363ops@g.us
level=DEBUG msg="group policy: not responding"
```

## Architecture

```
Message arrives (group)
      ↓
Is group blocked? → Yes → Ignore
      ↓ No
Get group config (or default)
      ↓
Check policy:
  - disabled → Ignore
  - allowlist → Check if user is allowed
  - open → Continue
      ↓
Check quiet hours → Active → Ignore
      ↓
Check activation mode:
  - always → Respond
  - mention → Check trigger
  - reply → Check reply
  - keyword → Check keywords/trigger
      ↓
Should respond → Yes → Process message
```
