# Resumo de Tarefas de Implementação de Resiliência de streaming

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Config: knobs de resiliência (durações, env, validação) [P]
  - Requisitos: REQ-004
- [x] 2.0 llm: timeouts de streaming em camadas [M]
  - Requisitos: REQ-001
- [x] 3.0 llm: retry com backoff para falhas transientes (depende: 2.0) [M]
  - Requisitos: REQ-002, REQ-005
- [x] 4.0 agent: timeout de subagente stall-based [M]
  - Requisitos: REQ-003
- [x] 5.0 Wiring de config + doctor (depende: 1.0, 2.0, 3.0, 4.0) [P]
  - Requisitos: REQ-004, REQ-005
