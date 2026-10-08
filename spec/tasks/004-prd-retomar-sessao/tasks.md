# Resumo de Tarefas de Implementação de Retomar sessão (`kterminal --continue`)

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Fundações de snapshot em `internal/session` [G]
  - Requisitos: REQ-001, REQ-002
- [x] 2.0 Gravação de snapshot nos fins de turno e `SetMessages` no Agent (depende: 1.0) [M]
  - Requisitos: REQ-001
- [x] 3.0 Flags CLI `--continue`/`--session` e wiring de resume (depende: 2.0) [M]
  - Requisitos: REQ-003
- [x] 4.0 Reconstrução visual do histórico e hint bar `resumed` na TUI (depende: 3.0) [M]
  - Requisitos: REQ-004
- [x] 5.0 Verificação final integrada e checagem de critérios de aceite (depende: 4.0) [P]
  - Requisitos: REQ-001, REQ-002, REQ-003, REQ-004
