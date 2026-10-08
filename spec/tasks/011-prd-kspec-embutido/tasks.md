# Resumo de Tarefas de Implementação de kspec embutido no kterminal

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Vendoring e loader base do kspec (sync script, árvore embutida, frontmatter, List) [G]
  - Requisitos: REQ-001, REQ-002 (parcial)
- [x] 2.0 Resolução projeto-first com templates inlined, Rules, Version e Source (depende: 1.0) [M]
  - Requisitos: REQ-002
- [x] 3.0 System prompt do agente, skill ativa e persistência em sessão (depende: 2.0) [G]
  - Requisitos: REQ-003, REQ-004, REQ-008
- [x] 4.0 Tool ask_user com evento bloqueante e wizard na TUI [G]
  - Requisitos: REQ-005
- [x] 5.0 Comandos /kspec-* dinâmicos, tab-completion, hint bar e /kspec-version nativo (depende: 2.0, 3.0, 4.0) [M]
  - Requisitos: REQ-006, REQ-009 (parcial)
- [x] 6.0 Tool kspec_bootstrap com materialização da árvore kspec (depende: 2.0, 4.0) [M]
  - Requisitos: REQ-007
- [x] 7.0 Diagnóstico no --doctor e verificação final do fluxo SDD (depende: 3.0, 5.0, 6.0) [P]
  - Requisitos: REQ-009
