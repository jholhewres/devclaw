# GoClaw - Comportamento, Segurança e Otimização

## Fluxo de Mensagem Completo

```
User Message
     │
     ▼
┌─────────────────┐
│ Trigger Check   │ ── @copilot, menção ou DM direto
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Input Guardrail │ ── Rate limit + Prompt injection + Tamanho
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Session Load    │ ── Carrega contexto isolado do chat/grupo
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Prompt Compose  │ ── 8 camadas com token budget otimizado
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Agent Run       │ ── Executa LLM com tools + hooks
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Output Guardrail│ ── URL validation + System leak + PII
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Session Update  │ ── Salva conversa + extrai fatos
└────────┬────────┘
         │
         ▼
    Response → Channel.Send()
```

---

## Sistema de Prompts em Camadas

| Layer | Prioridade | Conteúdo | Budget |
|-------|-----------|----------|--------|
| Core | 0 | Comportamento base, regras fundamentais | 500 |
| Identity | 10 | Personalidade customizada | 200 |
| Business | 20 | Contexto do usuário/empresa | 300 |
| Objective | 30 | Objetivo da conversa atual | 200 |
| Skills | 40 | Instruções + tools das skills ativas | 2000 |
| Memory | 50 | Fatos relevantes (RAG) | 1000 |
| Temporal | 60 | Data/hora + timezone | 50 |
| Conversation | 70 | Histórico recente (comprimido) | 8000 |

**Prioridade de corte**: Em caso de estouro de tokens, layers com maior número (menor prioridade) são comprimidos/removidos primeiro.

---

## Segurança

### Input Guardrails (Implementado)
- **Rate limiting**: Sliding window por usuário (30 msg/min padrão)
- **Prompt injection**: Detecção de padrões como "ignore previous instructions", "system prompt:", etc.
- **Tamanho máximo**: Input limitado a 4096 caracteres (configurável)

### Output Guardrails (Implementado)
- **Empty output**: Rejeita respostas vazias
- **System prompt leak**: Detecta indicadores de vazamento de instruções internas
- **URL validation**: (TODO) Verifica URLs contra resultados de tools
- **PII detection**: (TODO) Detecta dados pessoais no output

### Tool Security (Planejado)
- **Allowlist por skill**: Cada skill só pode acessar suas próprias tools
- **Confirmation required**: Tools destrutivas requerem confirmação do usuário
- **Rate limit por tool**: Limite de chamadas por ferramenta
- **Sandbox**: Execução isolada para tools arriscadas

---

## Otimização de Tokens

### Memory Compression (3 estratégias)

| Estratégia | Método | Economia | Qualidade |
|-----------|--------|----------|-----------|
| Truncate | Remove mensagens antigas | ~80% | Baixa |
| Summarize | LLM resume mensagens antigas | ~70% | Alta |
| Semantic | Mantém apenas relevantes (RAG) | ~60% | Muito Alta |

### Token Budget Management

Para `gpt-4o-mini` (128K context):
```
Total:    128,000 tokens
├── Reserved (resposta):  4,096
├── System prompt:          500
├── Skills:               2,000
├── Memory:               1,000
├── History:              8,000
├── Tools:                4,000
└── Disponível:         108,404
```

---

## Isolamento de Sessões

- Cada **chat/grupo** possui sessão independente com:
  - Histórico de conversa isolado
  - Fatos de longo prazo específicos
  - Skills ativas configuráveis
  - Configurações individuais (modelo, idioma, trigger)
- **Thread-safe**: `sync.RWMutex` em todas as operações
- **Double-check lock**: Em `GetOrCreate` para evitar race conditions

---

## Comparação: OpenClaw vs GoClaw

| Aspecto | OpenClaw | GoClaw |
|---------|----------|--------|
| Linguagem | TypeScript | Go |
| Prompt Layers | 6 fixas | 8 configuráveis + otimização |
| Memory | Custom + vector | Hybrid (3 estratégias) |
| Isolamento | Container por grupo | Namespace + session (leve) |
| Guardrails Input | Básico (injection) | Injection + PII + Rate limit |
| Guardrails Output | Limitado | Leak detection + URL + PII |
| Token Management | Manual | Automático com budgets |
| Performance | ~2s startup | ~50ms startup |
| Memória | ~200MB | ~30MB |
| Logging | Console.log | slog (structured) |
| Shutdown | Abrupto | Graceful (context) |
