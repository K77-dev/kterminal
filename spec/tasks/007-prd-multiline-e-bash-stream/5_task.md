# Tarefa 5.0: Verificação final integrada e checagem de critérios de aceite

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Prompt multi-linha
- REQ-002 — Histórico de prompts
- REQ-003 — Stream do output do bash
- REQ-004 — Bloco de output ao vivo na TUI
- REQ-005 — Transcript enxuto

## Dependências

- 3.0, 4.0 (feature completa: stream e multi-linha implementados e testados)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechamento da feature: rodar a verificação obrigatória do projeto completa, checar cruzadamente os critérios de aceite do PRD contra o que foi implementado e confirmar a conformidade com os padrões do código. Esta task não escreve código novo — se qualquer verificação falhar, o ajuste volta para a task responsável (1.0, 2.0, 3.0 ou 4.0). Ao final, a funcionalidade está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.

<skills>
### Conformidade com Skills Padrões

- Verificação obrigatória do projeto (PRD): `go build ./... && go vet ./... && gofmt -l . && go test ./...`.
- Skills do fluxo kspec aplicáveis: `kspec-qa` (próximo passo — E2E: stream linha a linha; `go test ./...` demorado com live block; `shift+enter`; ↑ recupera prompt), `kspec-pr-review` (revisão semântica contra PRD/Tech Spec/tasks).
- Padrões do projeto a conferir: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante); receivers por valor na TUI; `strings.Builder` sempre por ponteiro (nunca por valor no Model).
</skills>

<requirements>
- `go build ./...`, `go vet ./...`, `gofmt -l .` (saída vazia) e `go test ./...` todos verdes em uma única execução encadeada.
- Checklist dos critérios de aceite do PRD percorrido item a item, com evidência (teste que o cobre ou deferimento explícito ao QA).
- Conformidade com os padrões de código do projeto confirmada por inspeção do diff.
- Nenhum requisito do PRD deixado de lado; itens fora de escopo do PRD não implementados (ANSI colorida do stream, streaming de outros tools, tratamento especial de paste, histórico persistido, stdin interativo).
</requirements>

## Subtarefas

- [ ] 5.1 Executar `go build ./... && go vet ./... && gofmt -l . && go test ./...` e confirmar tudo verde
- [ ] 5.2 Percorrer o checklist de critérios de aceite do PRD (ver Critérios de Sucesso) e registrar a evidência de cada item
- [ ] 5.3 Inspecionar o diff completo contra os padrões do projeto (sem comentários, `[]string` no Model — nunca `strings.Builder` por valor, receivers por valor)
- [ ] 5.4 Confirmar prontidão para `kspec-qa`: listar os cenários E2E da techspec que o QA deve executar

## Detalhes de Implementação

- Sem código novo. Verificação obrigatória definida no PRD (seção **Restrições Técnicas de Alto Nível**) e na techspec (seção **Infraestrutura**).
- Cenários E2E para o QA estão na techspec, seção **Abordagem de Testes → Testes de E2E**.
- Se um critério de aceite falhar, não corrigir nesta task — reabrir a task responsável (1.0 tools, 2.0 agent, 3.0 live block, 4.0 multi-linha) e corrigir lá.

## Critérios de Sucesso

- Verificação encadeada completa passa sem erros.
- Checklist de aceite com evidência por item:
  - `shift+enter` cria segunda linha; envio preserva as quebras (`TestShiftEnterCreatesNewline` + `TestEnterSendsMultiline`).
  - Viewport nunca encolhe abaixo de 3 linhas com prompt no máximo (`TestPromptHeightClamped`).
  - ↑ com input vazio recupera o prompt anterior; ↓ avança (`TestHistoryNavigation`).
  - Comando `for i in $(seq 1 10)...` mostra números um a um; ordem/contagem fidedignas (`TestExecuteStreamEmitsLinesInOrder`; ritmo visível — QA).
  - Bloco ao vivo contém as linhas conforme saem; resultado final substitui (`TestLiveBlockAccumulatesAndReplaces`).
  - Comando silencioso longo mostra spinner sem bloco vazio (`TestSilentCommandNoLiveBlock`).
  - JSONL não cresce com eventos por linha (`TestToolOutputNotWrittenToTranscript`).
- Diff conforme os padrões do projeto.

## Testes da Tarefa

- [ ] Testes de unidade — todos os suites das tasks 1.0, 2.0, 3.0 e 4.0 passando (`go test ./...`).
- [ ] Testes de integração — cobertos pelos testes de agent com mock (task 2.0).
- [ ] Testes E2E — nenhum novo nesta task; execução deferida ao `kspec-qa` com a lista de cenários montada na subtarefa 5.4.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/tools.go`, `internal/tools/bash.go`, `internal/tools/bash_test.go` — diff da task 1.0
- `internal/agent/agent.go`, `internal/agent/agent_test.go` — diff da task 2.0
- `internal/tui/tui.go`, `internal/tui/tui_test.go` — diff das tasks 3.0 e 4.0
- `spec/tasks/007-prd-multiline-e-bash-stream/prd.md` — critérios de aceite (fonte do checklist)
- `spec/tasks/007-prd-multiline-e-bash-stream/techspec.md` — cenários E2E para o QA
