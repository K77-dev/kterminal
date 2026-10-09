# Relatório de Code Review — 013-prd-resiliencia-de-streaming — Task 4.0: timeout de subagente stall-based

## Resumo
- Data: 2026-10-08
- Branch: 013-prd-resiliencia-de-streaming
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 2 (`internal/agent/agent.go`, `internal/agent/agent_test.go`)
- Escopo: apenas `internal/agent` (tasks 1.0/2.0/3.0 em paralelo em `internal/config`/`internal/llm` — não tocados)

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Go 1.27, stdlib only | OK | Único import novo: `sync/atomic` (stdlib); `sync`/`time` pré-existentes |
| Eventos via canal | OK | Estrutura do drain (range sobre `sub.Events` → `a.emit` não-bloqueante) preservada; apenas `watchdog.reset()` inserido no topo do loop |
| gofmt / vet | OK | `gofmt -l internal/agent/` vazio; `go vet ./internal/agent/` limpo |
| Código em inglês | OK | Identificadores e mensagens em inglês |

## Aderência à TechSpec (seção "Stall de subagente")
| Requisito | Implementado | Evidência |
|-----------|--------------|-----------|
| Default `subagentTimeout` 5m → 10m | SIM | `const subagentTimeout = 10 * time.Minute` (agent.go:24) |
| Setter público `SetSubagentTimeout(time.Duration)` | SIM | Setter simples ao lado de `SetPinned`/`SetRouters` (agent.go:160); valor copiado para filhos via `newSubagent`/`RunSyncPersona` (pré-existente, agent.go:478/545) |
| Substituir `context.WithTimeout` total por ctx derivado | SIM | `context.WithCancel(ctx)` + `defer cancel()` (agent.go:372-373) |
| Watchdog `time.AfterFunc(timeout, fire)`; fire marca flag atômica e cancela | SIM | `newStallWatchdog(timeout, func(){ stalled.Store(true); cancel() })` com `var stalled atomic.Bool` (agent.go:375-380) |
| Reset do timer no drain goroutine a cada evento de `sub.Events` | SIM | `watchdog.reset()` no topo do loop de drenagem (agent.go:385) — cobre deltas, rotas, tool calls/resultados, compaction |
| Mensagem dinâmica `error: subtask stalled for %s without progress` | SIM | `fmt.Sprintf("error: subtask stalled for %s without progress", timeout)` com o valor efetivo pós-normalização `<=0` (agent.go:414-416); default renderiza "10m0s" conforme techspec |
| `ctx.Err()` sozinho não distingue stall de Esc | SIM | Distinção por `stalled.Load()`, não por `ctx.Err()`; cancelamento do pai segue o caminho de abort existente (`return "", err`) |
| Sem timer/goroutine vazando | SIM | `defer watchdog.stop()` + stops explícitos após `<-drained` (idempotentes sob mutex); `timer.Stop()` garantido em todo caminho de retorno; drain goroutine continua joined via `close(sub.Events)`/`<-"drained"` (estrutura inalterada) |

## Race do timer — como foi resolvida
`stallWatchdog` serializa `trigger`/`reset`/`stop` com um único `sync.Mutex` e um latch `done` (padrão "mutex simples protegendo stop/reset/fired" autorizado pela task):

1. **trigger × reset**: se o timer expira e um evento chega quase simultaneamente, quem pegar o mutex primeiro vence. Se `trigger` vence: latch `done`, `fire()` roda → stall (correto: a janela inteira de silêncio já decorreu). Se `reset` vence: `timer.Reset(window)` rearma; o `trigger` já agendado ainda roda e dispara — também correto pela mesma razão (silêncio de janela completa antes do reset). Não existe caminho que dispare stall sem janela completa de silêncio nem que perca um stall real.
2. **trigger × stop (leitura da flag)**: `trigger` executa `fire()` **segurando o mutex**. Assim `stop()` só retorna depois de `fire()` completo (ou trava o latch e o trigger vira no-op). Isso fecha a janela em que `stop()` retornaria entre o latch e o `fire()`, o que permitiria um stall real ser lido como `stalled == false` e propagado como abort. `fire()` é não-bloqueante (`atomic.Store` + `cancel()`) e não reentra no watchdog → sem deadlock.
3. **stop explícito antes de consultar a flag**: `watchdog.stop()` após `<-drained` em ambos os caminhos de erro torna `stalled.Load()` deterministicamente pós-todos-os-disparos (o `defer` extra cobre panics/retornos antecipados e é idempotente).
4. **AfterFunc sem canal**: o pattern perigoso de `Reset`/drain de `timer.C` não se aplica — `AfterFunc` não tem canal para drenar; a exclusão mútua via mutex substitui o pattern documentado.

## Tasks Verificadas
| Subtask | Status | Observações |
|---------|--------|-------------|
| 4.1 ctx derivado + watchdog resetável + flag `stalled` atômica | COMPLETA | `runSubagent` + tipo `stallWatchdog` |
| 4.2 Reset do timer no drain a cada evento | COMPLETA | `watchdog.reset()` no range de `sub.Events` |
| 4.3 Mensagem dinâmica de stall; abort do pai não vira stall | COMPLETA | `stalled.Load()` gateia a mensagem; abort retorna `("", err)` como antes |
| 4.4 `SetSubagentTimeout` + default 10m propagado | COMPLETA | Setter novo; cópia para filhos já existia em `newSubagent`/`RunSyncPersona` |
| 4.5 Testes (silencioso → stall; progresso contínuo sobrevive; Esc aborta) | COMPLETA | 3 testes abaixo + regressão `TestEscCancelsSubagent` verde |

## Testes
| Teste | Tipo | O que prova |
|-------|------|-------------|
| `TestSubagentTimeout` (adaptado) | Unitário | Subagente emite 1 delta e sila; janela 50ms → resultado exato `error: subtask stalled for 50ms without progress` (dinâmico com o valor injetado); `EventTurnAborted` depth 1 com o parcial streamado; principal segue vivo e completa o turno |
| `TestSubagentStallWatchdogResetByProgress` (novo) | Unitário | Janela 250ms; stream do subagente emite 13 deltas espaçados de 50ms (~650ms totais, 2,6x a janela que mataria na semântica antiga de parede) → completa com o texto integral; nenhum abort depth 1; elapsed ≥ 600ms assertado |
| `TestSubagentParentCancelPropagatesAbort` (novo) | Unitário | `RunSync` direto com ctx do pai cancelado durante o stream → retorna `("", context.Canceled)` — erro de abort propagado, não mensagem de stall |
| `TestEscCancelsSubagent` (regressão, inalterado) | Integração | Esc durante subagente → abort depth 1 + abort depth 0 + follow-up funciona — caminho de abort intacto |

- Total no pacote: 104 testes, 104 passando, 0 falhas
- `-race` limpo; testes de timing executados 15+ repetições (`-count=5` e `-count=10`) sem flakiness; margens ≥5x entre spacing e janela (padrão de tolerância do `bash_test.go` citado na techspec)

## Interpretações Documentadas (desvios da letra da task, resolvidos a favor dos requisitos)
1. **Cheque de `stalled` também no caminho de erro de `ensureCandidates` (ressalva principal).** A task especifica o novo cheque no caminho pós-`runLoop`. Sob a semântica nova (stall → `cancel()`), um stall durante o listing de modelos produz erro wrapped em `context.Canceled`; sem o cheque, o `isAbort` do principal trataria como Esc e derrubaria o turno pai — regressão vs. hoje (o deadline interno de 15s de `ensureCandidates` sempre vencia o de 5m externo, produzindo tool error sem abort). Com o cheque, stall durante o listing devolve a mesma mensagem de stall. Necessário para o invariant "stall não derruba o principal" (REQ-003/010).
2. **Deadline herdado do caller agora é abort, não timeout.** O código antigo checava `ctx.Err() == context.DeadlineExceeded`; com `WithCancel`, um caller que passe ctx com deadline próprio não gera mais mensagem de timeout — flui pelo caminho de abort (`return "", err`), exatamente a distinção "não stalled → abort" exigida. Nenhum caller atual passa ctx com deadline (tools executam com ctx do turno, sem deadline).
3. **`fire()` sob o mutex do watchdog.** A task sugeria "mutex simples protegendo stop/reset/fired" sem especificar onde `fire` roda. Executá-lo sob o mutex transforma `stop()` em ponto de sincronização para a leitura de `stalled` (ver item 2 da seção da race). Sem isso existiria uma janela (latch→fire) em que stall real seria lido como abort.

## Pontos Positivos
- Distinção stall × Esc por flag explícita, não por inspeção de `ctx.Err()` — imune a deadlines de callers.
- Watchdog é um tipo mínimo (4 métodos, ~40 linhas) reutilizável e testável; sem goroutine própria (AfterFunc usa o runtime).
- Mensagem de resultado dinâmica elimina o hardcoded "after 5m" que mentia sobre o valor injetado (ressalva 5 da review 2.0 do 010).
- Estrutura de drenagem/re-emissão/transcript byte-idêntica: 100 testes pré-existentes do pacote passam sem nenhuma mudança de asserção além do `TestSubagentTimeout` adaptado.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/agent/agent.go | 396-404 | Cheque de stall no caminho de `ensureCandidates` além da letra da task (Interpretação 1) | Nenhuma — necessário para não derrubar o principal; documentado |
| Baixa | internal/agent/agent_test.go | 3124-3160 | `TestSubagentTimeout` mantém janela de 50ms do precedente (fase pré-stream sem eventos exposta à janela) — perfil de flakiness idêntico ao teste antigo, não pior | Opcional: subir para 100ms se flakiness aparecer em CI carregado — **APLICADO pós-review (orquestrador)**: janela 50ms → 100ms |

## Recomendações
- Task 5.0 (wiring): chamar `SetSubagentTimeout` no agente raiz após `config.Load()`; o valor propaga para subagentes e personas pela cópia existente. `--doctor` imprime `agent: subagent stall <valor>`.
- Evolução futura (fora de escopo): re-arm do watchdog por bytes de stream (nível llm) já coberto por REQ-001; não duplicar no agent.

## Conclusão
Implementação aderente à task 4.0 e à seção "Stall de subagente" da techspec: watchdog de stall resetável por evento drenado, flag atômica distinguindo stall de Esc, mensagem dinâmica com o valor efetivo, default 10m, setter público, sem vazamento de timer/goroutine. A race do timer foi resolvida com mutex único + latch `done` + `fire` sob o mutex (ponto de sincronização para leitura da flag), validada com `-race` e repetições. As três ressalvas são de severidade baixa, documentadas e não bloqueiam as tasks subsequentes. **APROVADO COM RESSALVAS**

## Checks Executados
| Check | Resultado |
|-------|-----------|
| `go build ./internal/agent/` | OK |
| `go vet ./internal/agent/` | OK |
| `gofmt -l internal/agent/` | OK (nenhum arquivo listado) |
| `go test -race ./internal/agent/ -count=1` | OK — 104/104 passando |
| `go test -race -run 'TestSubagent*|TestEscCancelsSubagent' -count=5` e `-count=10` | OK — sem flakiness (diligência extra para os testes de timing) |
