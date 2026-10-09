# Tarefa 3.0: Agent — `Event.TurnTokens`, ownership da Mesa, espelhos de contadores, transições de status, drain de `runSubagent` e passagem ao snapshot

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Painel da mesa (transições de status nos pontos do loop)
- REQ-003 — Atividade ao vivo (eventos aninhados enriquecidos com cumulativos)
- REQ-004 — Métricas por persona e orçamento da mesa (espelhos garantem igualdade com o enforcement)
- REQ-006 — Persistência e resume (Mesa ao snapshot, `RestoreMesa` no resume)

## Dependências

- 1.0 (tipo `squad.Mesa`)
- 2.0 (campos e assinatura em `internal/session`)

## Estimativa

- **Tamanho**: GG
- **Horas estimadas**: 8-12h

## Visão Geral

Tornar o Agent dono da Mesa e alimentá-la nos pontos existentes do loop agêntico — nenhum novo loop, canal ou event-kind. Três frentes: (1) `Event.TurnTokens int64` (aditivo) carimbado junto com `SessionCost` em `EventToolStart`/`EventTurnDone` após a acumulação de usage, dando métricas ao vivo por persona nos eventos aninhados; (2) campo `mesa *squad.Mesa` com getters `Mesa()` (cópia rasa sob lock) e `RestoreMesa()`, espelhando as mutações de `turnTokens`/`turnConvocations` nos mesmos pontos e transitando status em `RegisterKickoff`/`convokePersona`/`endTurn`; (3) drain de `runSubagent` chamando `ObservePersona` e `writeSnapshot` passando a Mesa.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, eventos via canal existente, espelhos colados às mutações existentes, testes com `-race` usando os mocks de `agent_test.go`.
</skills>

<requirements>
- `Event` ganha `TurnTokens int64` (aditivo); em depth 0 os campos são ignorados pela TUI hoje — zero regressão
- `SessionCost` passa a ser carimbado também em eventos aninhados (`tool_start`/`turn_done` de subagente), junto com `TurnTokens`
- Espelhos obrigatórios nos mesmos pontos de mutação: início de turno (`ResetTurn`), acumulação por chamada em depth 0 (`AddTokens`), retorno de `runSubagent` (`AddTokens`/`AddConvocation`), `convokePersona` (`StartDeliberation`)
- `RegisterKickoff` chama `Reset` (roles + disciplinas resolvidas); `endTurn` (depth 0) chama `FinishTurn`
- `Mesa()` devolve cópia rasa sob lock; `RestoreMesa(m)` injeta Mesa restaurada do snapshot
- `writeSnapshot` passa a Mesa (campo da task 2.0)
- Igualdade por construção: `Mesa.Tokens == turnTokens` e `Mesa.Convocations == turnConvocations` ao fim do turno, incluindo a defasagem mid-convocation existente
</requirements>

## Subtarefas

- [ ] 3.1 Adicionar `TurnTokens int64` em `Event`; carimbar `TurnTokens`/`SessionCost` em `EventToolStart` e `EventTurnDone` após a acumulação de usage (agent.go:713-789), inclusive em depth de subagente
- [ ] 3.2 Adicionar campo `mesa *squad.Mesa` + getters `Mesa()`/`RestoreMesa()`; inicializar Mesa vazia na ativação do modo squad
- [ ] 3.3 Espelhar contadores: `ResetTurn` no início do turno, `AddTokens` nas acumulações de usage em depth 0, `AddTokens`/`AddConvocation` no retorno de `runSubagent`
- [ ] 3.4 Transições: `RegisterKickoff` → `Reset`; `convokePersona` → `StartDeliberation` + `AddConvocation`; fim da contribuição → `FinishDeliberation`; `endTurn` → `FinishTurn`
- [ ] 3.5 Drain de `runSubagent`: `ObservePersona(name, model, tokens, cost)` com os cumulativos do subagente
- [ ] 3.6 `writeSnapshot` passa a Mesa; call site da task 2.0 deixa de passar `nil`
- [ ] 3.7 Testes com os mocks existentes: mesa completa com kickoff + 2 convocations, igualdade pós-turno, abort mid-convocation, acumulação sobre baseline

## Detalhes de Implementação

Consulte techspec.md — seções "Interfaces Principais", "Arquitetura do Sistema" (componente modificado `internal/agent`) e "Verificações Técnicas → Arquitetura". Pontos exatos em `agent.go`: `Event` (linha 61), campo mesa (121), `RegisterKickoff` (215), `writeSnapshot` (260), `endTurn` (284), drain de `runSubagent` (383), `convokePersona` (533), acumulações do `runLoop` (713). A defasagem mid-convocation (tokens do sub em voo só aterrissam no retorno de `runSubagent`) é comportamento existente e deve ser preservada.

## Critérios de Sucesso

- Mesa completa com kickoff + 2 convocations: statuses transitam `waiting`→`deliberating`→`done`, `Event.ToolStart`/`TurnDone` aninhados carregam `TurnTokens`/`SessionCost` cumulativos
- `Mesa.Tokens == turnTokens` e `Mesa.Convocations == turnConvocations` ao fim do turno (teste de igualdade pós-turno)
- Abort mid-convocation (Esc): `FinishTurn` marca a persona deliberando como `done`
- Segunda convocation da mesma persona acumula métricas sobre o baseline da primeira
- Snapshot contém a Mesa; `RestoreMesa` devolve estado idêntico via `Mesa()`
- `go test ./internal/agent/ -race` verde

## Testes da Tarefa

- [ ] Testes de unidade (`agent_test.go`, mocks existentes):
  - Ciclo completo: kickoff → convocation → contribuição → convergência, conferindo Mesa após cada etapa
  - Eventos aninhados `tool_start`/`turn_done` com `TurnTokens`/`SessionCost` cumulativos > 0
  - Igualdade `Mesa.Tokens`/`Convocations` × `turnTokens`/`turnConvocations` pós-turno
  - Abort mid-convocation → persona `done`
  - `Mesa()`/`RestoreMesa` round-trip (cópia, não ponteiro compartilhado)
  - Concorrência drain × runLoop × leitor sob `-race`
- [ ] Testes de integração: `writeSnapshot` produz linha `snapshot` com `mesa` no JSONL e `Load` a devolve (ponta-a-ponta Agent↔Session)
- [ ] Testes E2E: N/A (fica na 6.0)

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` (modificado — Event.TurnTokens, campo mesa, getters, espelhos, transições, drain, writeSnapshot)
- `internal/agent/agent_test.go` (modificado)
- `internal/squad/mesa.go` (dependência — task 1.0)
- `internal/session/session.go` (dependência — task 2.0)
