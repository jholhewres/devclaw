---
name: web
description: "Search the web and fetch content from URLs"
trigger: automatic
---

# Web Tools

Search the web for current information and fetch content from URLs.

## Tools
| Tool | Action |
|------|--------|
| `web_search` | Search the web |
| `web_fetch` | Fetch URL content |

## When to Use
| Tool | When |
|------|------|
| `web_search` | Need current info, news, documentation |
| `web_fetch` | Have specific URL to read |

## Web Search
```bash
web_search(query="golang generics tutorial 2026")
# Output:
# [1] Go Generics - go.dev/blog
# [2] Generics in Go - DigitalOcean
```

## Web Fetch
```bash
web_fetch(url="https://go.dev/blog/error-handling")
# Output: Full article content...
```

## Common Patterns

### Research Workflow
```bash
# 1. Search
web_search(query="microservices patterns best practices")

# 2. Fetch relevant articles
web_fetch(url="https://blog.example.com/microservices-guide")

# 3. Synthesize information
```

### Documentation Lookup
```bash
web_search(query="terraform aws provider documentation")
web_fetch(url="https://registry.terraform.io/providers/...")
```

## Tips
- Be specific with queries
- Include year for current info
- Use `site:` to search specific domains
- Fetch full content for details
- Always cite sources

## Common Mistakes
| Mistake | Correct Approach |
|---------|-----------------|
| Vague queries | Be specific with keywords |
| Only reading snippets | Fetch full content |
| Ignoring dates | Check publication date |
