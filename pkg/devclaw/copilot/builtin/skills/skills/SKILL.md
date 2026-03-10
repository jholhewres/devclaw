---
name: skills
description: "Create, install, and manage agent skills to extend capabilities"
trigger: on-demand
---

# Skill Management

Create, install, and manage skills that extend agent capabilities.

## IMPORTANT: Use Dedicated Skill Tools

**NEVER create skills with bash commands (mkdir, echo, etc.)**

Use the dedicated skill tools — they handle directory creation, SKILL.md generation, and database setup:

| Tool | Description |
|------|-------------|
| `skill_init` | Create a new skill |
| `skill_edit` | Edit skill instructions |
| `skill_add_script` | Add a script to a skill |
| `skill_list` | List all installed skills |
| `skill_test` | Test a skill |
| `skill_install` | Install from ClawHub, GitHub URL, or local path |
| `skill_defaults_list` | List bundled default skills |
| `skill_defaults_install` | Install bundled defaults |
| `skill_remove` | Remove an installed skill |
| `get_skill_instructions` | Load full instructions for a skill |

## Communication Guidelines

**When creating skills for users:**
1. NEVER show technical tool syntax in chat responses
2. ALWAYS ask: "Do you want this skill to have a database for storing structured data (contacts, tasks, etc.)?"
3. Explain the options:
   - **With database**: persistent data, structured queries, ideal for CRM, tasks, contacts
   - **Without database**: uses memory/vault, ideal for API integrations, automations

## Architecture
```
                      Agent Context
                           |
        +------------------+------------------+
        |                  |                  |
        v                  v                  v
+---------------+  +---------------+  +---------------+
|   CREATE      |  |   DISCOVER    |  |   MANAGE      |
+---------------+  +---------------+  +---------------+
| skill_init    |  | skill_install |  | skill_list    |
| skill_edit    |  | skill_        |  | skill_test    |
| skill_add_    |  |  defaults_    |  | skill_remove  |
|  script       |  |  list/install |  |               |
+---------------+  +---------------+  +---------------+
```

## Skill Structure
```
./skills/my-skill/           # Skills are created in the project's skills/ directory
  SKILL.md                   # Instructions for the agent
    ---
    name: my-skill
    description: "What this skill does"
    trigger: automatic
    ---
    # Markdown content
  scripts/                   # Executable scripts (optional)
  references/                # Reference docs (optional)
```

### Database Tools
| Tool | Action | Description |
|------|--------|-------------|
| `skill_db_create_table` | Create | Create a table for storing data |
| `skill_db_insert` | Create | Insert a new record |
| `skill_db_query` | Read | Query records with filters |
| `skill_db_update` | Update | Update a record by ID |
| `skill_db_delete` | Delete | Delete a record by ID |
| `skill_db_list_tables` | Read | List tables for a skill |
| `skill_db_describe` | Read | View table structure |
| `skill_db_drop_table` | Delete | Permanently drop a table |

## Creating Skills

### Initialize
```
skill_init(name="my-api-client", description="Interact with MyAPI service")
```

### Initialize with Database
```
skill_init(
    name="crm",
    description="Contact and lead management",
    with_database=true,
    database_table="contacts",
    database_schema={
        "name": "TEXT NOT NULL",
        "email": "TEXT",
        "phone": "TEXT",
        "status": "TEXT DEFAULT 'novo'"
    }
)
```

When `with_database=true`, the skill automatically gets a database table for storing structured data. The SKILL.md includes usage instructions for skill_db_* tools.

### Edit Instructions
```
skill_edit(name="my-api-client", content="# MyAPI Client\n\n## Authentication\nUse vault(action=get, name='MYAPI_KEY')")
```

### Add Script
```
skill_add_script(skill_name="my-api-client", script_name="fetch_users", content='#!/bin/bash\n...')
```

### Test
```
skill_test(name="my-api-client", input="fetch all users")
```

## Installing Skills

### From ClawHub or GitHub
```
skill_install(source="github-integration")
skill_install(source="https://github.com/user/skill-repo")
```

### Bundled Defaults
```
skill_defaults_list()
skill_defaults_install(names=["github", "jira"])
skill_defaults_install(names=["all"])
```

## Managing Skills

### List
```
skill_list()
```

### Remove
```
skill_remove(name="old-skill")
```

## Common Patterns

### Create Custom Integration
```
# 1. Initialize
skill_init(name="company-api", description="CompanyAPI client")

# 2. Add instructions
skill_edit(name="company-api", content="# CompanyAPI\n\nUse vault(action=get, name='COMPANY_API_KEY')")

# 3. Add script
skill_add_script(skill_name="company-api", script_name="get_employee", content='...')

# 4. Test
skill_test(name="company-api", input="get employee 123")
```

### Create Skill with Database (CRM Example)
```
# 1. Initialize with database
skill_init(
    name="crm",
    description="Customer relationship management",
    with_database=true,
    database_table="contacts",
    database_schema={
        "name": "TEXT NOT NULL",
        "email": "TEXT",
        "company": "TEXT",
        "status": "TEXT DEFAULT 'lead'"
    }
)

# 2. Use the database
skill_db_insert(skill_name="crm", table_name="contacts", data={
    "name": "John Smith",
    "email": "john@company.com"
})

# 3. Query data
skill_db_query(skill_name="crm", table_name="contacts", where={"status": "lead"})
```

## Tips
- Use descriptive names
- Test after creation
- Use vault for secrets in scripts
- Keep skills focused
- Use `with_database=true` for skills that need to store data
- Query with filters to avoid loading all data

## Related Skills
- **skill-db** - Database system for storing structured data in skills

## Common Mistakes
| Mistake | Correct Approach |
|---------|------------------|
| Hardcoding API keys | Use vault(action=get) in scripts |
| Vague description | Be specific about capabilities |
| Not testing | Always `skill_test(name=..., input=...)` after creation |
