# Agent Routing

Agent Routing allows you to configure specialized agents for different channels, users, or groups. Each agent profile can have its own model, instructions, and skills.

## Overview

| Feature | Description |
|---------|-------------|
| Channel routing | Route messages from specific channels to agents |
| User routing | Route messages from specific users to agents |
| Group routing | Route messages from specific groups to agents |
| Model override | Use different models per agent |
| Custom instructions | Use different system prompts per agent |
| Skill filtering | Enable different skills per agent |

## Configuration

```yaml
# config.yaml
agents:
  # Agent profiles
  profiles:
    - id: support
      model: gpt-4o-mini
      instructions: |
        You are a customer support agent.
        Be concise, helpful, and empathetic.
        Focus on resolving issues quickly.
      skills: [search, docs]
      channels: [whatsapp]

    - id: coding
      model: claude-sonnet-4
      instructions: |
        You are a software development expert.
        Focus on code quality, architecture, and best practices.
        Provide detailed explanations with code examples.
      skills: [github, docker, testing]
      channels: [discord, telegram]

    - id: devops
      model: claude-sonnet-4
      instructions: |
        You are a DevOps engineer.
        Focus on infrastructure, deployment, and monitoring.
        Provide practical solutions for operations.
      skills: [docker, kubernetes, ci-cd]
      users: ["5511999999999"]

    - id: vip-support
      model: gpt-4o
      instructions: |
        You are a premium support agent for VIP customers.
        Provide detailed, personalized assistance.
      groups: ["120363xxx@g.us"]

  # Routing configuration
  routing:
    default: support  # Default agent when no match
```

## Routing Priority

When a message arrives, the router checks in this order:

1. **User routing** (highest priority) - Match by sender JID
2. **Group routing** - Match by group JID (for group messages)
3. **Channel routing** - Match by channel name
4. **Default** - Fallback to the default agent

```
User match > Group match > Channel match > Default
```

## Profile Configuration

| Field | Description | Example |
|-------|-------------|---------|
| `id` | Unique identifier for the profile | `coding` |
| `model` | LLM model to use | `claude-sonnet-4` |
| `instructions` | Custom system prompt | (multi-line string) |
| `skills` | List of enabled skills | `[github, docker]` |
| `channels` | Route by channel name | `[whatsapp, discord]` |
| `users` | Route by user JID | `["5511999999999"]` |
| `groups` | Route by group JID | `["120363xxx@g.us"]` |
| `max_turns` | Max agent turns (0 = unlimited) | `50` |
| `run_timeout_seconds` | Max run time | `600` |

## Examples

### Multi-Channel Setup

Different agents for different platforms:

```yaml
agents:
  profiles:
    # WhatsApp: Quick, concise support
    - id: mobile-support
      model: gpt-4o-mini
      channels: [whatsapp]
      instructions: |
        Mobile messaging context. Keep responses short and scannable.
        Use emojis sparingly. Format for small screens.

    # Discord: Detailed technical help
    - id: discord-tech
      model: claude-sonnet-4
      channels: [discord]
      instructions: |
        Discord community context. Use markdown formatting.
        Can be more verbose. Include code blocks when relevant.

    # Telegram: Balanced approach
    - id: telegram-balanced
      model: gpt-4o
      channels: [telegram]
      instructions: |
        Telegram context. Balance brevity with detail.

  routing:
    default: mobile-support
```

### VIP User Routing

Premium support for specific users:

```yaml
agents:
  profiles:
    - id: standard
      model: gpt-4o-mini
      instructions: "Standard support agent."

    - id: premium
      model: claude-sonnet-4
      instructions: |
        Premium support for VIP customers.
        Provide detailed, personalized assistance.
        Escalate issues proactively.
      users:
        - "5511999999999"
        - "5511888888888"

  routing:
    default: standard
```

### Team-Specific Groups

Different agents for different team groups:

```yaml
agents:
  profiles:
    - id: dev-team
      model: claude-sonnet-4
      skills: [github, docker, testing]
      groups:
        - "120363dev-group@g.us"
        - "120363engineering@g.us"

    - id: sales-team
      model: gpt-4o
      skills: [crm, calendar]
      groups:
        - "120363sales@g.us"

  routing:
    default: dev-team
```

## Hot Reload

Agent routing configuration can be reloaded without restart:

```
/reload all
```

This reloads the entire configuration including agent profiles.

## Commands

There are no dedicated agent routing commands yet. Use `/reload` to refresh configuration.

## Best Practices

1. **Set a default agent** - Always configure a fallback
2. **Use appropriate models** - Lighter models for simple tasks
3. **Write clear instructions** - Each agent should have a distinct purpose
4. **Test routing** - Verify messages reach the correct agent
5. **Monitor usage** - Check logs to see which agents are being used

## Logging

Agent routing decisions are logged:

```
level=INFO msg="agent routed" profile=coding channel=discord user=1234567890 group=""
level=INFO msg="prompt composed" prompt_chars=1234 model_override=claude-sonnet-4 agent_profile=coding
```

## Architecture

```
Message arrives
      ↓
AgentRouter.Route(channel, user, group)
      ↓
Priority check: user > group > channel > default
      ↓
AgentProfile found?
      ↓ Yes                    ↓ No
Override model             Use default
Override instructions      Use default config
      ↓                         ↓
Compose prompt            Compose prompt
      ↓                         ↓
Execute agent             Execute agent
```
