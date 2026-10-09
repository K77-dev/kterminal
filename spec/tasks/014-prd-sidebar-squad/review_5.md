# Review — Task 5.0: TUI — integração do sidebar na `View()`, larguras no resize, eventos de persona, atividade transiente e resume no `main.go`

**Veredito: APROVADO** (com 5 decisões de interpretação documentadas — ver "Notas e divergências")

## Escopo implementado

- `internal/tui/tui.go` (modificado): `View()` compõe chat + sidebar com `lipgloss.JoinHorizontal(lipgloss.Top, ...)` somente em modo squad com terminal ≥ 100 cols; corpo do chat extraído para `chatView()` (receiver por valor); `WindowSizeMsg` deriva largura do chat via `chatWidth()` + `fitChatColumn()`; `hintBar` renderiza na largura do chat; `handleAgentEvent` trata `EventKickoff` (limpa `personaActivity` + re-render); `handleNestedEvent` mantém o mapa transiente `personaActivity` a partir de `tool_start` aninhado com `Agent` identificado.
- `internal/tui/commands.go` (modificado, +1 linha): `selectMode` chama `fitChatColumn()` após `ActivateMode` — ver divergência #2.
- `main.go` (modificado, +1 linha): `ag.RestoreMesa(resumed.Mesa)` após o bloco de restauração do modo.
- `internal/tui/tui_test.go` (modificado, +634 linhas): 10 testes novos + 6 helpers.
- Nada além do escopo: nenhum handler de tecla novo, nenhum event-kind novo, nenhuma tecla existente alterada de comportamento, popups/wizards intocados, nada da task 6.0 (E2E/dogfooding) antecipado.

## Evidências do diff

### `View()` — composição condicionada (REQ-001, REQ-005)

- `View()` mantém o switch modal intacto (config/confirm/ask retornam full-width antes de qualquer composição) e, no estado chat: `chat := m.chatView(); width := m.sidebarWidth(); if width <= 0 { return chat }; return lipgloss.JoinHorizontal(lipgloss.Top, chat, m.sidebarView(width))`.
- `chatView()` é byte-a-byte o corpo pré-sidebar da `View()` — provado por fixture de regressão (ver testes).
- Em sdd e em squad < 100 cols, `sidebarWidth() == 0` → retorna o chat puro em largura total, idêntico ao pré-sidebar.

### Larguras derivadas (REQ-005)

- `chatWidth() = m.width − sidebarWidth()`; `fitChatColumn()` (pointer receiver, padrão dos `fit*` existentes) deriva `mdWidth`, largura do input e `vp.Width` da coluna do chat — chamado tanto no `WindowSizeMsg` quanto no `selectMode`.
- `WindowSizeMsg` recria o viewport com `maxInt(m.chatWidth()−2·chatInset, 1)`; o wizard ask continua recebendo a largura total do terminal (estado modal full-width, decisão da techspec).
- `hintBar` usa `chatWidth()` para o truncamento do cwd e para o gap — em sdd `chatWidth() == m.width`, logo byte-a-byte idêntico.

### `EventKickoff` (REQ-002, REQ-003)

- Case novo em `handleAgentEvent`: `m.personaActivity = nil; m.refreshContent()`. A Mesa reconstruída pelo `RegisterKickoff` (task 3.0) é lida via getter no render seguinte; nenhum bloco de chat é adicionado (o evento era ignorado antes e o kickoff já aparece no chat via linha `● squad_kickoff(...)` da tool).

### Atividade transiente (REQ-003)

- `handleNestedEvent`, case `EventToolStart` com `e.Agent != ""`: `personaActivity[agent] = "tool(args)"` com `firstLine(args, 100)` — mesmo formato da linha de tool do chat; a elisão à largura do painel é o `truncate` do render (task 4.0).
- Persona concluída mantém a última ação: o mapa nunca é limpo em `turn_done` aninhado — só no `EventKickoff` (nova mesa) — e não persiste no snapshot (campo da TUI, fora de `session.Snapshot`).
- Persona sem tools → atividade vazia → "—" no painel (render da 4.0); `tool_start` aninhado sem `Agent` (subagente anônimo) não toca o mapa.
- `TurnTokens`/`SessionCost` aninhados (task 3.0) alimentam o rodapé ao vivo por construção: o drain do agent chama `ObservePersona` antes do `emit`, então o frame seguinte ao evento já carrega modelo/tokens/custo da persona — a TUI não duplica acumuladores (decisão #1 da techspec).

### Resume (REQ-006)

- `main.go`: `ag.RestoreMesa(resumed.Mesa)` logo após o bloco `ActivateMode(resumed.Mode)` — a ordem importa: `ActivateMode("squad")` inicializa Mesa vazia e o `RestoreMesa` a substitui pelo clone do snapshot; snapshot sem `mesa` → `RestoreMesa(nil)` é no-op → painel "mesa not started". Snapshot com `mode=sdd` e Mesa populada também restaura (mesa preservada ao voltar para squad, alinhado ao fluxo da techspec).

## Cobertura de testes (10 novos no pacote tui)

Mapeamento contra a lista mandatória da task:

- **Matriz de visibilidade (sdd/squad × 80/100/140/200)** → `TestSidebarVisibilityMatrix`: sdd nunca renderiza "maestro" em nenhuma largura; squad < 100 sem sidebar; squad ≥ 100 com sidebar + "mesa not started"; largura do view conferida em todas as células.
- **Clamp (100→24, 140→35, 200→40)** → `TestSidebarClampDerivesChatColumn`: valores do clamp + derivação da coluna do chat (`vp.Width == terminal − sidebar − 2·chatInset`) + largura do view composto.
- **Resize 80→120→200 com sessão aberta** → `TestSidebarResizeKeepsPanelsIntact`: conteúdo no chat (user block + tool line) e Mesa com persona deliberando; a cada passo `lipgloss.Width(m.View()) == width − chatInset`, nenhuma linha excede o terminal, ambos os painéis íntegros (120 exercita largura de sidebar não redonda: 29).
- **Render sdd byte-a-byte idêntico ao pré-sidebar (fixture)** → `TestSDDViewByteIdenticalToPreSidebar`: réplica in-test da composição pré-sidebar (`legacyChatView` + `legacyHintBar` com `m.width`, fórmulas originais) comparada byte-a-byte contra `m.View()` em 4 cenários: sdd busy com todos os segmentos do hint bar (spinner+subagent, copyFlash, scrolled, pinned, modelo, custo, TPS, resumed, warning, chips, mode popup), sdd idle, sdd vazio a 80 cols, e squad a 80 cols (prova que squad estreito também é byte-idêntico — chat em largura total).
- **`EventKickoff` limpa atividade e re-renderiza** → `TestKickoffClearsPersonaActivity`: mapa populado é zerado, nenhum bloco de chat adicionado, painel do frame seguinte mostra as personas do kickoff + `0/4 convocations`, atividade velha ausente.
- **`tool_start` aninhado atualiza atividade; persona concluída mantém última ação** → `TestNestedToolStartUpdatesPersonaActivity`: mapa por persona, atividade renderizada no painel e em 2 cópias no view completo (chat + sidebar), persona que termina de deliberar mantém a última ação após `turn_done` aninhado, `tool_start` anônimo (depth 2, sem Agent) não altera o mapa.
- **Mesa via `RestoreMesa` após resume simulado** → `TestRestoreMesaRebuildsPanelAfterResume`: wiring na ordem do `main.go` (`ActivateMode("squad")` → `RestoreMesa` → `New` → resize); painel com persona/status/métricas/rodapé do snapshot; agente squad sem `RestoreMesa` → "mesa not started".
- **Integração Agent→TUI com mock gateway** → `TestSquadFlowSidebarIntegration`: fluxo real (kickoff → convocation → convergência) com gateway sequenciado e **gates** nas calls 1 e 2 (padrão `callGate` do agent_test) para congelar o agent nos pontos de asserção: painel populado com `waiting`/`0/4`/`15/100.0k` logo após o kickoff; `architect · deliberating`/`1/4`/`30/100.0k` durante a convocation (rota aninhada); `12 tok` + custo da persona no frame seguinte ao `turn_done` aninhado; `architect · done`/`1/4`/`63/100.0k` ao fim do turno; largura do view == 200−chatInset em **todos** os frames; persona sem tools deixa o mapa vazio.
- **Extras (REQ-001/REQ-007 da task)** → `TestModeSwitchTogglesSidebarWithoutRestart` (/mode popup real via teclas: sidebar some, chat volta à largura total, reaparece, sem restart) e `TestModalsStayFullScreenInSquad` (confirm/ask/config full-width sem sidebar; sidebar reaparece ao voltar ao chat nos três casos).

## Checks executados

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ ok |
| `go vet ./...` | ✅ ok |
| `gofmt -l .` | ✅ vazio |
| `go test ./internal/tui/ -race -count=1` | ✅ ok (todos os testes do pacote, 10 novos; também rodado com `-count=3` para flakiness) |
| `go test ./... -count=1` | ✅ ok (14 pacotes, zero regressão) |

## Notas e divergências documentadas

1. **Largura do view == `terminal − chatInset`, não == terminal**: a task pede "`lipgloss.Width(m.View())` == largura do terminal", mas o chat pré-sidebar já renderiza `terminal − chatInset` de largura (o gap do `hintBar` subtrai `2·chatInset` e a linha recebe só 1 inset à esquerda — gutter direito de 2 cols pré-existente). Igualar literalmente ao terminal exigiria mudar o render base do chat, violando o requisito mais forte "sdd byte-a-byte idêntico". O PRD (fonte da verdade) exige apenas "painéis íntegros, sem overflow" (REQ-005). Asserções: view == `terminal − chatInset` (sdd puro, squad estreito e squad composto — o join fecha exatamente nessa mesma largura) e nenhuma linha > terminal.
2. **`selectMode` (commands.go) chama `fitChatColumn()`**: o arquivo não está na lista de "arquivos relevantes" da task, mas sem isso a troca de modo via `/mode` sem resize deixa o viewport na largura antiga — squad a 200 cols estouraria (chat 196 + sidebar 40 = 236). É exigência direta do REQ-001 ("aparece e some automaticamente com o `/mode`, sem restart") combinado com "sem overflow" do REQ-005; 1 linha, coberta por `TestModeSwitchTogglesSidebarWithoutRestart`.
3. **Teste de integração com gates**: asserções sobre a Mesa ao vivo são inerentemente racy (o agent continua avançando enquanto o teste renderiza — constatado na primeira versão do teste, que falhou com a Mesa já em `deliberating` no frame do kickoff). Adotado o padrão `callGate` do `agent_test.go`: gateway bloqueia nas calls 1 e 2, congelando o agent nos pontos de asserção. O rodapé mid-convocation assertado respeita a defasagem documentada na techspec ("tokens do sub em voo só aterrissam no retorno de `runSubagent`"): 30/100.0k durante a convocation, 63/100.0k ao fim.
4. **Altura no join**: `JoinHorizontal` preenche o bloco mais curto com linhas em branco — o bg do painel termina onde o conteúdo termina (comportamento tipo opencode). Não há clamp de altura do sidebar: em terminais muito baixos com 7+ personas o painel pode superar a altura do chat e rolar a tela. Fora do escopo da task/techspec (nenhum requisito de altura); anotado para eventual fast-follow.
5. **`EventKickoff` não adiciona bloco de chat**: o `Text` do evento (resumo do kickoff) permanece não renderizado no chat — o kickoff já é visível pela linha da tool `● squad_kickoff(...)` e o painel é a superfície do estado da mesa. Mudar o chat violaria "chat permanece inalterado" do PRD.

## Pontos positivos

- Zero regressão por construção: toda a derivação de largura passa por `chatWidth()`, que colapsa para `m.width` quando o sidebar está oculto — o fixture byte-a-byte (réplica independente da composição pré-sidebar) prende isso em 4 cenários.
- Atividade transiente sem acumulador duplicado: a TUI só mantém o mapa efêmero; métricas e status vêm da Mesa do agent (fonte única da techspec), lida via getter a cada render.
- O teste de integração valida o fluxo ponta a ponta real (tecla enter → agent → eventos → frames) com determinismo via gates, incluindo a defasagem mid-convocation documentada.
- Sem comentários; receivers por valor em `View`/`chatView`/`hintBar`/render; mutadores com pointer receiver no padrão dos `fit*` existentes; `lipgloss.Width` em todas as asserções de largura.

## Observação para a task 6.0

- O dogfooding deve exercitar resize ao vivo (80/120/200), `/mode` nos dois sentidos com mesa ativa e `--continue` em sessão squad — os pontos de integração estão todos cobertos por testes, o E2E valida o comportamento em terminal real (incluindo a nota #4 sobre altura com muitas personas).
