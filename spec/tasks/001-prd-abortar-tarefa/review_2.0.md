# Review — Task 2.0: Cancelamento do turno agêntico no Agent

- **Task**: 2.0 (`spec/tasks/001-prd-abortar-tarefa/2_task.md`)
- **Feature**: abortar tarefa em execução (Esc) — REQ-001, itens 1, 3, 4 e 5
- **Reviewer**: kspec-review-runner
- **Data**: 2026-10-03
- **Veredito**: **APROVADO**

## Veredito

**APROVADO** — a implementação está em conformidade com o PRD, a Tech Spec e a Task 2.0. O núcleo da feature (ctx cancelável por turno, `Cancel()` thread-safe, classificação central, caminho de abort na ordem exata, sintéticos preservando o contrato OpenAI, parcial fora do contexto LLM) está completo e provado pelos 4 testes exigidos. Nenhuma regra crítica foi violada; nenhuma mudança fora de escopo. O desvio reportado (ordem `Cancel` → `false` no teste 2) foi avaliado e é **aceito** — ver seção própria.

## Checklist percorrido com evidência

### 1. Conformidade com a spec

| Requisito da spec | Evidência | Status |
| --- | --- | --- |
| `Cancel()` thread-safe, nil-safe, idempotente | `internal/agent/agent.go:126-132` — mutex protege o `cancel context.CancelFunc`; nil-check; `cancelFunc` é idempotente por natureza | OK |
| `Run()` cria ctx cancelável de `context.Background()` e lança `loop(ctx, userInput)` | `agent.go:119-123` — idêntico à seção *Estrutura do cancelamento no Agent* da techspec; campos `mu sync.Mutex` + `cancel` em `agent.go:65-66` | OK |
| ctx propagado a `ensureCandidates`, `decide`, `ChatStream`, espera de confirmação e `Tools.Execute` | `agent.go:136` (`ensureCandidates(ctx)`), `:153` (`decide(ctx, ...)`), `:179` (`ChatStream(ctx, ...)`), `:214` (`approved = <-ch` + check `:215`), `:224` (`Tools.Execute(ctx, ...)`) | OK |
| Verificação de `ctx.Err() != nil` imediatamente após cada operação bloqueante | Pós-`ensureCandidates`: `agent.go:144` (check puro, cobre retorno nil com ctx morto). Pós-`decide`: `isAbort` em `:154` (avalia `ctx.Err()` mesmo com `err == nil`). Pós-`ChatStream` com erro: `:185`; sem erro: `:192`. Pós-`<-ch`: `:215`. Pós-`Execute`: `isAbort` em `:225` | OK |
| Classificação central `errors.Is(err, context.Canceled) \|\| ctx.Err() != nil` → abort; outros erros → `emitError` | `agent.go:264-266` (`isAbort`), único ponto de classificação, usado em `:137`, `:154`, `:185`, `:225` — exatamente a regra da techspec | OK |
| Caminho de abort na ordem exata: sintéticos → `turn_aborted` no transcript → `EventTurnAborted` → return | `agent.go:268-274` (`abortTurn`): (1) sintéticos `"user aborted this turn"` com `ToolCallID` em `:269-271`; (2) `Session.Write(session.Event{Type: "turn_aborted", Model: ..., Content: parcial})` em `:272`; (3) `emit(Event{Kind: EventTurnAborted, ...})` em `:273`; (4) `return` em todos os call sites (`:139`, `:146`, `:156`, `:187`, `:194`, `:217`, `:227`) | OK |
| Parcial em `strings.Builder` por ponteiro no callback de deltas | `agent.go:135` (`var partial strings.Builder` no escopo do loop), `:181` (`partial.WriteString(s)` no callback, além do `EventDelta`), passado como `&partial` a `abortTurn` — builder nunca copiado por valor | OK |
| `const EventTurnAborted EventKind = "turn_aborted"` reutilizando `Text`/`Model` | `agent.go:30`; emit em `:273` usa apenas campos existentes de `Event` | OK |
| Borda ChatStream-retorna-normal com ctx cancelado: com tool calls abort vence (nada anexado); sem tool calls turno concluído | `agent.go:192-195` — o check vem **antes** do append do assistant com tool calls (`:205`), logo nada do passo entra no histórico; sem tool calls o fluxo segue para `EventTurnDone` (`:251-259`). Bônus: o check também precede a contabilização de custo (`:197`), preservando "turnos abortados não contabilizam custo" | OK |

### 2. Invariantes críticos

- **Cancelamento nunca gera `EventError` nem `session.Event{Type: "error"}`**: `emitError` (`agent.go:109-112`) é a única fonte de ambos e todos os seus 5 call sites no `loop` são precedidos por `isAbort`/`ctx.Err() != nil` (`:137→141`, `:154→158`, `:185→189`, `:225→229`; o de maxSteps `:261` não é alcançável por cancelamento sem antes abortar em operação bloqueante). `abortTurn` nunca chama `emitError`. Provado nos 4 testes: `waitForEvent` (`agent_test.go:350-352`) fataliza ao ver qualquer `EventError`.
- **Parcial NÃO entra no histórico LLM do próximo turno**: estruturalmente, `partial` nunca é anexado a `a.messages`; o user message do turno abortado permanece (decisão #3 da techspec, "não descartar o contexto do pedido"). Provado em `TestAgentCancelAbortsStream` (`agent_test.go:485-492`): o request do 2º turno captura exatamente 2 mensagens (`user "long running question"`, `user "follow up"`) e nenhuma contém `"partial answer"`.
- **Par `tool_calls` ↔ tool results preservado**: `pending := result.ToolCalls` (`agent.go:206`) é consumido com `pending = pending[1:]` após cada resultado real (`:238`); abort durante confirmação (`:215-218`) ou execução (`:225-228`) sintetiza resultado para **todo** call ainda sem resposta com `ToolCallID` correto (`:269-271`) — calls anteriores já têm resultado real anexado. Na borda do `:192`, nem o assistant nem sintéticos são anexados (histórico íntegro: nada do passo cancelado entra). Provado em `TestAgentCancelDuringConfirmAbortsTurn` (`agent_test.go:510-524`): histórico do 2º turno = `user` → `assistant` (tool call `call_1`) → `tool` sintético `"user aborted this turn"`/`ToolCallID: call_1` → `user`.
- **Sem comentários no código**: o diff não introduz nenhum comentário em `agent.go` nem em `agent_test.go`.
- **Emit não-bloqueante preservado**: `emit` (`agent.go:102-107`) inalterado (select/default), canal com buffer 512 (`:78`); `EventTurnAborted` usa o mesmo emit.
- **Direção de dependência inalterada**: imports novos são apenas stdlib (`errors`, `sync` — `agent.go:5,8`); nada em `tui → agent → llm/tools/router/session` mudou de direção.

### 3. Testes exigidos

Os 4 testes da techspec existem e assertam o que a task pede (`internal/agent/agent_test.go`):

| Teste | O que prova | Evidência |
| --- | --- | --- |
| `TestAgentCancelAbortsStream` (`agent_test.go:465`, mandatório do PRD) | Gateway bloqueante envia 1 chunk e bloqueia em `<-r.Context().Done()` (`:414`); `Run` → `EventDelta` → `Cancel` → `EventTurnAborted` com `Text: "partial answer"` e `Model: "glm-5.3"`; sem `EventError`; sem goroutine vazando; parcial fora do histórico LLM | `blockingGateway` (`:401-429`); asserções `:475-480`; prova de não-vazamento = 2º `Run` contra o mesmo gateway completa com `EventTurnDone` dentro do timeout de 5s (`:482-483`); mensagens capturadas do 2º request = 2, sem o parcial (`:485-492`) |
| `TestAgentCancelDuringConfirmAbortsTurn` (`agent_test.go:495`) | `Confirm = true`, tool call bash mutante; aguarda `EventConfirm`; `Cancel` + `false` no `ApproveCh`; `EventTurnAborted` sem `EventError`; sintético verificado no histórico do 2º `Run` | `confirmGateway` (`:431-463`); asserções do histórico capturado `:510-524` (4 mensagens, `role: tool` sintético com `ToolCallID: call_1`) |
| `TestAgentCancelWhenIdleIsNoOp` (`agent_test.go:527`) | `Cancel()` antes de qualquer `Run` não causa panic (`cancel == nil` → no-op); `Run` posterior funciona | `:532-535` — `Run` posterior completa com `EventTurnDone` |
| `TestAgentDoubleCancelIsIdempotent` (`agent_test.go:538`) | Dois `Cancel` no mesmo turno produzem um único `EventTurnAborted` | `:546-548` (duplo cancel + 1º abort); `:550-562` conta aborts adicionais no 2º turno e exige total == 1 |

### 4. Verificação obrigatória

`go build ./... && go vet ./... && gofmt -l . && go test ./...` — tudo verde:

```
?   kterminal        [no test files]
ok  kterminal/internal/agent
?   kterminal/internal/catalog    [no test files]
?   kterminal/internal/clipboard [no test files]
?   kterminal/internal/config     [no test files]
?   kterminal/internal/jev        [no test files]
ok  kterminal/internal/llm
?   kterminal/internal/router     [no test files]
?   kterminal/internal/session    [no test files]
ok  kterminal/internal/tools
ok  kterminal/internal/tui
```

- `go vet ./...`: sem achados. `gofmt -l .`: sem output.
- `go test -race -count=1 ./internal/agent/`: `ok kterminal/internal/agent 1.882s` — sem data races (crítico dado o `Cancel()` concorrente com o loop e o `partial` escrito no callback do stream).

### 5. Escopo

- `git status`/`git diff`: mudanças apenas em `internal/agent/agent.go` e `internal/agent/agent_test.go` (esta task), mais o diff pré-aprovado da Task 1.0 em `internal/tools/{tools,bash,fs}.go` + testes novos não-rastreados, e o checkbox `[x] 1.0` em `tasks.md` (bookkeeping da task anterior).
- **Nenhuma** mudança em `internal/session`, `internal/llm`, `internal/router`, `internal/jev`, `internal/tui` — conforme esperado. `session.Event` aceita `Type: "turn_aborted"` com os campos existentes (`session.go:12-17`), sem mudança estrutural, como a techspec determina.

## Problemas encontrados

Nenhum problema bloqueante ou corretivo.

## Avaliação do desvio reportado (item 6 do checklist)

**Desvio**: no teste 2, a ordem é `ag.Cancel()` **antes** do `false` no `ApproveCh` (`agent_test.go:503-504`); a techspec sugere "enviar `false` no `ApproveCh` + `Cancel`".

**Avaliação: desvio ACEITO — o implementador tem razão.**

- O loop bloqueia em `approved = <-ch` (`agent.go:214`); `Cancel()` cancela o ctx mas **não desbloqueia o canal** — quem desbloqueia é o envio no `ApproveCh` (buffered, tamanho 1, `agent.go:212`, logo o envio nunca bloqueia).
- Com a ordem literal da techspec (`false` → `Cancel`), o check `ctx.Err() != nil` em `agent.go:215` ralaria com o `Cancel()` do teste: se o loop vencesse a corrida, tomaria o caminho `approved = false` ("user declined this tool call" anexado como resultado real, `EventToolResult` emitido) e o abort só aconteceria no próximo ponto bloqueante — o turno ainda abortaria sem `EventError` (a classificação central pega no `decide`/`ChatStream` seguinte), mas a asserção do sintético no histórico do 2º `Run` (`agent_test.go:519-521`) seria flaky.
- Com `Cancel()` antes do `false`, a relação happens-before (cancel → send → receive) garante `ctx.Err() != nil` quando `<-ch` retorna → abort determinístico com sintético. O contrato da TUI (responder o canal de confirmação) continua exercitado pelo envio do `false`.
- Conclusão: a ordem da techspec era uma receita de teste underspecified/racy; a inversão é necessária para o determinismo e não enfraquece nenhuma asserção.

**Recomendação a repassar à Task 3.0** (TUI, REQ-002): no handler do `Esc` no estado `confirm`, chamar `agent.Cancel()` **antes** de enviar `false` no `ApproveCh` — garante o sintético `"user aborted this turn"` e um único desfecho limpo. E: é **obrigatório** sempre responder o `ApproveCh` ao cancelar a partir do estado confirm — `Cancel()` sozinho não desbloqueia `<-ch` e o loop ficaria pendurado sem emitir `EventTurnAborted` (vazamento de goroutine). O design é o da techspec (check pós-`<-ch`), e o REQ-002 já prescreve "recusa o tool pendente e cancela o turno"; a Task 3.0 deve explicitar a ordem.

## Recomendações (não-bloqueantes)

1. **`partial.Reset()` por passo** (`agent.go:152`): decisão de implementação não prescrita na spec, porém alinhada com o modelo da TUI — `m.stream` é resetado a cada `EventToolStart` (`tui.go:272`) e `EventTurnDone.Text` carrega apenas o conteúdo do último passo. Em turnos multi-passo, `EventTurnAborted.Text` = parcial do passo corrente = exatamente o que a TUI acumulou em `m.stream`. Repassar à Task 3.0: no caso `EventTurnAborted`, renderizar `m.stream` (ou `e.Text` — coincidem) como markdown + meta `⊘ interrompido`.
2. **`EventTurnAborted` com `Model: ""`** quando o abort ocorre antes do `decide` (ex.: durante `ensureCandidates`, `agent.go:138,145`): aceitável — nenhum modelo foi decidido ainda; a meta line da TUI deve tolerar model vazio.
3. **Pré-existente, fora do escopo do diff**: a variável `deltas` (`agent.go:178,180`) é incrementada e nunca lida. Não foi introduzida por esta task; registrar para housekeeping futuro.

## Conclusão

A task 2.0 entrega o núcleo da feature conforme a spec: cancelamento por turno com `Cancel()` thread-safe e idempotente, classificação central que nunca vira `EventError`, caminho de abort na ordem exata com tool results sintéticos preservando o contrato OpenAI, parcial acumulado por ponteiro e comprovadamente fora do contexto LLM, borda do ChatStream tratada como a techspec define, e os 4 testes exigidos (incluindo o mandatório do PRD com prova de não-vazamento) passando com `-race`. O desvio de ordem no teste 2 é correto e deve virar recomendação explícita de ordenamento para a Task 3.0. A base está pronta para a task 3.0 (Esc na TUI, renderização e hint bar).
