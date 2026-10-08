# Resumo de Tarefas de Implementação de Subagentes (tool `task`)

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Extrair `runLoop` do loop do Agent (refactor puro) [M]
  - Requisitos: REQ-002
- [x] 2.0 `depth`, `newSubagent` e `RunSync` com re-emissão e limites (depende: 1.0) [GG]
  - Requisitos: REQ-002, REQ-003, REQ-004, REQ-005
- [x] 3.0 Tool `task` com `AttachTaskTool` (depende: 2.0) [M]
  - Requisitos: REQ-001
- [x] 4.0 Campo `Depth` na session e renderização aninhada na TUI (depende: 2.0) [M]
  - Requisitos: REQ-003, REQ-006
- [x] 5.0 Wiring no `main.go` e verificação final integrada (depende: 3.0, 4.0) [P]
  - Requisitos: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006
