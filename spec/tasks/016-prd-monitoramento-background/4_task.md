# Tarefa 4.0: TUI — hint bar enxuta: remoção de segmentos redundantes, fusão modelo+pin e churn de fixtures

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa não será invalidada</critical>

## Requisitos Atendidos

- REQ-005 — Status bar enxuta (remoção de segmentos redundantes, fusão modelo+pin, acesso preservado via `/jev`, `--doctor`, `/config`, `/mode`)

## Dependências

- Nenhuma (paralelizável com 1.0 — a hint bar enxuta não toca procmon)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Reestruturar `hintBar()` (tui.go:1412) em lugar para exibir apenas estado que muda o comportamento da sessão ou que o usuário não deduz de outra forma. Saem como segmentos: `/ commands` (dica permanente aprendida uma vez), `mode squad` (redundante com a sidebar), `resumed · N messages` (informação única que persistia a sessão inteira) e `jev` no estado ativo — apenas o desvio `jev off` continua visível. `pinned <model>` e o modelo corrente fundem-se num único segmento: o modelo em uso, com `⚑` prefixado apenas quando há pin ativo.

O churn de fixtures acontece **junto** com a reestruturação (mesmo commit): o teste byte-idêntico `TestSDDViewByteIdenticalToPreSidebar` usa o fixture `legacyHintBar` (tui_test.go:2618, usado em :2723, testado em :2740) e falharia enganosamente se a barra mudasse sem o fixture — o propósito do teste é guardar o SDD view contra regressão da **sidebar**, não da hint bar, e atualizá-lo mantém a guarda honesta.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, textos de TUI em inglês, receivers por valor, zero estilos novos em theme.go (reutiliza os existentes).
</skills>

<requirements>
- Lado esquerdo inalterado: spinner+verbo quando busy, cwd quando idle
- Lado direito na ordem exata da tabela da techspec (seção "Hint Bar"): 1. `copyFlash` (successStyle) → 2. `↑ scrolled · end to jump` quando `!follow` (warningStyle) → 3. `esc to interrupt` quando busy (warningStyle) → 4. `skill <name>` quando skill ativa (primaryStyle) → 5. `bg N` quando `bgCount() > 0` (primaryStyle — segmento adicionado na task 5.0; nesta task a ordem já fica reservada) → 6. `jev off` apenas desvio (warningStyle) → 7. `<model>` ou `⚑ <model>` (routeStyle; fallback para o pin; `⚑` só com pin ativo) → 8. custo quando `sessionCost > 0` (routeStyle) → 9. `N tok/s` quando `lastTPS > 0` (routeStyle)
- Removidos: `/ commands`, `resumed · N messages`, `mode squad`, `jev` positivo, `pinned X` como segmento próprio
- Fusão modelo+pin: `model := m.currentModel; if model == "" { model = m.agent.Pinned() }`, renderizado uma única vez, com `⚑` prefixado apenas quando `m.agent.Pinned() != ""` — nunca `pinned X` + modelo duplicados
- `jev off` permanece na barra como hoje (desvio visível — critério do PRD)
- Largura: `hintBar` já usa `chatWidth()` (tui.go:1413, 818), que subtrai a sidebar no modo squad; o enxugamento reduz o risco de overflow
- Churn mapeado (quebra intencional): `TestHintBarShowsRoutingState` (tui_test.go:952) reescrito — `pinned glm-5.3` vira `⚑ glm-5.3` no segmento modelo, `jev` positivo vira assert negativo, `jev off` permanece; `TestHintBarShowsResumed` (tui_test.go:1296) **deletado** (segmento não existe mais); `legacyHintBar` (tui_test.go:2618) **atualizada** para a nova barra (não deletada); `TestModeCommandSwitchesToSquad` (commands_test.go) — assert de `mode squad` na barra vira assert negativo (a sidebar continua mostrando o modo)
</requirements>

## Subtarefas

- [ ] 4.1 Reestruturar `hintBar()` (tui.go:1412) com a nova ordem de segmentos, remoções e fusão modelo+pin com `⚑`
- [ ] 4.2 Reescrever `TestHintBarShowsRoutingState` (tui_test.go:952) e deletar `TestHintBarShowsResumed` (tui_test.go:1296)
- [ ] 4.3 Atualizar o fixture `legacyHintBar` (tui_test.go:2618) para a nova barra — mesmo commit da reestruturação
- [ ] 4.4 Ajustar `TestModeCommandSwitchesToSquad` (commands_test.go) para assert negativo de `mode squad` na barra
- [ ] 4.5 Adicionar `TestHintBarSlimmedSegments` e `TestHintBarJevOffStillShown` (tabela de integração da techspec)

## Detalhes de Implementação

Consulte techspec.md — seções "Hint Bar (REQ-002 + REQ-005)" (tabela completa de segmentos com condições e estilos) e "Riscos Conhecidos" (lockstep de fixtures). O churn completo de testes existentes está em "Abordagem de Testes → Churn de testes existentes (quebra intencional, mapeada)".

## Critérios de Sucesso

- `go test ./internal/tui/ -race` verde
- Sessão típica (squad, jev on, sem pin, sem monitoramentos): barra exibe apenas cwd (idle) + modelo + custo + tok/s (≤4 segmentos persistentes — métrica do PRD)
- `jev off` continua aparecendo na barra (desvio visível)
- Pin ativo: modelo aparece uma única vez com `⚑` — sem duplicação `pinned X` + modelo
- `/ commands`, `mode squad`, `resumed · N messages` e `jev` positivo não aparecem em nenhum estado da barra
- `TestSDDViewByteIdenticalToPreSidebar` verde com o fixture atualizado (guarda da sidebar preservada)

## Testes da Tarefa

- [ ] Testes de unidade: N/A (hint bar é render puro, coberto como integração de TUI)
- [ ] Testes de integração (`internal/tui`, padrão `step()`/`newTestModel` existente):
  - [ ] `TestHintBarSlimmedSegments` — sessão squad+jev on+pin+resumed: barra **sem** `/ commands`, `mode squad`, `resumed`, `jev` positivo, `pinned X`; modelo 1× com `⚑`
  - [ ] `TestHintBarJevOffStillShown` — jev desligado: `jev off` presente
  - [ ] `TestHintBarShowsRoutingState` reescrito — `⚑ glm-5.3` no segmento modelo; `jev` positivo ausente; `jev off` presente
  - [ ] `TestModeCommandSwitchesToSquad` — assert negativo de `mode squad` na barra
  - [ ] `TestSDDViewByteIdenticalToPreSidebar` verde com `legacyHintBar` atualizada
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` (modificado — `hintBar()`)
- `internal/tui/tui_test.go` (modificado — `TestHintBarShowsRoutingState` reescrito, `TestHintBarShowsResumed` deletado, `legacyHintBar` atualizada, testes novos)
- `internal/tui/commands_test.go` (modificado — assert de `mode squad`)
- `spec/tasks/016-prd-monitoramento-background/techspec.md` (referência — seções citadas acima)
