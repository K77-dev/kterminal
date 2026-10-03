# Tarefa 2.0: Cancelamento do turno agêntico no Agent

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Cancelamento do turno agêntico via Esc (itens 1, 3, 4 e 5: `Cancel()`, `EventTurnAborted`, transcript, nenhum `EventError`, parcial fora do contexto LLM)

## Dependências

- 1.0 (assinaturas de `Executor`/`Registry.Execute` com ctx)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

O Agent é o núcleo da feature e o ponto único de classificação de cancelamento. Esta task faz cada `Run()` criar um contexto cancelável derivado de `context.Background()`, guardado com mutex e disparado pelo novo método público `Cancel()`. O `loop` passa a receber esse `ctx`, cobrindo todo o ciclo do turno: `ensureCandidates`, `decide` (Jev), `ChatStream`, espera de confirmação de tool e `Tools.Execute`. Qualquer retorno com `ctx.Err() != nil` (ou `errors.Is(err, context.Canceled)`) segue o caminho de abort: anexa tool results sintéticos pendentes, grava `turn_aborted` no transcript, emite `EventTurnAborted` com o texto parcial acumulado e encerra — nunca `EventError`. Ao final desta task, um teste contra um gateway mock que bloqueia até o contexto do request morrer prova o abort de ponta a ponta (mandatório do PRD), sem goroutine vazando.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27 — apenas stdlib (`context`, `sync`, `errors`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E), `kspec-pr-review` (revisão semântica).
- Padrões do projeto (PRD/techspec): sem comentários no código; eventos via canal com emit não-bloqueante (buffer 512); `strings.Builder` sempre por ponteiro; direção de dependência inalterada (`tui → agent → llm/tools/router/session`).
</skills>

<requirements>
- `Agent` expõe `Cancel()` thread-safe (mutex protege o `cancel context.CancelFunc`); seguro chamar antes de qualquer `Run` e múltiplas vezes (idempotente).
- `Run()` cria o ctx cancelável e o propaga a todo o ciclo do turno via `loop(ctx, userInput)`.
- Verificar `ctx.Err() != nil` imediatamente após cada operação bloqueante do loop: `ensureCandidates`, `decide`, `ChatStream`, `<-ch` (confirmação de tool) e `Tools.Execute`.
- Classificação central: `errors.Is(err, context.Canceled) || ctx.Err() != nil` → caminho de abort; qualquer outro erro → `emitError` (comportamento atual). Cancelamento nunca gera `EventError` nem `session.Event{Type: "error"}`.
- Acumular o texto parcial do stream num `strings.Builder` (por ponteiro) no callback de deltas, além do `EventDelta` emitido.
- Caminho de abort, em ordem: (1) tool results sintéticos `"user aborted this turn"` para todo tool call do passo corrente ainda sem resultado; (2) `Session.Write(session.Event{Type: "turn_aborted", Model: ..., Content: parcial})`; (3) `emit(Event{Kind: EventTurnAborted, Model: ..., Text: parcial})`; (4) return.
- O texto parcial NÃO entra no histórico de mensagens enviado ao LLM no próximo turno (fica visível no chat e no transcript apenas).
- Borda definida na techspec: `ChatStream` retorna normalmente com ctx cancelado no mesmo instante — com tool calls, o abort vence (nada é anexado ao histórico); sem tool calls, o turno é concluído (`EventTurnDone`).
</requirements>

## Subtarefas

- [ ] 2.1 Adicionar campos `mu sync.Mutex` + `cancel context.CancelFunc` ao `Agent`; criar o ctx cancelável em `Run()`; implementar `Cancel()` público
- [ ] 2.2 Fazer `loop` receber `ctx` e propagá-lo a `ensureCandidates`, `decide`, `ChatStream`, espera de confirmação e `Tools.Execute`
- [ ] 2.3 Acumular o parcial do stream com `strings.Builder` por ponteiro no callback de deltas
- [ ] 2.4 Implementar a classificação central de cancelamento (abort vs `emitError`) após cada operação bloqueante
- [ ] 2.5 Implementar o caminho de abort na ordem definida (sintéticos → transcript → `EventTurnAborted` → return)
- [ ] 2.6 Definir `const EventTurnAborted EventKind = "turn_aborted"`, reutilizando os campos `Text` e `Model` do `Event` existente
- [ ] 2.7 Tratar a borda "ChatStream retorna normal com ctx cancelado" conforme techspec
- [ ] 2.8 Escrever os 4 testes de unidade da techspec (ver Testes da Tarefa), incluindo o mandatório do PRD

## Detalhes de Implementação

- Estrutura do cancelamento (struct, `Run`, `Cancel`), regras do `loop(ctx, userInput)` e a ordem do caminho de abort estão na techspec, seção **Design de Implementação** (subseções **Interfaces Principais**, **Estrutura do cancelamento no Agent** e **Regras do `loop`**).
- Mensagem sintética: `llm.Message{Role: "tool", Content: "user aborted this turn", ToolCallID: tc.ID}` — preserva o par `tool_calls` ↔ tool results exigido pelo gateway OpenAI-compatible (techspec, **Modelos de Dados**).
- `EventTurnAborted` usa o mesmo emit não-bloqueante do canal existente (techspec, **Arquitetura → Padrões do projeto preservados**).
- Riscos já mapeados na techspec (seção **Riscos Conhecidos**): race Esc ↔ conclusão natural (benigno), Esc durante `ensureCandidates` (coberto pela classificação central), deltas em race pós-cancelamento (ficam no parcial — desejado).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- `TestAgentCancelAbortsStream` (mandatório do PRD) passa: `EventTurnAborted` com o parcial, ausência de `EventError`, ausência de goroutine vazando (um novo `Run` contra gateway normal completa dentro de timeout).
- Cancelamento durante confirmação de tool produz turno abortado com a última mensagem do histórico `role: tool` sintética (contrato OpenAI preservado).
- `Cancel()` antes de qualquer `Run` não causa panic; `Run` posterior funciona.
- Dois `Cancel` no mesmo turno produzem um único `EventTurnAborted`.
- O parcial não aparece no histórico enviado ao LLM no próximo turno.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`, estendendo os harnesses existentes `mockGateway`, `mockJev`, `collectEvents` — nomes e passos completos na techspec, **Abordagem de Testes → Testes Unidade**):
  - `TestAgentCancelAbortsStream` — gateway envia um chunk e bloqueia em `<-r.Context().Done()`; `Run` → aguardar `EventDelta` → `Cancel` → assertar `EventTurnAborted` com parcial, sem `EventError`, sem goroutine vazando.
  - `TestAgentCancelDuringConfirmAbortsTurn` — `Confirm = true`, tool call mutante; aguardar `EventConfirm`; enviar `false` no `ApproveCh` + `Cancel`; assertar `EventTurnAborted` sem `EventError`; segundo `Run` confirma última mensagem `role: tool` sintética.
  - `TestAgentCancelWhenIdleIsNoOp` — `Cancel()` antes de qualquer `Run`; sem panic; `Run` posterior funciona.
  - `TestAgentDoubleCancelIsIdempotent` — dois `Cancel` no mesmo turno; um único `EventTurnAborted`.
- [ ] Testes de integração — fora do escopo conforme techspec (a morte do bash via contexto é garantida pelo design da task 1.0 e validada no QA).
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — ctx cancelável, `Cancel()`, `EventTurnAborted`, classificação central, parcial, sintéticos, transcript
- `internal/agent/agent_test.go` — 4 testes de unidade (incl. mandatório do PRD)
- `internal/session/session.go` — sem mudança estrutural; novo valor de `Type`: `"turn_aborted"`
- `internal/llm/llm.go`, `internal/router/router.go`, `internal/jev/jev.go` — sem mudança (já aceitam ctx)
