# Exec Analysis

Exec Analysis analyzes bash/shell commands before execution to assess risk and take appropriate action.

## Overview

| Feature | Description |
|---------|-------------|
| Risk categorization | Safe, Moderate, Dangerous, Blocked |
| Automatic actions | Allow, Log, Require Approval, Deny |
| Suspicious pattern detection | Command injection, pipes to shell |
| Role-based trust levels | Owner, Admin, User permissions |
| Safe binaries whitelist | Known safe command paths |

## Risk Categories

| Category | Examples | Action |
|----------|----------|--------|
| **Safe** | `ls`, `cat`, `grep`, `echo` | Execute immediately |
| **Moderate** | `npm install`, `git pull`, `docker ps` | Log + execute |
| **Dangerous** | `rm`, `sudo`, `chmod`, `chown` | Require approval |
| **Blocked** | `mkfs`, `dd if=/dev/zero`, `shutdown` | Always deny |

## Configuration

```yaml
security:
  exec_analysis:
    enabled: true

    # Risk categories with patterns
    categories:
      safe:
        patterns: ["ls*", "cat *", "echo *", "grep *", "head *", "tail *"]
        action: allow

      moderate:
        patterns: ["npm *", "yarn *", "git *", "docker *", "go *", "python *"]
        action: allow_log

      dangerous:
        patterns: ["rm *", "sudo *", "chmod *", "chown *"]
        action: require_approval
        notify: [owners]

      blocked:
        patterns: ["mkfs*", "dd if=*", "shutdown*", "reboot*"]
        action: deny
        message: "This command is blocked for safety reasons"

    # Safe binary paths (always allowed)
    safe_bins:
      - /usr/bin/ls
      - /usr/bin/cat
      - /usr/bin/grep
      - /usr/bin/find

    # Role-based trust levels
    trust:
      owner: dangerous    # Can execute up to dangerous without approval
      admin: moderate     # Can execute up to moderate without approval
      user: safe          # Can only execute safe commands

    # Suspicious pattern detection
    suspicious_patterns:
      - "\\$\\([^)]*\\)"      # Command substitution $(...)
      - "`[^`]*`"            # Backtick execution
      - "\\|\\s*(sh|bash)"   # Pipe to shell
      - "&&\\s*(rm|sudo)"    # Chained dangerous commands
      - ";\s*(rm|sudo)"      # Semicolon dangerous commands
      - ">\\s*/etc/"         # Writing to /etc
      - ">\\s*/dev/"         # Writing to /dev

    # Default action for unmatched commands
    default_action: allow_log
```

## Actions

| Action | Description |
|--------|-------------|
| `allow` | Execute immediately without logging |
| `allow_log` | Log the command and execute |
| `require_approval` | Queue for admin approval before execution |
| `deny` | Block execution and return error |

## Suspicious Patterns

Suspicious patterns indicate potentially dangerous constructs:

| Pattern | Description | Example |
|---------|-------------|---------|
| `$(...)` | Command substitution | `echo $(whoami)` |
| `` `...` `` | Backtick execution | `` echo `id` `` |
| `\| sh` | Pipe to shell | `curl ... \| bash` |
| `&& rm` | Chained dangerous | `cd / && rm -rf *` |
| `; sudo` | Semicolon dangerous | `ls; sudo rm -rf /` |
| `> /etc/` | Writing to system config | `echo x > /etc/hosts` |
| `> /dev/` | Writing to device files | `cat /dev/urandom > /dev/sda` |

When suspicious patterns are found in otherwise safe commands, the action is escalated to `allow_log`.

## Role-Based Trust

Trust levels define the maximum risk a role can execute without approval:

| Role | Default Trust | Example |
|------|---------------|---------|
| `owner` | Dangerous | Can run `rm` but needs approval for blocked |
| `admin` | Moderate | Can run `npm install` but needs approval for `rm` |
| `user` | Safe | Can only run `ls`, `cat`, etc. |

## API Usage

### Basic Analysis

```go
analyzer := NewExecAnalyzer(cfg, logger)

result := analyzer.Analyze("rm -rf /tmp/test")
// result.Risk = "dangerous"
// result.Action = "require_approval"

result := analyzer.Analyze("ls -la")
// result.Risk = "safe"
// result.Action = "allow"
```

### Role-Based Analysis

```go
result := analyzer.AnalyzeForRole("rm file.txt", "owner")
// May be allowed based on owner trust level

result := analyzer.AnalyzeForRole("rm file.txt", "user")
// Will require approval (user trust = safe)
```

### Analysis Result

```go
type ExecAnalysisResult struct {
    Risk              RiskLevel   // safe, moderate, dangerous, blocked
    Action            RiskAction  // allow, allow_log, require_approval, deny
    Reason            string      // Explanation
    MatchedPattern    string      // Pattern that matched
    IsSuspicious      bool        // Suspicious constructs found
    SuspiciousMatches []string    // Which suspicious patterns matched
}
```

## Integration

Exec Analysis integrates with the bash tool execution:

1. User sends message requesting bash command
2. Agent calls bash tool with command
3. ExecAnalyzer.Analyze() evaluates the command
4. Based on action:
   - `allow`: Execute immediately
   - `allow_log`: Log and execute
   - `require_approval`: Queue for approval
   - `deny`: Return error to agent

## Best Practices

1. **Start with logging** - Use `allow_log` as default until confident
2. **Customize for your environment** - Adjust patterns for your workflows
3. **Set appropriate trust levels** - Match roles to organizational needs
4. **Review suspicious patterns** - Add patterns specific to your threat model
5. **Monitor blocked commands** - Review logs for attempted dangerous commands

## Examples

### Read-Only Operations (Safe)

```bash
ls -la /var/log
cat /etc/hosts
grep "error" /var/log/syslog
find . -name "*.go"
head -n 100 large_file.txt
```

### Development Operations (Moderate)

```bash
npm install
yarn build
git pull origin main
docker compose up -d
go test ./...
python -m pytest
```

### System Operations (Dangerous)

```bash
rm -rf ./node_modules
sudo apt update
chmod 600 ~/.ssh/id_rsa
chown www-data:www-data /var/www
```

### Always Blocked

```bash
mkfs.ext4 /dev/sda1
dd if=/dev/zero of=/dev/sda
shutdown -h now
reboot
```
