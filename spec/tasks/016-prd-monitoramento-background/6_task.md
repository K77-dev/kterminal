# Tarefa 6.0: TUI — popup `/bg` para parar monitoramentos individuais

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — Seletor interativo para parar monitoramentos (lista com número/comando/uptime, navegação setas+enter, esc sem efeito, escalonamento TERM→KILL, empty-state)

## Dependências

- 5.0 (usa `WithProcMon`, `bgCount()` e o wiring do manager na TUI)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Construir o seletor interativo `/bg` como espelho do padrão `/mode` (commands.go:105-121): estados `bgOpen`/`bgSelected` no `Model`, helpers `openBgPopup`/`closeBgPopup`/`selectBg`/`bgPopupView`. A lista é pull-based — `m.procMon.List()` por render, sempre fresca. `enter`/`tab` param o selecionado via `Stop(id)` e o popup **permanece aberto** com a lista atualizada (o usuário pode parar vários em sequência); `esc`/`q` fecham sem parar nada. Com zero monitoramentos, `/bg` mostra `no background processes running` no chat, sem popup.

O bloco `bgOpen` entra **primeiro** na cadeia de `handleChatKey`, antes de modeOpen/modelOpen/mentionOpen/cmdOpen — um popup de gerenciamento de processos não deve ser preemptado por sugestões de digitação.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, navegação idêntica aos popups existentes (`/mode`, `/model`), zero estilos novos em theme.go.
</skills>

<requirements>
- Estados `bgOpen`/`bgSelected` no `Model`; helpers `openBgPopup` (fecha `cmdPopup`, zera seleção)/`closeBgPopup`/`selectBg`/`bgPopupView` em commands.go
- Abertura: `/bg` em `handleCommand` (tui.go:1030) → `openBgPopup()`; com zero monitoramentos: `resultStyle.Render("no background processes running")` no chat, sem popup
- Lista: `m.procMon.List()` por render — linhas `#N · <comando truncado> · <uptime>`; comando truncado via `truncate()` existente (rune-safe, testado em tui_test.go:1249); uptime `time.Since(entry.Started).Round(time.Second)` no formato `5m32s` (estático por render, sem ticker)
- Navegação: `up`/`down` com wrap (idêntico a modeOpen, tui.go:648-680); `enter`/`tab` → `selectBg()`: `Stop(id)` no selecionado, popup permanece aberto com lista atualizada; `esc`/`q` fecham sem parar nada
- Clamp pós-stop: `if bgSelected >= len(list) { bgSelected = len(list)-1 }` (0 se vazia — vazia mostra empty-state e enter vira no-op)
- Posição na cadeia: bloco `bgOpen` **primeiro** em `handleChatKey`, antes de modeOpen/modelOpen/mentionOpen/cmdOpen
- Visual: `chromeLabelStyle.Render("BACKGROUND")` de título, itens com `mentionItemStyle`/`mentionSelectedStyle` (`▸ ` no selecionado), container `mentionPopupStyle`, help line `↑↓ move · enter stop · esc close` — mesmos estilos e glifos dos popups existentes, zero estilos novos em theme.go
- Renderizado em `chatView()` junto aos demais popups (tui.go:1632-1643)
- `/bg` em `nativeCommands` (ordem alfabética: antes de `/clear`), `/help` e `handleCommand`
- Churn mapeado: `TestCommandPopupListsNativeAndKspec` / `TestCommandPopupWithoutStoreListsNativesOnly` (commands_test.go) ganham `/bg`; `TestHelpListsKspecCommands` ganha a linha `/bg — background processes`
- Edge cases: terminal estreito — popup herda comportamento do mentionPopup (largura do conteúdo); 50+ monitoramentos — lista renderiza completa (popups existentes não têm cap)
</requirements>

## Subtarefas

- [ ] 6.1 Adicionar estados `bgOpen`/`bgSelected` no `Model` e helpers `openBgPopup`/`closeBgPopup`/`selectBg`/`bgPopupView` em commands.go
- [ ] 6.2 Registrar `/bg` em `nativeCommands`, `handleCommand` (com empty-state) e `/help`; bloco `bgOpen` primeiro em `handleChatKey` com navegação wrap + clamp
- [ ] 6.3 Renderizar o popup em `chatView()` junto aos demais popups
- [ ] 6.4 Atualizar churn de commands_test.go (listas de popup com `/bg`, linha do `/help`)
- [ ] 6.5 Testes: `TestBgPopupOpenListStop`, `TestBgPopupEscStopsNothing`, `TestBgPopupEmptyState`, `TestBgPopupSelectionClamps`, `TestBgPopupKeyRouting`

## Detalhes de Implementação

Consulte techspec.md — seção "Popup `/bg` (REQ-003)" (especificação completa: abertura, lista, navegação, posição na cadeia, visual e edge cases). Tabela de testes em "Abordagem de Testes → Testes de Integração → `internal/tui`" e churn em "Churn de testes existentes".

## Critérios de Sucesso

- `go test ./internal/tui/ -race` verde
- `/bg` com 2 monitoramentos lista ambos, navegáveis por setas com wrap
- `enter` no segundo item encerra apenas esse processo; o primeiro segue rodando e a barra mostra `bg 1`; popup permanece aberto com lista atualizada
- `esc` não encerra nenhum processo
- `/bg` com 0 monitoramentos informa `no background processes running`, sem popup
- Teclas de digitação com popup aberto não caem no input; setas não movem o cursor do textarea
- Seleção clampada após stops sucessivos; sem pânico com lista vazia
- Popup segue o padrão visual dos existentes (mesmos estilos, zero mudanças em theme.go)

## Testes da Tarefa

- [ ] Testes de unidade: N/A (popup é interação de TUI, coberto como integração)
- [ ] Testes de integração (`internal/tui`, padrão `step()`/`newTestModel` existente):
  - [ ] `TestBgPopupOpenListStop` — 2 starts → `/bg` → popup lista 2; `down` + `enter` → `Count()==1`; popup aberto com lista atualizada; barra `bg 1`
  - [ ] `TestBgPopupEscStopsNothing` — 2 starts → `/bg` → `esc` → popup fecha; `Count()==2`
  - [ ] `TestBgPopupEmptyState` — `/bg` com 0 → mensagem `no background processes running`; sem popup
  - [ ] `TestBgPopupSelectionClamps` — 2 starts → `/bg` → para ambos → seleção clampa; sem pânico com lista vazia
  - [ ] `TestBgPopupKeyRouting` — popup aberto → teclas de digitação não caem no input; setas não movem o cursor do textarea
  - [ ] Churn: `TestCommandPopupListsNativeAndKspec`/`TestCommandPopupWithoutStoreListsNativesOnly` com `/bg`; `TestHelpListsKspecCommands` com a linha nova
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` (modificado — estados no `Model`, `handleChatKey`, `handleCommand`, `/help`, `chatView`)
- `internal/tui/commands.go` (modificado — `/bg` em `nativeCommands`, helpers do popup)
- `internal/tui/commands_test.go` (modificado — churn + testes novos)
- `internal/tui/tui_test.go` (modificado — testes novos de popup)
- `spec/tasks/016-prd-monitoramento-background/techspec.md` (referência — seções citadas acima)
