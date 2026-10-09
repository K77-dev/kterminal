# Relatório de Code Review - Sidebar de agentes do squad (Task 4.0)

## Resumo
- Data: 2026-10-09
- Branch: 014-prd-sidebar-squad
- Status: APROVADO
- Arquivos Modificados: 4 (`internal/tui/sidebar.go` novo, `internal/tui/sidebar_test.go` novo, `internal/tui/theme.go`, `internal/tui/tui.go`)
- Linhas Adicionadas: ~426 (146 + 258 novos; +9 theme.go; +13 tui.go)
- Linhas Removidas: 12 (realinhamento do bloco de campos em tui.go)

Escopo estrito da task 4.0. Mudanças de 1.0–3.0 (`internal/squad/`, `internal/agent/`, `internal/session/`, `main_test.go`) presentes no working tree mas fora deste review — já aprovadas em reviews anteriores.

## Veredito sobre os 4 desvios/interpretações declarados

| # | Desvio | Veredito | Validação independente |
|---|--------|----------|------------------------|
| a | "mesa not started" em inglês | **ACEITO** | AGENTS.md manda textos da TUI em inglês; a própria task fixa status em inglês (`waiting`/`deliberating`/`done`). "mesa" é o nome do tipo de domínio (`squad.Mesa`) — preservá-lo mantém a rastreabilidade tipo↔UI. REQ-002 exige estado textual explícito, não um texto específico. |
| b | Mesa vazia (entries vazias) também renderiza "mesa not started" | **ACEITO — necessário** | Verificado em `agent.go:204-206`: `ActivateMode("squad")` chama `initMesa()`, que cria Mesa não-nil vazia. O estado real entre `/mode squad` e o kickoff é entries vazias; tratar só nil renderizaria painel silencioso, violando o critério de aceite do REQ-002 ("Estado de mesa não iniciada é exibido entre a ativação do modo e o kickoff"). Ambos os caminhos testados (`TestSidebarViewMesaNotStarted`). |
| c | Campo `personaActivity` declarado em tui.go nesta task | **ACEITO** | A assinatura da techspec é `sidebarView(width int)` — sem parâmetro de atividade; o campo do Model é o único veículo compatível com a spec. A techspec lista o campo no Model (seção "Modelos de Dados" e "Arquivos relevantes": campos do Model em tui.go). Declarado como armazenamento inerte: leitura de mapa nil é segura em Go, população via `handleNestedEvent`/`EventKickoff` permanece na 5.0 (grep confirma: campo só referenciado em `sidebar.go` e testes). |
| d | Status do maestro derivado de `m.busy` | **ACEITO** | A techspec especifica "identificação, status e modelo" para o maestro mas não a fonte do status. `busy` é o sinal existente de loop ativo; o sidebar só renderiza em modo squad, então `deliberating` quando o maestro executa convocações é semanticamente correto. Reusa os termos canônicos do squad. Testado em ambos os estados e no piso de largura. |

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Zero comentários em `sidebar.go`, `sidebar_test.go` e nos diffs de `theme.go`/`tui.go` |
| Receivers por valor em render | OK | `sidebarWidth`, `sidebarView`, `writeSidebarMaestro`, `writeSidebarEntry`, `sidebarActivityLine` — todos `(m Model)` |
| `strings.Builder` por ponteiro | OK | `writeSidebarLine(b *strings.Builder, ...)`, `writeSidebarMaestro(&b, ...)`; builder local em `sidebarHeaderLine` é idiomático |
| Textos da TUI em inglês | OK | `squad`, `maestro`, `waiting`/`deliberating`/`done`, `mesa not started`, `convocations`, `tokens` (ver desvio a) |
| Truncar por runes com `truncate` existente | OK | Todo conteúdo passa por `truncate` (styles.go:66, runes + "…") **antes** de estilizar — nenhum corte de sequência ANSI |
| Medir com `lipgloss.Width` nos testes | OK | `assertSidebarLineWidth` valida largura exata linha a linha em 3 testes |
| Estilos em `theme.go`, paleta existente | OK | `sidebarPanelStyle` (leftBorder/colBorder/colBgPanel/Padding(0,1)), `sidebarDividerStyle` (colBorder); reuso de `agentLabelStyle`/`disciplineColor`, `titleStyle`, `primaryStyle`, `helpStyle`, `toolStyle`, `warningStyle`, `successStyle` — zero cor nova |
| Sem dependências novas | OK | Só lipgloss/squad já usados |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `sidebarWidth() int` (0 quando oculto) | SIM | `m.agent.Mode() != "squad" || m.width < 100` → 0; proporcional `24 + (W−100)·11/40` com clamp `min(max(w,24),40)` |
| Clamp exato: 100→24, 140→35, 200→40, <100→0, 250→40 | SIM | Matemática conferida: 140→24+440/40=35; 200→51→40 (clamp); 250→65→40 (clamp). Testado com fronteiras extras (99→0) |
| `sidebarView(width int) string`: cabeçalho, maestro no topo, entradas na ordem de convocação, rodapé | SIM | `squad` (título) → maestro (2 linhas) → divisor → entradas na ordem de `mesa.Entries` (= ordem do kickoff) → divisor → rodapé `n/max convocations` + `k/budget tokens` |
| Maestro sem métricas próprias | SIM | Linhas do maestro: `▸ maestro · <status>` + modelo. Sem tokens/custo — validado por contagem de "$" no teste |
| Métricas por persona: modelo, tokens, custo | SIM | `modelo · N tok · $custo` via `formatTokensK`/`formatCost` existentes — mesmos formatadores do chat, valores batem com o transcript por construção |
| Elisão "…" sem quebrar layout | SIM | Truncate-then-style em todos os caminhos; orçamento por segmento no cabeçalho (`nameBudget = width − 2 − 3 − len(status)`) faz o **nome** absorver o corte — o status (indicador de acessibilidade) nunca é truncado. `lipgloss.Width` == width em todas as linhas |
| Mesa nil → estado textual explícito | SIM | `mesa not started` (ver desvio b para entries vazias); sem rodapé (tetos desconhecidos — `0/0` seria ruído) |
| Persona sem atividade → "—" | SIM | Atividade vazia → `—`; métricas vazias → `—` |
| Sidebar somente leitura, sem foco, sem handler | SIM | Funções de render puras; grep confirma ausência total em `Update`/handlers de tecla |
| Sidebar NÃO composto na `View()` nesta task | SIM | `sidebarView`/`sidebarWidth` não referenciados em `tui.go` fora da definição; `View()`, `WindowSizeMsg`, `handleAgentEvent`, `handleNestedEvent`, `hintBar`, `main.go` intocados — tasks 5.0–6.0 preservadas |
| Segurança | N/A | TUI local, sem backend/API/input. Sidebar somente leitura por construção; nenhum dado sensível novo (Mesa não carrega prompts) |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 4.1 `sidebarWidth()` com clamp e 0 quando oculto | COMPLETA | Fórmula + clamp + ocultação por modo/largura; matriz testada |
| 4.2 `sidebarView(width)`: cabeçalho, maestro, entradas, rodapé | COMPLETA | Estrutura completa; maestro sem métricas; rodapé com degradação quando tetos ausentes |
| 4.3 Mesa não iniciada; elisão "…"; "—" sem atividade | COMPLETA | Nil e entries vazias; elisão por runes antes do estilo; "—" para atividade e métricas |
| 4.4 Estilos do sidebar em `theme.go` | COMPLETA | Gutter/borda `colBorder`, fundo `colBgPanel`, reuso de `disciplineColor` |
| 4.5 Testes (clamp, conteúdo, elisão, mesa não iniciada, contraste) | COMPLETA | 9 testes cobrindo 100% da lista mandatória da task + extras |

## Testes
- Total de Testes (novos, sidebar): 9
- Passando: 9 (`-race -count=1`)
- Falhando: 0
- Coverage (statements, `internal/tui`): 86.2% total do pacote; `sidebar.go`: 13/13 funções cobertas — `sidebarWidth`, `writeSidebarMaestro`, `sidebarActivityLine`, `sidebarHeaderLine`, `sidebarStatusStyle`, `sidebarDivider`, `writeSidebarLine`, `padSidebarLine` a 100%; `sidebarView` 94.1%, `writeSidebarEntry` 83.3%, `sidebarMetricsText` 83.3%, `sidebarBudgetConvocations`/`sidebarBudgetTokens` 66.7% (ramos defensivos — ver Problemas #2)

Mapeamento contra a lista mandatória da task:

- Matriz de largura 80/100/140/200/250 → `TestSidebarWidthMatrix` (+ fronteira 99, + `TestSidebarWidthHiddenInSDDMode`: sdd nunca, mesmo a 200 cols)
- Conteúdo com Mesa injetada via `RestoreMesa` → `TestSidebarViewRendersPopulatedMesa`: nome + status textual das 2 personas, modelo/tokens/custo (`5.0k tok`, `$0.0100`), rodapé `2/8 convocations` e `12.4k/200.0k tokens`, atividade `bash(npm test)`, maestro sem métricas (contagem de "$"), largura exata == 40
- Elisão → `TestSidebarViewTruncatesLongContent`: nome e atividade de 200 chars a 24 cols → "…" presente, conteúdo longo ausente, **status sobrevive à elisão do nome**, `lipgloss.Width` == 24 em todas as linhas
- Mesa nil → "mesa not started" → `TestSidebarViewMesaNotStarted`: nil (agente fresco) e Mesa vazia (pós-`ActivateMode("squad")`), ambas com maestro presente e sem rodapé
- Persona sem atividade → "—" → `TestSidebarViewPersonaWithoutActivity` (métricas E atividade)
- Cor nunca única → `TestSidebarStatusAlwaysTextual` (3 status textuais em plain text) + `TestSidebarReusesDisciplineColor` (ANSI truecolor `#9d7cd8` da disciplina E nome+status textuais no mesmo render)
- Extras → `TestSidebarMaestroStatusTracksBusy` (busy→`deliberating`, idle→`waiting`, largura exata a 24 cols com o status mais longo)

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/tui/sidebar.go | 88-94 | `sidebarHeaderLine` não truncaria um `entry.Status` anômalo (ex.: snapshot corrompido com status arbitrário longo): `nameBudget` negativo → nome vira "…" mas o status íntegro estouraria a largura da linha (`padSidebarLine` não corta). Teórico — status canônico tem máx 12 runes e o piso de 24 cols garante `nameBudget ≥ 4`; snapshots são arquivos locais do próprio usuário. | Fallback defensivo: `truncate(status, ...)` como último recurso se `nameBudget` colapsar. Pode ficar para a 5.0. |
| Baixa | internal/tui/sidebar_test.go | — | Ramos defensivos sem teste: degradação do rodapé com tetos ausentes (`sidebarBudgetConvocations`/`sidebarBudgetTokens` 66.7%), `status == ""` → `waiting` em `writeSidebarEntry`, `model == ""` → "—" em `sidebarMetricsText`, guarda `width < 24` de `sidebarView`. Todos fora da lista mandatória da task, mas são edge cases de estados degenerados da Mesa. | Um teste com Mesa sem tetos (`MaxConvocations`/`TokenBudget` zero) e entry sem status/modelo cobriria os 4 ramos de uma vez. Fast-follow ou 5.0. |
| Baixa | internal/tui/sidebar_test.go | 61 | `TestSidebarViewRendersPopulatedMesa` seta `m.currentModel = "glm-5.3"` igual ao model do architect — a asserção `Contains("glm-5.3")` não distingue a linha do maestro da do architect. A contagem de "$" é o que realmente valida o maestro sem métricas, então o teste não perde poder. | Usar model distinto para o maestro (ex.: `glm-5.2`) tornaria a asserção inequívoca. |

## Pontos Positivos
- Truncate-then-style elimina por construção a classe de bug "elisão quebra ANSI", e o orçamento por segmento no cabeçalho faz o nome absorver o corte — o status, indicador de acessibilidade, nunca é a vítima
- Largura exata do painel assertada linha a linha (`lipgloss.Width == width`), não apenas do bloco — pega regressão de padding/overflow no piso de 24 cols
- O desvio b (entries vazias) foi identificado pelo implementador e é a diferença entre atender ou violar o REQ-002 no estado real pré-kickoff — interpretação correta da spec sobre a letra da techspec
- Reuso máximo: `agentLabelStyle`/`disciplineColor`, `formatTokensK`, `formatCost`, `truncate`, `leftBorder` — zero cor nova, zero formato novo, valores batem com o transcript por construção
- Disciplina de escopo exemplar: `View()`/`WindowSizeMsg`/handlers intocados, campo `personaActivity` inerte — tasks 5.0–6.0 chegam sem débito

## Recomendações
- Task 5.0: ao compor com `lipgloss.JoinHorizontal`, tratar o preenchimento de altura (o painel renderiza só as linhas de conteúdo) e derivar `width − sidebarWidth()` para a coluna do chat no `WindowSizeMsg` — como anotado no self-review
- Fast-follow: teste dos ramos defensivos (tetos ausentes, entry sem status/modelo) e truncagem defensiva do status anômalo
- Observação para a 5.0: `m.busy` como fonte do status do maestro significa que o maestro fica `waiting` entre convocações dentro do mesmo turno (loop ativo, `busy` true o turno todo — na prática `deliberating` durante o turno); se o comportamento desejado for distinto, revisitar lá

## Conclusão

**APROVADO.** A implementação atende integralmente os requisitos da task 4.0 (REQ-002/003/004/005) e as decisões da techspec, com os 9 testes da lista mandatória passando sob `-race` e os checks completos verdes (`go build`, `go vet`, `gofmt -l .` vazio, `go test ./internal/tui/ -race`, `go test ./...` — 14 pacotes, zero regressão). Os 4 desvios declarados foram validados independentemente e são todos legítimos: (a) segue a regra de inglês do AGENTS.md; (b) é **necessário** para o REQ-002 dado o comportamento real de `ActivateMode` (Mesa vazia não-nil); (c) é o único veículo compatível com a assinatura `sidebarView(width int)` da spec; (d) preenche lacuna não especificada com o sinal existente correto. Os 3 problemas encontrados são de severidade baixa, não bloqueantes, e registrados com sugestões para a 5.0/fast-follow. Acessibilidade confirmada: nome e status sempre textuais, cor nunca é o único indicador — validado em plain text e no pior caso de elisão.
