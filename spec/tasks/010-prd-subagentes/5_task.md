# Tarefa 5.0: Wiring no `main.go` e verificação final integrada

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Tool `task`
- REQ-002 — Subagente isolado
- REQ-003 — Eventos aninhados
- REQ-004 — Limites e timeout
- REQ-005 — Cancelamento compartilhado
- REQ-006 — TUI e transcript

## Dependências

- 3.0, 4.0 (feature completa: subagente, tool `task` e TUI implementados e testados)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechamento da feature: ligar `ag.AttachTaskTool()` no wiring do `main.go` (após a construção do agent), rodar a verificação obrigatória do projeto completa, checar cruzadamente os critérios de aceite do PRD contra o que foi implementado e confirmar a conformidade com os padrões do código. Fora o wiring de uma linha, esta task não escreve código novo — se qualquer verificação falhar, o ajuste volta para a task responsável (1.0, 2.0, 3.0 ou 4.0). Ao final, a funcionalidade está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.

<skills>
### Conformidade com Skills Padrões

- Verificação obrigatória do projeto (PRD): `go build ./... && go vet ./... && gofmt -l . && go test ./...`.
- Skills do fluxo kspec aplicáveis: `kspec-qa` (próximo passo — E2E: delegação visível indentada; limites; Esc cancela tudo; hint bar), `kspec-pr-review` (revisão semântica contra PRD/Tech Spec/tasks).
- Padrões do projeto a conferir: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante nos dois níveis); receivers por valor na TUI; sem goroutines paralelas (v1 sequencial — requisito não negociável).
</skills>

<requirements>
- `main.go`: `ag.AttachTaskTool()` após a construção do agent.
- `go build ./...`, `go vet ./...`, `gofmt -l .` (saída vazia) e `go test ./...` todos verdes em uma única execução encadeada.
- Checklist dos critérios de aceite do PRD percorrido item a item, com evidência (teste que o cobre ou deferimento explícito ao QA).
- Conformidade com os padrões de código do projeto confirmada por inspeção do diff.
- Nenhum requisito do PRD deixado de lado; itens fora de escopo do PRD não implementados (paralelismo real, múltiplos subagentes simultâneos, profundidade > 1, subagentes nomeados/persistidos, progresso granular na UI).
</requirements>

## Subtarefas

- [ ] 5.1 Ligar `ag.AttachTaskTool()` no wiring do `main.go`
- [ ] 5.2 Executar `go build ./... && go vet ./... && gofmt -l . && go test ./...` e confirmar tudo verde
- [ ] 5.3 Percorrer o checklist de critérios de aceite do PRD (ver Critérios de Sucesso) e registrar a evidência de cada item
- [ ] 5.4 Confirmar prontidão para `kspec-qa`: listar os cenários E2E da techspec que o QA deve executar

## Detalhes de Implementação

- Wiring: `ag.AttachTaskTool()` após a construção do agent (techspec, seção **Arquitetura do Sistema** — componente `main.go`).
- Verificação obrigatória definida no PRD (seção **Restrições Técnicas de Alto Nível**) e na techspec (seção **Infraestrutura**).
- Cenários E2E para o QA estão na techspec, seção **Abordagem de Testes → Testes de E2E**.
- Se um critério de aceite falhar, não corrigir nesta task — reabrir a task responsável (1.0 refactor, 2.0 subagente, 3.0 tool, 4.0 TUI) e corrigir lá.

## Critérios de Sucesso

- Verificação encadeada completa passa sem erros.
- Checklist de aceite com evidência por item:
  - O agente principal pode chamar `task`; o subagente não vê a tool (`TestTaskToolRunsSubagent` + `TestSubagentDoesNotSeeTaskTool`).
  - Subagente roda com roteamento próprio, modelo independente (`TestSubagentRoutesIndependently`).
  - A resposta final chega como tool result e o principal continua com ela (`TestTaskToolRunsSubagent`).
  - Eventos do subagente renderizam aninhados, não misturados ao fluxo principal (`TestNestedEventsCarryDepth` + `TestNestedEventsRenderIndented`).
  - Subagente que estoura 10 steps ou 5min devolve erro como resultado — o principal segue vivo (`TestSubagentStepsLimit` + `TestSubagentTimeout`).
  - Esc cancela o turno inteiro, incluindo subagentes (`TestEscCancelsSubagent`).
  - Hint bar mostra `subagent running` enquanto há subagente ativo (`TestHintBarShowsSubagentRunning`).
  - Transcript grava eventos aninhados com campo de profundidade (`TestSubagentWritesTranscriptWithDepth`).
- Diff conforme os padrões do projeto.

## Testes da Tarefa

- [ ] Testes de unidade — todos os suites das tasks 1.0, 2.0, 3.0 e 4.0 passando (`go test ./...`).
- [ ] Testes de integração — cobertos pelos testes de agent com mocks (tasks 2.0 e 3.0).
- [ ] Testes E2E — nenhum novo nesta task; execução deferida ao `kspec-qa` com a lista de cenários montada na subtarefa 5.4.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `main.go` — `AttachTaskTool` no wiring
- `internal/agent/agent.go`, `internal/agent/agent_test.go` — diff das tasks 1.0, 2.0 e 3.0
- `internal/tools/tools.go` — `Register` exportado (task 3.0)
- `internal/session/session.go` — campo `Depth` (task 2.0)
- `internal/tui/tui.go`, `internal/tui/tui_test.go` — diff da task 4.0
- `spec/tasks/010-prd-subagentes/prd.md` — critérios de aceite (fonte do checklist)
- `spec/tasks/010-prd-subagentes/techspec.md` — cenários E2E para o QA
