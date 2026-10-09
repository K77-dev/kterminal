# Tarefa 4.0: Tools — `NewReadOnlyRegistry`, `task` com arg `persona`, tool `squad_kickoff`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Convocação dinâmica e subset de rules
- REQ-006 — Kickoff, tetos e critério de saída
- REQ-009 — Ponte SDD e execução do plano

## Dependências

- 1.0 (pacote `internal/squad` — `Store`, `Resolve`, `ValidateKickoff`)
- 3.0 (Agent core — `newSubagent` com persona, `RegisterKickoff`, acumuladores de turno)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Modificar `internal/tools` para suportar squad mode: (1) `NewReadOnlyRegistry()` com apenas read/glob/grep, (2) tool `task` ganha argumento opcional `persona` que resolve a persona no `squad.Store`, monta o system prompt e cria subagente com registry read-only, (3) tool `squad_kickoff` registra a mesa proposta (papéis, tetos, critério de saída) no Agent raiz com validação mecânica no Go.

## Conformidade com Skills Padrões

- Go 1.27, sem comentários
- Tools registradas no construtor (precedente: `NewRegistry()` em `tools.go:38`)
- Personas deliberam com registry somente-leitura: sem `write`, `edit`, `bash`, `kspec_bootstrap`
- Escrita só na fase de execução pós-convergência, via `task` com registry completo

## Requisitos

- `NewReadOnlyRegistry()` retorna registry com apenas `read`, `glob`, `grep`
- Tool `task` ganha propriedade opcional `persona` (string) no schema
- Quando `persona` está presente em `task`: resolver persona no `squad.Store`, montar system prompt (prompt da persona + rules do subset), criar subagente com `SetPinned(cfg.Squad.Pins[persona])` quando houver pin, registry read-only e `Agent = persona` nos eventos
- Sem kickoff registrado, `task(persona=...)` falha com instrução para convocar o kickoff primeiro
- Tool `squad_kickoff` com args `roles []string`, `max_convocations int`, `token_budget int`, `exit_criterion string`
- O Go valida kickoff: papéis existem no catálogo; tetos clampados aos máximos do config; override declarado no kickoff prevalece sobre o default
- Teto de convocations: convocation além do limite falha com sinal de fim
- Teto de tokens: convocation bloqueada quando acumulador do turno excede o budget

## Subtarefas

- [ ] 4.1 Implementar `NewReadOnlyRegistry()` em `internal/tools/tools.go` — registra apenas `readTool()`, `globTool()`, `grepTool()`
- [ ] 4.2 Estender schema da tool `task` com propriedade opcional `persona` (string)
- [ ] 4.3 Modificar `Execute` da tool `task`: quando `persona` presente, verificar kickoff registrado, resolver persona, montar system prompt, criar subagente com registry read-only, pin e `Agent = persona`
- [ ] 4.4 Implementar tool `squad_kickoff` — schema com `roles`, `max_convocations`, `token_budget`, `exit_criterion`; `Execute` valida contra config e registra no Agent raiz
- [ ] 4.5 Implementar enforcement de tetos: convocation além de `max_convocations` falha; tokens além de `token_budget` bloqueia convocation seguinte
- [ ] 4.6 Implementar subset de rules por disciplina — mapeamento rule → disciplina via frontmatter (`disciplines` na rule); persona recebe apenas rules cujas disciplinas intersectam `persona.Rules`

## Detalhes de Implementação

Consulte "Design de Implementação → Interfaces Principais" e "Segurança" na `techspec.md`:

- Convocation — extensão da tool `task` (schema): propriedade opcional `persona` (string). No `Execute`, quando `persona` está presente: resolver a persona no `squad.Store`, montar system prompt (prompt da persona + rules do subset), criar subagente com `SetPinned(cfg.Squad.Pins[persona])` quando houver pin, registry read-only e `Agent = persona` nos eventos emitidos
- Kickoff — tool `squad_kickoff` com args `roles []string`, `max_convocations int`, `token_budget int`, `exit_criterion string`. O Go valida: papéis existem no catálogo de personas; tetos ≤ defaults do config; sem kickoff registrado, `task(persona=...)` falha com instrução para convocar o kickoff primeiro
- Frontmatter de rule (mapeamento rule → disciplina): `disciplines: [backend, architecture]` — rules sem frontmatter ficam fora do subset de persona

## Critérios de Sucesso

- `NewReadOnlyRegistry()` expõe apenas read/glob/grep (sem write/edit/bash/kspec_bootstrap)
- `task` com `persona` inexistente retorna erro claro
- `task` com `persona` sem kickoff registrado falha com instrução para convocar kickoff primeiro
- `squad_kickoff` valida papéis inexistentes e clampa tetos
- Teto de convocations: 9ª convocation falha com sinal de fim (default 8)
- Teto de tokens: convocation bloqueada quando acumulador excede budget
- Personas não escrevem arquivos nem executam comandos de escrita durante a deliberação

## Testes da Tarefa

- [ ] Testes de unidade:
  - `NewReadOnlyRegistry` expõe apenas read/glob/grep (verificar `Definitions()` e `IsMutating()`)
  - `task` com `persona` inexistente retorna erro claro
  - `squad_kickoff` com papéis inexistentes retorna erro
  - `squad_kickoff` clampa tetos acima do máximo do config
- [ ] Testes de integração (mocks de `agent_test.go`):
  - Mesa completa: kickoff → 2 convocations → convergência; eventos carregam `Agent`
  - Teto de convocations: 9ª convocation falha; maestro encerra com plano parcial
  - Teto de tokens: gateway reporta usage alto; convocation seguinte bloqueada
  - Sem kickoff, `task(persona=...)` falha com instrução

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/tools.go` (modificado — NewReadOnlyRegistry, task com persona, squad_kickoff)
- `internal/tools/tools_test.go` (modificado)
- `internal/agent/agent.go` (modificado — AttachTaskTool com persona, RegisterKickoff)
- `internal/agent/agent_test.go` (modificado)
