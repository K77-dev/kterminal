# Resumo de Tarefas de Implementação de Diagnósticos pós-edição (go vet hook)

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 `GoVetHook` e detecção de módulo em `internal/tools/diagnostics.go` [G]
  - Requisitos: REQ-001, REQ-002, REQ-003
- [x] 2.0 Hook `OnGoEdit` no Registry (depende: 1.0) [M]
  - Requisitos: REQ-001
- [x] 3.0 Loop de auto-correção no Agent e injeção no `main.go` (depende: 2.0) [M]
  - Requisitos: REQ-004
- [x] 4.0 Verificação final integrada e checagem de critérios de aceite (depende: 3.0) [P]
  - Requisitos: REQ-001, REQ-002, REQ-003, REQ-004
