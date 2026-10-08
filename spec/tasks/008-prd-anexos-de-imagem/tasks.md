# Resumo de Tarefas de Implementação de Anexos de imagem no prompt

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Content parts no `llm.Message` com `MarshalJSON` condicional [M]
  - Requisitos: REQ-001
- [x] 2.0 Capacidade de visão no catálogo [P]
  - Requisitos: REQ-002
- [x] 3.0 Comandos `/image`/`/unimage` e chips de anexos na TUI (depende: 1.0) [G]
  - Requisitos: REQ-003
- [x] 4.0 `RunWithAttachments`, roteamento por visão e transcript sem base64 (depende: 1.0, 2.0, 3.0) [G]
  - Requisitos: REQ-004, REQ-005
- [x] 5.0 Verificação final integrada e checagem de critérios de aceite (depende: 4.0) [P]
  - Requisitos: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005
