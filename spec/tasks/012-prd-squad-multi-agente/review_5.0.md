# Review 5.0 — Session: `Agent` no `Event`, `Mode` no `Snapshot`, restore no resume

## Status: APROVADO

## Verificação por subtask

- [x] 5.1 `session.Event.Agent string` com tag `json:"agent,omitempty"` (session.go)
- [x] 5.2 `Session.Mode string` em `Snapshot` (session.go)
- [x] 5.3 `WriteSnapshot(messages, skill, mode)` grava o modo ativo; `agent.writeSnapshot()` passa `a.mode`; todos os `Session.Write` passam `Agent: a.agentName`
- [x] 5.4 `Load` restaura `Mode` do snapshot (e `LoadLatest` propaga via `Load`)
- [x] 5.5 Compatibilidade retroativa: JSONL sem `agent`/`mode` carrega sem erro (omitempty + zero values)

## Extras implementados

- `agent.Agent.rebuildSystemPrompt()` — preserva o prompt de maestro quando `mode == "squad"` mesmo após `RestoreSkill`/`ClearSkill`/`ActivateSkill`. Sem isso, o `RestoreSkill` de `main.go` (chamado após `ActivateMode`) sobrescreveria o prompt de maestro.
- `main.go` — restore de modo no resume: aplica `resumed.Mode` quando difere do `cfg.Squad.DefaultMode`.

## Testes

- `TestEventAgentSerializesWithOmitEmpty` — serializa `"agent":"architect"` e omite quando vazio
- `TestSnapshotCarriesMode` — `Mode: "squad"` persistido e restaurado por `Load`
- `TestSnapshotOmitsEmptyMode` — campo `mode` ausente quando vazio
- `TestLoadOldJSONLWithoutAgentOrMode` — compatibilidade retroativa
- `TestRestoreSkillPreservesSquadPrompt` — restore de skill não clobbera prompt de maestro; troca para sdd restaura base

Todos passam: `go test ./...` → ok

## Build

- `go build ./...` → ok
- `gofmt -w` → sem diffs
- `go vet ./...` → ok

## Call sites atualizados

`WriteSnapshot` mudou de assinatura; atualizados `internal/session/session_test.go`, `main_test.go`.
