# Resumo de Tarefas de Implementação de Squad mode (loop de engenharia multi-agente)

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Pacote `internal/squad` — loader de personas, assets embutidos e validação de kickoff [G]
  - Requisitos: REQ-002, REQ-003
- [x] 2.0 Seção `[squad]` no config TOML — defaults, pins e `default_mode` [P]
  - Requisitos: REQ-001, REQ-005, REQ-006
- [x] 3.0 Catalog tags + Agent core — `Event.Agent`, `buildState` por papel, ativação de modo, fila de mensagens, acumuladores de turno, wiring no `main.go` (depende: 1.0, 2.0) [G]
  - Requisitos: REQ-002, REQ-004, REQ-005, REQ-007, REQ-008
- [x] 4.0 Tools — `NewReadOnlyRegistry`, `task` com arg `persona`, tool `squad_kickoff` (depende: 1.0, 3.0) [M]
  - Requisitos: REQ-004, REQ-006, REQ-009
- [x] 5.0 Session — campo `Agent` no `Event`, `Mode` no `Snapshot`, restore no resume (depende: 3.0) [P]
  - Requisitos: REQ-008
- [ ] 6.0 TUI — popup `/mode`, hint bar com modo ativo, render `▸ <papel>:` com cor por disciplina, feedback de enfileiramento (depende: 3.0, 5.0) [G]
  - Requisitos: REQ-001, REQ-007, REQ-008
- [ ] 7.0 Rules Go mínimas com frontmatter + dogfooding da primeira mesa real (depende: 1.0, 3.0, 4.0, 6.0) [M]
  - Requisitos: REQ-004, REQ-009
