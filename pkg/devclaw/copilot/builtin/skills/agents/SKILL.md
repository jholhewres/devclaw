---
name: agents
description: "Create and manage agents with isolated sessions and routing"
trigger: on-demand
---

# Agents

Agents are isolated assistant profiles with their own instructions, model, skills, and session memory. Use the `agent_manage` tool to create and manage them.

## When to Create an Agent

- Different persona per channel (formal for Slack, casual for WhatsApp)
- Isolated conversations per team/project
- Different models per context (fast model for simple queries, powerful for coding)
- Specialized assistant per use case (support, sales, dev)

## Actions via agent_manage tool

| Action | Required Params | Description |
|--------|----------------|-------------|
| create | name | Create agent (auto-generates ID from name) |
| list | — | List all agents with status |
| get | agent_id | Get agent details |
| update | agent_id | Update agent settings |
| delete | agent_id | Remove agent |
| set_default | agent_id | Set as default for unassigned users |

## Fields Reference

| Field | Purpose | Example |
|-------|---------|---------|
| name | Display name | "Work Assistant" |
| description | Short description | "Handles work-related queries" |
| model | LLM model override | "claude-sonnet-4-20250514" |
| instructions | System prompt | "You are a formal assistant" |
| channels | Route all messages from channel | ["slack"] |
| skills | Enabled skills (empty=none, omit=all) | ["memory"] |
| emoji | Identity emoji name | "briefcase" |
| tool_profile | Tool access level | "coding" |
| max_turns | Max agent loop iterations | 15 |
| active | Enable/disable | true |

## Tips

- Agent ID is a slug (lowercase, hyphens) auto-generated from name
- Only one agent can be the default — it handles unassigned users
- Channel routing: ALL messages from that channel go to the agent
- Members/Groups: granular routing by specific user or group JID
- Plugin agents appear as read-only (source: "plugin")
- Tool profiles: minimal, coding, messaging, full, or custom name
