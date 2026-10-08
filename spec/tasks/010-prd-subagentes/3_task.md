# Tarefa 3.0: Tool `task` com `AttachTaskTool`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Tool `task`

## Dependências

- 2.0 (`RunSync` e `newSubagent` disponíveis)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

A tool vista pelo modelo: `Registry` ganha o método exportado `Register(t Tool)` (hoje `register` é unexported) e o agent principal ganha `AttachTaskTool()` — registra a tool `task` **apenas quando `depth == 0`** (subagentes não a veem nas definições — limite de profundidade 1). Schema: `description` (obrigatória) e `guidance` (opcional); `Mutating: true` — herda as regras de `--confirm` (a tela de confirmação mostra `task(description)` como qualquer tool mutante). O `Execute` da tool chama `parent.RunSync(ctx, description, guidance)` e devolve o texto final como resultado.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa`, `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; tools via registry; `Mutating: true` para herdar `--confirm`.
</skills>

<requirements>
- `Register(t Tool)` exportado no `Registry`.
- `AttachTaskTool()` no `Agent` — registra `task` apenas quando `depth == 0`.
- Schema: `description` (string, obrigatória) e `guidance` (string, opcional); descrição "Delegate a focused subtask to an isolated subagent and get its final answer".
- `Mutating: true` — herda as regras de confirmação do modo `--confirm`.
- `Execute` chama `RunSync(ctx, description, guidance)` e devolve o texto final.
- O subagente não vê a tool `task` nas suas definições (registry próprio sem `task` — task 2.0).
</requirements>

## Subtarefas

- [ ] 3.1 Exportar `Register(t Tool)` no `Registry`
- [ ] 3.2 Implementar `AttachTaskTool` com o schema da tool `task` e guarda de `depth == 0`
- [ ] 3.3 Escrever os testes 1 e 3 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Snippet completo do registro da tool (schema, `Mutating`, `Execute`): techspec, seção **Design de Implementação → Interfaces Principais** (subseção "Tool `task`").
- Decisão "limite de profundidade estrutural" (registry do subagente sem `task` — impossível de burlar): techspec, seção **Considerações Técnicas → Decisões Principais** (item 2).
- `task` é mutante e passa pelo `--confirm` — delegação é decisão visível: techspec, seção **Verificações Técnicas → Segurança**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste mandatório prova: principal chama `task` → subagente roda → texto final chega como tool result → o principal responde ao usuário com o resultado; `EventTurnDone` no fim.
- Teste prova: o request do subagente **não** contém `task` nas tools; o request do principal contém.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**):
  - `TestTaskToolRunsSubagent` (mandatório) — delegação completa com `EventTurnDone` (item 1).
  - `TestSubagentDoesNotSeeTaskTool` — request do subagente sem `task`; do principal com (item 3).
- [ ] Testes de integração — cobertos pelos testes de agent (principal→subagente→gateway com mocks).
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/tools.go` — `Register` exportado
- `internal/agent/agent.go` — `AttachTaskTool` + schema da tool `task`
- `internal/agent/agent_test.go` — delegação e invisibilidade da tool no subagente
