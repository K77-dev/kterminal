# Resumo de Tarefas de Implementação de Interromper tarefa em execução (Esc)

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [ ] 1.0 Propagar `context.Context` pelos tools (`internal/tools`) [M]
  - Requisitos: REQ-001
- [ ] 2.0 Cancelamento do turno agêntico no Agent (`internal/agent`) (depende: 1.0) [G]
  - Requisitos: REQ-001
- [ ] 3.0 Tecla Esc na TUI, renderização do turno abortado e hint bar (`internal/tui`) (depende: 2.0) [G]
  - Requisitos: REQ-002, REQ-003
- [ ] 4.0 Verificação final integrada e checagem de critérios de aceite (depende: 3.0) [P]
  - Requisitos: REQ-001, REQ-002, REQ-003
