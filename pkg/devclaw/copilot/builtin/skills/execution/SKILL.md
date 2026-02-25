---
name: execution
description: "Run shell commands locally and on remote machines via SSH"
trigger: automatic
---

# Execution

Execute shell commands locally with full system access or on remote machines via SSH.

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
│     bash      │                    │      ssh      │
│  (local shell)│                    │  (remote via  │
│               │                    │   ~/.ssh/config)│
└───────────────┘                    └───────────────┘
        │                                     │
        │                             ┌───────┴───────┐
        │                             │               │
        ▼                             ▼               ▼
┌───────────────┐            ┌───────────────┐ ┌───────────────┐
│    set_env    │            │      scp      │ │     exec      │
│ (env vars for │            │ (file transfer│ │  (sandboxed)  │
│   session)    │            │   to remote)  │ │               │
└───────────────┘            └───────────────┘ └───────────────┘
```

## Tools
| Tool | Action | Access Level |
|------|--------|--------------|
| `bash` | Execute shell command locally | Full user access |
| `exec` | Execute in sandboxed environment | Limited |
| `ssh` | Execute command on remote machine | Via SSH config |
| `scp` | Copy files to/from remote machine | Via SSH config |
| `set_env` | Set persistent environment variable | Session only |

## When to Use
| Tool | When |
|------|------|
| `bash` | Local operations, git, docker, npm, builds |
| `exec` | Safer execution with limits |
| `ssh` | Run command on remote server |
| `scp` | Transfer files to/from remote |
| `set_env` | Set variable for subsequent commands |

## Local Execution

### Bash Commands
```bash
bash(command="git status")
# Output:
# On branch main
# Your branch is up to date with 'origin/main'.

bash(command="npm install && npm run build")
# Output: Build output...

bash(command="docker ps -a")
# Output:
# CONTAINER ID   IMAGE     STATUS    NAMES
# abc123def456   nginx     Up 2h     web-server
```

### Chaining Commands
```bash
# Sequential with && (stops on error)
bash(command="cd /project && npm install && npm test")

# Sequential with ; (continues on error)
bash(command="cd /project; npm install; npm test")

# Conditional execution
bash(command="test -f config.json && echo 'exists' || echo 'missing'")
```

### Environment Variables
```bash
# Set variable for session
set_env(name="NODE_ENV", value="production")

# Use in subsequent bash calls
bash(command="echo $NODE_ENV")
# Output: production
```

## Remote Execution

### SSH Configuration
Uses `~/.ssh/config` for host aliases:

```
# ~/.ssh/config
Host production
    HostName 192.168.1.100
    User deploy
    IdentityFile ~/.ssh/id_rsa
    Port 22

Host staging
    HostName staging.example.com
    User ubuntu
```

### SSH Commands
```bash
# Using host alias from config
ssh(host="production", command="systemctl status nginx")

# Direct connection
ssh(host="user@192.168.1.50", command="df -h")

# With port
ssh(host="user@example.com:2222", command="uptime")
```

### SCP File Transfer
```bash
# Upload to remote
scp(
  source="/local/project.tar.gz",
  dest="production:/home/deploy/project.tar.gz"
)

# Download from remote
scp(
  source="production:/var/log/nginx/access.log",
  dest="/local/logs/access.log"
)
```

## Common Patterns

### Git Workflow
```bash
bash(command="git status")
bash(command="git checkout -b feature/new-api")
bash(command="git add .")
bash(command='git commit -m "Add new API endpoint"')
bash(command="git push -u origin feature/new-api")
```

### Docker Workflow
```bash
bash(command="docker build -t myapp:v1.0 .")
bash(command="docker run -d -p 8080:80 --name web myapp:v1.0")
bash(command="docker logs -f web")
bash(command="docker stop web && docker rm web")
```

### Build and Deploy
```bash
bash(command="npm run build")
scp(source="./dist/", dest="production:/var/www/app/")
ssh(host="production", command="sudo systemctl restart nginx")
```

### Remote Debugging
```bash
ssh(host="production", command="systemctl status myapp")
ssh(host="production", command="journalctl -u myapp -n 100 --no-pager")
ssh(host="production", command="free -h && df -h")
```

## Troubleshooting

### Command not found
**Cause:** Binary not in PATH or not installed.
**Debug:**
```bash
bash(command="which node")
bash(command="echo $PATH")
```

### SSH connection refused
**Cause:** Host unreachable, wrong port, or SSH not running.
**Debug:**
```bash
bash(command="ping server.example.com")
bash(command="nc -zv server.example.com 22")
```

### Permission denied (SSH)
**Cause:** Wrong key, wrong user, or server rejected key.
**Debug:**
```bash
bash(command="ssh -v production 'echo connected'")
bash(command="ls -la ~/.ssh/")
```

## Tips
- **Working directory persists**: `cd` affects subsequent calls
- **Quote commands properly**: Use single quotes when command contains `$`
- **Use `&&` for sequences**: Stops on first error
- **SSH uses your keys**: Must have access configured
- **SCP overwrites**: Destination files are replaced silently

## Common Mistakes
| Mistake | Correct Approach |
|---------|-----------------|
| Not quoting `$` variables | `bash(command='echo "$HOME"')` |
| `cd` in separate command | Chain with `&&`: `cd /dir && command` |
| SSH without configured host | Use `user@host` or add to `~/.ssh/config` |
