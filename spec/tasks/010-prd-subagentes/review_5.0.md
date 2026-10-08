# Relatório de Code Review - Subagentes: Wiring no `main.go` e verificação final integrada (Task 5.0)

## Resumo
- Data: 2026-10-05
- Branch: 002-010-prds-kterminal
- Status: APROVADO
- Arquivos Modificados: 1 (`main.go` — wiring da task 5.0)
- Linhas Adicionadas: 1
- Linhas Removidas: 0
- Escopo do review: wiring da task 5.0 + verificação final integrada da feature 010 completa (tasks 1.0–4.0 já revisadas: 1.0 APROVADO, 2.0 APROVADO COM RESSALVAS, 3.0 APROVADO, 4.0 APROVADO COM RESSALVAS)

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | `grep -rn "//"` em `agent.go`, `tools.go`, `session.go`, `main.go` — zero comentários (apenas URLs em strings) |
| Go 1.27, módulo `kterminal`, stdlib (bubbletea/lipgloss na TUI) | OK | `go.mod` sem mudanças da feature 010 (termenv direto veio de feature anterior); subagente usa apenas stdlib |
| Eventos via canal (buffer 512, emit não-bloqueante nos dois níveis) | OK | `make(chan Event, 512)` em `New` (linha 105) e `newSubagent` (linha 242); `emit` com `select/default` (linhas 139-144) |
| Receivers por valor na TUI | OK | `handleAgentEvent`/`Update`/`hintBar` por valor; mutadores internos (`handleNestedEvent`, `resetLiveBlock`) por ponteiro sobre a cópia local — padrão pré-existente do projeto |
| Sem goroutines paralelas (v1 sequencial) | OK | Únicos spawn points: wrapper `go func` do `loop` (pré-existente) e drain forwarder do `RunSync` — joined via `<-drained` antes do retorno; loop do subagente roda síncrono na goroutine do passo da tool |
| Formatação/lint | OK | `gofmt -l .` vazio; `go vet ./...` limpo |
| Nomenclatura | OK | `AttachTaskTool` chamado uma única vez no wiring, conforme recomendação do review 3.0 (`Registry.Register` não é idempotente) |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `main.go`: `ag.AttachTaskTool()` após a construção do agent | SIM | Inserido após `ag.Telemetry = store`, antes de `SetMessages`; chamada única (grep confirma 1 ocorrência em `main.go`) |
| Arquitetura: `depth`, `runLoop` extraído, `newSubagent`, `RunSync`, `AttachTaskTool`, re-emissão, limites | SIM | Tasks 1.0–3.0 (reviews próprios); `subagentMaxSteps = 10`, `subagentTimeout = 5 * time.Minute` |
| `Register(t Tool)` exportado | SIM | Task 3.0 (pull-forward documentado no review 2.0) |
| `session.Event` com `Depth int` (`json:"depth,omitempty"`) | SIM | Task 4.0; eventos do fluxo principal permanecem sem o campo |
| TUI: indentação 2 espaços por nível em `colSecondary`, hint bar `subagent running` | SIM | Task 4.0; `nestedPrefix(e.Depth)` + `nestedStyle`; flag `subagentActive` com lifecycle completo (limpa em tool result/turn done/turn aborted/error) |
| Fluxo: prompt → `task` → `RunSync` síncrono → re-emissão `Depth: 1`/`ParentTool: "task"` → tool result → principal re-roteia | SIM | Provado ponta a ponta por `TestTaskToolRunsSubagent` (3 calls: parent, subagent, parent) |
| Erro de steps/timeout como resultado (não `EventError`) — principal segue vivo | SIM | `"error: subtask exceeded max steps (10)"` / `"error: subtask timed out after 5m"` como tool result |
| Cancelamento compartilhado (ctx derivado do turno) | SIM | `RunSync` deriva do ctx do passo; `TestEscCancelsSubagent` prova subagente+principal abortados e turno seguinte funcional |
| Fora de escopo do PRD não implementado | SIM | Verificação negativa abaixo |

Verificação negativa (sem scope creep):
- Paralelismo real entre subagentes: NÃO implementado — `RunSync` é sequencial no passo da tool.
- Múltiplos subagentes simultâneos: NÃO implementado — um por vez.
- Profundidade > 1: NÃO implementado — limite estrutural (registry do subagente sem `task`; `AttachTaskTool` recusa `depth != 0`).
- Subagentes nomeados/persistidos/reutilizáveis: NÃO implementado — estado por chamada; snapshots do subagente suprimidos (`writeSnapshot` pula `depth > 0`; transcript termina com snapshot do principal).
- Progresso granular na UI: NÃO implementado — TUI renderiza apenas rota/tool/result aninhados; deltas/tool outputs aninhados ignorados (decisão documentada no review 4.0).

## Checklist de Critérios de Aceite do PRD (subtarefa 5.3)
| Critério de Aceite | REQ | Evidência | Status |
|---|---|---|---|
| O agente principal pode chamar `task`; o subagente não vê a tool nas suas definições | REQ-001 | `TestTaskToolRunsSubagent` (principal chama `task`, 3 calls no gateway) + `TestSubagentDoesNotSeeTaskTool` (request do subagente sem `task`, com tools padrão; requests do principal com `task`) + `TestAttachTaskToolDepthGuard` (guarda `depth == 0`, `Mutating: true`); wiring ativa a tool no app real | OK |
| Subagente roda com roteamento próprio (modelo independente do principal) | REQ-002 | `TestSubagentRoutesIndependently` (subagente usa `deepseek-v4.1-flash` enquanto principal usa `glm-5.3`; 3 decisões Jev; state menciona a description; mensagens system+user corretas) | OK |
| A resposta final chega como tool result e o principal continua com ela | REQ-002 | `TestTaskToolRunsSubagent` (tool result = texto final do subagente, depth 0; `EventTurnDone` do principal responde com o resultado; último evento fecha o turno) | OK |
| Eventos do subagente renderizam aninhados, não misturados ao fluxo principal | REQ-003 | `TestNestedEventsCarryDepth` (eventos depth 1 com `ParentTool: "task"`; depth 0 sem) + `TestNestedEventsRenderIndented` (prefixo 2 espaços em `colSecondary`; fluxo principal mantém `colTextMuted` sem indent; deltas aninhados não vazam para o stream) | OK |
| Subagente que estoura 10 steps ou 5min devolve erro como resultado — o principal segue vivo | REQ-004 | `TestSubagentStepsLimit` (exatamente 10 tool starts do subagente; result `error: subtask exceeded max steps (10)`; principal completa o turno) + `TestSubagentTimeout` (result `error: subtask timed out after 5m` com timeout injetado de 50ms; `EventTurnAborted` depth 1) | OK |
| Esc cancela o turno inteiro, incluindo subagentes | REQ-005 | `TestEscCancelsSubagent` (`Cancel()` → `EventTurnAborted` depth 1 (subagente) e depth 0 (principal); turno seguinte completa — sem goroutine vazando) | OK |
| Hint bar mostra `subagent running` enquanto há subagente ativo | REQ-006 | `TestHintBarShowsSubagentRunning` (hint durante `EventToolStart` da task; some no tool result; limpa também no abort sem result — leak coberto) | OK |
| Transcript grava eventos aninhados com campo de profundidade | REQ-006 | `TestSubagentWritesTranscriptWithDepth` (JSONL com eventos `"depth": 1` do subagente, eventos do principal sem o campo, exatamente 1 snapshot — do principal; `tool_result` da task com a resposta final) | OK |
| `task` é mutante e herda `--confirm` | REQ-001 | `TestSubagentConfirmInherited` (confirm da `task` em depth 0 + confirm do `bash` do subagente em depth 1) + `TestAttachTaskToolDepthGuard` (`IsMutating`) | OK |

## Prontidão para o `kspec-qa` (subtarefa 5.4) — cenários E2E
1. **Delegação real visível**: prompt que induza o modelo a chamar `task` → linha `● task(...)` com estilo de tool normal → eventos do subagente (rota, tools, resultados) indentados 2 espaços em `colSecondary` → resultado final volta como tool result → principal responde ao usuário com o resultado.
2. **Limite de steps**: subtarefa que exija > 10 steps → resultado `error: subtask exceeded max steps (10)` renderizado → agente principal segue vivo e decide o próximo passo. (Timeout de 5min: validado por teste unitário com timeout injetado; E2E real de 5min é impraticável — cobrir via limite de steps.)
3. **Esc cancela tudo**: durante subagente ativo, Esc → turno inteiro abortado (subagente + principal), hint bar remove `subagent running`, turno seguinte funciona normalmente.
4. **Hint bar**: durante a execução da delegação, hint mostra `subagent running` (no lugar de `Thinking`); após o tool result, volta a `Thinking` enquanto o turno continua.
5. **Transcript**: JSONL da sessão contém eventos do subagente com `"depth": 1` e eventos do fluxo principal sem o campo; último snapshot é do fluxo principal (resume não é poluído).
6. **`--confirm`**: com a flag, a delegação pausa para aprovação (`task` é mutante); tools mutantes do subagente também pausam (eventos com depth 1).

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 5.1 Ligar `ag.AttachTaskTool()` no wiring do `main.go` | COMPLETA | Chamada única após a construção do agent |
| 5.2 Verificação encadeada completa verde | COMPLETA | `go build ./... && go vet ./... && gofmt -l . && go test ./... -count=1` — tudo verde |
| 5.3 Checklist de critérios de aceite com evidência | COMPLETA | Tabela acima — 9/9 critérios com teste que o cobre |
| 5.4 Cenários E2E para o QA listados | COMPLETA | Seção acima — 6 cenários |
| 1.0–4.0 (feature) | COMPLETAS | Reviews próprios: 1.0 APROVADO, 2.0 APROVADO COM RESSALVAS (decisões de design documentadas, nenhuma pendente), 3.0 APROVADO, 4.0 APROVADO COM RESSALVAS (idem) |

## Testes
- Total de Testes: 185 (main 9, agent 57, catalog 4, llm 5, session 9, telemetry 11, tools 31, tui 59)
- Passando: 185
- Falhando: 0
- Coverage: n/a (sem tool de coverage configurado no projeto); suíte de subagente cobre delegação, roteamento independente, invisibilidade da tool, depth, limites de steps, timeout, cancelamento, transcript, confirm, indentação e hint bar
- Race detector: `go test -race ./internal/agent/ ./internal/tui/ -count=1` limpo (diligência extra — feature tem drain forwarder concorrente)

Testes desta task: nenhum novo (conforme definido na task — "fora o wiring de uma linha, esta task não escreve código novo"); a verificação é a suíte completa das tasks 1.0–4.0 passando, com o wiring real compilado no binário.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| - | - | - | Nenhum problema encontrado | - |

## Pontos Positivos
- Wiring mínimo e exato: uma linha, uma chamada, posição correta (após construção completa do agent) — a feature inteira fica ativa no app real sem tocar em mais nada.
- Recomendação do review 3.0 atendida: chamada única evita duplicação da definição de `task` no request (`Registry.Register` não é idempotente).
- Cobertura de aceite impecável: cada critério do PRD mapeia a pelo menos um teste que o prova com comportamento real (mocks sequenciais de gateway/Jev, transcript JSONL inspecionado, ANSI da TUI verificado).
- Verificação negativa confirma zero scope creep: nada do "Fora de Escopo" do PRD foi implementado.
- Fluxo ponta a ponta provado por testes de integração: prompt → tool call `task` → subagente com roteamento próprio → re-emissão aninhada → tool result → principal responde → transcript com depth.

## Recomendações
- `kspec-qa`: executar os 6 cenários E2E listados (subtarefa 5.4) contra gateway real.
- `kspec-pr-review`: revisão semântica da entrega completa contra PRD/TechSpec/tasks como próximo passo do fluxo.
- Se o QA julgar perda de informação, a renderização de tool output aninhado (live block em `colSecondary`) pode ser adicionada depois como melhoria incremental — fora de escopo (recomendação herdada do review 4.0).

## Ressalvas Restantes
Nenhuma pendente. As ressalvas dos reviews 2.0 e 4.0 são decisões de design documentadas para lacunas/contradições da spec (drain forwarder joined, gravação de transcript no motor, snapshots do subagente suprimidos, eventos aninhados ignorados seletivamente na TUI, constante local `taskTool`), todas cobertas por teste e nenhuma exigindo correção.

## Conclusão
A task 5.0 fecha a feature 010 exatamente como especificado: o wiring de uma linha ativa a tool `task` no agente principal do app real (chamada única, conforme recomendação do review 3.0), a verificação encadeada completa passa (`go build ./... && go vet ./... && gofmt -l . && go test ./...` — 185/185, race detector limpo), os 9 critérios de aceite do PRD foram percorridos item a item com evidência de teste para cada um, os padrões do projeto foram confirmados por inspeção (sem comentários, canais buffer 512 com emit não-bloqueante, v1 sequencial, receivers conforme padrão) e a verificação negativa confirma que nenhum item fora de escopo foi implementado. A feature está pronta para o `kspec-qa` (cenários E2E listados) e para o `kspec-pr-review`. **APROVADO**.

## Checks Executados
| Check | Resultado |
|-------|-----------|
| `go build ./...` | OK |
| `go vet ./...` | OK |
| `gofmt -l .` | OK (saída vazia) |
| `go test ./... -count=1` | OK — 185/185 passando, 0 falhas |
| `go test -race ./internal/agent/ ./internal/tui/ -count=1` | OK (diligência extra) |
