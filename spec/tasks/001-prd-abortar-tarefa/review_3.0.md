# Review — Task 3.0: Tecla Esc na TUI, renderização do turno abortado e hint bar

- **Task**: 3.0 (`spec/tasks/001-prd-abortar-tarefa/3_task.md`)
- **Feature**: abortar tarefa em execução (Esc) — REQ-002, REQ-003
- **Reviewer**: kspec-review-runner
- **Data**: 2026-10-03
- **Veredito**: **APROVADO**

## Veredito

**APROVADO** — a implementação está em conformidade com o PRD (REQ-002/REQ-003), a Tech Spec e a Task 3.0. O contrato de interação por estado está completo e correto: Esc-busy cancela, Esc-idle é no-op, Esc-em-confirm segue a ordenação **obrigatória** da review 2.0 (`Cancel()` antes do `false` no `ApproveCh`), `n` sozinho apenas recusa, `ctrl+c` intocado nos três estados. O caso `EventTurnAborted` espelha o `EventTurnDone` com a meta `⊘ interrompido · <model>`, a hint bar segue o padrão visual existente e o marcador é textualmente legível no conteúdo plano copiável (provado em teste). Os 5 testes exigidos existem, assertam o que a task pede e passam com `-race`. Nenhuma mudança fora de escopo; o diff pré-aprovado das Tasks 1.0/2.0 está intacto. As três observações do implementador foram avaliadas e são aceitas — ver seção própria.

## Checklist percorrido com evidência

### 1. Conformidade com a spec

| Requisito da spec | Evidência | Status |
| --- | --- | --- |
| Esc com `busy = true` chama `agent.Cancel()` | `internal/tui/tui.go:339-343` — `case "esc": if m.busy { m.agent.Cancel() }; return m, nil` | OK |
| Esc com `busy = false` é no-op real | `tui.go:339-343` — o guard `if m.busy` faz o Esc-idle não chamar `Cancel()` nem alterar estado/máquina de estados; `Cancel()` em agent idle é nil-safe (`agent.go:126-132`), logo nem o caminho busy é perigoso | OK |
| Esc em confirm: `Cancel()` **antes** do `false` no `ApproveCh` (recomendação obrigatória da review 2.0) | `tui.go:517-519` — `case "esc": m.agent.Cancel(); m.pendingConfirm.ApproveCh <- false`, exatamente a ordenação exigida; o envio nunca bloqueia (canal criado buffered tamanho 1 pelo Agent, `agent.go:212`) | OK |
| `n` sozinho apenas recusa (comportamento atual preservado) | `tui.go:515-516` — `case "n", "N": m.pendingConfirm.ApproveCh <- false`, sem `Cancel()`; o caso `esc` foi removido da lista `n, N` do código anterior | OK |
| `ctrl+c` inalterado em TODOS os estados | Chat `tui.go:337-338`, config `tui.go:445-446`, confirm `tui.go:511-512` — `tea.Quit` nos três; o diff não toca nenhuma dessas linhas | OK |
| Caso `EventTurnAborted`: markdown igual ao `EventTurnDone` + meta `⊘ interrompido · <model>`, `busy = false`, stream resetado | `tui.go:294-302` — mesmo bloco do `EventTurnDone` (`tui.go:282-293`): `renderMarkdown(m.stream.String(), ...)`, meta `metaMarkStyle.Render("⊘ ") + metaStyle.Render(abortMeta(e))`, `m.stream.Reset()`, `m.busy = false`, `m.refreshContent()`; `abortMeta` (`tui.go:326-332`) junta `"interrompido"` + modelo — espelho do meta `▣` (Decisão #4 da techspec) | OK |
| Hint bar `esc to interrupt` no lado direito apenas quando busy | `tui.go:638-640` — `if m.busy { right += warningStyle.Render("esc to interrupt") + helpStyle.Render(" · ") }`, no lado direito, mesmo padrão de `↑ scrolled`/`pinned` (`warningStyle` + separador `helpStyle`) | OK |
| Fluxo de dados do abort, passos 1 e 5 (techspec) | Passo 1: Esc → `Cancel()` → cancelFunc idempotente (`tui.go:339-343`, `:517-519`; `agent.go:126-132`). Passo 5: TUI renderiza parcial + meta `⊘`, `busy = false`, input liberado (`tui.go:294-302`) | OK |

### 2. Invariantes críticos

- **Marcador textual legível no conteúdo plano copiável (acessibilidade do PRD)**: a meta line é texto puro (`"⊘ interrompido · <model>"`) envolvida em estilos de cor; `refreshContent` (`tui.go:528-551`) computa `contentPlain = plainLines(m.contentRaw)` — `plainLines`/`stripANSI` (`selection.go:17-23`) removem ANSI — e `contentPlain` é a fonte de `copySelection` (`tui.go:584-599`). Provado em teste: `TestTurnAbortedRendersMarker` assertion em `strings.Join(m.contentPlain, "\n")` contém `interrompido` (`tui_test.go:405-407`).
- **Sem comentários no código**: o diff não introduz nenhum comentário; o único `//` do package é o pragma pré-existente `//go:embed markdown.json` (`styles.go:11`).
- **Receivers por valor na TUI**: `abortMeta` é função pura; os handlers alterados (`handleAgentEvent`, `handleChatKey`, `handleConfirmKey`) e `hintBar` mantêm receiver `(m Model)`; nenhum método de valor novo foi introduzido.
- **Hint bar segue o padrão visual existente**: `warningStyle.Render(...) + helpStyle.Render(" · ")` — idêntico aos itens `↑ scrolled` e `pinned` existentes.
- **Sem mudanças em `internal/agent/`**: os line numbers do diff de `agent.go` coincidem exatamente com as citações da review 2.0 (`Cancel` :126-132, `isAbort` :264-266, `abortTurn` :268-274, const `EventTurnAborted` :30, `<-ch` :214-215) — o diff pré-aprovado da Task 2.0 está intacto; nada novo foi adicionado a `internal/agent` ou `internal/tools` por esta task.
- **Robustez de borda**: abort pré-roteamento (parcial vazio) — `renderMarkdown("")` retorna `""` e o bloco de markdown não é criado; a meta `⊘ interrompido` ainda é renderizada. Esc repetido em confirm: o primeiro Esc limpa `pendingConfirm` e volta a `stateChat` (`tui.go:523-524`); o segundo Esc cai no handler de chat e `Cancel()` é idempotente.

### 3. Testes exigidos

Os 5 testes da techspec existem em `internal/tui/tui_test.go` e assertam o que a task pede:

| Teste | O que prova | Evidência |
| --- | --- | --- |
| `TestEscCancelsWhenBusy` (`tui_test.go:223`) | Fluxo real ponta a ponta: agent com gateway bloqueante (`blockingGateway` :146-161, chunk + `<-r.Context().Done()`), Enter → `busy=true`, `EventDelta`, Esc → `EventTurnAborted` com `Text: "partial answer"` e `Model: "glm-5.3"`; `busy=false` após o evento | `:239-250`; `waitForAgentEvent` (:204-221) fataliza ao ver qualquer `EventError` — prova "nenhum erro espúrio" |
| `TestEscNoOpWhenIdle` (`:253`) | Esc-idle não altera `state`, `busy`, valor do input, `blocks`, `stream` nem o viewport; sem panic | `:259-279` — comparação before/after campo a campo |
| `TestEscInConfirmDeclinesAndCancels` (`:282`) | Três partes: (1) confirm sintético + Esc → `ApproveCh` recebe `false`, `state=chat`, `pendingConfirm=nil` (:283-306); (2) agent real com `Confirm=true` + Esc → `EventTurnAborted` — e fataliza se `EventToolResult`/`EventTurnDone` chegarem primeiro, provando a distinção Esc-aborta-turno vs tool-meros-recusado (:308-343); (3) `n` sozinho → `EventToolResult` "declined" + `EventTurnDone` — loop continua (:345-363) | A parte (2) exercita a ordenação `Cancel()` → `false` da review 2.0 pelo caminho real da TUI |
| `TestHintBarShowsEscToInterruptWhenBusy` (`:366`) | `View()` com `busy=true` contém `esc to interrupt`; ocioso não contém (checado antes e depois) | `:370-382` |
| `TestTurnAbortedRendersMarker` (`:385`) | Deltas + `EventTurnAborted` → render contém o parcial, `interrompido` e `⊘` em texto plano; **`contentPlain` (copiável) contém `interrompido`**; `stream.Len()==0`; `busy=false` | `:394-413`; prova dupla: viewport renderizado (via `stripANSI`) + conteúdo copiável |

Padrão `newTestModel`/`step` preservado; harness de gateway bloqueante replica o padrão dos testes do agent, e o estado é dirigido via `agentEventMsg` conforme a techspec. Os cmds retornados por `Update` são descartados pelo `step`, logo não há leitor concorrente do canal de eventos — sem flakiness.

### 4. Verificação obrigatória

`go build ./... && go vet ./... && go test ./...` — tudo verde (suíte fresca com `-count=1`):

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
- `go test -race -count=1 ./internal/tui/`: `ok kterminal/internal/tui 1.762s` — sem data races (relevante: `Cancel()` concorrente com o loop do agent nos testes de cancelamento).
- `go test -race -count=1 ./internal/agent/`: `ok kterminal/internal/agent 1.357s` — sem regressão cruzada.

### 5. Escopo

- `git status`/`git diff`: mudanças desta task APENAS em `internal/tui/tui.go` (+30/-4) e `internal/tui/tui_test.go` (+311). Os diffs de `internal/agent/{agent,agent_test}.go` e `internal/tools/{tools,bash,fs}.go` são os das Tasks 1.0/2.0 já aprovados — verificados intactos por inspeção de line numbers contra a review 2.0.
- Não rastreados pré-existentes: `internal/tools/{bash,tools}_test.go` (Task 1.0), `review_1.0.md`, `review_2.0.md`.
- `tasks.md`: bookkeeping — `[x] 1.0`/`[x] 2.0` marcados (consistente com as aprovações anteriores); `[ ] 3.0` ainda desmarcado, correto enquanto a review pendia.

## Problemas encontrados

Nenhum problema bloqueante ou corretivo.

## Avaliação das observações do implementador (item 6 do checklist)

**(a) `abortMeta` degrada para `⊘ interrompido` sem modelo quando `Model == ""` — ACEITO.**
A review 2.0 (recomendação #2) já previa exatamente este caso: abort pré-roteamento (ex.: durante `ensureCandidates`, `agent.go:136-146`) emite `EventTurnAborted` com `Model: ""` porque nenhum modelo foi decidido. Mostrar modelo vazio com o separador (`interrompido ·`) seria feio; omitir o marcador seria violar a acessibilidade. A degradação para `interrompido` puro preserva o marcador textual (a parte crítica do requisito) sem inventar estado. Correto.

**(b) Uso de `stripANSI` no teste de render — ACEITO.**
`stripANSI` é helper pré-existente do package (`selection.go:17`) — o mesmo que `plainLines` usa para computar o conteúdo copiável. O viewport renderizado contém códigos ANSI (estilos lipgloss); afirmar que o marcador sobrevive ao render exige removê-los. O teste faz a prova dupla mais forte que a exigida: viewport renderizado em texto plano (`:395-404`) E `contentPlain` copiável (`:405-407`). Uso apropriado, sem duplicar lógica de produção.

**(c) Hint bar ausente na `confirmView` — CONCORDO que está fora de escopo.**
A `confirmView` (`tui.go:711-723`) é um modal pré-existente que substitui a tela inteira e **nunca** renderizou a hint bar — não é uma omissão desta task. O requisito da REQ-003 ("hint bar exibe `esc to interrupt` enquanto busy") trata da hint bar do chat view, onde foi implementado. Registrado como recomendação de UX abaixo, não como defeito.

## Recomendações (não-bloqueantes)

1. **UX da `confirmView`**: o help line do prompt de confirmação (`tui.go:721`) diz apenas `y approve · n decline` — o novo comportamento do Esc (abortar o turno inteiro) não é descobrível ali. Numa passada futura de UX, acrescentar ex.: `· esc abort turn` (e avaliar se a confirmView merece hint bar). Não é exigido por nenhuma spec; o `kspec-qa` pode opinar no E2E.
2. **Nuance do no-op do Esc-idle**: `handleChatKey` abre com `m.copyFlash = ""` (`tui.go:335`) — comportamento pré-existente que limpa o flash de cópia em **qualquer** tecla no estado chat. O Esc-idle herda esse efeito universal (antes da task, o Esc caía no fallthrough e também o limpava). Não é mudança de estado específica do Esc nem violação do "no-op"; registro apenas para deixar a análise explícita.
3. **Render do parcial via `m.stream`**: o handler usa `m.stream.String()` (não `e.Text`) — coincide com o `EventTurnAborted.Text` do Agent (review 2.0, recomendação #1) e é consistente com o `EventTurnDone`, que também renderiza `m.stream`. Em abort pré-roteamento ambos são vazios e o guard `md != ""` evita bloco vazio. Correto; manter assim.
4. **Bookkeeping**: marcar `[x] 3.0` em `tasks.md` após esta aprovação.
5. **E2E deferido**: latência ≤ 100ms Esc→input e marcador no conteúdo plano copiável seguem para o `kspec-qa` conforme a techspec — a base de testes de unidade desta task cobre o comportamento; o QA valida a percepção.

## Conclusão

A task 3.0 fecha o fluxo de ponta a ponta da feature conforme a spec: o contrato de interação do Esc está implementado nos três estados com a ordenação obrigatória da review 2.0, o turno abortado renderiza com o marcador textual `⊘ interrompido` acessível no conteúdo copiável, a hint bar sinaliza a interrupção seguindo o padrão visual existente, e `ctrl+c` permanece intocado. Os 5 testes exigidos provam cada requisito — incluindo a distinção Esc-aborta vs `n`-recusa pelo caminho real do agent — e passam com `-race`. Sem mudanças fora de escopo; observações do implementador todas legítimas. A feature está pronta para o `kspec-qa`.
