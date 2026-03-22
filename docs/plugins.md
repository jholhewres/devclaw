# Plugin System

DevClaw supports a YAML-first plugin system that extends the platform with custom agents, tools, hooks, skills, services, and channels. Plugins are self-contained directories with a `plugin.yaml` manifest and optional supporting files. No Go code is required for most use cases -- bash scripts, HTTP endpoints, and Markdown files are first-class handler types.

## Table of Contents

- [Overview](#overview)
- [Plugin Structure](#plugin-structure)
- [Agents](#agents)
- [Tools](#tools)
- [Hooks](#hooks)
- [Skills](#skills)
- [Configuration](#configuration)
- [UI Configuration](#ui-configuration)
- [Requirements](#requirements)
- [Lifecycle](#lifecycle)
- [WebUI Management](#webui-management)
- [Creating Your First Plugin](#creating-your-first-plugin)
- [Example Plugins](#example-plugins)

---

## Overview

The plugin system is built around three principles:

1. **YAML-first**: Everything a plugin does is declared in a single `plugin.yaml` manifest. No build step, no compilation, no SDK. Drop a directory with a manifest into a plugin directory and restart.

2. **Convention over configuration**: Tools are namespaced automatically (`{pluginID}_{toolName}`), config fields resolve through a standard precedence chain (overrides > vault > env > defaults), and agents wire into the existing session and routing infrastructure.

3. **Progressive complexity**: Start with a bash script tool. Add an agent with a Markdown prompt. Graduate to HTTP endpoints or native Go `.so` libraries when you need them. Each handler type is a single field in the YAML.

Plugins can provide any combination of:

| Capability | Description |
|------------|-------------|
| **Agents** | LLM agents with custom instructions, triggers, tools, and escalation |
| **Tools** | Functions callable by agents (script, HTTP, or native Go) |
| **Hooks** | Event-driven scripts that fire on system events |
| **Skills** | SKILL.md knowledge files surfaced to agents |
| **Services** | HTTP endpoints exposed through the DevClaw gateway |
| **Channels** | Native messaging channel implementations (requires `.so`) |

---

## Plugin Structure

A plugin is a directory containing a `plugin.yaml` manifest and optional supporting files.

### Directory Layout

```
my-plugin/
  plugin.yaml              # Manifest (required)
  prompts/                 # Agent instruction files
    agent-name.md
  skills/                  # Skill definitions
    skill-name/
      SKILL.md
  lib.so                   # Optional native Go library
```

### Manifest Reference (`plugin.yaml`)

The manifest is the single source of truth for everything a plugin provides.

#### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | **Yes** | Unique plugin identifier. Used for tool namespacing and config keys. |
| `name` | string | No | Human-readable name (defaults to `id` if omitted). |
| `version` | string | No | Semver version string (defaults to `0.0.0`). |
| `description` | string | No | Short description shown in the WebUI. |
| `author` | string | No | Author or organization. |
| `license` | string | No | License identifier (e.g. `MIT`, `Apache-2.0`). |
| `min_devclaw` | string | No | Minimum DevClaw version required. |
| `requires` | [PluginRequirements](#requirements) | No | External runtime requirements. |
| `config` | [PluginConfigSchema](#configuration) | No | Configuration schema with typed fields. |
| `agents` | [AgentDef[]](#agents) | No | Agent definitions. |
| `tools` | [ToolDef[]](#tools) | No | Tool definitions. |
| `hooks` | [HookDef[]](#hooks) | No | Hook definitions. |
| `services` | ServiceDef[] | No | HTTP service endpoints. |
| `channels` | ChannelDef[] | No | Channel implementations (native `.so` only). |
| `skills` | [SkillDef[]](#skills) | No | Skill definitions. |
| `ui` | [PluginUIConfig](#ui-configuration) | No | WebUI settings panel layout. |
| `native_lib` | string | No | Path to `.so` library (relative to plugin directory). |

#### Minimal Example

```yaml
id: my-plugin
name: My Plugin
version: 1.0.0
description: A minimal plugin

tools:
  - name: hello
    description: Say hello
    script: echo "Hello from my-plugin!"
```

---

## Agents

Plugin agents are LLM-powered agents with custom system prompts, tool profiles, and trigger-based activation. When a user message matches an agent's trigger keywords, that agent handles the conversation instead of the main DevClaw agent.

### AgentDef Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `id` | string | -- | Agent identifier (unique within the plugin). |
| `name` | string | -- | Human-readable agent name. |
| `description` | string | `""` | Explains the agent's purpose. |
| `instructions` | string | -- | System prompt: inline text or path to a `.md` file (relative to plugin directory). |
| `model` | string | `""` | LLM model override (empty = use default). |
| `triggers` | string[] | `[]` | Keywords that activate this agent (case-insensitive substring match against message content). |
| `tools` | AgentToolProfile | `{}` | Tool allow/deny profile. |
| `max_turns` | int | `0` | Maximum agent loop turns (`0` = unlimited). |
| `timeout_sec` | int | `0` | Max execution time in seconds (`0` = default). |
| `session_mode` | string | `""` | `"isolated"` for a separate session, `"shared"` to use the parent session. |
| `escalation` | EscalationConfig | `nil` | Escalation behavior to the main agent. |
| `channels` | string[] | `[]` | Restrict to specific channels (empty = all channels). |

### Instructions

Instructions can be provided inline or as a path to a Markdown file:

```yaml
# Inline instructions
agents:
  - id: helper
    instructions: You are a helpful assistant that answers questions concisely.

# File-based instructions (recommended for longer prompts)
agents:
  - id: helper
    instructions: prompts/helper.md
```

File paths ending in `.md` or containing a path separator are treated as file references relative to the plugin directory. All other strings are treated as inline instructions. File paths are validated to prevent directory traversal -- they must resolve within the plugin directory.

### Tool Profiles

Control which tools an agent can access with `allow` and `deny` lists:

```yaml
agents:
  - id: reader
    tools:
      allow: ["read_file", "web_search", "my-plugin_lookup"]
      deny: ["bash", "write_file", "delete_file"]
```

- If `allow` is empty, the agent can use all available tools (minus those in `deny`).
- If `allow` is specified, only those tools are available.
- `deny` always takes precedence over `allow`.
- Plugin tools are referenced by their namespaced name: `{pluginID}_{toolName}`.

### Triggers

Triggers are keywords matched as case-insensitive substrings against incoming message content. The agent with the highest-scoring trigger match handles the message.

```yaml
triggers: ["hello", "hi", "greet"]
```

The match score is calculated as `len(trigger) / (len(message) + 1)`. A minimum score threshold of `0.01` prevents false positives on very long messages. When multiple agents match, the one with the highest score wins.

Triggers can be restricted to specific channels:

```yaml
agents:
  - id: support-bot
    triggers: ["help", "support"]
    channels: ["slack", "discord"]
```

### Escalation

Agents can escalate conversations back to the main DevClaw agent when they encounter tasks outside their scope.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Turn escalation on/off. |
| `keywords` | string[] | `[]` | Phrases that trigger automatic escalation. |
| `max_turns` | int | `0` | Escalate after this many turns without resolution. |
| `on_failure` | string | `""` | Behavior on escalation failure: `"retry"` or `"drop"`. |
| `explicit_only` | bool | `false` | Only escalate via explicit `escalate_to_main` tool call. |

```yaml
escalation:
  enabled: true
  keywords: ["I need help", "talk to human", "escalate"]
  max_turns: 8
  on_failure: retry
```

When escalation fires, a formatted message is injected into the parent session's follow-up queue:

```
[Escalation from plugin:my-plugin agent:helper]
Reason: User requested human assistance
Context: <conversation summary>
```

---

## Tools

Tools are functions that agents can call during their execution loop. Each plugin tool is automatically namespaced as `{pluginID}_{toolName}` to prevent collisions.

### ToolDef Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | -- | Tool name (namespaced at registration as `{pluginID}_{name}`). |
| `description` | string | -- | Description shown to the LLM in the tool schema. |
| `parameters` | object | `{}` | JSON Schema defining tool parameters. |
| `permission` | string | `""` | Required access level: `"owner"`, `"admin"`, or `"user"`. |
| `hidden` | bool | `false` | If `true`, the tool is excluded from the LLM tool schema (only callable programmatically). |
| `script` | string | `""` | Inline bash script (script handler). |
| `endpoint` | string | `""` | HTTP URL (HTTP handler). |
| `method` | string | `"GET"` | HTTP method for endpoint-based tools. |
| `headers` | map | `{}` | Additional HTTP headers for endpoint-based tools. |
| `handler` | string | `""` | Go symbol name in the native `.so` (native handler). |

Exactly one of `script`, `endpoint`, or `handler` must be specified. If none is set, the tool fails to register.

### Parameters

Tool parameters are defined using JSON Schema notation:

```yaml
tools:
  - name: search
    description: Search the knowledge base
    parameters:
      type: object
      properties:
        query:
          type: string
          description: Search query
        limit:
          type: integer
          description: Max results to return
      required: ["query"]
```

If no parameters are specified, a default empty schema is used: `{"type": "object", "properties": {}, "additionalProperties": false}`.

### Handler Types

#### 1. Script Handler

The simplest handler type. Runs an inline bash script with tool arguments injected as `PLUGIN_*` environment variables and plugin config values as `PLUGIN_CFG_*` environment variables.

```yaml
tools:
  - name: greet
    description: Generate a personalized greeting
    parameters:
      type: object
      properties:
        name:
          type: string
          description: Person to greet
      required: ["name"]
    script: |
      echo "Hello, ${PLUGIN_NAME}! Welcome to DevClaw."
```

Environment variable mapping:

| Source | Variable Pattern | Example |
|--------|-----------------|---------|
| Tool arguments | `PLUGIN_{ARG_NAME}` | `name` arg -> `PLUGIN_NAME` |
| Plugin config | `PLUGIN_CFG_{KEY}` | `api_key` config -> `PLUGIN_CFG_API_KEY` |

The script runs with `bash -c` in the plugin directory. Output is captured from stdout (max 6000 characters, truncated with `... (truncated)` if longer). Errors are captured from stderr.

#### 2. HTTP Handler

Calls an external HTTP endpoint. Useful for wrapping REST APIs.

```yaml
tools:
  - name: translate
    description: Translate text using an external API
    endpoint: https://api.translation.example.com/v1/translate
    method: POST
    headers:
      X-Custom-Header: my-value
    parameters:
      type: object
      properties:
        body:
          type: string
          description: JSON request body
        path:
          type: string
          description: Additional path appended to the endpoint URL
```

- If the plugin config contains a `token` field, it is automatically sent as `Authorization: Bearer {token}`.
- Response bodies are limited to 1 MiB and output is truncated at 6000 characters.
- HTTP 4xx/5xx responses return an error with the status code and response body.
- Default timeout is 30 seconds per request.

#### 3. Native Handler

Calls a Go function exported from a `.so` shared library. This is the most powerful handler type, with full access to the Go runtime.

```yaml
native_lib: lib.so

tools:
  - name: compute
    description: Run a native computation
    handler: ComputeHandler
```

The handler symbol must match the signature:

```go
var ComputeHandler func(context.Context, map[string]any) (any, error)
```

The `.so` must be built with `go build -buildmode=plugin` and compiled against the same Go version as DevClaw.

### Namespacing

All plugin tools are namespaced at registration time:

```
Tool "greet" in plugin "hello-world" -> registered as "hello-world_greet"
```

When referencing plugin tools in agent `allow` lists or SKILL.md files, always use the namespaced name: `hello-world_greet`.

---

## Hooks

Hooks are event-driven handlers that fire when specific system events occur. They run asynchronously and do not block the event source.

### HookDef Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | -- | Hook identifier. |
| `description` | string | `""` | Explains the hook's purpose. |
| `events` | string[] | -- | Events this hook listens to. |
| `priority` | int | `100` | Execution order (lower = earlier). |
| `script` | string | `""` | Inline bash script. |
| `handler` | string | `""` | Go symbol name for native handlers. |

### Events

Hooks listen to system events such as:

- `user_prompt_submit` -- fired when a user sends a message

Additional events may be available depending on the DevClaw version. See the [hooks documentation](hooks.md) for details.

### Priority

Hooks with lower priority values execute first. The default priority is `100`. Use values below 100 to run before default hooks, or above 100 to run after.

```yaml
hooks:
  - name: early-logger
    events: ["user_prompt_submit"]
    priority: 10
    script: echo "Runs first"

  - name: late-logger
    events: ["user_prompt_submit"]
    priority: 200
    script: echo "Runs after default hooks"
```

### Script Hooks

Script hooks work the same way as script tool handlers. The event name is passed as `event` in the data map, and all event data fields are available as environment variables.

```yaml
hooks:
  - name: log-messages
    events: ["user_prompt_submit"]
    priority: 200
    script: |
      echo "[my-plugin] $(date): Message received" >> /tmp/my-plugin.log
```

Plugin config values are available as `PLUGIN_CFG_*` environment variables, just as with script tool handlers.

---

## Skills

Skills are knowledge files that agents can reference during conversations. Each skill is a Markdown file (`SKILL.md`) that provides structured guidance, examples, and instructions.

### SkillDef Reference

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Skill name. |
| `description` | string | Short description of the skill's purpose. |
| `skill_md` | string | Relative path to the SKILL.md file within the plugin directory. |

```yaml
skills:
  - name: greeting-guide
    description: Guide for creating effective greetings
    skill_md: skills/greeting/SKILL.md
```

### SKILL.md Structure

A SKILL.md file is a standard Markdown document. There is no enforced structure, but a typical skill file includes:

```markdown
# Skill Name

Brief description of the skill.

## Usage

How to use the tools or techniques this skill covers.

## Examples

Concrete examples showing inputs and expected outputs.

## Tips

Best practices and common pitfalls.
```

The skill path is validated to ensure it stays within the plugin directory (no directory traversal via `../` or symlinks).

---

## Configuration

Plugins declare a typed configuration schema. Values are resolved at load time through a four-level precedence chain.

### Config Schema

```yaml
config:
  fields:
    - key: api_key
      name: API Key
      description: Authentication key for the external API
      type: secret
      required: true
      vault_key: my_plugin_api_key
      env_var: MY_PLUGIN_API_KEY

    - key: base_url
      name: Base URL
      type: string
      default: "https://api.example.com"

    - key: max_retries
      name: Max Retries
      type: int
      default: 3

    - key: verbose
      name: Verbose Logging
      type: bool
      default: false
```

### PluginConfigField Reference

| Field | Type | Description |
|-------|------|-------------|
| `key` | string | Config key used in the resolved config map and env variables. |
| `name` | string | Human-readable field name (shown in WebUI). |
| `description` | string | Explains the field's purpose. |
| `type` | string | Field type: `"string"`, `"int"`, `"bool"`, or `"secret"`. |
| `required` | bool | Whether the field must be provided (loading fails if missing). |
| `default` | any | Default value if not provided by any other source. |
| `env_var` | string | Environment variable to read the value from. |
| `vault_key` | string | Encrypted vault key to read the value from. |

### Resolution Order

Configuration values are resolved in the following precedence order (highest to lowest):

1. **Overrides** -- values from `plugins.overrides.{pluginID}` in DevClaw's `config.yaml`
2. **Vault** -- values read from the encrypted vault via `vault_key`
3. **Environment variables** -- values read from the OS environment via `env_var`
4. **Defaults** -- the `default` value from the schema

The first source that provides a value wins. If a field is `required: true` and no source provides a value, the plugin fails to load with a config validation error.

### DevClaw Config

Plugin configuration is set in DevClaw's `config.yaml`:

```yaml
plugins:
  dirs: ["./plugins", "./examples/plugins"]
  enabled: []       # empty = load all discovered plugins
  disabled: []      # plugins to skip
  overrides:
    my-plugin:
      api_key: "sk-..."
      base_url: "https://custom.api.example.com"
```

| Field | Description |
|-------|-------------|
| `dirs` | Directories to scan for plugin subdirectories. |
| `dir` | Legacy single directory (merged with `dirs`). |
| `enabled` | Plugin IDs to load (empty = all). |
| `disabled` | Plugin IDs to skip (takes precedence over `enabled`). |
| `overrides` | Per-plugin config overrides (highest precedence). |

### Secret Fields

Fields with `type: "secret"` receive special handling:

- Values are redacted as `"••••••••"` in API responses and the WebUI.
- When updating config via the WebUI, sending the redacted placeholder preserves the original secret value (it is not overwritten).
- Use `vault_key` to store secrets in the encrypted vault rather than in `config.yaml`.

---

## UI Configuration

The `ui` section controls how a plugin appears and is configured in the WebUI's plugin management panel.

### PluginUIConfig Reference

| Field | Type | Description |
|-------|------|-------------|
| `icon` | string | Lucide icon name for the plugin card (e.g. `"bot"`, `"globe"`, `"hand-wave"`). |
| `category` | string | Groups the plugin in the UI (e.g. `"communication"`, `"productivity"`, `"examples"`). |
| `color` | string | Hex accent color for the plugin card (e.g. `"#3B82F6"`). |
| `sections` | UISection[] | Configuration form layout -- groups of config fields. |
| `actions` | UIAction[] | Quick-action buttons shown in the plugin detail view. |

### Sections

Sections organize config fields into logical groups in the settings form.

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Section heading. |
| `description` | string | Optional subtitle/explanation. |
| `fields` | string[] | Config field keys to include in this section (references `config.fields[].key`). |
| `collapsible` | bool | Whether the section can be collapsed in the UI. |

```yaml
ui:
  icon: "settings"
  category: "integrations"
  color: "#8B5CF6"
  sections:
    - title: Authentication
      description: API credentials for the external service
      fields: ["api_key", "base_url"]
    - title: Behavior
      description: Tune how the plugin operates
      fields: ["max_retries", "verbose"]
      collapsible: true
```

### Actions

Actions add quick-action buttons to the plugin detail view, allowing users to invoke plugin tools directly from the UI.

| Field | Type | Description |
|-------|------|-------------|
| `label` | string | Button text. |
| `description` | string | Tooltip text. |
| `tool` | string | Namespaced tool name to invoke (e.g. `"my-plugin_sync"`). |
| `confirm` | bool | Show a confirmation dialog before executing. |
| `icon` | string | Lucide icon name for the button. |

```yaml
ui:
  actions:
    - label: Sync Now
      description: Trigger a manual data sync
      tool: my-plugin_sync
      confirm: true
      icon: refresh-cw
    - label: Clear Cache
      description: Clear the plugin's local cache
      tool: my-plugin_clear_cache
      icon: trash-2
```

---

## Requirements

Plugins can declare external requirements that must be satisfied for the plugin to load. If any requirement is not met, the plugin enters the `error` state with the message `"requirements not met"`.

### PluginRequirements Reference

| Field | Type | Description |
|-------|------|-------------|
| `bins` | string[] | Required binaries that must all be in `$PATH`. |
| `any_bins` | string[] | Binaries where at least one must be in `$PATH`. |
| `env` | string[] | Required environment variables (must be non-empty). |
| `os` | string[] | Supported operating systems (e.g. `"linux"`, `"darwin"`). Matched case-insensitively against `runtime.GOOS`. |

```yaml
requires:
  bins: ["docker", "kubectl"]
  any_bins: ["podman", "docker"]
  env: ["KUBECONFIG"]
  os: ["linux", "darwin"]
```

All checks must pass for the plugin to be eligible:

- **bins**: Every binary in the list must exist in `$PATH`.
- **any_bins**: At least one binary in the list must exist in `$PATH`.
- **env**: Every listed environment variable must be set and non-empty.
- **os**: The current `runtime.GOOS` must match at least one entry (case-insensitive).

If `requires` is omitted or `nil`, the plugin is always eligible.

---

## Lifecycle

Plugins progress through a series of states from discovery to shutdown.

### States

```
Discovered -> Loaded -> Registered -> Started -> Stopped
                                                    |
                                         (any state can go to)
                                                    v
                                                  Error
```

| State | Description |
|-------|-------------|
| `discovered` | Plugin directory found with a valid `plugin.yaml`. |
| `loaded` | Manifest parsed, config resolved, requirements checked, native `.so` loaded (if any). |
| `registered` | Tools, hooks, agents, and skills registered with the runtime. |
| `started` | Service context created, plugin fully operational. |
| `stopped` | Services stopped, tools unregistered (reverse registration order). |
| `error` | Something failed -- check `ErrorMsg` for details. |

### Lifecycle Phases

#### 1. Discovery

The `Loader` scans all directories in `plugins.dirs` for subdirectories containing a `plugin.yaml` file. Each valid manifest produces a `PluginInstance` in the `discovered` state.

Legacy `.so` files without a `plugin.yaml` are also discovered and wrapped in a synthetic manifest for backward compatibility.

#### 2. Loading

For each discovered plugin:

1. **Requirements check** -- `IsEligible()` validates `bins`, `any_bins`, `env`, and `os`.
2. **Config resolution** -- `ResolveConfig()` merges overrides, vault, env, and defaults.
3. **Config validation** -- `ValidateConfig()` checks that required fields are present.
4. **Native library loading** -- If `native_lib` is set, the `.so` is opened and `Channel`/`Plugin` symbols are extracted.
5. State advances to `loaded`.

#### 3. Registration

The `Registry` iterates all loaded plugins and registers their components:

- **Tools**: Built via `BuildToolHandler`, namespaced, and registered with the tool executor.
- **Hooks**: Wired to the hook system with priority ordering.
- **Skills**: SKILL.md paths validated and registered with the skill registrar.
- **Agents**: Instructions resolved (inline or from `.md` file) and indexed for trigger matching.

State advances to `registered`.

#### 4. Start

A service context (`context.Context`) is created for each registered plugin. Native plugins with lifecycle hooks have their `Init` function called during loading.

State advances to `started`.

#### 5. Stop

Plugins are stopped in reverse registration order (last registered = first stopped):

1. Service context is cancelled.
2. All registered tools are unregistered from the tool executor.
3. Native plugins with a `Shutdown` function have it called.
4. State advances to `stopped`.

---

## WebUI Management

The plugin management interface is available at `/plugins` in the DevClaw WebUI. It provides a visual dashboard for viewing, configuring, enabling, and disabling plugins.

### REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/plugins` | List all plugins with state, registered components, and redacted config. |
| `GET` | `/api/plugins/:id` | Get details for a specific plugin. |
| `POST` | `/api/plugins/:id/toggle` | Enable or disable a plugin. |
| `PUT` | `/api/plugins/:id/config` | Update plugin configuration (secrets sent as `"••••••••"` are preserved). |

### Plugin Info Response

The `GET /api/plugins` endpoint returns an array of `PluginInfo` objects:

```json
{
  "id": "hello-world",
  "name": "Hello World Plugin",
  "version": "0.1.0",
  "description": "Example plugin demonstrating agents, tools, hooks, and skills",
  "author": "DevClaw Team",
  "state": "started",
  "enabled": true,
  "dir": "./examples/plugins/hello-world",
  "tools": ["hello-world_greet"],
  "hooks": ["log-messages"],
  "agents": ["greeter"],
  "skills": ["greeting-guide"],
  "ui": { "icon": "hand-wave", "category": "examples", "color": "#10B981" },
  "config_schema": { "fields": [...] },
  "config_values": { "greeting_style": "friendly" },
  "loaded_at": "2026-03-21T10:00:00Z"
}
```

Secret config values are always redacted in API responses.

---

## Creating Your First Plugin

This walkthrough creates a simple plugin step by step.

### Step 1: Create the Plugin Directory

```bash
mkdir -p plugins/my-greeter
```

### Step 2: Write the Manifest

Create `plugins/my-greeter/plugin.yaml`:

```yaml
id: my-greeter
name: My Greeter
version: 0.1.0
description: A simple greeting plugin

config:
  fields:
    - key: greeting_style
      name: Greeting Style
      type: string
      default: "friendly"

tools:
  - name: greet
    description: Generate a personalized greeting
    parameters:
      type: object
      properties:
        name:
          type: string
          description: Person to greet
      required: ["name"]
    script: |
      STYLE="${PLUGIN_CFG_GREETING_STYLE:-friendly}"
      if [ "$STYLE" = "formal" ]; then
        echo "Good day, ${PLUGIN_NAME}. Welcome to DevClaw."
      else
        echo "Hey ${PLUGIN_NAME}! Welcome to DevClaw!"
      fi
```

### Step 3: Add an Agent (Optional)

Create `plugins/my-greeter/prompts/greeter.md`:

```markdown
# Greeter Agent

You are a friendly greeting agent. Your role is to welcome users.

## Behavior

- Greet users warmly using the `my-greeter_greet` tool
- If the user asks about anything beyond greetings, escalate to the main agent
```

Update `plugin.yaml` to include the agent:

```yaml
agents:
  - id: greeter
    name: Greeter Agent
    description: Welcomes users
    instructions: prompts/greeter.md
    triggers: ["hello", "hi", "greet"]
    tools:
      allow: ["my-greeter_greet"]
      deny: ["bash", "write_file"]
    max_turns: 10
    session_mode: isolated
    escalation:
      enabled: true
      keywords: ["help", "escalate"]
```

### Step 4: Add a Skill (Optional)

Create `plugins/my-greeter/skills/greeting/SKILL.md`:

```markdown
# Greeting Guide

Use the `my-greeter_greet` tool to generate greetings.

## Styles

- **friendly**: Warm, casual greeting (default)
- **formal**: Professional greeting
```

Update `plugin.yaml`:

```yaml
skills:
  - name: greeting-guide
    description: Guide for creating effective greetings
    skill_md: skills/greeting/SKILL.md
```

### Step 5: Configure and Run

Ensure your DevClaw config includes the plugins directory:

```yaml
plugins:
  dirs: ["./plugins"]
```

Restart DevClaw. The plugin will be discovered, loaded, and registered automatically. Check the logs for confirmation:

```
plugins: discovered  id=my-greeter name="My Greeter" dir=./plugins/my-greeter
plugins: loaded      id=my-greeter name="My Greeter" state=loaded
plugins: registered  id=my-greeter tools=1 hooks=0 agents=1 skills=1
plugins: started     id=my-greeter
```

---

## Example Plugins

### hello-world

The built-in example plugin at `examples/plugins/hello-world/` demonstrates all major capabilities in a single plugin:

- **Tool**: `hello-world_greet` -- script-based greeting tool
- **Agent**: `greeter` -- a greeting agent with triggers (`hello`, `hi`, `greet`), tool allow/deny profile, isolated sessions, and escalation
- **Hook**: `log-messages` -- logs `user_prompt_submit` events to `/tmp/hello-world.log`
- **Skill**: `greeting-guide` -- a SKILL.md covering greeting styles and best practices
- **UI**: Custom icon, category, color, and settings section for the `greeting_style` config field

Source: [`examples/plugins/hello-world/`](../examples/plugins/hello-world/)

### CodeFlow

[CodeFlow](https://github.com/jholhewres/devclaw-plugin-codeflow) is a community plugin that demonstrates a more advanced use case. It serves as a reference for building production-grade plugins with external API integrations and multi-agent workflows.

Install by cloning the repository into a plugin directory:

```bash
cd plugins/
git clone https://github.com/jholhewres/devclaw-plugin-codeflow.git codeflow
```

Then ensure the `plugins/` directory is in your `plugins.dirs` config and restart DevClaw.
