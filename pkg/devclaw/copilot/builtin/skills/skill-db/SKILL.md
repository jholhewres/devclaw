---
name: skill-db
description: "Database system for skills to store and retrieve structured data like contacts, tasks, notes, CRM, etc."
trigger: automatic
---

# Skill Database

A built-in database system that allows skills to store structured data (contacts, tasks, notes, CRM, etc.) without requiring SQL knowledge or custom scripts.

 Skills can create their own tables using `skill_db(action="create_table")`.

**IMPORTANT: Communication Guidelines**

- NEVER show technical tool syntax in chat responses
- Describe actions naturally: "Saved the contact" instead of "skill_db_insert(...)`
- NEVER show tool syntax like `skill_db_insert(skill_name="crm", table_name="contacts", data={...})`
- NEVER show tool calls in chat responses

**When creating skills with database:**
1. ALWAYS ASK: "Você quer que essa skill tenha um banco de dados para salvar it?"
2. ALWAYS use `skill_db_query` to list data - it's the most intuitive and direct!
- Use `skill_db_query` for natural language

**Example Usage:**
```
skill_db(action="query", skill_name="oss_ideas", table_name="ideas")
```

**Common Patterns:**
```
skill_db(action="create_table", skill_name="oss_ideas", table_name="ideas", columns={
    "title": "TEXT NOT NULL",
    "description": "TEXT",
    "project": "TEXT",
    "status": "TEXT DEFAULT 'nova'",
    "priority": "INTEGER DEFAULT 3,
    "tags": "TEXT",
})

```

**Tips:**
- Use `skill_db` for all skill operations
- Keep skills focused
- Use `with_database=true` for skills that need to store data
- Query with filters to avoid loading all data
- Always use underscores in skill_name (convert hyphens to underscores: `oss_ideas` not `oss-ideas`)


**Related Skills**
- **skills** - Create, install, and manage skills
- **skill-db** - Database system for storing structured data in skills

**Common mistakes**

| Mistake | Correct Approach |
|---------|------------------|
| Hardcoding API keys | Use vault_get in scripts |
| Vague description | Be specific about capabilities |
| Not testing | Always test_skill after creation |
| Creating duplicate skills | Delete duplicate tables (DROP_table) | Use skill_db(action="drop_table") instead

