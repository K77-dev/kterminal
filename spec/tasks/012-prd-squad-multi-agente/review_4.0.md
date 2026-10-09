# Review 4.0 — Tools: `NewReadOnlyRegistry`, `task` com `persona`, tool `squad_kickoff`

## Status: APROVADO COM RESSALVAS → CORRIGIDO → APROVADO

## Verificação por subtask

- [x] 4.1 `NewReadOnlyRegistry()` em `internal/tools/tools.go` — registra apenas `readTool()`, `globTool()`, `grepTool()`; nenhum mutating
- [x] 4.2 Schema da tool `task` estendido com propriedade opcional `persona` (string), não obrigatória
- [x] 4.3 `Execute` da tool `task`: com `persona` presente exige kickoff registrado, resolve persona no `squad.Store`, monta system prompt (corpo da persona + subset de rules), cria subagente com registry read-only, pin e `Agent = persona` nos eventos
- [x] 4.4 Tool `squad_kickoff` — schema com `roles`, `max_convocations`, `token_budget`, `exit_criterion` (só `roles` obrigatório); valida papéis contra `store.List()` e registra no Agent raiz
- [x] 4.5 Enforcement de tetos: `max_convocations` e `token_budget` bloqueiam a convocation seguinte com sinal de fim; clamps nos defaults do config
- [x] 4.6 Subset de rules por disciplina — `parseRuleFrontmatter` lê `disciplines`; rules sem frontmatter são excluídas do subset

## Conformidade com os Critérios de Sucesso

- [x] `NewReadOnlyRegistry()` expõe apenas read/glob/grep — `TestNewReadOnlyRegistryExposesOnlyReadTools` (contagem exata = 3), `TestNewReadOnlyRegistryNotMutating`
- [x] `task` com persona inexistente retorna erro claro — `TestConvokePersonaUnknownRoleFails`
- [x] `task` com persona sem kickoff falha com instrução — `TestConvokePersonaWithoutKickoffFails`, `TestTaskToolPersonaWithoutKickoffFailsInline`
- [x] `squad_kickoff` valida papéis inexistentes — `TestSquadKickoffToolRejectsUnknownRole`; clampa tetos — `TestSquadKickoffToolClampsLimits`
- [x] Teto de convocations: excedente falha com sinal de fim — `TestConvokePersonaExceedsConvocationLimit`
- [x] Teto de tokens: convocation bloqueada quando acumulador excede budget — `TestConvokePersonaExceedsTokenBudget`
- [x] Personas não escrevem durante deliberação — registry read-only verificado em `TestPersonaRegistryIsReadOnly`

## Ressalva encontrada (corrigida)

**Teto de tokens não contabilizava o uso da convocation.** A `techspec` exige que o teto seja medido pela *usage real somada de todas as chamadas LLM do turno*. O `convokePersona` lia `a.turnTokens`, mas o consumo do subagente (`sub.turnTokens`) nunca era propagado ao pai em `runSubagent` — apenas as chamadas do próprio maestro contavam. Uma mesa real poderia estourar o `token_budget` sem que o limite disparasse.

**Correção aplicada** (`internal/agent/agent.go`, `runSubagent`):

```go
text, err := sub.runLoop(ctx)
a.turnTokens += sub.turnTokens
```

**Teste de regressão** `TestConvocationsAccumulateIntoTokenBudget` — mesa com kickoff + 1 convocation acumula `63` tokens no turno (15 kickoff + 15 chamada `task` do pai + 12 subagente + 21 chamada final do pai), comprovando que a usage da convocation entra no orçamento do turno.

## Testes da Tarefa

- [x] `TestNewReadOnlyRegistryExposesOnlyReadTools` / `TestNewReadOnlyRegistryNotMutating`
- [x] `TestSquadKickoffToolRegistersValidated` / `RejectsUnknownRole` / `ClampsLimits` / `RejectsMissingRoles`
- [x] `TestPersonaRegistryIsReadOnly`, `TestPersonaRulesSubsetByDiscipline`, `TestPersonaPromptIncludesPersonaBody`
- [x] `TestPersonaRulesSubsetFiltersEmbeddedWithoutDisciplines` (rule do projeto + filtro por disciplina)
- [x] `TestTaskToolConvenesPersonas` (mesa completa: kickoff → 2 convocations → convergência; `Agent` nos eventos)
- [x] `TestTaskToolPersonaWithoutKickoffFailsInline`
- [x] `TestConvokePersonaWithoutKickoffFails` / `UnknownRoleFails` / `ExceedsConvocationLimit` / `ExceedsTokenBudget`
- [x] `TestConvocationsAccumulateIntoTokenBudget` (novo, ressalva acima)

Todos passam: `go test ./...` → ok

## Build

- `go build ./...` → ok
- `gofmt -l` → sem diffs
- `go vet ./...` → ok
