---
name: skill-db
description: "Database for skills to store structured data (contacts, tasks, notes, CRM)"
trigger: automatic
---

# Skill Database

Store and retrieve structured data without SQL knowledge.

## CRITICAL: Communication Rules

- **NEVER** show tool syntax in chat (e.g., `skill_db(...)`)
- Describe actions naturally: "Salvei o contato" not "skill_db(action=insert...)"
- Respond in the same language as the user

## The ONE Tool

```
skill_db(action, skill_name, table_name, ...)
```

### Actions

| Action | What it does | When to use |
|--------|--------------|-------------|
| `query` | **LIST records** | User asks to "list", "show", "what are" |
| `insert` | Add new record | User wants to save/store something |
| `update` | Modify record | User wants to change existing data |
| `delete` | Remove record | User wants to remove something |
| `list_tables` | Show table metadata | Checking what tables exist |
| `describe` | Show table columns | Understanding table structure |
| `create_table` | Create new table | Setting up a new skill |
| `drop_table` | Delete table permanently | Removing a skill's data |

## Common Patterns

### List ideas (MOST COMMON - use this!)
```
skill_db(action="query", skill_name="oss_ideas", table_name="ideas")
```
Returns: `{"count": 1, "records": [...]}`

### Add an idea
```
skill_db(action="insert", skill_name="oss_ideas", table_name="ideas", data={
    "title": "My idea",
    "description": "Details here"
})
```

### Filter by status
```
skill_db(action="query", skill_name="oss_ideas", table_name="ideas", where={"status": "nova"})
```

## Important Notes

1. **skill_name uses underscores**: `oss_ideas` not `oss-ideas`
2. **Use query for listing**: NOT `list_tables` or `describe`
3. **One tool, one action**: The `action` parameter determines behavior

## Architecture

```
./data/skill_database.db
├── _skill_tables_registry    # Table metadata
└── {skill}_{table}           # Actual data tables
```
