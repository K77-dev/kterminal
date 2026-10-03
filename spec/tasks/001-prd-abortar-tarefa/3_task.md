# Tarefa 3.0: Tecla Esc na TUI, renderização do turno abortado e hint bar

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Tecla Esc na TUI e estados
- REQ-003 — Renderização do turno abortado e feedback visual

## Dependências

- 2.0 (`Agent.Cancel()` e `EventTurnAborted` existem)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Esta task conecta a interação do usuário ao mecanismo de cancelamento do Agent. No estado de chat, `Esc` com agente ocupado chama `agent.Cancel()`; ocioso, não faz nada (comportamento atual preservado). No prompt de confirmação de tool mutante (estado `confirm`), `Esc` recusa o tool pendente (envia `false` no `ApproveCh`) e cancela o turno inteiro. O novo caso `EventTurnAborted` no `handleAgentEvent` renderiza o texto parcial como markdown — mesmo tratamento do `EventTurnDone` — acrescido da meta line `⊘ interrompido · <model>`, volta `busy = false` e libera o input. Enquanto ocupado, a hint bar exibe `esc to interrupt` no lado direito. `ctrl+c` continua encerrando o app em todos os estados. Ao final desta task, o fluxo de ponta a ponta da feature funciona na TUI.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, Bubble Tea — sem novas dependências.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E de latência e marcador), `kspec-pr-review` (revisão semântica).
- Padrões do projeto (PRD/techspec): sem comentários no código; receivers por valor na TUI (modelo Bubble Tea); eventos via canal; hint bar segue o padrão visual existente.
</skills>

<requirements>
- `Esc` com `busy = true` chama `agent.Cancel()`.
- `Esc` com `busy = false` não altera nenhum estado (no-op).
- `Esc` no estado `confirm` envia `false` no `ApproveCh` e chama `Cancel()` — o resultado é turno abortado, não apenas tool recusado com o loop continuando.
- `n` sozinho no estado `confirm` apenas recusa o tool (comportamento atual preservado).
- `ctrl+c` mantém o comportamento atual: encerra o app em todos os estados.
- Caso `agent.EventTurnAborted` no `handleAgentEvent`: renderiza o parcial como markdown (mesmo tratamento do `EventTurnDone`) com meta line `⊘ interrompido · <model>`; `busy = false`; stream resetado.
- Hint bar exibe `esc to interrupt` no lado direito enquanto `busy`; ocioso, não exibe.
- O marcador de interrupção é textual (legível no conteúdo plano copiável — acessibilidade do PRD), não apenas cor.
</requirements>

## Subtarefas

- [ ] 3.1 Tratar `tea.KeyEsc` no estado de chat: `busy` → `agent.Cancel()`; ocioso → no-op
- [ ] 3.2 Tratar `tea.KeyEsc` no estado `confirm`: enviar `false` no `ApproveCh` + `agent.Cancel()`
- [ ] 3.3 Garantir que `ctrl+c` permanece inalterado em todos os estados
- [ ] 3.4 Adicionar o caso `agent.EventTurnAborted` no `handleAgentEvent`: markdown + meta `⊘ interrompido · <model>`, `busy = false`, stream resetado
- [ ] 3.5 Exibir `esc to interrupt` no lado direito da hint bar enquanto `busy`
- [ ] 3.6 Escrever os 5 testes de unidade da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Contrato de interação por estado está no PRD (REQ-002) e a visão de componentes da TUI na techspec, seção **Arquitetura do Sistema → Visão Geral dos Componentes** (`internal/tui`).
- A meta line `⊘ interrompido · <model>` espelha o meta de turno concluído (`▣`) — decisão registrada na techspec, **Considerações Técnicas → Decisões Principais** (item 4).
- Para os testes de cancelamento, construir o agent com o gateway bloqueante (mesmo harness dos testes do agent) e dirigir o estado via `agentEventMsg`, seguindo o padrão `newTestModel`/`step` existente (techspec, **Abordagem de Testes → Testes Unidade, bloco TUI**).
- Fluxo de dados do abort ponta a ponta: techspec, **Arquitetura do Sistema → Fluxo de dados do abort** (passos 1 e 5 são desta task).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- `Esc` com `busy = true` resulta em `EventTurnAborted` chegando no canal de eventos e `busy = false` ao final.
- `Esc` com `busy = false` não altera estado e não causa panic.
- `Esc` no estado `confirm` recusa o tool e aborta o turno; `n` sozinho apenas recusa o tool.
- Render de turno abortado contém o parcial e `interrompido`; stream resetado; `busy = false`.
- Hint bar contém `esc to interrupt` quando ocupado e não contém quando ocioso.
- `ctrl+c` encerra o app como hoje.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tui/tui_test.go`, padrão `newTestModel`/`step`; nomes e passos completos na techspec, **Abordagem de Testes → Testes Unidade, bloco TUI**):
  - `TestEscCancelsWhenBusy` — `busy = true` + `tea.KeyEsc` → `EventTurnAborted` chega; `busy = false` ao final.
  - `TestEscNoOpWhenIdle` — `busy = false` + `tea.KeyEsc` → nenhum estado alterado, sem panic.
  - `TestEscInConfirmDeclinesAndCancels` — estado confirm com `pendingConfirm` + `tea.KeyEsc` → `ApproveCh` recebe `false` e o turno aborta; `n` sozinho apenas recusa o tool.
  - `TestHintBarShowsEscToInterruptWhenBusy` — render com `busy = true` contém `esc to interrupt`; ocioso não contém.
  - `TestTurnAbortedRendersMarker` — deltas + `EventTurnAborted` → conteúdo renderizado contém o parcial e `interrompido`; stream resetado; `busy = false`.
- [ ] Testes de integração — fora do escopo conforme techspec.
- [ ] Testes E2E — deferidos para `kspec-qa` (latência ≤ 100ms entre Esc e input aceito; marcador textual no conteúdo plano copiável).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` — Esc nos estados chat/confirm, caso `EventTurnAborted`, meta `⊘ interrompido`, hint bar
- `internal/tui/tui_test.go` — 5 testes de unidade (key-handling e renderização)
- `internal/agent/agent.go` — consome `Cancel()` e `EventTurnAborted` (sem mudanças nesta task)
