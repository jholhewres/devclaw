# Deployment Guide

This guide covers different ways to run DevClaw in production.

## Table of Contents

- [Docker Compose](#docker-compose)
- [systemd (Linux)](#systemd-linux)
- [PM2](#pm2)
- [Ansible](#ansible)
- [Makefile (simple)](#makefile-simple)

---

## Docker Compose

Easiest option for quick deployment with all dependencies included.

```bash
cd install/docker
docker compose up -d
docker compose logs -f devclaw
```

Data is persisted in a Docker volume (`devclaw-state`). Rebuilds (`docker compose build`) preserve all sessions, memory, and configuration.

To give the agent access to host directories, add bind mounts in `docker-compose.yml`:

```yaml
volumes:
  - ./skills:/home/devclaw/skills
  - ./workspace:/home/devclaw/workspace
  - /path/to/projects:/home/devclaw/projects  # custom mount
```

---

## systemd (Linux)

Recommended for production Linux servers. Provides automatic restart, logging, and resource management.

### Setup

```bash
# Option 1: Create dedicated user (recommended for production)
sudo useradd -m -s /bin/bash devclaw

# Option 2: Use existing user
# Skip useradd and adjust the service file below

# Build and install
make build
sudo cp bin/devclaw /usr/local/bin/

# Setup directory and config
sudo mkdir -p /opt/devclaw
sudo cp config.yaml /opt/devclaw/
sudo chown -R devclaw:devclaw /opt/devclaw
```

### Service File

Create `/etc/systemd/system/devclaw.service`:

```ini
[Unit]
Description=DevClaw - AI Agent for Tech Teams
After=network.target
Documentation=https://github.com/jholhewres/devclaw

[Service]
Type=simple
User=devclaw
Group=devclaw
WorkingDirectory=/opt/devclaw
ExecStart=/usr/local/bin/devclaw serve --config /opt/devclaw/config.yaml
Restart=on-failure
RestartSec=5

# Security options
NoNewPrivileges=false    # Set to true to disable sudo access
PrivateTmp=true
ProtectKernelTunables=true

# Resource limits
LimitNOFILE=65535

# Environment (optional - for vault password)
# EnvironmentFile=-/opt/devclaw/.env

[Install]
WantedBy=multi-user.target
```

### Enable and Manage

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now devclaw

# Useful commands
sudo systemctl status devclaw
sudo systemctl restart devclaw
sudo systemctl stop devclaw
sudo journalctl -u devclaw -f        # follow logs
sudo journalctl -u devclaw --since "1 hour ago"
```

### Security Notes

| Setting | Effect |
|---------|--------|
| `NoNewPrivileges=false` | Agent can run sudo commands (if allowed in config) |
| `NoNewPrivileges=true` | Agent cannot elevate privileges (more secure) |
| `PrivateTmp=true` | Isolated /tmp directory |
| `ProtectKernelTunables=true` | Cannot modify kernel settings |

### Using with Existing User

If running under your own user instead of a dedicated `devclaw` user:

```ini
[Service]
User=yourusername
Group=yourusername
WorkingDirectory=/home/yourusername/devclaw
ExecStart=/usr/bin/make serve
# ... rest of config
```

---

## PM2

Node.js process manager - good for development or when PM2 is already in your stack.

### Install and Start

```bash
# Install PM2 if needed
npm install -g pm2

# Build DevClaw
make build

# Start with PM2
pm2 start ./bin/devclaw --name devclaw -- serve

# Save and enable auto-start
pm2 save
pm2 startup   # follow the command it prints
```

### Management Commands

```bash
pm2 logs devclaw      # view logs
pm2 restart devclaw   # restart
pm2 stop devclaw      # stop
pm2 delete devclaw    # remove from PM2
pm2 monit             # resource monitor
pm2 list              # all processes
```

### Ecosystem File

For more control, create `ecosystem.config.js`:

```javascript
module.exports = {
  apps: [{
    name: 'devclaw',
    script: './bin/devclaw',
    args: 'serve',
    cwd: '/home/youruser/devclaw',
    instances: 1,
    autorestart: true,
    watch: false,
    max_memory_restart: '1G',
    env: {
      NODE_ENV: 'production'
    }
  }]
}
```

Then: `pm2 start ecosystem.config.js`

---

## Ansible

Deploy to multiple Linux servers with a single playbook.

```bash
cd install/providers/ansible
cp inventory.example inventory
# Edit inventory with your server details
ansible-playbook -i inventory playbook.yml
```

See [providers/ansible/README.md](providers/ansible/README.md) for details.

---

## Makefile (Simple)

Quick background execution without additional tools.

```bash
# Run in background with nohup
make serve &

# With output redirect
nohup make serve > devclaw.log 2>&1 &

# Find and kill process
ps aux | grep devclaw
kill <pid>
```

---

## Comparison

| Method | Best For | Auto-restart | Logging | Resource Limits |
|--------|----------|--------------|---------|-----------------|
| Docker Compose | Development, quick start | Yes | docker logs | Container limits |
| systemd | Production Linux | Yes | journalctl | Yes |
| PM2 | Node.js stacks | Yes | pm2 logs | Yes |
| Ansible | Multi-server deploy | N/A | N/A | N/A |
| Makefile | Development only | No | Manual | No |

---

## Next Steps

After deployment:

1. Open WebUI at `http://your-server:8090/setup`
2. Configure API provider and key
3. Setup channels (WhatsApp, Discord, etc.)
4. Install skills: `devclaw skill install github`
5. Test with a message

See the [main README](../README.md) for configuration options.
