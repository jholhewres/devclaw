# Environment Variable Substitution

DevClaw supports environment variable substitution in configuration files, allowing you to keep sensitive credentials and environment-specific values out of your config files.

## Overview

| Feature | Description |
|---------|-------------|
| Basic substitution | `${VAR}` or `$VAR` |
| Default values | `${VAR:-default}` |
| Required variables | `${VAR:?error message}` |
| **Encrypted Vault** | `.devclaw.vault` for secure credential storage |

## Priority Order

Variables are resolved in this order (highest to lowest priority):

1. **Vault secrets** - Injected at startup from encrypted `.devclaw.vault`
2. **System environment variables** - Set by the OS/container
3. **Default values in syntax** - `${VAR:-default}`
4. **Placeholder preserved** - `${VAR}` if no value found

---

## Encrypted Vault

The vault provides secure, encrypted storage for sensitive credentials. Secrets are stored in `.devclaw.vault` using AES-256-GCM encryption with Argon2id key derivation.

### Features

- **AES-256-GCM encryption** - Industry-standard authenticated encryption
- **Argon2id key derivation** - OWASP-recommended password hashing
- **Master password** - Never stored, only used to derive encryption key
- **Hot reload** - Use `/reload vault` to refresh without restart

### Vault Commands

| Command | Description |
|---------|-------------|
| `/vault list` | List all secret names |
| `/vault set <key> <value>` | Add or update a secret |
| `/vault get <key>` | Show a secret (masked) |
| `/vault delete <key>` | Remove a secret |
| `/vault unlock` | Unlock the vault |
| `/vault lock` | Lock the vault |
| `/vault status` | Show vault status |

### CLI Management

```bash
# Create a new vault
devclaw vault create

# Unlock vault (prompts for password)
devclaw vault unlock

# List secrets
devclaw vault list

# Add a secret
devclaw vault set OPENAI_API_KEY sk-xxx

# Get a secret
devclaw vault get OPENAI_API_KEY

# Delete a secret
devclaw vault delete OLD_KEY
```

### Hot Reload

To reload vault secrets without restarting:

```
/reload vault
```

This re-injects all vault secrets as environment variables and reloads the configuration.

---

## Syntax

### Basic Substitution

```yaml
api:
  api_key: ${OPENAI_API_KEY}
  base_url: ${API_BASE_URL}
```

If the environment variable is not set, the placeholder is preserved in the config.

### Default Values

Use `${VAR:-default}` to provide a fallback value when the variable is not set:

```yaml
api:
  base_url: ${API_BASE_URL:-https://api.openai.com/v1}

channels:
  whatsapp:
    session_dir: ${WHATSAPP_SESSION_DIR:-./sessions/whatsapp}

database:
  path: ${DATABASE_PATH:-./data/devclaw.db}
```

### Required Variables

Use `${VAR:?error message}` to make a variable required. If the variable is not set, DevClaw will fail to start with an error:

```yaml
api:
  api_key: ${API_KEY:?API key is required - set API_KEY in vault}

security:
  tool_guard:
    secret_token: ${SECRET_TOKEN:?Secret token must be configured}
```

---

## Complete Configuration Example

```yaml
name: DevClaw
model: gpt-4o

api:
  base_url: ${API_BASE_URL:-https://api.openai.com/v1}
  api_key: ${OPENAI_API_KEY}

database:
  path: ${DATABASE_PATH:-./data/devclaw.db}

channels:
  whatsapp:
    enabled: ${WHATSAPP_ENABLED:-true}
    session_dir: ${WHATSAPP_SESSION_DIR:-./sessions/whatsapp}

security:
  tool_guard:
    secret_token: ${SECRET_TOKEN:?Secret token must be configured}
```

---

## Security Best Practices

1. **Use the vault for sensitive credentials** - API keys, tokens, passwords

2. **Use required variables for critical secrets** - Fail fast if missing:
   ```yaml
   api:
     api_key: ${API_KEY:?API key is required}
   ```

3. **Set restrictive file permissions**:
   ```bash
   chmod 600 config.yaml
   chmod 600 .devclaw.vault
   ```

4. **Lock the vault when not in use** - Use `/vault lock` to clear the key from memory

---

## Error Handling

When a required variable is missing:

```bash
$ devclaw serve
Error: expanding environment variables: config error: API_KEY - API key is required
```

The error message includes:
- The variable name (`API_KEY`)
- The custom error message from the config
