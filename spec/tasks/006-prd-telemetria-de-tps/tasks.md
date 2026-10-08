# Resumo de Tarefas de Implementação de Telemetria real de TPS alimentando o catálogo

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Pacote `internal/telemetry` — estatísticas e persistência [G]
  - Requisitos: REQ-001, REQ-002
- [x] 2.0 Campos calculados e `Criteria()` alternável no catálogo [M]
  - Requisitos: REQ-003
- [x] 3.0 Coleta pós-chamada e injeção de TPS medido no Agent (depende: 1.0, 2.0) [M]
  - Requisitos: REQ-001, REQ-003
- [x] 4.0 Wiring no `main.go` e tabela de telemetria no `--doctor` (depende: 3.0) [P]
  - Requisitos: REQ-002, REQ-004
- [x] 5.0 Verificação final integrada e checagem de critérios de aceite (depende: 4.0) [P]
  - Requisitos: REQ-001, REQ-002, REQ-003, REQ-004
