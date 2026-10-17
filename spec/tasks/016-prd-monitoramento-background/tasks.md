# Resumo de Tarefas de Implementação de Monitoramento de processos em background e status bar enxuta

**Legenda de tamanho**: P (< 2h) | M (2-4h) | G (4-8h) | GG (> 8h)

## Tarefas

- [x] 1.0 Pacote `internal/procmon` — Manager de processos em background: spawn com grupo próprio, graça de spawn de 250ms, watcher com `Wait()` único, drain de pipes, `Stop` TERM→KILL, `KillAll` idempotente e canal `Events()` [G]
  - Requisitos: REQ-001, REQ-003, REQ-004
- [x] 2.0 Tool `bash` com `run_in_background` — dedup em `Register`, `SetProcMon`/`ProcMon`, branch background antes do caminho síncrono e schema (depende: 1.0) [M]
  - Requisitos: REQ-001
- [x] 3.0 Agente — propagação do manager para subagentes e documentação do parâmetro no prompt (depende: 2.0) [M]
  - Requisitos: REQ-001
- [x] 4.0 TUI — hint bar enxuta: remoção de segmentos redundantes, fusão modelo+pin e churn de fixtures [M]
  - Requisitos: REQ-005
- [x] 5.0 TUI — contador `bg N` no rodapé e wiring `WithProcMon`/`procMonMsg` (depende: 1.0, 4.0) [M]
  - Requisitos: REQ-002
- [x] 6.0 TUI — popup `/bg` para parar monitoramentos individuais (depende: 5.0) [M]
  - Requisitos: REQ-003
- [x] 7.0 main.go — wiring dos três consumidores, `KillAll` nos dois caminhos e seguro de sinal externo (depende: 2.0, 3.0, 5.0) [P]
  - Requisitos: REQ-004
- [x] 8.0 Verificação final — suite completa com `-race` e checagem manual das métricas do PRD (depende: 6.0, 7.0) [P]
  - Requisitos: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005
