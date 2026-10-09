# Resumo de Tarefas de Implementação de Sidebar de agentes do squad

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Tipo `squad.Mesa` — estado persistível da mesa com transições seguras para concorrência [G]
  - Requisitos: REQ-002, REQ-004, REQ-006
- [x] 2.0 Session — campos aditivos `Event.Mesa`/`Snapshot.Mesa` e assinatura de `WriteSnapshot` (depende: 1.0) [P]
  - Requisitos: REQ-006
- [x] 3.0 Agent — `Event.TurnTokens`, ownership da Mesa, espelhos de contadores, transições de status, drain de `runSubagent` e passagem ao snapshot (depende: 1.0, 2.0) [GG]
  - Requisitos: REQ-002, REQ-003, REQ-004, REQ-006
- [x] 4.0 TUI — render do sidebar (`sidebar.go` + estilos): largura, mesa, maestro, personas, rodapé de orçamento, elisão (depende: 3.0) [G]
  - Requisitos: REQ-002, REQ-003, REQ-004, REQ-005
- [x] 5.0 TUI — integração do sidebar na `View()`, larguras no resize, eventos de persona, atividade transiente e resume no `main.go` (depende: 3.0, 4.0) [GG]
  - Requisitos: REQ-001, REQ-003, REQ-005, REQ-006
- [x] 6.0 Testes de integração E2E, regressão completa e dogfooding documentado (depende: 5.0) [M]
  - Requisitos: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006
