---
name: subagents
description: "Spawn isolated agents and communicate across sessions"
trigger: automatic
---

# Subagents & Sessions

Multi-agent system for spawning isolated workers and coordinating across sessions.

## ⚠️ REGRAS CRÍTICAS

| Regra | Motivo |
|-------|--------|
| Use `spawn_subagent` para tarefas complexas/paralelas | Não sobrecarregue contexto principal |
| **NÃO** use `cron_add` quando pedir subagente | São ferramentas diferentes |
| Aguarde resultado com `wait_subagent` | Se precisa do resultado |
| Use labels descritivos | Facilita identificação |

### ❌ Errado
```
Usuário: "cria um subagente para criar uma skill"
Agente: Usa cron_add ❌
Agente: Cria agendamento ❌
```

### ✓ Correto
```
Usuário: "cria um subagente para criar uma skill"
Agente: spawn_subagent(task="Criar skill...", label="skill-creator") ✓
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Main Agent                              │
│                   (Current Session)                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
         ┌─────────────────┴─────────────────┐
         │                                   │
         ▼                                   ▼
┌─────────────────┐                 ┌─────────────────┐
│    SPAWNING     │                 │ COMMUNICATION   │
├─────────────────┤                 ├─────────────────┤
│ spawn_subagent  │                 │ sessions_list   │
│ list_subagents  │                 │ sessions_send   │
│ wait_subagent   │                 │ sessions_export │
│ stop_subagent   │                 │ sessions_delete │
└────────┬────────┘                 └────────┬────────┘
         │                                   │
         ▼                                   ▼
┌─────────────────┐                 ┌─────────────────┐
│  Subagent Run   │                 │ Existing Session│
│  (isolated)     │                 │ (another agent) │
│  - Nova sessão  │                 │ - Chat separado │
│  - Contexto limitado             │ - Mensagem entre │
│  - Reporta de volta              │   agentes        │
└─────────────────┘                 └─────────────────┘
```

## Two Modes

| Mode | Tools | Purpose |
|------|-------|---------|
| **Spawning** | `spawn_subagent`, `list_subagents`, `wait_subagent`, `stop_subagent` | Criar novos agentes isolados |
| **Communication** | `sessions_list`, `sessions_send`, `sessions_export`, `sessions_delete` | Comunicar com agentes existentes |

---

## Mode 1: Spawning Subagents

### Quando Usar

| Cenário | Use spawn_subagent |
|---------|-------------------|
| Tarefa longa de pesquisa | ✓ |
| Múltiplas tarefas paralelas | ✓ |
| Criar skills/plugins | ✓ |
| Análise complexa | ✓ |
| Processamento em background | ✓ |

### Quando NÃO Usar

| Cenário | Alternativa |
|---------|-------------|
| Agendar tarefa para horário específico | `cron_add` |
| Lembrete | `cron_add` com type="at" |
| Comunicação com agente existente | `sessions_send` |

### Spawn a Subagent

```bash
spawn_subagent(
  task="Criar uma skill para a API do ViaCEP. Quando o usuário informar um CEP, buscar e retornar as informações de endereço.",
  label="viacep-skill-creator"
)
# Output: Subagent spawned with ID: sub_abc123
```

### List Running Subagents

```bash
list_subagents()
# Output:
# Subagents (2):
# - sub_abc123 [viacep-skill-creator]: running, 2m elapsed
# - sub_def456 [research-api]: completed, 45s
```

### Wait for Completion

```bash
wait_subagent(subagent_id="sub_abc123", timeout=300)
# Output:
# Subagent completed.
# Result: Skill 'viacep' created successfully...
```

### Stop a Subagent

```bash
stop_subagent(subagent_id="sub_abc123")
# Output: Subagent sub_abc123 stopped
```

### Spawning Patterns

#### Fire and Forget
```bash
# Spawn e continue trabalhando
spawn_subagent(
  task="Generate weekly report and save to reports/ folder",
  label="reporter"
)

# Continue com outras tarefas...
# Verifique depois com list_subagents()
```

#### Blocking Wait
```bash
# Spawn
spawn_subagent(
  task="Analyze logs for errors in the past 24 hours",
  label="log-analyzer"
)

# Espere resultado (bloqueia até completar)
result = wait_subagent(subagent_id="sub_abc123", timeout=600)
# Use o resultado...
```

#### Parallel Tasks
```bash
# Spawn múltiplos subagentes
spawn_subagent(task="Research topic A", label="research-a")
spawn_subagent(task="Research topic B", label="research-b")
spawn_subagent(task="Research topic C", label="research-c")

# Verifique status
list_subagents()

# Espere cada um
result_a = wait_subagent(subagent_id="sub_001")
result_b = wait_subagent(subagent_id="sub_002")
result_c = wait_subagent(subagent_id="sub_003")
```

---

## Mode 2: Inter-Agent Communication

### Discover Other Agents

```bash
sessions_list()
# Output:
# Active sessions (3):
# - [whatsapp] 5511999999 (id: abc123, ws: main) — 15 msgs — last active: 2m ago
# - [webui] user-session (id: def456, ws: dev) — 8 msgs — last active: 5m ago
```

### Send Message to Another Agent

```bash
sessions_send(
  session_id="abc123",
  message="Task completed. Results saved to output.md",
  sender_label="research-agent"
)
# Output: Message delivered to session abc123 (channel: whatsapp).
```

### Export Session

```bash
sessions_export(session_id="abc123")
# Output:
# {
#   "session_id": "abc123",
#   "messages": [...],
#   "metadata": {...}
# }
```

---

## Complete Workflow Examples

### Criar Skill via Subagent
```bash
# Usuário: "cria um subagente para criar uma skill do https://viacep.com.br/"

# 1. Spawn subagent (NÃO cron_add!)
spawn_subagent(
  task="Criar skill para ViaCEP API (https://viacep.com.br/).
        A skill deve:
        1. Aceitar CEP do usuário
        2. Buscar informações na API
        3. Retornar endereço formatado
        Incluir script para fazer a requisição.",
  label="viacep-skill"
)

# 2. Verificar status
list_subagents()

# 3. Aguardar resultado
result = wait_subagent(subagent_id="sub_abc", timeout=300)

# 4. Informar usuário
send_message("Skill ViaCEP criada! " + result)
```

### Background Research
```bash
# 1. Spawn research agent
spawn_subagent(
  task="Research best practices for GraphQL pagination. Include cursor-based and offset-based approaches.",
  label="research-graphql"
)

# 2. Continue main work
# ... do other tasks ...

# 3. Check status
list_subagents()

# 4. Get results when ready
wait_subagent(subagent_id="sub_abc123")
```

### Cross-Agent Collaboration
```bash
# 1. Find collaborator session
sessions_list()

# 2. Request help
sessions_send(
  session_id="backend-agent-session",
  message="Need API endpoint for user preferences. Can you create GET /api/user/preferences?",
  sender_label="frontend-agent"
)

# Backend agent responds via sessions_send to your session
```

---

## Subagent Isolation

| Aspecto | Comportamento |
|---------|---------------|
| Context | Bootstrap limitado (AGENTS.md + TOOLS.md) |
| Session | Separada do spawner |
| Result | Anunciado de volta ao chat do requester |
| Lifetime | Até completar ou ser parado |

---

## Troubleshooting

### "Subagent timeout"

**Causa:** Tarefa demorando mais que timeout.

**Solução:**
```bash
# Aumente timeout
wait_subagent(subagent_id="sub_abc", timeout=600)  # 10 minutos

# Ou verifique status sem esperar
list_subagents()
```

### Usou ferramenta errada

**Causa:** Confusão entre spawn e cron.

**Correção:**
- Subagent = execução imediata em paralelo
- Cron = agendar para horário específico

### "Session not found"

**Causa:** Sessão não existe ou ID errado.

**Solução:**
```bash
sessions_list()  # Liste todas primeiro
```

---

## Tips

- **Labels descritivos**: "viacep-skill" não "agent1"
- **Timeouts razoáveis**: Não espere infinitamente
- **Verifique status antes de esperar**: Evite bloquear se já completou
- **Pare subagentes travados**: Libere recursos
- **Fire and forget para não-urgente**: Não espere se não precisa

## Common Mistakes

| Erro | Correção |
|------|----------|
| Usar `cron_add` quando pedir subagent | Use `spawn_subagent` |
| Não verificar list_subagents | Verifique status |
| Esperar sem timeout | Sempre defina timeout |
| Task vaga no spawn | Seja específico sobre o esperado |
| Não parar agentes travados | Limpe com `stop_subagent` |
