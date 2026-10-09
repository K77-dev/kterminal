# Tarefa 5.0: TUI — integração do sidebar na `View()`, larguras no resize, eventos de persona, atividade transiente e resume no `main.go`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Exibição condicionada ao modo squad (aparece/some com o `/mode`, sem capturar input)
- REQ-003 — Atividade ao vivo (mapa transiente alimentado por eventos aninhados)
- REQ-005 — Layout responsivo (larguras derivadas no resize, chat íntegro)
- REQ-006 — Persistência e resume (wiring de `RestoreMesa` no resume)

## Dependências

- 3.0 (Mesa no Agent, eventos aninhados com cumulativos)
- 4.0 (render do sidebar)

## Estimativa

- **Tamanho**: GG
- **Horas estimadas**: 8-12h

## Visão Geral

Conectar o sidebar à TUI: `View()` compõe chat + sidebar com `lipgloss.JoinHorizontal` somente em modo squad com terminal ≥ 100 cols; `WindowSizeMsg` deriva as larguras da coluna de chat; `handleAgentEvent` passa a tratar `EventKickoff` (hoje ignorado) e limpa a atividade; `handleNestedEvent` mantém o mapa transiente `personaActivity` (persona → `tool(args)` truncado) com os cumulativos que chegam nos eventos aninhados; `hintBar` usa a largura do chat; `main.go` chama `ag.RestoreMesa(resumed.Mesa)` após restaurar o modo. Estados modais (config/confirm/ask) continuam full-width.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, receivers por valor em `View`/render, eventos via canal existente (nenhum novo event-kind), textos da TUI em inglês, `lipgloss.Width` nos testes de largura.
</skills>

<requirements>
- `View()`: `lipgloss.JoinHorizontal` chat + sidebar somente quando modo squad e largura ≥ 100 cols; caso contrário, chat em largura total — modo sdd renderiza byte-a-byte idêntico ao atual
- `WindowSizeMsg` deriva largura do chat e do sidebar; `hintBar` renderiza na largura do chat
- `handleAgentEvent`: `EventKickoff` re-renderiza (Mesa reconstruída via getter) e limpa `personaActivity`; nenhum handler de tecla novo — sidebar não captura input
- `handleNestedEvent`: `personaActivity[persona]` = `tool(args)` truncado a partir de `tool_start` aninhado; persona concluída mantém a última ação da sessão corrente
- Atividade é transiente: não persiste no snapshot; persona sem tools mostra "—"
- `main.go`: `ag.RestoreMesa(resumed.Mesa)` após restaurar o modo (resume em squad reconstrói o painel; snapshot sem mesa → "mesa não iniciada")
- Popups/wizards existentes (config/confirm/ask) continuam full-width; sidebar reaparece ao voltar ao chat
- Nenhuma tecla existente muda de comportamento
</requirements>

## Subtarefas

- [ ] 5.1 Compor `View()` com `lipgloss.JoinHorizontal` (chat + sidebar) condicionado a modo squad e largura ≥ 100; sdd e terminal estreito usam largura total
- [ ] 5.2 Derivar larguras em `WindowSizeMsg` (coluna de chat + sidebar) e ajustar `hintBar` para a largura do chat
- [ ] 5.3 Tratar `EventKickoff` em `handleAgentEvent`: re-render + limpeza de `personaActivity`
- [ ] 5.4 Manter `personaActivity` em `handleNestedEvent` a partir de `tool_start` aninhado (com `TurnTokens`/`SessionCost` da 3.0 disponíveis para o rodapé ao vivo)
- [ ] 5.5 Wiring de resume no `main.go`: `ag.RestoreMesa(resumed.Mesa)` após restaurar o modo
- [ ] 5.6 Testes: matriz de visibilidade, resize 80→120→200, render sdd byte-a-byte, atividade ao vivo, kickoff limpando atividade

## Detalhes de Implementação

Consulte techspec.md — seções "Arquitetura do Sistema" (componente modificado `internal/tui`, fluxo de dados) e "Considerações Técnicas → Decisões Principais" (estados modais full-width, atividade transiente). Pontos exatos em `tui.go`: campos do Model (75), `WindowSizeMsg` (299), `handleAgentEvent` (396), `handleNestedEvent` (497), `hintBar` (1383), `View` (1442); `main.go:112` para o resume. O cache de glamour invalida por largura no resize — comportamento existente, custo baixo.

## Critérios de Sucesso

- Matriz de visibilidade: sdd nunca mostra sidebar; squad < 100 cols sem sidebar; squad ≥ 100 cols com sidebar
- Resize 80→120→200 com sessão aberta: `lipgloss.Width` da View == terminal, ambos os painéis íntegros, sem overflow
- Render em modo sdd é byte-a-byte idêntico ao atual (zero regressão)
- `EventKickoff` limpa atividade e re-renderiza; `tool_start` aninhado atualiza a atividade da persona no frame seguinte
- Resume em squad reconstrói o painel com personas/status/métricas do snapshot
- `go test ./internal/tui/ -race` verde

## Testes da Tarefa

- [ ] Testes de unidade (`tui_test.go`):
  - Matriz de visibilidade (sdd/squad × larguras 80/100/140/200)
  - Clamp de largura do sidebar (100→24, 140→35, 200→40)
  - Resize 80→120→200: `lipgloss.Width(m.View())` == largura do terminal em cada passo
  - Render sdd byte-a-byte idêntico ao pré-sidebar (teste de regressão com fixture)
  - `EventKickoff` limpa `personaActivity` e dispara re-render
  - Evento aninhado `tool_start` atualiza atividade da persona; persona concluída mantém última ação
  - Mesa injetada via `RestoreMesa` aparece no painel após resume simulado
- [ ] Testes de integração: fluxo Agent→TUI com mock gateway — kickoff popula painel, convocation marca deliberando, `turn_done` aninhado atualiza métricas no frame seguinte
- [ ] Testes E2E: N/A (dogfooding na 6.0)

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` (modificado — View, WindowSizeMsg, handleAgentEvent, handleNestedEvent, hintBar, campos do Model)
- `internal/tui/tui_test.go` (modificado)
- `internal/tui/sidebar.go` (dependência — task 4.0)
- `main.go` (modificado — RestoreMesa no resume)
- `internal/agent/agent.go` (dependência — task 3.0)
