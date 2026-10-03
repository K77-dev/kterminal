# Tarefa 4.0: Verificação final integrada e checagem de critérios de aceite

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Cancelamento do turno agêntico via Esc
- REQ-002 — Tecla Esc na TUI e estados
- REQ-003 — Renderização do turno abortado e feedback visual

## Dependências

- 3.0 (feature completa: tools, agent e TUI implementados e testados)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechamento da feature: rodar a verificação obrigatória do projeto completa, checar cruzadamente os critérios de aceite do PRD contra o que foi implementado e confirmar a conformidade com os padrões do código. Esta task não escreve código novo — se qualquer verificação falhar, o ajuste volta para a task responsável (1.0, 2.0 ou 3.0). Ao final, a funcionalidade está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.

<skills>
### Conformidade com Skills Padrões

- Verificação obrigatória do projeto (PRD): `go build ./... && go vet ./... && gofmt -l . && go test ./...`.
- Skills do fluxo kspec aplicáveis: `kspec-qa` (próximo passo — E2E: Esc em stream longo, Esc em `sleep 60`, nova mensagem pós-abort, latência ≤ 100ms, marcador textual), `kspec-pr-review` (revisão semântica contra PRD/Tech Spec/tasks).
- Padrões do projeto a conferir: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante); receivers por valor na TUI; `strings.Builder` sempre por ponteiro.
</skills>

<requirements>
- `go build ./...`, `go vet ./...`, `gofmt -l .` (saída vazia) e `go test ./...` todos verdes em uma única execução encadeada.
- Checklist dos critérios de aceite do PRD percorrido item a item, com evidência (teste que o cobre ou deferimento explícito ao QA).
- Conformidade com os padrões de código do projeto confirmada por inspeção do diff.
- Nenhum requisito do PRD deixado de lado; itens fora de escopo do PRD não implementados (pausar/retomar, cancelar tool seletivo, rollback, remapeamento de teclas, reenvio do turno abortado).
</requirements>

## Subtarefas

- [ ] 4.1 Executar `go build ./... && go vet ./... && gofmt -l . && go test ./...` e confirmar tudo verde
- [ ] 4.2 Percorrer o checklist de critérios de aceite do PRD (ver Critérios de Sucesso) e registrar a evidência de cada item
- [ ] 4.3 Inspecionar o diff completo contra os padrões do projeto (sem comentários, eventos via canal, receivers por valor, `strings.Builder` por ponteiro)
- [ ] 4.4 Confirmar prontidão para `kspec-qa`: listar os cenários E2E da techspec que o QA deve executar

## Detalhes de Implementação

- Sem código novo. Verificação obrigatória definida no PRD (seção **Restrições Técnicas de Alto Nível**) e na techspec (seção **Infraestrutura**).
- Cenários E2E para o QA estão na techspec, seção **Abordagem de Testes → Testes de E2E**.
- Se um critério de aceite falhar, não corrigir nesta task — reabrir a task responsável (1.0 tools, 2.0 agent, 3.0 TUI) e corrigir lá.

## Critérios de Sucesso

- Verificação encadeada completa passa sem erros.
- Checklist de aceite com evidência por item:
  - Esc durante stream: stream para, `EventTurnAborted` emitido, TUI volta a aceitar input, conversa preservada (coberto por `TestAgentCancelAbortsStream` + `TestEscCancelsWhenBusy`).
  - Esc durante `sleep 60`: processo morto via contexto (coberto por teste de unidade do bash; kill real validado no QA).
  - Nova mensagem após abort funciona limpa, sem o parcial no contexto LLM (coberto por `TestAgentCancelDuringConfirmAbortsTurn` — segundo `Run`).
  - Nenhuma mensagem de erro por cancelamento (coberto pelos 4 testes do agent).
  - `Esc` com `busy = false` não altera estado (`TestEscNoOpWhenIdle`).
  - `Esc` no `confirm` aborta o turno inteiro (`TestEscInConfirmDeclinesAndCancels`).
  - `ctrl+c` encerra o app como hoje (comportamento inalterado — inspeção).
  - Marcador visual distinto de turno concluído (`TestTurnAbortedRendersMarker`).
  - Hint bar alterna ocupado/ocioso (`TestHintBarShowsEscToInterruptWhenBusy`).
  - Latência ≤ 100ms entre Esc e input aceito (design revisado; medida percebida no QA).
- Diff conforme os padrões do projeto.

## Testes da Tarefa

- [ ] Testes de unidade — todos os suites das tasks 1.0, 2.0 e 3.0 passando (`go test ./...`).
- [ ] Testes de integração — fora do escopo conforme techspec.
- [ ] Testes E2E — nenhum novo nesta task; execução deferida ao `kspec-qa` com a lista de cenários montada na subtarefa 4.4.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/` — diff da task 1.0
- `internal/agent/agent.go`, `internal/agent/agent_test.go` — diff da task 2.0
- `internal/tui/tui.go`, `internal/tui/tui_test.go` — diff da task 3.0
- `spec/tasks/001-prd-abortar-tarefa/prd.md` — critérios de aceite (fonte do checklist)
- `spec/tasks/001-prd-abortar-tarefa/techspec.md` — cenários E2E para o QA
