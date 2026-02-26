---
name: vault
description: "store and retrieve secrets securely with AES-256-GCM encryption"
trigger: automatic
---

# Vault

Secure credential storage for API keys, tokens, passwords, and other sensitive data.

## ⚠️ CRITICAL RULES

| Rule | Reason |
|------|--------|
| **NEVER** call `vault_delete` before `vault_save` | Causes data loss |
| **NEVER** store in `memory` | Use vault for secrets |
| `vault_save` overwrites automatically | NO need to delete first |

## Architecture
```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┴──────────────────┐
        │                                     │
        ▼                                     ▼
┌───────────────┐                    ┌───────────────┐
│  vault_save   │                    │  vault_get    │
│ (encrypt &    │                    │ (decrypt &    │
│   store)      │                    │   retrieve)   │
└───────┬───────┘                    └───────┬───────┘
        │                                     │
        │     ┌───────────────────────┐       │
        └────▶│   Encrypted Vault     │◀──────┘
              │  (AES-256-GCM)        │
              └───────────────────────┘
```

## Tools
| Tool | Action | Risk Level |
|------|--------|------------|
| `vault_save` | Store a secret | Low (overwrites) |
| `vault_get` | Retrieve a secret | Low |
| `vault_list` | List secret names | None (names only) |
| `vault_delete` | Remove a secret | **HIGH** (destructive) |

## When to Use
| Tool | When |
|------|------|
| `vault_save` | User provides API key, token, password |
| `vault_get` | Need credential for API call |
| `vault_list` | Check what credentials are available |
| `vault_delete` | User **explicitly** asks to remove |

## Saving Secrets
```bash
vault_save(name="OPENAI_API_KEY", value="sk-proj-xxxxx")
# Output: Secret 'OPENAI_API_KEY' saved.
```

## Retrieving Secrets
```bash
vault_get(name="OPENAI_API_KEY")
# Output: sk-proj-xxxxx
```

## Listing Secrets
```bash
vault_list()
# Output:
# Vault contains 3 secrets:
# - ANTHROPIC_API_KEY
# - DATABASE_URL
# - OPENAI_API_KEY
```

## Deleting Secrets
**⚠️ ONLY use when user explicitly requests deletion**
```bash
vault_delete(name="OLD_API_KEY")
```

## Common Patterns

### Store New API Key
```bash
# User: "Here's my OpenAI key: sk-proj-xxxxx"
vault_save(name="OPENAI_API_KEY", value="sk-proj-xxxxx")
send_message("Your OpenAI API key has been stored securely.")
```

### Use Stored Credential
```bash
key = vault_get(name="OPENAI_API_KEY")
bash(command='curl -H "Authorization: Bearer ' + key + '" https://api.openai.com/v1/models')
```

### Update Existing Secret
```bash
# Just save - it overwrites automatically
vault_save(name="GITHUB_TOKEN", value="ghp_newToken")

# DO NOT delete first - unnecessary and risky
```

## Vault vs Memory
| Use Vault | Use Memory |
|-----------|------------|
| API keys | User preferences |
| Passwords | General facts |
| Database URLs | Project context |
| Access tokens | Conversation summaries |

**Rule:** If exposure could cause harm → vault, not memory.

## Common Mistakes
| Mistake | Correct Approach |
|---------|-----------------|
| `vault_delete` then `vault_save` | Just `vault_save` - it overwrites |
| Storing in `memory` | Use vault for secrets |
| Deleting "to clear space" | Never delete unless explicitly asked |
