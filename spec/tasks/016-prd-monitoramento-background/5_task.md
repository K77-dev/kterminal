# Tarefa 5.0: TUI — contador `bg N` no rodapé e wiring `WithProcMon`/`procMonMsg`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Contador dinâmico `bg N` no rodapé (sempre visível, tempo real, oculto com zero monitoramentos)

## Dependências

- 1.0 (consome `procmon.Manager.Count()`/`Events()`)
- 4.0 (o segmento `bg N` entra na hint bar já reestruturada, na posição 5 da ordem)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Conectar o manager à TUI com a mesma filosofia pull-based do par `listenAgent`/`agentEventMsg` (tui.go:296-307): a TUI **não cacheia** estado de processos — `hintBar()` lê `Count()` ao vivo a cada render (como já lê `m.agent.Mode()`/`Pinned()`), e um canal dedicado `Events()` + `procMonMsg` apenas acorda o renderer. Eventos de processo **não** entram no canal do agente: `emit()` descarta quando o buffer de 512 enche, eventos de subagente são roteados como linhas aninhadas e viraria um canal de turno num bus geral — três motivos pelos quais o contador dessincronizaria ou renderizaria errado.

O segmento `bg N` entra na posição 5 da hint bar (entre `skill` e `jev off`), com `primaryStyle`. Com zero monitoramentos o segmento é oculto e a barra é byte-a-byte a da mesma sessão sem o indicador (critério REQ-002 do PRD).

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, receivers por valor, eventos via canal, espelho do padrão `listenAgent`/`agentEventMsg` existente.
</skills>

<requirements>
- `procMonMsg struct{}` + `listenProcMon(ch <-chan struct{}) tea.Cmd` (bloqueia em `<-ch` e retorna `procMonMsg{}`) — espelho de `listenAgent`
- `WithProcMon(pm *procmon.Manager) Option` — `func(m *Model) { m.procMon = pm }`
- Helper nil-guardado `bgCount()`: manager nil → 0 → segmento oculto
- `Init()` (tui.go:296): `return tea.Batch(m.blink, listenAgent(m.agent.Events), listenProcMon(m.procMon.Events()))` — nil-safe: option ausente → `m.procMon == nil` → canal nil → `listenProcMon` bloqueia para sempre (nunca acorda, nunca re-arma — comportamento desejado)
- `Update` ganha o case `procMonMsg`: `return m, listenProcMon(m.procMon.Events())` (re-arma; sem mutação de estado — o render é pull-based, o próximo `View()` já lê `Count()` fresco)
- Segmento `bg N` na hint bar, posição 5, `primaryStyle`, condição `bgCount() > 0` — lê `Count()` ao vivo por render
- Contador em tempo real: incrementa ao lançar, decrementa em término natural ou parada (o wake via `Events()` + render pull-based entrega ambos)
- Visível em qualquer modo (sdd/squad) e largura — `hintBar` já usa `chatWidth()` que subtrai a sidebar
</requirements>

## Subtarefas

- [ ] 5.1 Adicionar campo `procMon` no `Model`, option `WithProcMon`, helper `bgCount()` nil-guardado
- [ ] 5.2 Adicionar `procMonMsg`/`listenProcMon` e o case em `Update` que re-arma o listener
- [ ] 5.3 Estender `Init()` com `listenProcMon(m.procMon.Events())` no batch
- [ ] 5.4 Adicionar o segmento `bg N` na posição 5 da `hintBar()` com `primaryStyle`
- [ ] 5.5 Testes: `TestBgBadgeHiddenWhenZero`, `TestBgBadgeShowsCount`, `TestBgBadgeLiveDecrement`, `TestInitBatchesProcMonListener`

## Detalhes de Implementação

Consulte techspec.md — seções "Hint Bar (REQ-002 + REQ-005)" (tabela de segmentos, posição 5) e "Wiring TUI" (bloco de código completo de `procMonMsg`/`listenProcMon`/`WithProcMon`/`bgCount` e o `Init()`). Racional do canal dedicado em "Considerações Técnicas → Decisões Principais" (decisão 2). Tabela de testes em "Abordagem de Testes → Testes de Integração → `internal/tui`".

## Critérios de Sucesso

- `go test ./internal/tui/ -race` verde
- Lançar 1 processo → barra mostra `bg 1`; lançar outro → `bg 2`
- Parar um dos dois → barra mostra `bg 1` imediatamente
- Processo que termina sozinho (`sleep 5`) → contador decrementa sozinho ao fim dos 5s (via `procMonMsg` no `Update`)
- Com zero monitoramentos, `hintBar()` é byte-idêntico ao de model sem a option (critério REQ-002 do PRD)
- `Init()` do model com `WithProcMon` contém o listener no batch; model sem a option nunca re-arma (canal nil)

## Testes da Tarefa

- [ ] Testes de unidade: N/A (wiring + render, coberto como integração de TUI)
- [ ] Testes de integração (`internal/tui`, padrão `step()`/`newTestModel` existente):
  - [ ] `TestBgBadgeHiddenWhenZero` — `WithProcMon(pm)`, 0 processos: `hintBar()` byte-idêntico ao de model sem a option
  - [ ] `TestBgBadgeShowsCount` — `pm.Start("sleep 30")` → render contém `bg 1`; segundo start → `bg 2`
  - [ ] `TestBgBadgeLiveDecrement` — `Start("sleep 1")` → `procMonMsg` via `Update` → render sem `bg` (contador cai sozinho)
  - [ ] `TestInitBatchesProcMonListener` — `New(..., WithProcMon(pm))` → `Init()` contém o listener no batch
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` (modificado — `procMon`/`WithProcMon`/`bgCount`/`procMonMsg`/`listenProcMon`, `Init()`, `Update`, `hintBar()`)
- `internal/tui/tui_test.go` (modificado — 4 testes novos)
- `spec/tasks/016-prd-monitoramento-background/techspec.md` (referência — seções citadas acima)
