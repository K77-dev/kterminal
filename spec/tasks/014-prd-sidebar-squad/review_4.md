# Review — Task 4.0: TUI — render do sidebar (`sidebar.go` + estilos): largura, mesa, maestro, personas, rodapé de orçamento, elisão

**Veredito: APROVADO** (com 4 decisões de interpretação documentadas — ver seção "Notas e divergências")

## Escopo implementado

- `internal/tui/sidebar.go` (novo, 155 linhas): `sidebarWidth() int` (0 quando oculto; proporcional com piso/teto) e `sidebarView(width int) string` (cabeçalho, maestro no topo, entradas por persona na ordem de convocação, rodapé de orçamento, estado de mesa não iniciada), mais helpers puros de composição (`sidebarHeaderLine`, `sidebarStatusStyle`, `sidebarMetricsText`, `sidebarBudgetConvocations`, `sidebarBudgetTokens`, `sidebarDivider`, `writeSidebarLine`, `padSidebarLine`).
- `internal/tui/sidebar_test.go` (novo, 9 testes + 2 helpers).
- `internal/tui/theme.go` (modificado, +9): `sidebarPanelStyle` (gutter `colBorder` + fundo `colBgPanel` + `Padding(0,1)`) e `sidebarDividerStyle` (`colBorder`).
- `internal/tui/tui.go` (modificado, +1 campo): `personaActivity map[string]string` no Model — ver divergência #3.
- Nada além do escopo: `View()` intocada (sidebar NÃO composto nesta task), `WindowSizeMsg`/`handleAgentEvent`/`handleNestedEvent`/`hintBar` intocados, nenhum wiring de resume no `main.go` — tasks 5.0–6.0 preservadas.

## Evidências do diff

### `sidebarWidth()` (REQ-005)

- 0 quando oculto: `m.agent.Mode() != "squad"` (cobre sdd e qualquer modo) OU `m.width < 100`.
- Proporcional entre as âncoras da spec: `24 + (W−100)·11/40` — interpolação exata de (100→24, 140→35) — com clamp `min(max(w,24),40)`.
- Matriz conferida por teste: 80→0, 99→0 (fronteira extra), 100→24, 140→35, 200→40, 250→40.

### `sidebarView(width)` (REQ-002, REQ-003, REQ-004)

- Estrutura: `squad` (titleStyle) → linha do maestro (identificação + status + modelo, sem métricas próprias) → divisor → entradas na ordem de `mesa.Entries` (= ordem de convocação do kickoff) → divisor → rodapé `n/max convocations` e `k/budget tokens`.
- Maestro: status derivado de `m.busy` (`deliberating`/`waiting`), modelo de `m.currentModel` (vazio → "—"). Sem tokens/custo — verificado por teste (contagem de "$" == nº de personas com custo).
- Persona: linha 1 = `▸ <nome> · <status>` com nome na cor da disciplina (`agentLabelStyle` → `disciplineColor` existente) e status textual colorido por estado (`waiting` muted / `deliberating` warning / `done` success); linha 2 = `modelo · N tok · $custo` ("—" quando a persona ainda não tem métricas); linha 3 = atividade corrente de `m.personaActivity[name]` ("—" quando vazia).
- Rodapé: `Convocations/MaxConvocations` e `formatTokensK(Tokens)/formatTokensK(TokenBudget)`; tetos ausentes (≤0) degradem para o consumo absoluto sem denominador.

### Elisão e integridade de layout (REQ-003, REQ-005)

- Todo conteúdo passa por `truncate` (runes, "…" explícito) **antes** de estilizar — truncar texto puro e então aplicar estilo elimina o risco de cortar sequências ANSI.
- Orçamento por segmento na linha de cabeçalho: `nameBudget = contentWidth − 2 − 3 − len(status)` — o **nome absorve o corte primeiro**; o status (indicador de acessibilidade) nunca é truncado. Conferido no pior caso: width 24 + `deliberating` → `▸ mae… · deliberating` com largura exata.
- `padSidebarLine` completa cada linha com espaços até `contentWidth`; o painel (`sidebarPanelStyle`: borda 1 + padding 2) renderiza **todas** as linhas com `lipgloss.Width == width` — assertado linha a linha em 3 testes.
- Visual conferido no piso (24 cols): nomes e métricas elidem com "…", status/rodapé íntegros, nenhuma linha estoura.

### Estados

- Mesa nil **ou** sem entries → `mesa not started` textual (nunca vazio silencioso), sem rodapé (tetos desconhecidos). Ambos os caminhos testados: agente fresco (`Mesa() == nil`) e agente squad pós-`ActivateMode` (Mesa vazia inicializada — o estado real pré-kickoff).
- Sidebar somente leitura: funções de render puras, sem foco, sem handler de tecla, sem estado mutável.

## Cobertura de testes (9 novos no pacote tui)

Mapeamento contra a lista mandatória da task:

- **Matriz de largura** → `TestSidebarWidthMatrix` (80/99/100/140/200/250) + `TestSidebarWidthHiddenInSDDMode` (sdd nunca, mesmo a 200 cols).
- **Conteúdo com Mesa injetada via `RestoreMesa`** → `TestSidebarViewRendersPopulatedMesa`: nome + status textual das duas personas, modelo/tokens/custo por persona (`5.0k tok`, `$0.0100`), rodapé `2/8 convocations` e `12.4k/200.0k tokens`, atividade `bash(npm test)`, "—" para persona sem atividade, maestro sem métricas próprias, `lipgloss.Width` de cada linha == 40.
- **Elisão** → `TestSidebarViewTruncatesLongContent`: nome e atividade de 200 chars a 24 cols → "…" presente, conteúdo longo ausente, status sobrevive, largura exata em todas as linhas.
- **Mesa nil** → `TestSidebarViewMesaNotStarted`: nil → "mesa not started" + maestro presente + sem rodapé; Mesa vazia pós-ativação do modo → mesmo estado.
- **Persona sem atividade** → `TestSidebarViewPersonaWithoutActivity`: bloco da persona renderiza "—" para métricas e atividade.
- **Cor nunca única** → `TestSidebarStatusAlwaysTextual` (os 3 status textuais em plain text) + `TestSidebarReusesDisciplineColor` (ANSI truecolor `#9d7cd8` da disciplina `architecture` sobre o nome **e** nome+status textuais no mesmo render).
- **Extras** → `TestSidebarMaestroStatusTracksBusy` (busy→`deliberating`, idle→`waiting`, e largura exata a 24 cols com o status mais longo).

## Checks executados

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ ok (um panic transitório do linker de cache na primeira execução, reprodutível não; rebuild limpo) |
| `go vet ./...` | ✅ ok |
| `gofmt -l .` | ✅ vazio |
| `go test ./internal/tui/ -race -count=1` | ✅ ok (todos os testes do pacote, 9 novos) |
| `go test ./... -count=1` | ✅ ok (14 pacotes, zero regressão) |

## Notas e divergências documentadas

1. **Estado de mesa não iniciada em inglês ("mesa not started")**: a task descreve o estado como "mesa não iniciada" (prosa pt-BR), mas o AGENTS.md manda textos de TUI em inglês e a própria task fixa status em inglês. Renderizado como `mesa not started` — o termo de domínio "mesa" é preservado (é o nome do tipo `squad.Mesa`).
2. **Condição de mesa não iniciada = nil OU entries vazias**: a techspec cita "`Mesa == nil`", mas `ActivateMode("squad")` inicializa uma Mesa vazia (task 3.0, `initMesa`) — o estado real entre `/mode squad` e o kickoff é Mesa não-nil sem entries. Ambos tratados igualmente; sem isso, o painel pré-kickoff renderizaria vazio silencioso, violando o REQ-002.
3. **Campo `personaActivity` adicionado ao Model agora**: a task diz que a atividade "chega por parâmetro/campo do Model" e que o mapa é "mantido na 5.0". A assinatura da techspec é `sidebarView(width int)` (sem parâmetro de atividade), então o campo é o único veículo — declarado nesta task como armazenamento inerte (leitura de mapa nil é segura), populado apenas nos testes; a manutenção via `handleNestedEvent`/`EventKickoff` permanece 100% na 5.0.
4. **Status do maestro derivado de `m.busy`**: a techspec especifica "identificação, status, modelo" para o maestro mas não a fonte do status. `busy` é o sinal existente de loop ativo → `deliberating`/`waiting`, reusando os mesmos termos canônicos. Rodapé omitido no estado não iniciada (tetos desconhecidos — renderizar `0/0` seria ruído).

Decisões de interpretação (sem divergência de spec):

- Fórmula de largura `24 + (W−100)·11/40` com clamp: única reta que satisfaz simultaneamente 100→24 e 140→35 com piso/teto 24/40; 200/250 caem no teto conforme a matriz mandatória.
- Métricas por persona usam `formatTokensK`/`formatCost` existentes — mesmos formatadores do chat/hint bar, valores batem com o transcript por construção (a Mesa espelha `turnTokens` desde a 3.0).
- `sidebarView` recusa `width < 24` (retorna ""): `sidebarWidth()` só produz 0 ou [24,40]; o piso garante os orçamentos de segmento do cabeçalho (status mais longo = 12, contentWidth mínimo = 21).
- Divisores `─` em `colBorder` dão a separação visual "borda ou gutter" do PRD; o gutter esquerdo do painel usa o mesmo `leftBorder`/`colBorder` dos boxes existentes do chat.

## Pontos positivos

- Truncar plain-then-style elimina por construção a classe de bug "elisão quebra ANSI" — e o orçamento por segmento garante que o indicador de acessibilidade (status textual) nunca é a vítima do corte.
- Largura exata do painel é assertada linha a linha (`lipgloss.Width == width`), não apenas do bloco — pega qualquer regressão de padding/overflow no piso de 24 cols.
- Reuso máximo da paleta: `agentLabelStyle`/`disciplineColor`, `titleStyle`, `primaryStyle`, `helpStyle`, `toolStyle`, `warningStyle`, `successStyle`, `formatTokensK`, `formatCost`, `truncate` — zero nova cor, zero novo formato.
- Zero comentários; receivers por valor em todo render; `strings.Builder` por ponteiro nos helpers de escrita.

## Observação para tasks futuras

- Task 5.0: compor com `lipgloss.JoinHorizontal` exige altura uniforme — o painel renderiza apenas as linhas de conteúdo (sem padding vertical); a composição deve lidar com o preenchimento de altura (pad com bg) do lado de lá. `sidebarView` já garante largura exata por linha, então o `WindowSizeMsg` só precisa derivar `width − sidebarWidth()` para a coluna do chat. O campo `personaActivity` está pronto para ser populado em `handleNestedEvent` e limpo no `EventKickoff`.
