# Tarefa 6.0: TUI — popup `/mode`, hint bar com modo ativo, render `▸ <papel>:` com cor por disciplina, feedback de enfileiramento

<critical>Ler os arquivos de prd.md e techspec.md desta pasta; se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Seletor de modo `/mode`
- REQ-007 — Fila de mensagens do usuário
- REQ-008 — Eventos, transcript e identidade visual por papel

## Dependências

- 3.0 (Agent core — `ActivateMode`, `Mode`, `EnqueueUserMessage`, `Event.Agent`)
- 5.0 (Session — `Mode` no snapshot para persistência)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Modificar `internal/tui` para a experiência do squad mode: (1) popup `/mode` com navegação por setas (`sdd`/`squad`), aplicação imediata da seleção, (2) modo ativo visível no hint bar em ambos os modos, (3) render de contribuições como `▸ <papel>:` com cor por disciplina (paleta do tema), (4) enfileiramento de mensagens quando busy em squad (feedback visual, não desaparece), (5) render de `agent` para eventos aninhados.

## Conformidade com Skills Padrões

- Go 1.27, sem comentários, TUI com receivers por valor (padrão do `tui.go`)
- Popup `/mode` segue o padrão do popup de comandos (`commands.go` — navegação por setas, Enter confirma, Esc cancela)
- Rótulo textual sempre presente; cor nunca é o único indicador de papel (acessibilidade)
- Cores da paleta do tema com contraste adequado

## Requisitos

- `/mode` abre popup navegável com `sdd` e `squad`; a seleção aplica imediatamente via `agent.ActivateMode`
- O modo ativo é visível no hint bar em ambos os modos
- Contribuições renderizadas como `▸ <papel>:` com cor da disciplina (paleta do tema)
- A cor nunca é o único indicador do papel (rótulo textual sempre presente)
- Mensagem digitada durante a mesa dá feedback de enfileiramento (não desaparece) e é enfileirada via `agent.EnqueueUserMessage`
- Navegação do popup 100% por teclado (setas, Enter, Esc)

## Subtarefas

- [ ] 6.1 Adicionar estado dedicado do popup `/mode` em `tui.go` (análogo a `cmdOpen`/`mentionOpen`)
- [ ] 6.2 Implementar `/mode` em `handleCommand` (`commands.go`) — abrir popup com `sdd`/`squad`
- [ ] 6.3 Implementar navegação por setas/Enter/Esc no popup de modo; aplicar seleção via `agent.ActivateMode`
- [ ] 6.4 Adicionar modo ativo no hint bar em ambos os modos
- [ ] 6.5 Implementar render `▸ <papel>:` para eventos com `Agent` não vazio — estilo com cor por disciplina
- [ ] 6.6 Adicionar cores por disciplina em `theme.go` (paleta do tema)
- [ ] 6.7 Estender `handleNestedEvent` para usar o rótulo `agent` quando presente
- [ ] 6.8 Implementar enfileiramento: quando `busy` e modo squad, Enter enfileira via `agent.EnqueueUserMessage` e mostra feedback visual (não o warning "busy — wait")

## Detalhes de Implementação

Consulte "Experiência do Usuário" e "Design de Implementação" na `techspec.md`:

- Popup `/mode` segue o padrão do popup de comandos (navegação por setas, Enter confirma, Esc cancela)
- Modo ativo sempre visível no hint bar
- Contribuições renderizadas como `▸ <papel>:` com cor da disciplina na paleta do tema
- Mensagem digitada durante a mesa dá feedback de enfileiramento (não desaparece)

Referências no código: popup de comandos em `commands.go` (`commandPopupView`, `handleChatKey`), estado em `tui.go` (`cmdOpen`, `cmdItems`, `cmdSelected`), hint bar em `tui.go` (`hintBar`), render de eventos aninhados em `tui.go` (`handleNestedEvent`), cores em `theme.go`.

O `handleAgentEvent` já trata `e.Depth > 0` via `handleNestedEvent`. Adicionar o rótulo `▸ <papel>:` usando `e.Agent` quando não vazio.

## Critérios de Sucesso

- `/mode` abre popup com `sdd`/`squad`; setas navegam; Enter aplica imediatamente
- Modo ativo aparece no hint bar em ambos os modos
- Contribuições de persona renderizam `▸ <papel>:` com cor da disciplina
- Rótulo textual presente mesmo quando a cor é usada
- Mensagem mid-mesa em squad é enfileirada com feedback (não desaparece, não mostra o warning de busy)
- Esc continua abortando o turno inteiro

## Testes da Tarefa

- [ ] Testes de unidade (`tui_test.go`):
  - `/mode` abre o popup com os dois itens
  - Navegação por setas altera a seleção; Enter aplica o modo
  - Render de evento com `Agent: "architect"` produz rótulo `▸ architect:`
  - Estilo por disciplina retorna cor distinta por papel
  - Enter quando busy em squad enfileira (não mostra warning de busy)
- [ ] Testes de integração:
  - Fluxo `/mode` → `squad` → pedido → kickoff anunciado → convocações rotuladas → convergência
  - Esc mid-mesa gera `turn_aborted` e descarta a mesa
- [ ] Testes E2E — sem TestSprite (TUI local); E2E = dogfooding documentado (tarefa 7.0)

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` (modificado — /mode, hint bar, render por papel, fila)
- `internal/tui/commands.go` (modificado — comando /mode e popup)
- `internal/tui/theme.go` (modificado — cores por disciplina)
- `internal/tui/tui_test.go` (modificado)
- `internal/tui/commands_test.go` (modificado)
