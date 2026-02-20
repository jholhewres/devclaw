# DM Pairing System

The DM Pairing System allows administrators to generate shareable tokens that new users can send to the bot to request access. This eliminates the need for manual user approval via `/allow <phone>` commands.

## Overview

| Feature | Description |
|---------|-------------|
| Token Generation | Create shareable invite tokens |
| Auto-Approval | Optionally grant access immediately |
| Expiration | Set time limits on tokens |
| Use Limits | Limit how many times a token can be used |
| Workspace Assignment | Automatically assign users to workspaces |
| Admin Review | Review pending access requests |

## Commands

| Command | Description |
|---------|-------------|
| `/pairing generate [opts]` | Generate a new pairing token |
| `/pairing list [--all]` | List active tokens |
| `/pairing info <token_or_id>` | Show token details |
| `/pairing revoke <token_or_id>` | Revoke a token |
| `/pairing requests` | List pending access requests |
| `/pairing approve <request_id>` | Approve a pending request |
| `/pairing deny <request_id> [reason]` | Deny a pending request |

## Token Generation

### Basic Syntax

```
/pairing generate [expires] [max_uses] [role] [options]
```

### Parameters

| Parameter | Values | Default | Description |
|-----------|--------|---------|-------------|
| `expires` | `1h`, `24h`, `7d`, `30d`, `never` | never | Token expiration time |
| `max_uses` | number or `unlimited` | unlimited | Maximum redemptions |
| `role` | `user` or `admin` | user | Access level to grant |

### Options

| Option | Description |
|--------|-------------|
| `--auto` | Auto-approve users (no admin review needed) |
| `--ws <id>` | Assign approved users to workspace |
| `--note <text>` | Admin note for the token |

### Examples

```
# Basic token (never expires, unlimited uses, requires approval)
/pairing generate

# Auto-approve token that expires in 24 hours, max 5 uses
/pairing generate 24h 5 user --auto

# Token for specific workspace with note
/pairing generate 7d unlimited user --ws team-alpha --note "Team Alpha invites"

# Single-use admin token
/pairing generate never 1 admin --note "Backup admin access"

# Long-lived token for recruiting
/pairing generate 30d unlimited user --auto --note "Career fair 2025"
```

## User Experience

### For Users

1. Receive token from admin (e.g., via email, Slack, etc.)
2. Send token to the bot via DM
3. Receive confirmation:
   - If auto-approve: "Access granted!"
   - If manual approval: "Access request submitted. An administrator will review."

### For Admins

1. Generate token: `/pairing generate`
2. Share token with new user
3. Check pending requests: `/pairing requests`
4. Approve or deny:
   - `/pairing approve <request_id>`
   - `/pairing deny <request_id> [reason]`

## Token Management

### List Tokens

```
/pairing list          # Active tokens only
/pairing list --all    # Include revoked/expired tokens
```

Example output:
```
*Pairing Tokens (3):*

• `a1b2c3d4e5f6...` active
  Role: user | Uses: 2/5 | By: admin@example.com
• `f7e8d9c0b1a2...` active [auto]
  Role: user | Uses: 15/unlimited | By: admin@example.com
• `34567890abcd...` revoked
  Role: admin | Uses: 1/1 | By: owner@example.com
```

### Token Info

```
/pairing info a1b2c3d4e5f6
```

Shows detailed token information:
- ID and full token value
- Status (active/revoked/expired)
- Role and permissions
- Expiration date
- Use count and limits
- Auto-approve setting
- Workspace assignment
- Creator and creation date
- Admin notes

### Revoke Token

```
/pairing revoke a1b2c3d4e5f6
```

Immediately invalidates the token. Users who haven't yet redeemed it will receive an error.

## Request Management

### View Pending Requests

```
/pairing requests
```

Example output:
```
*Pending Requests (2):*

• ID: `12345678`
  User: +1234567890@s.whatsapp.net
  Name: John Doe
  Role: user | Created: 2024-01-15 14:30
  Token Note: Team Alpha invites

• ID: `87654321`
  User: +0987654321@s.whatsapp.net
  Name: Jane Smith
  Role: user | Created: 2024-01-15 15:45

Use /pairing approve <id> or /pairing deny <id> to respond.
```

### Approve Request

```
/pairing approve 12345678
```

Grants access to the user and assigns them to the workspace if specified in the token.

### Deny Request

```
/pairing deny 87654321 Reason for denial
```

Denies the request. The user is not notified automatically.

## Security

### Token Format

- **Length**: 48 hexadecimal characters (24 bytes)
- **Generation**: Cryptographically secure random (`crypto/rand`)
- **Example**: `a1b2c3d4e5f6789012345678901234567890abcdef123456`

### Best Practices

1. **Use expiration**: Set reasonable expiration times
   ```
   /pairing generate 24h 10 user --auto
   ```

2. **Limit uses for sensitive tokens**:
   ```
   /pairing generate never 1 admin --note "Emergency access"
   ```

3. **Use notes for tracking**:
   ```
   /pairing generate 7d unlimited user --note "Q1 Hiring"
   ```

4. **Revoke unused tokens**:
   ```
   /pairing revoke <token>
   ```

5. **Review pending requests promptly**:
   ```
   /pairing requests
   ```

### Security Considerations

- Tokens are stored in the database with the token value
- Tokens can be revoked at any time
- Expired tokens are automatically rejected
- Rate limiting prevents token brute-forcing
- All pairing operations are logged in the audit log

## Configuration

The pairing system is enabled by default when the database is available. No additional configuration is required.

### Database Tables

#### pairing_tokens

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT | Unique identifier |
| `token` | TEXT | Token value (48 hex chars) |
| `role` | TEXT | Role to grant (user/admin) |
| `max_uses` | INTEGER | Maximum uses (0 = unlimited) |
| `use_count` | INTEGER | Current use count |
| `auto_approve` | INTEGER | Auto-approve flag |
| `workspace_id` | TEXT | Workspace assignment |
| `note` | TEXT | Admin note |
| `created_by` | TEXT | Creator JID |
| `created_at` | TEXT | Creation timestamp |
| `expires_at` | TEXT | Expiration timestamp |
| `revoked` | INTEGER | Revocation flag |

#### pairing_requests

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT | Unique identifier |
| `token_id` | TEXT | Reference to token |
| `user_jid` | TEXT | Requester JID |
| `user_name` | TEXT | Requester display name |
| `status` | TEXT | Request status (pending/approved/denied) |
| `reviewed_by` | TEXT | Reviewer JID |
| `reviewed_at` | TEXT | Review timestamp |
| `created_at` | TEXT | Creation timestamp |

## Message Flow

```
1. User sends message to bot
2. AccessManager.Check() → Denied
3. ExtractTokenFromMessage() → Found token
4. PairingManager.ProcessTokenRedemption()
   ├─ If auto_approve: Grant access, return "Access granted!"
   └─ If manual: Create request, return "Request submitted"
5. User receives response
```

## Use Cases

### Team Onboarding

```
# Generate token for new team members
/pairing generate 7d unlimited user --ws developers --note "Dev team Q1"

# Share token via email or Slack
# Users send token to bot
# Admin reviews requests
/pairing requests
/pairing approve <id>
```

### Career Fair / Event

```
# Auto-approve token for event
/pairing generate 48h 50 user --auto --note "TechConf 2025"

# Share token at booth
# Users get immediate access
```

### Temporary Access

```
# Time-limited access for contractors
/pairing generate 30d 1 user --ws contractors --note "Project Alpha"
```

### Emergency Admin

```
# Single-use admin token for emergencies
/pairing generate 24h 1 admin --note "Emergency break-glass"
```

## Troubleshooting

### Token Not Working

1. Check token hasn't expired: `/pairing info <token>`
2. Check token isn't revoked
3. Verify user is sending token correctly (just the token, no extra text)

### User Already Has Access

If user already has access, they'll receive "You already have access to this bot."

### Request Not Showing

Requests are only created when:
- Token is valid
- User doesn't already have access
- Token has `auto_approve = false`

### Database Errors

If pairing commands fail with database errors:
1. Check database connectivity
2. Verify tables exist (created automatically on startup)
3. Check file permissions on database file
