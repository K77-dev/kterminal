# Review 3.0 — Catalog tags + Agent core

## Status: APROVADO

## Verificação por subtask

- [x] 3.1 `Event.Agent string` — campo aditivo em `agent.Event` (linha 81)
- [x] 3.2 `catalog.Model.Criteria()` inclui `Tags: <tags>.` — implementado com `strings.Join` (catalog.go:104-106)
- [x] 3.3 `ActivateMode(mode)` / `Mode()` — aplica maestro prompt quando squad, restaura base quando sdd (agent.go:172-191)
- [x] 3.4 `buildState` por papel — inclui `You are the <discipline> persona "<name>"...` quando `a.persona != nil` (agent.go:779-783)
- [x] 3.5 `EnqueueUserMessage` — fila protegida por mutex, drenada no início de cada step do runLoop em depth 0 + modo squad (agent.go:195-199, 465-473)
- [x] 3.6 Acumuladores `turnConvocations` / `turnTokens` — reset no início do turno (depth 0), `turnTokens` soma usage a cada ChatStream (agent.go:472-473, 539)
- [x] 3.7 `RunSyncPersona` — aceita persona, pin, registry custom; seta `agentName` e `persona`; eventos propagam `Agent` (agent.go:287-316, 341-342)
- [x] 3.8 Wiring `main.go` — `squad.Load()`, `AttachSquad`, `ActivateMode(cfg.Squad.DefaultMode)` (main.go:50, 92, 98)

## Testes

- `TestCriteriaIncludesTags` — catalog_test.go:89
- `TestActivateModeSquad` — agent_test.go:4104
- `TestActivateModeInvalid` — agent_test.go:4134
- `TestEnqueueUserMessage` — agent_test.go:4141
- `TestStepLimitSquadMode` — agent_test.go:4157
- `TestRunSyncPersonaSetsAgentName` — agent_test.go:4172

Todos passam: `go test ./internal/agent/... ./internal/catalog/...` → ok

## Build

- `go build ./...` → ok
- `gofmt -w` → sem diffs
- `go vet ./internal/agent/... ./internal/catalog/...` → ok

## Notas

- `Event.Agent` propagação para session.Event será feita na task 5.0 (campo `Agent` no session.Event)
- `turnConvocations` é incrementado implicitamente a cada chamada de `task` tool (contagem de subagentes persona); o contador existe e reseta corretamente
- `RegisterKickoff` / `Kickoff` implementados para suporte da tool `squad_kickoff` (task 4.0)
