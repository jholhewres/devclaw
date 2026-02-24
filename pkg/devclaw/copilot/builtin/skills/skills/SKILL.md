---
name: skills
description: "Create, install, and manage agent skills to extend capabilities"
trigger: automatic
---

# Skill Management

Create, install, and manage skills that extend agent capabilities.

## Architecture
```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
┌───────────────┐  ┌───────────────┐  ┌───────────────┐
│   CREATE      │  │   DISCOVER    │  │   MANAGE      │
├───────────────┤  ├───────────────┤  ├───────────────┤
│ init_skill    │  │ search_skills │  │ list_skills   │
│ edit_skill    │  │ skill_defaults│  │ test_skill    │
│ add_script    │  │    _list      │  │ remove_skill  │
└───────────────┘  │ install_skill │  └───────────────┘
                     └───────────────┘
```

## Skill Structure
```
~/.devclaw/skills/my-skill/
├── SKILL.md           # Instructions for the agent
│   ├── ---
│   │   name: my-skill
│   │   description: "What this skill does"
│   │   trigger: automatic
│   │   ---
│   └── # Markdown content
├── scripts/           # Executable scripts (optional)
└── references/        # Reference docs (optional)
```

## Tools
| Tool | Action | Category |
|------|--------|----------|
| `init_skill` | Create new skill | Create |
| `edit_skill` | Edit instructions | Create |
| `add_script` | Add executable | Create |
| `list_skills` | List installed | Manage |
| `test_skill` | Test skill | Manage |
| `install_skill` | Install from source | Discover |
| `search_skills` | Search ClawHub | Discover |
| `skill_defaults_list` | List bundled | Discover |
| `skill_defaults_install` | Install bundled | Discover |
| `remove_skill` | Remove skill | Manage |

## Creating Skills

### Initialize
```bash
init_skill(name="my-api-client", description="Interact with MyAPI service")
# Output: Skill created at ~/.devclaw/skills/my-api-client/
```

### Edit Instructions
```bash
edit_skill(name="my-api-client", content="# MyAPI Client\n\n## Authentication\nUse vault_get('MYAPI_KEY')")
```

### Add Script
```bash
add_script(skill_name="my-api-client", script_name="fetch_users", script_content='#!/bin/bash\n...')
```

### Test
```bash
test_skill(name="my-api-client", input="fetch all users")
```

## Installing Skills

### From ClawHub
```bash
search_skills(query="github")
install_skill(source="github-integration")
```

### From GitHub
```bash
install_skill(source="https://github.com/user/skill-repo")
```

### Bundled Defaults
```bash
skill_defaults_list()
skill_defaults_install(names="github,jira")
skill_defaults_install(names="all")
```

## Managing Skills

### List
```bash
list_skills()
# Output:
# Installed skills (5):
# [builtin] memory
# [user] my-api-client
# [installed] github
```

### Remove
```bash
remove_skill(name="old-skill")
```

## Common Patterns

### Create Custom Integration
```bash
# 1. Initialize
init_skill(name="company-api", description="CompanyAPI client")

# 2. Add instructions
edit_skill(name="company-api", content="# CompanyAPI\n\nUse vault_get('COMPANY_API_KEY')")

# 3. Add script
add_script(skill_name="company-api", script_name="get_employee", script_content='...')

# 4. Test
test_skill(name="company-api", input="get employee 123")
```

## Tips
- Use descriptive names
- Test after creation
- Use vault for secrets in scripts
- Keep skills focused

## Common Mistakes
| Mistake | Correct Approach |
|---------|------------------|
| Hardcoding API keys | Use vault_get in scripts |
| Vague description | Be specific about capabilities |
| Not testing | Always test_skill after creation |
