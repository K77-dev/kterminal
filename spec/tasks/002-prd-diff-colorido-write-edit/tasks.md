# Resumo de Tarefas de Implementação de Diff colorido para write/edit

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Algoritmo de diff de linhas (LCS) em `internal/tools/diff.go` [M]
  - Requisitos: REQ-001
- [x] 2.0 Diff nas tools `write`/`edit`, `Result{Output, Diff}` e `PendingDiff` (depende: 1.0) [G]
  - Requisitos: REQ-001, REQ-002
- [x] 3.0 Propagação do diff no Agent e no transcript (depende: 2.0) [M]
  - Requisitos: REQ-002, REQ-003
- [x] 4.0 Renderização do diff na TUI: cores, truncamento e `confirmView` (depende: 3.0) [M]
  - Requisitos: REQ-001, REQ-002
- [x] 5.0 Verificação final integrada e checagem de critérios de aceite (depende: 4.0) [P]
  - Requisitos: REQ-001, REQ-002, REQ-003
