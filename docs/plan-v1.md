# GoClaw (AgentGo Copilot) - Plano Principal v1

## Visão Geral

**GoClaw** é um assistente pessoal open-source em Go que funciona como CLI e serviço de mensagens (WhatsApp, Discord, Telegram). Binário único, sem dependências de runtime, cross-compilável.

### Diferencial Competitivo

| Feature | NanoClaw | OpenClaw | **GoClaw** |
|---------|----------|----------|------------|
| Binário único | ❌ (Node.js) | ❌ (Node.js) | ✅ |
| Memory footprint | ~200MB | ~500MB | **~30MB** |
| Startup time | ~2s | ~5s | **~50ms** |
| Runtime dependency | Node.js 20+ | Node.js 20+ | **Nenhum** |
| Cross-compile | ❌ | ❌ | ✅ (Linux/Mac/Win/ARM) |
| WhatsApp | Baileys | Baileys | **Whatsmeow (Go nativo)** |
| Graceful shutdown | Parcial | Parcial | **Completo (context-based)** |
| Observabilidade | Logs básicos | Logs básicos | **slog + health checks** |

---

## Arquitetura

```
┌─────────────────────────────────────────────────────────────────┐
│                          GoClaw                                  │
├─────────────────────────────────────────────────────────────────┤
│  CLI Layer (cmd/copilot/)                                        │
│  ├── chat      - Chat interativo / single-shot                   │
│  ├── serve     - Modo daemon com canais                          │
│  ├── schedule  - Tarefas agendadas (CRUD)                        │
│  ├── skill     - search / install / list / update                │
│  ├── config    - init / show / set                               │
│  └── remember  - Adicionar fatos à memória                       │
├─────────────────────────────────────────────────────────────────┤
│  Channel Layer (pkg/goclaw/channels/)                            │
│  ├── Channel interface + Manager (multi-channel)                 │
│  ├── whatsapp/   - Whatsmeow wrapper                             │
│  ├── discord/    - Discordgo wrapper                             │
│  └── telegram/   - telego wrapper                                │
├─────────────────────────────────────────────────────────────────┤
│  Skills Layer (pkg/goclaw/skills/)                               │
│  ├── Skill interface + Registry + Loaders                        │
│  ├── Search index (by category, tag, author)                     │
│  └── skills/ (submodule: goclaw-skills)                          │
├─────────────────────────────────────────────────────────────────┤
│  Copilot Core (pkg/goclaw/copilot/)                              │
│  ├── Assistant   - Orquestrador principal                        │
│  ├── PromptComposer - Prompt em camadas (8 layers)               │
│  ├── SessionStore - Isolamento por chat/grupo                    │
│  └── security/   - Input/Output guardrails + Rate limiting       │
├─────────────────────────────────────────────────────────────────┤
│  Scheduler (pkg/goclaw/scheduler/)                               │
│  ├── Cron-based job execution                                    │
│  ├── Persistent storage                                          │
│  └── Channel routing de resultados                               │
├─────────────────────────────────────────────────────────────────┤
│  Infrastructure                                                  │
│  ├── Dockerfile (multi-stage, ~30MB)                             │
│  ├── docker-compose.yml                                          │
│  ├── systemd service (hardened)                                  │
│  └── Makefile                                                    │
└─────────────────────────────────────────────────────────────────┘
```

---

## Melhorias em relação ao plano original

1. **Graceful shutdown**: Todos os componentes usam `context.Context` para cancelamento propagado
2. **Structured logging**: `log/slog` nativo do Go 1.21+ em vez de logs ad-hoc
3. **Health checks**: Cada canal reporta `HealthStatus` com latência, erros e conexão
4. **Channel Manager**: Agregação de mensagens de múltiplos canais em stream único
5. **Error types**: Erros tipados (`ErrChannelDisconnected`, `ErrRateLimited`, etc.)
6. **Rate limiter**: Sliding window por usuário implementado no security layer
7. **Prompt injection detection**: Padrões de injection detectados no input guardrail
8. **System prompt leak detection**: Output guardrail detecta vazamento de instruções
9. **Dockerfile multi-stage**: Imagem final Alpine ~30MB com health check
10. **systemd hardening**: `ProtectSystem=strict`, `PrivateTmp`, `MemoryMax`

---

## Roadmap

### Fase 1: Core MVP (v0.1.0)
- [x] Estrutura de diretórios e go.mod
- [x] Channel interface + Manager
- [x] Skills interface + Registry
- [x] Scheduler + Job management
- [x] Copilot Assistant (orquestrador)
- [x] Prompt Composer (8 camadas)
- [x] Session Store (isolamento por chat)
- [x] Security guardrails (input + output)
- [x] CLI completa (chat, serve, schedule, skill, config, remember)
- [x] Dockerfile + docker-compose + systemd + Makefile
- [x] Repositório de skills como submodule

### Fase 2: Channels (v0.2.0)
- [ ] WhatsApp channel (whatsmeow)
- [ ] Discord channel (discordgo)
- [ ] Telegram channel (telego)
- [ ] Reconexão automática com backoff exponencial
- [ ] Circuit breaker para canais instáveis

### Fase 3: Agent Integration (v0.3.0)
- [ ] Integração com SDK AgentGo (agent.Run, tools)
- [ ] Memory persistence (SQLite)
- [ ] Memory compression (summarize, semantic, truncate)
- [ ] RAG com embeddings para seleção de contexto
- [ ] Chat interativo REPL

### Fase 4: Skills Ecosystem (v0.4.0)
- [ ] Skill loader (filesystem, embedded, registry)
- [ ] 10 skills iniciais no repositório de skills
- [ ] CLI: `copilot skill install/search/update`
- [ ] Skill templates + scaffolding

### Fase 5: Production (v1.0.0)
- [ ] Multi-user support
- [ ] Web dashboard
- [ ] Metrics/telemetry (Prometheus)
- [ ] Voice interface
- [ ] 50+ skills

---

## Dependências

```
go 1.23.0

go.mau.fi/whatsmeow         # WhatsApp (Go nativo)
github.com/bwmarrin/discordgo  # Discord
github.com/mymmrac/telego      # Telegram
github.com/robfig/cron/v3      # Scheduler (cron)
github.com/spf13/cobra         # CLI
github.com/spf13/viper         # Config
```

---

## Estrutura de Repositórios

```
goclaw/                 (repositório principal)
├── cmd/copilot/        CLI application
├── pkg/goclaw/         Core packages
│   ├── channels/       Channel interface + Manager
│   ├── skills/         Skills interface + Registry
│   ├── copilot/        Assistant + Prompt + Session + Security
│   └── scheduler/      Job scheduling
├── skills/             (submodule → goclaw-skills)
├── configs/            Config examples
├── docs/               Documentação e planos
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── go.mod

goclaw-skills/          (repositório de skills - submodule)
├── index.yaml          Índice global
├── schemas/            Validation schemas
├── skills/             Skills organizadas por categoria
│   ├── builtin/        weather, calculator
│   ├── productivity/   calendar, todoist
│   ├── development/    github, gitlab
│   └── ...
└── templates/          Templates para novas skills
```
