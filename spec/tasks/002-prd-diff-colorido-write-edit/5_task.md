# Tarefa 5.0: Verificação final integrada e checagem de critérios de aceite

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Diff pós-execução
- REQ-002 — Diff antes da aprovação (modo `--confirm`)
- REQ-003 — Diff no transcript

## Dependências

- 4.0 (feature completa: algoritmo, tools, agent e TUI implementados e testados)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechamento da feature: rodar a verificação obrigatória do projeto completa, checar cruzadamente os critérios de aceite do PRD contra o que foi implementado e confirmar a conformidade com os padrões do código. Esta task não escreve código novo — se qualquer verificação falhar, o ajuste volta para a task responsável (1.0, 2.0, 3.0 ou 4.0). Ao final, a funcionalidade está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.

<skills>
### Conformidade com Skills Padrões

- Verificação obrigatória do projeto (PRD): `go build ./... && go vet ./... && gofmt -l . && go test ./...`.
- Skills do fluxo kspec aplicáveis: `kspec-qa` (próximo passo — E2E: diff colorido no chat, diff no confirm, truncamento legível), `kspec-pr-review` (revisão semântica contra PRD/Tech Spec/tasks).
- Padrões do projeto a conferir: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante); receivers por valor na TUI; `strings.Builder` sempre por ponteiro.
</skills>

<requirements>
- `go build ./...`, `go vet ./...`, `gofmt -l .` (saída vazia) e `go test ./...` todos verdes em uma única execução encadeada.
- Checklist dos critérios de aceite do PRD percorrido item a item, com evidência (teste que o cobre ou deferimento explícito ao QA).
- Conformidade com os padrões de código do projeto confirmada por inspeção do diff.
- Nenhum requisito do PRD deixado de lado; itens fora de escopo do PRD não implementados (diff word-level, syntax highlight, diff binário, diff de mudanças via `bash`, edição interativa de hunks).
</requirements>

## Subtarefas

- [ ] 5.1 Executar `go build ./... && go vet ./... && gofmt -l . && go test ./...` e confirmar tudo verde
- [ ] 5.2 Percorrer o checklist de critérios de aceite do PRD (ver Critérios de Sucesso) e registrar a evidência de cada item
- [ ] 5.3 Inspecionar o diff completo contra os padrões do projeto (sem comentários, eventos via canal, receivers por valor, `strings.Builder` por ponteiro)
- [ ] 5.4 Confirmar prontidão para `kspec-qa`: listar os cenários E2E da techspec que o QA deve executar

## Detalhes de Implementação

- Sem código novo. Verificação obrigatória definida no PRD (seção **Restrições Técnicas de Alto Nível**) e na techspec (seção **Infraestrutura**).
- Cenários E2E para o QA estão na techspec, seção **Abordagem de Testes → Testes de E2E**.
- Se um critério de aceite falhar, não corrigir nesta task — reabrir a task responsável (1.0 algoritmo, 2.0 tools, 3.0 agent/transcript, 4.0 TUI) e corrigir lá.

## Critérios de Sucesso

- Verificação encadeada completa passa sem erros.
- Checklist de aceite com evidência por item:
  - Edit em arquivo existente mostra remoções e adições coloridas (`TestEditToolDiff` + `TestDiffBlockRendersColors`).
  - Write em arquivo novo mostra todo o conteúdo como adição (`TestWriteToolDiffNewFile`).
  - Diff de arquivo sem mudanças não renderiza bloco vazio (`TestLineDiffEqual` + `TestEmptyDiffRendersNoBlock`).
  - Modo `--confirm` exibe o diff antes da aprovação, refletindo a edição (`TestPendingDiffDoesNotTouchDisk` + `TestConfirmViewShowsDiff`).
  - Eventos de tool result de `write`/`edit` no transcript carregam as linhas do diff (`TestToolResultEventCarriesDiff`).
- Diff conforme os padrões do projeto.

## Testes da Tarefa

- [ ] Testes de unidade — todos os suites das tasks 1.0, 2.0, 3.0 e 4.0 passando (`go test ./...`).
- [ ] Testes de integração — cobertos pelos testes de agent com mock gateway (task 3.0).
- [ ] Testes E2E — nenhum novo nesta task; execução deferida ao `kspec-qa` com a lista de cenários montada na subtarefa 5.4.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/diff.go`, `internal/tools/diff_test.go` — diff da task 1.0
- `internal/tools/fs.go`, `internal/tools/tools.go` — diff da task 2.0
- `internal/agent/agent.go`, `internal/session/session.go` — diff da task 3.0
- `internal/tui/theme.go`, `internal/tui/tui.go` — diff da task 4.0
- `spec/tasks/002-prd-diff-colorido-write-edit/prd.md` — critérios de aceite (fonte do checklist)
- `spec/tasks/002-prd-diff-colorido-write-edit/techspec.md` — cenários E2E para o QA
