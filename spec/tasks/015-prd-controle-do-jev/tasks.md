# Resumo de Tarefas de Implementação de Controle do Jev: toggle de roteamento e modelo fixo

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Config — campos `Typesafe.Enabled`/`Config.Model`, accessors, env overrides e `Save()` normalizado [M]
  - Requisitos: REQ-001, REQ-002
- [x] 2.0 Router — `FixedRouter` determinístico e factory `Wire()` (depende: 1.0) [M]
  - Requisitos: REQ-002
- [x] 3.0 main.go — wiring via `Wire()`, validação fatal de modelo no startup e `--doctor` com estado do Jev (depende: 1.0, 2.0) [M]
  - Requisitos: REQ-002, REQ-004
- [x] 4.0 TUI — `rebuildAgent()` via `Wire()`, `/model` persistente e picker de modelos (depende: 1.0, 2.0) [G]
  - Requisitos: REQ-003
- [x] 5.0 TUI — comando `/jev on|off|status` com re-wiring sem restart (depende: 4.0) [M]
  - Requisitos: REQ-001
- [x] 6.0 Descoberta — tela `/config` com 6 inputs, status bar `jev`/`jev off`, `/help` e sugestões (depende: 4.0, 5.0) [M]
  - Requisitos: REQ-004
- [x] 7.0 Verificação final — suite completa e métricas do PRD via transcript JSONL (depende: 1.0, 2.0, 3.0, 4.0, 5.0, 6.0) [P]
  - Requisitos: REQ-001, REQ-002, REQ-003, REQ-004
