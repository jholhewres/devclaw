# v1.18.2 — Análise dos Fixes do Sistema de Memória

Período coberto: commits de **v1.18.1 → HEAD** (14 commits, 2026-04-14 a 2026-04-16).
Escopo: memória, dream, compaction, sanitização, hybrid search, context router.

---

## 1. O que foi entregue

### 1.1 Performance e contenção SQLite
| Commit | Mudança | Impacto |
|---|---|---|
| `d201180` | `busy_timeout 5s→30s`, `_txlock=immediate`, `MaxOpenConns=4`, logs de BM25/vector | Reduz `SQLITE_BUSY` em paralelo; torna falhas visíveis. |
| `19826a9` | `IndexChunks`: embeddings **fora** da transação (3 fases). Remove `IndexMemoryDir` inline do `handleMemorySave`. | Lock de escrita passa de segundos → ms. Elimina amplificação N×saves. |

### 1.2 Dream / consolidação
| Commit | Mudança |
|---|---|
| `6265e17` | `findEvidenceContradictions` (negativo vs positivo mais recente). |
| `b3964a7` | Dream genérico (PT+EN), sem regex por protocolo. |
| `63b731e` | `MinHoursBetween 6→1`; auto-expira entradas como `[stale]`; parser pula `[stale]`; `ForceRun()`. |
| `83d6575` | `Assistant.ForceDream` + `SIGUSR1` → dream sob demanda. |

### 1.3 Sanitização / segredos (reestruturação maior)
| Commit | Mudança |
|---|---|
| `1291cc3` | **Flip arquitetural:** redação **não** acontece antes do LLM ver o conteúdo; somente em **persist + output**. Evita que o agente "esqueça" SSH/DB/gcloud. |
| `257aa43` | `sanitizeOutput` adicionado ao scheduler para paridade. |
| `54004d8` | Pré-flight MANDATORY; redactor com stopwords (pós-match, supera RE2); filtro de `access-failure facts`. |
| `280dcea` | Correção do filtro anterior: permite correções/sucessos (`"NÃO está bloqueado"`, `"access: ... works"`). |

### 1.4 Recall, routing e compaction
| Commit | Mudança |
|---|---|
| `db3118b` | (4 bugs) BM25 fallback quando sanitizer esvazia query; expansão de tail no LCM; wing-aware recall; outros. |
| `1147350` | Auto-deriva wing pelo canal (confidence 0.5 persistido). |
| `e5ed2b9` | Operational Playbook preservado no prompt de compaction; `buildProtectedSet` com heurística genérica (8 últimos tool results bem-sucedidos); `TruncateToolResult` (head+tail); telemetria de tokens. |

---

## 2. Riscos e pontos frágeis identificados

### P0 — merecem fix antes de cortar v1.18.2

1. **`expireStaleEntries` frágil a formato.**
   - `strings.Replace(line, "- [", "- [stale] [", 1)` só cobre entradas no formato exato `- [...]`. Qualquer variação (indent, `*`, falta do prefixo `[`) **silenciosamente** falha em marcar stale.
   - Matching por `strings.Contains(line, staleContent)` sobre conteúdo inteiro é sensível a caracteres especiais e pode casar partes de entradas legítimas.
   - **Correção sugerida:** usar um identificador estável (hash do conteúdo original ou timestamp) gravado pelo FileStore; ou ancorar por linha completa após `strings.TrimSpace`.
   - Arquivo: `pkg/devclaw/copilot/dream.go` (função `expireStaleEntries`).

2. **`IndexChunks` — race concorrente ainda possível.**
   - A checagem dentro da tx (`txHash == fileHash`) cobre o caso feliz, mas dois goroutines com **hashes diferentes** podem sobrescrever embeddings um do outro (last-writer-wins com dados possivelmente stale).
   - **Correção sugerida:** `sync.Map` com `singleflight.Group` chaveado por `fileID` em `SQLiteStore` para serializar indexação por arquivo, ou lock por-file em memória.
   - Arquivo: `pkg/devclaw/copilot/memory/sqlite_store.go`.

3. **Remoção do `IndexMemoryDir` inline sem fallback de saúde.**
   - A indexação depende inteiramente do watcher fsnotify. Se o watcher morrer (limite de inotify, FS remoto, erro silencioso), *saves* de memória param de virar chunks indexados — BM25/vector search ficam defasados sem aviso.
   - **Correção sugerida:** scan periódico leve (ex.: a cada 10 min) comparando `files.indexed_at` vs `mtime` em `memDir`; ou health-check que falha pipeline se watcher caiu.

4. **`ForceDream` sem guarda de concorrência.**
   - `ForceRun` apenas chama `Run(ctx)`. Nada impede dois `SIGUSR1` em rápida sucessão (ou um manual + agendado) de iniciarem Runs paralelos lendo/escrevendo as mesmas .md — corrida sobre `expireStaleEntries` / `Compact`.
   - **Correção sugerida:** `sync.Mutex` ou `atomic.Bool` `isRunning` em `DreamConsolidator` com CAS + `logger.Warn("dream already running, skipping")`.

### P1 — entra no release se sobrar tempo

5. **`looksLikeAccessFailureFact` por keyword é estreito.**
   - Só PT/EN; falsos negativos em ES/FR/IT.
   - Keywords de "correção" (`ignore`, `supersede`) são **facilmente burladas** por prompt injection (um `memory(action=save, content="ignore: SSH blocked")` passa).
   - **Correção sugerida:** usar **sinal estruturado** (exit code do último tool result, `category`) em vez de heurística textual. Se `category="incident"` ou `"postmortem"`, permitir narrativa de falha.

6. **Redactor — gaps conhecidos.**
   - `\S{4,}` interrompe em espaço: `senha: "uma frase longa"` não é redigida.
   - Sem padrões para `AWS_SECRET_ACCESS_KEY`, `GOOGLE_APPLICATION_CREDENTIALS`, JWTs, `ghp_*`, `sk-*`.
   - **Correção sugerida:** adicionar regex para padrões conhecidos + modo "quoted value" (`label: "..."` captura até a aspa de fechamento).

7. **`buildProtectedSet` — "sucesso" definido por string-match.**
   - `Exit code: non-zero`, `Permission denied`, `command not found` como proxy de erro é frágil e sensível a locale do servidor remoto.
   - **Correção sugerida:** propagar campo estruturado `ToolResult.IsError bool` do agente, e usar isso aqui.

8. **Dream a cada 1h com N=8 mensagens pode custar caro.**
   - A cadência 6×↑ multiplica chamadas LLM para KG-extraction e summarization.
   - **Ação:** instrumentar contadores (`dream_runs_total`, `dream_duration_seconds`, `dream_llm_tokens_total`) antes de subir — se necessário, introduzir dedupe por janela ou backoff adaptativo quando não houver contradições.

### P2 — observabilidade e testes

9. **Cobertura de testes dos novos caminhos.** Não vi testes adicionados para:
   - `looksLikeAccessFailureFact` (correction indicators) — `memory_tools_test.go` não existe ainda nesse escopo.
   - `isCredentialStopwordMatch` (falsos positivos PT/EN).
   - `expireStaleEntries` (variações de formato em .md).
   - `IndexChunks` sob concorrência (dois goroutines, hashes distintos).
   - `findEvidenceContradictions` com múltiplas entidades + corrupção parcial.
   - `buildProtectedSet` — nova heurística dos 8 tool results.

   **Ação:** adicionar pelo menos os três primeiros como blockers do v1.18.2.

10. **Compact on demand.** `[stale]` marcado, mas `Compact()` só roda em dream cycle. Expor `memory(action="compact")` para permitir saneamento imediato (já existe estrutura, falta expor o tool).

11. **Auto-derived wing sem expiry.** `confidence=0.5` persistido é sticky; se o canal muda de semântica, o mapping errado permanece. Adicionar TTL ou renormalização por uso.

12. **SIGUSR1 conflita com profilers (pprof/runtime).** Documentar em `docs/internal-routines.md` + README ops.

13. **Redação — sem métricas.** Adicionar counter `redactions_total{path="sendReply|persist|heartbeat|..."}` para detectar redações excessivas ou ausentes em produção.

---

## 3. Proposta de escopo v1.18.2 (fix release)

### Must-fix (P0)
- [ ] Hardening `expireStaleEntries` — identificador estável + match por linha inteira.
- [ ] Serialização `IndexChunks` por `fileID` (singleflight).
- [ ] Fallback de saúde do watcher fsnotify (scan periódico ou alerta).
- [ ] Guarda de concorrência em `DreamConsolidator.Run`.

### Should-fix (P1)
- [ ] `looksLikeAccessFailureFact`: sinal estruturado + allow-list por categoria.
- [ ] Redactor: quoted values + padrões de secret key conhecidos.
- [ ] `buildProtectedSet`: usar `ToolResult.IsError` em vez de string match.
- [ ] Instrumentação: `dream_*`, `redactions_total`, `index_queue_depth`.

### Nice-to-have (P2)
- [ ] `memory(action="compact")` tool.
- [ ] Testes das 6 superfícies listadas.
- [ ] TTL/decay em auto-derived wing.
- [ ] Documentar `SIGUSR1` e conflitos.

---

## 4. Notas de regressão a verificar

1. **"Agente esquecendo SSH/DB/gcloud":** o flip 1291cc3 é correto em teoria, mas reduz defense-in-depth. Validar em produção que `RedactCredentials` nos persist paths ainda captura 100% dos caminhos — grep por `session.AddMessage*` e confirmar wrapping.
2. **Throughput após remover `IndexMemoryDir` inline:** tempos de `memory save` devem cair; porém busca logo após um save pode retornar stale até o watcher coalescer (~500 ms). Avaliar se isso afeta UX de testes automatizados.
3. **`[stale]` em MEMORY.md:** garantir que `FormatForChannel` / export / backup **não** vaza entradas stale para o usuário final — verificar filtros em `formatter.go` / UI.
4. **Wing auto-derive confidence=0.5:** interage com boost/penalty da hybrid search. Se o canal é de subagent e escapou do exclude, pode enviesar ranking.

---

## 5. Referências (commits)

```
280dcea fix(copilot): allow correction facts past access-failure filter
54004d8 fix(copilot): stronger pre-flight + stopword-aware redactor + block access-failure facts
83d6575 feat(copilot): expose ForceDream via SIGUSR1 for operator-triggered consolidation
63b731e feat(copilot): dream auto-expires contradicted facts + 1h cycle + [stale] filter
19826a9 perf(memory): eliminate SQLite contention — embeddings outside transaction + remove inline re-index
b3964a7 refactor(copilot): generalize operational protections — no hardcoded patterns
6265e17 feat(copilot): proactive skill/memory recall + dream evidence-based fact validation
257aa43 fix(copilot): add sanitizeOutput to scheduler output path for consistency
1291cc3 fix(copilot): move sanitization from LLM input to persist+output layers
e5ed2b9 fix(copilot): preserve operational playbook through compaction + observability
d201180 fix(copilot): SQLite connection pool tuning + BM25 error logging
1147350 fix(copilot): auto-derive wing from channel when no mapping exists
db3118b fix(copilot): memory recall and context assembly — 4 bug fixes
```
