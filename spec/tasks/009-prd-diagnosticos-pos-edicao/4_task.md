# Tarefa 4.0: Verificação final integrada e checagem de critérios de aceite

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Hook pós-edição em arquivos Go
- REQ-002 — Diagnóstico best-effort com `go vet`
- REQ-003 — Feedback explícito ao modelo
- REQ-004 — Correção autônoma no mesmo turno

## Dependências

- 3.0 (feature completa: hook, registry e injeção implementados e testados)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechamento da feature: rodar a verificação obrigatória do projeto completa, checar cruzadamente os critérios de aceite do PRD contra o que foi implementado e confirmar a conformidade com os padrões do código — com atenção especial ao requisito não negociável (nenhum erro de diagnóstico pode falhar a tool em si). Esta task não escreve código novo — se qualquer verificação falhar, o ajuste volta para a task responsável (1.0, 2.0 ou 3.0). Ao final, a funcionalidade está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.

<skills>
### Conformidade com Skills Padrões

- Verificação obrigatória do projeto (PRD): `go build ./... && go vet ./... && gofmt -l . && go test ./...`.
- Skills do fluxo kspec aplicáveis: `kspec-qa` (próximo passo — E2E: auto-correção visível no mesmo turno; `Diagnostics: clean`; edição fora de módulo), `kspec-pr-review` (revisão semântica contra PRD/Tech Spec/tasks).
- Padrões do projeto a conferir: sem comentários no código; best-effort como contrato (`GoVetHook` nunca retorna erro); hook injetado via setter (pacote testável sem Go).
</skills>

<requirements>
- `go build ./...`, `go vet ./...`, `gofmt -l .` (saída vazia) e `go test ./...` todos verdes em uma única execução encadeada.
- Checklist dos critérios de aceite do PRD percorrido item a item, com evidência (teste que o cobre ou deferimento explícito ao QA).
- Conformidade com os padrões de código do projeto confirmada por inspeção do diff.
- Nenhum requisito do PRD deixado de lado; itens fora de escopo do PRD não implementados (LSP, outras linguagens, gofmt/fixes automáticos, diagnóstico de arquivos não editados, `go build`/`go test` no hook).
</requirements>

## Subtarefas

- [ ] 4.1 Executar `go build ./... && go vet ./... && gofmt -l . && go test ./...` e confirmar tudo verde
- [ ] 4.2 Percorrer o checklist de critérios de aceite do PRD (ver Critérios de Sucesso) e registrar a evidência de cada item
- [ ] 4.3 Inspecionar o diff completo contra os padrões do projeto (sem comentários; best-effort preservado; injeção no main)
- [ ] 4.4 Confirmar prontidão para `kspec-qa`: listar os cenários E2E da techspec que o QA deve executar

## Detalhes de Implementação

- Sem código novo. Verificação obrigatória definida no PRD (seção **Restrições Técnicas de Alto Nível**) e na techspec (seção **Infraestrutura**).
- Cenários E2E para o QA estão na techspec, seção **Abordagem de Testes → Testes de E2E**.
- Se um critério de aceite falhar, não corrigir nesta task — reabrir a task responsável (1.0 hook, 2.0 registry, 3.0 agent/main) e corrigir lá.

## Critérios de Sucesso

- Verificação encadeada completa passa sem erros.
- Checklist de aceite com evidência por item:
  - Edit em arquivo `.go` dentro de módulo dispara o diagnóstico (`TestExecuteCallsHookOnGoEdit` + `TestVetHookReturnsDiagnostic`).
  - Edit em arquivo não-Go não roda vet (`TestExecuteSkipsHookForNonGoOrOtherTools`).
  - Edit fora de módulo Go não roda vet (`TestVetHookNoModuleEmpty`).
  - Ambiente sem `go` no PATH → tool funciona normalmente sem diagnóstico (`TestVetHookNoGoInPath`).
  - `go vet` que demora mais que 30s não bloqueia indefinidamente (`TestVetHookTimeoutKillsProcess`).
  - Edit que introduz `undefined: Foo` → resultado contém a linha do erro; o modelo corrige no passo seguinte (`TestAgentSelfCorrectsWithinTurn`).
  - Edit válido → resultado contém `Diagnostics: clean` (`TestVetHookCleanOnValidFile`).
  - Em um turno completo, edição que quebra é corrigida pelo próprio agente, sem input do usuário (`TestAgentSelfCorrectsWithinTurn`).
- Diff conforme os padrões do projeto.

## Testes da Tarefa

- [ ] Testes de unidade — todos os suites das tasks 1.0, 2.0 e 3.0 passando (`go test ./...`).
- [ ] Testes de integração — cobertos por `TestAgentSelfCorrectsWithinTurn` (task 3.0).
- [ ] Testes E2E — nenhum novo nesta task; execução deferida ao `kspec-qa` com a lista de cenários montada na subtarefa 4.4.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/diagnostics.go`, `internal/tools/diagnostics_test.go` — diff da task 1.0
- `internal/tools/tools.go`, `internal/tools/tools_test.go` — diff da task 2.0
- `internal/agent/agent_test.go`, `main.go` — diff da task 3.0
- `spec/tasks/009-prd-diagnosticos-pos-edicao/prd.md` — critérios de aceite (fonte do checklist)
- `spec/tasks/009-prd-diagnosticos-pos-edicao/techspec.md` — cenários E2E para o QA
