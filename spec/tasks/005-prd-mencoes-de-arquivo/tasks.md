# Resumo de Tarefas de Implementação de @menções de arquivo no prompt

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 `ExpandMentions` — expansão de menções como função pura [M]
  - Requisitos: REQ-001, REQ-003
- [x] 2.0 Expansão no envio, chat com texto original e aviso inline (depende: 1.0) [M]
  - Requisitos: REQ-001, REQ-003, REQ-004
- [x] 3.0 Popup de autocomplete de arquivos (depende: 1.0) [G]
  - Requisitos: REQ-002
- [x] 4.0 Verificação final integrada e checagem de critérios de aceite (depende: 2.0, 3.0) [P]
  - Requisitos: REQ-001, REQ-002, REQ-003, REQ-004
