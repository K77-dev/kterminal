# Tarefa 5.0: Verificação final integrada e checagem de critérios de aceite

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Snapshot por turno
- REQ-002 — Carregamento de sessão
- REQ-003 — Flags CLI
- REQ-004 — Sessão retomada é a mesma sessão

## Dependências

- 4.0 (feature completa: session, agent, CLI e TUI implementados e testados)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechamento da feature: rodar a verificação obrigatória do projeto completa, checar cruzadamente os critérios de aceite do PRD contra o que foi implementado e confirmar a conformidade com os padrões do código. Esta task não escreve código novo — se qualquer verificação falhar, o ajuste volta para a task responsável (1.0, 2.0, 3.0 ou 4.0). Ao final, a funcionalidade está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.

<skills>
### Conformidade com Skills Padrões

- Verificação obrigatória do projeto (PRD): `go build ./... && go vet ./... && gofmt -l . && go test ./...`.
- Skills do fluxo kspec aplicáveis: `kspec-qa` (próximo passo — E2E: conversar→sair→retomar; `--session` inexistente; `--continue` sem sessões; histórico no viewport), `kspec-pr-review` (revisão semântica contra PRD/Tech Spec/tasks).
- Padrões do projeto a conferir: sem comentários no código; eventos via canal; receivers por valor na TUI; `strings.Builder` sempre por ponteiro; snapshot como única fonte da verdade (nunca replay).
</skills>

<requirements>
- `go build ./...`, `go vet ./...`, `gofmt -l .` (saída vazia) e `go test ./...` todos verdes em uma única execução encadeada.
- Checklist dos critérios de aceite do PRD percorrido item a item, com evidência (teste que o cobre ou deferimento explícito ao QA).
- Conformidade com os padrões de código do projeto confirmada por inspeção do diff.
- Nenhum requisito do PRD deixado de lado; itens fora de escopo do PRD não implementados (picker de sessões, busca/filtragem, exportação, múltiplas sessões simultâneas, fork de sessão).
</requirements>

## Subtarefas

- [ ] 5.1 Executar `go build ./... && go vet ./... && gofmt -l . && go test ./...` e confirmar tudo verde
- [ ] 5.2 Percorrer o checklist de critérios de aceite do PRD (ver Critérios de Sucesso) e registrar a evidência de cada item
- [ ] 5.3 Inspecionar o diff completo contra os padrões do projeto (sem comentários, snapshot como fonte da verdade, receivers por valor, `strings.Builder` por ponteiro)
- [ ] 5.4 Confirmar prontidão para `kspec-qa`: listar os cenários E2E da techspec que o QA deve executar

## Detalhes de Implementação

- Sem código novo. Verificação obrigatória definida no PRD (seção **Restrições Técnicas de Alto Nível**) e na techspec (seção **Infraestrutura**).
- Cenários E2E para o QA estão na techspec, seção **Abordagem de Testes → Testes de E2E**.
- Se um critério de aceite falhar, não corrigir nesta task — reabrir a task responsável (1.0 session, 2.0 agent, 3.0 CLI, 4.0 TUI) e corrigir lá.

## Critérios de Sucesso

- Verificação encadeada completa passa sem erros.
- Checklist de aceite com evidência por item:
  - Snapshot gravado após cada turno concluído, fiel ao estado (`TestSnapshotWrittenAfterTurnDone`).
  - Resume usa o snapshot mais recente; sem snapshot → erro explícito (`TestLoadUsesLastSnapshot` + `TestLoadWithoutSnapshot`).
  - Conversar → sair → `--continue` → pergunta nova responde com contexto (E2E — QA).
  - `--session` inexistente → stderr + exit 1 (E2E — QA; contrato de erro no wiring da task 3.0).
  - `--continue` sem sessões → aviso no chat e app funcional (E2E — QA).
  - Mensagens pós-resume gravadas no mesmo arquivo JSONL (`TestAppendWriterAppends`).
  - Viewport mostra histórico reconstruído ao iniciar (`TestResumedReconstructsBlocks`).
  - Hint bar indica `resumed · N mensagens` (`TestHintBarShowsResumed`).
- Diff conforme os padrões do projeto.

## Testes da Tarefa

- [ ] Testes de unidade — todos os suites das tasks 1.0, 2.0 e 4.0 passando (`go test ./...`).
- [ ] Testes de integração — cobertos pelos testes de session/agent (tasks 1.0 e 2.0).
- [ ] Testes E2E — nenhum novo nesta task; execução deferida ao `kspec-qa` com a lista de cenários montada na subtarefa 5.4.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/session/session.go`, `internal/session/session_test.go` — diff da task 1.0
- `internal/agent/agent.go`, `internal/agent/agent_test.go` — diff da task 2.0
- `main.go` — diff da task 3.0
- `internal/tui/tui.go`, `internal/tui/tui_test.go` — diff da task 4.0
- `spec/tasks/004-prd-retomar-sessao/prd.md` — critérios de aceite (fonte do checklist)
- `spec/tasks/004-prd-retomar-sessao/techspec.md` — cenários E2E para o QA
