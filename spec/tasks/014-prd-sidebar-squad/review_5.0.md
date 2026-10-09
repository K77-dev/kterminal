# Relatório de Code Review — Sidebar de agentes do squad (Task 5.0)

## Resumo

- Data: 2026-10-09
- Branch: `014-prd-sidebar-squad`
- Status: **APROVADO**
- Reviewer: kspec-review-runner (revisão independente — o self-review do implementador em `review_5.md` foi validado item a item, não aceito por referência)
- Arquivos modificados (escopo da task): 4
- Linhas adicionadas: 684 (tui_test.go +634, tui.go +48, commands.go +1, main.go +1)
- Linhas removidas: 16 (todas em tui.go — extração de `chatView`/`fitChatColumn`)

## Conformidade com Rules

Padrões Go do código-base (AGENTS.md) — as rules de `.agents/rules/` são TS/Java e não se aplicam, conforme registrado na techspec:

| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário nos diffs dos 4 arquivos |
| Receivers por valor em `View`/render | OK | `View`, `chatView`, `hintBar`, `chatWidth`, `sidebarView` por valor; mutadores `fitChatColumn`/`handleNestedEvent` por ponteiro, no padrão dos `fit*` existentes |
| Eventos via canal existente, nenhum event-kind novo | OK | Apenas novos *cases* para `EventKickoff` (tui.go:414) e uso do campo aditivo `e.Agent` em `EventToolStart` aninhado (tui.go:501) |
| Textos da TUI em inglês | OK | `mesa not started`, `convocations`, `tokens`; "mensagens" no hintBar (tui.go:1430) é pré-existente, fora do escopo |
| `lipgloss.Width` nas asserções de largura | OK | Todos os testes de largura usam `lipgloss.Width` |
| `strings.Builder` por ponteiro | OK | `chatView`, `sidebarView` e fixture usam `&b` |
| Tools registradas no construtor / sem deps novas | OK | Nenhuma dependência nova; nenhum toque no registry |

## Aderência à TechSpec

| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `View()` compõe chat + sidebar via `JoinHorizontal` somente em squad ≥ 100 cols | SIM | tui.go:1465-1483; `sidebarWidth()` retorna 0 em sdd e < 100 cols (sidebar.go:26) |
| `WindowSizeMsg` deriva larguras da coluna de chat | SIM | `fitChatColumn()` + `viewport.New(maxInt(m.chatWidth()-2*chatInset, 1))` (tui.go:300-305); em sdd os valores são idênticos aos pré-sidebar (verificado contra HEAD) |
| `hintBar` na largura do chat | SIM | tui.go:1406,1415,1451 — `chatWidth()` colapsa para `m.width` sem sidebar |
| `handleAgentEvent` trata `EventKickoff` + limpa atividade | SIM | tui.go:414-416 — `personaActivity = nil` + `refreshContent()`; Mesa lida via getter no render seguinte |
| `handleNestedEvent` mantém mapa transiente `personaActivity` | SIM | tui.go:501-506 — `tool(args)` com `firstLine(args, 100)`; mapa nunca limpo em `turn_done` (persona concluída mantém última ação); campo da TUI, fora do snapshot |
| Atividade transiente; persona sem tools mostra "—" | SIM | Render da 4.0 (`sidebarActivityLine`); `tool_start` aninhado sem `Agent` não toca o mapa (testado) |
| `main.go`: `RestoreMesa(resumed.Mesa)` após restaurar o modo | SIM | main.go:118, após `ActivateMode` (108/113); `RestoreMesa(nil)` é no-op (agent.go:276) → snapshot sem mesa exibe "mesa not started" |
| Estados modais full-width; sidebar reaparece ao voltar | SIM | Switch modal antes da composição (tui.go:1469-1476); wizard ask continua com largura total do terminal (tui.go:306-307) |
| Nenhum handler de tecla novo; sidebar não captura input | SIM | Sidebar é render puro; nenhum `KeyMsg` novo no diff |

## Veredito sobre os 5 desvios declarados (validação independente)

| # | Desvio | Veredito | Análise independente |
|---|--------|----------|----------------------|
| a | `lipgloss.Width(View)` == `terminal − chatInset`, não == terminal | **JUSTIFICADO** | Verifiquei contra `HEAD` (pré-sidebar): o hintBar já renderizava `chatInset + left + gap + right` com `gap = m.width − 2·chatInset − leftW − rightW`, ou seja, largura final `m.width − chatInset`; o viewport idem (`(m.width − 2·chatInset) + chatInset`). O critério literal da task ("== largura do terminal") **nunca foi verdadeiro nem no código pré-sidebar** — satisfazê-lo exigiria mudar o render base, violando o requisito mais forte do PRD (REQ-001: sdd "sem qualquer perda de largura no chat"; zero regressão byte-a-byte). O PRD (fonte da verdade) exige apenas "painéis íntegros, sem overflow" (REQ-005). O join composto fecha exatamente em `terminal − chatInset` (chat `terminal − sidebar − chatInset` + painel `sidebar`), consistente em todos os frames — confirmado nos testes |
| b | `selectMode` (commands.go) chama `fitChatColumn()` — arquivo fora da lista da task | **JUSTIFICADO** | Sem isso, `/mode` sem resize deixa `vp`/`input`/`mdWidth` nas larguras antigas: squad a 200 cols renderizaria chat 196 + sidebar 40 = 236 → overflow, violando REQ-001 ("aparece e some automaticamente com o `/mode`, sem restart") e REQ-005 ("sem overflow"). Grep confirmou que `selectMode` é o **único** caminho de troca de modo na TUI. 1 linha, coberta por `TestModeSwitchTogglesSidebarWithoutRestart` (verifica `vp.Width` nos dois sentidos da troca) |
| c | Teste de integração com gates (padrão `callGate`) | **JUSTIFICADO** | O padrão existe em agent_test.go:4677-4700 (precedente da 3.0). Asserções sobre a Mesa ao vivo seriam racy por construção (o agent avança enquanto o teste renderiza); os gates congelam o agent nos pontos de asserção. As asserções mid-convocation respeitam a defasagem documentada na techspec ("tokens do sub em voo só aterrissam no retorno de `runSubagent`"). Verde com `-race` e `-count=3` |
| d | Altura do sidebar sem clamp no join | **ACEITÁVEL — fast-follow** | PRD REQ-005 trata exclusivamente de **largura** ("largura proporcional", "piso e teto", "80 a 200+ **colunas**"); a techspec não menciona altura em nenhum requisito. `JoinHorizontal(lipgloss.Top)` preserva o alinhamento; o caso patológico (7+ personas em terminal muito baixo → painel mais alto que o chat → scroll cosmético) não viola nenhum critério de aceite. Registrado como recomendação não bloqueante |
| e | `EventKickoff` sem bloco de chat | **JUSTIFICADO** | A task pede apenas re-render + limpeza de atividade; o PRD exige "chat permanece inalterado"; o kickoff já é visível no chat pela linha `● squad_kickoff(...)` (EventToolStart). `TestKickoffClearsPersonaActivity` verifica que nenhum bloco é adicionado |

## Tasks Verificadas

| Subtask | Status | Observações |
|---------|--------|-------------|
| 5.1 Compor `View()` com `JoinHorizontal` condicionado | COMPLETA | sdd e squad < 100 cols retornam o chat puro em largura total |
| 5.2 Derivar larguras no `WindowSizeMsg` + `hintBar` no chat | COMPLETA | `fitChatColumn` cobre também o `/mode` (desvio b) |
| 5.3 Tratar `EventKickoff` | COMPLETA | Limpa atividade + re-render; sem bloco de chat (desvio e) |
| 5.4 `personaActivity` em `handleNestedEvent` | COMPLETA | Métricas ao vivo vêm da Mesa (drain chama `ObservePersona` antes do emit) — sem acumulador duplicado |
| 5.5 Wiring de resume no `main.go` | COMPLETA | Ordem correta: `ActivateMode` → `RestoreMesa`; nil é no-op |
| 5.6 Testes | COMPLETA | 10 testes novos cobrindo toda a matriz mandatória (abaixo) |

## Testes

- Total de testes novos (task 5.0): 10 — todos passando
- Total no pacote tui: 118 passando; suíte completa: 14 pacotes ok
- Coverage (pacote tui): 88.3%
- `go test ./internal/tui/ -race -count=1`: **ok** (6.2s); `-count=3`: **ok** (14.3s — checagem de flakiness do teste com gates)
- `go test ./... -count=1`: **ok** (zero regressão)

Mapeamento contra a lista mandatória da task:

| Exigência da task | Teste | Avaliação |
|-------------------|-------|-----------|
| Matriz de visibilidade (sdd/squad × 80/100/140/200) | `TestSidebarVisibilityMatrix` | Suficiente — sdd nunca renderiza "maestro"; squad < 100 sem sidebar; ≥ 100 com sidebar + "mesa not started"; largura conferida em todas as células |
| Clamp (100→24, 140→35, 200→40) | `TestSidebarClampDerivesChatColumn` | Suficiente — fórmula conferida independentemente (200 → 51 bruto → clamp 40; 120 → 29 exercitado no resize) |
| Resize 80→120→200, largura da View, painéis íntegros | `TestSidebarResizeKeepsPanelsIntact` | Suficiente — largura por linha ≤ terminal, conteúdo do chat e persona preservados, `vp.Width` derivado a cada passo |
| Render sdd byte-a-byte (fixture) | `TestSDDViewByteIdenticalToPreSidebar` | Suficiente — fixture independente (`legacyChatView`/`legacyHintBar` com as fórmulas originais em `m.width`) comparado byte-a-byte em 4 cenários: sdd busy com todos os segmentos do hint bar, sdd idle, sdd vazio a 80 cols e squad a 80 cols (prova que squad estreito também é byte-idêntico) |
| `EventKickoff` limpa atividade + re-render | `TestKickoffClearsPersonaActivity` | Suficiente — mapa zerado, nenhum bloco de chat, painel do frame seguinte com personas do kickoff, atividade velha ausente |
| `tool_start` aninhado atualiza atividade; concluída mantém última | `TestNestedToolStartUpdatesPersonaActivity` | Suficiente — mapa por persona, atividade no painel e em 2 cópias no view, persistência pós-`turn_done`, `tool_start` anônimo (depth 2) ignorado |
| Mesa via `RestoreMesa` após resume simulado | `TestRestoreMesaRebuildsPanelAfterResume` | Suficiente — wiring na ordem do `main.go`; persona/status/métricas/rodapé do snapshot; sem `RestoreMesa` → "mesa not started" |
| Integração Agent→TUI com mock gateway | `TestSquadFlowSidebarIntegration` | Suficiente — fluxo real ponta a ponta (kickoff → convocation → convergência) com gates; painel `waiting`/`0/4`/`15/100.0k` pós-kickoff, `deliberating`/`1/4`/`30/100.0k` mid-convocation, `12 tok` + custo no frame seguinte ao `turn_done` aninhado, `done`/`1/4`/`63/100.0k` ao fim; largura do view conferida em **todos** os frames |
| /mode sem restart; modais full-width (REQ-001) | `TestModeSwitchTogglesSidebarWithoutRestart`, `TestModalsStayFullScreenInSquad` | Suficiente — popup real via teclas nos dois sentidos; confirm/ask/config full-width com sidebar reaparecendo ao voltar |

Edge cases cobertos: squad abaixo do threshold (99/100), largura não redonda de sidebar (29 a 120 cols), `tool_start` aninhado sem `Agent`, snapshot sem mesa, persona sem tools. Testes verificam comportamento real (largura medida com `lipgloss.Width`, conteúdo textual pós-`stripANSI`), não apenas ausência de erro.

## Verificação de Segurança

**N/A — funcionalidade integralmente local à TUI.** Sem backend/API/endpoints: o sidebar é uma string renderizada sem foco e sem handler de tecla (impossível capturar input); nenhum dado sensível novo (a Mesa carrega apenas nomes de persona, status, modelo, tokens e custo — dados que já constam no JSONL; sem prompts ou conteúdo de mensagens); nenhuma tool nova; sem secrets. Consistente com a seção de segurança da techspec.

## Problemas Encontrados

| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/tui/tui.go | 1482 | Altura do sidebar sem clamp: com 7+ personas em terminal muito baixo o painel excede a altura do chat e o view total excede a altura do terminal (scroll cosmético). Sem requisito de altura no PRD/techspec — desvio (d) | Fast-follow: clamp de linhas do painel por `m.viewportHeight()` ou truncar entradas com indicador de continuação |
| Baixa | internal/tui/tui_test.go | 2585 | Fixture `legacyChatView`/`legacyHintBar` duplica ~70 linhas do render pré-sidebar. É a técnica correta para a regressão byte-a-byte, mas o fixture deve permanecer **congelado** em mudanças futuras do `chatView` — é o ponto da comparação | Manter o fixture intocado em PRs futuros; se o layout base do chat mudar por nova feature, atualizar o fixture deliberadamente na mesma PR |
| Informativo | internal/tui/tui.go | 1430 | "resumed · %d mensagens" em pt-BR no hintBar — pré-existente (fora do escopo da task), replicado no fixture | Correção opportunista em task futura de TUI |

Nenhum problema de severidade Alta ou Média.

## Pontos Positivos

- **Zero regressão por construção**: toda derivação de largura passa por `chatWidth()`, que colapsa para `m.width` sem sidebar; o fixture byte-a-byte prende isso em 4 cenários, incluindo squad estreito
- **Fonte única de verdade respeitada**: a TUI mantém apenas o mapa efêmero de atividade; status e métricas vêm da Mesa do agent via getter — sem acumuladores duplicados (decisão #1 da techspec)
- **`fitChatColumn` idempotente e conservador**: guarda `m.vp.Width != vw` evita recriar o viewport no `/mode` (preserva YOffset), e a extração manteve exatamente as guards antigas (`> 6`, `> 0`)
- **Teste de integração determinístico**: gates congelam o agent nos pontos de asserção e o teste valida o fluxo real ponta a ponta, incluindo a defasagem mid-convocation documentada na techspec
- **Ordem de resume correta e à prova de nil**: `ActivateMode` inicializa Mesa vazia; `RestoreMesa` substitui por clone; `nil` é no-op → "mesa not started"

## Recomendações

- Fast-follow: clamp de altura do sidebar (problema Baixa acima) — anotar na task 6.0 (dogfooding) para exercitar resize vertical com mesa cheia
- O dogfooding da 6.0 deve cobrir: resize ao vivo 80/120/200, `/mode` nos dois sentidos com mesa ativa, `--continue` em sessão squad e conferência sidebar × transcript JSONL

## Checks Executados

| Check | Resultado |
|-------|-----------|
| `go build ./...` | ✅ ok |
| `go vet ./...` | ✅ ok |
| `gofmt -l .` | ✅ vazio |
| `go test ./internal/tui/ -race -count=1` | ✅ ok (118 testes; 10 novos da task) |
| `go test ./internal/tui/ -race -count=3` | ✅ ok (flakiness) |
| `go test ./... -count=1` | ✅ ok (14 pacotes) |

## Conclusão

**APROVADO.** A implementação atende integralmente os requisitos REQ-001/003/005/006 no escopo da task 5.0: composição condicionada correta, derivação de larguras sem regressão (fixture byte-a-byte contra réplica independente do render pré-sidebar), `EventKickoff` e atividade transiente conforme spec, resume wired na ordem certa, modais full-width e nenhum input capturado. Os 5 desvios declarados foram validados independentemente — todos justificados: o (a) foi verificado contra o código pré-sidebar em `HEAD` (o critério literal da task nunca foi satisfeito nem antes da feature; a interpretação adotada é a única compatível com o requisito mais forte do PRD), o (b) é exigência direta de REQ-001+REQ-005 com 1 linha e teste dedicado, o (c) segue precedente estabelecido da 3.0 e elimina race real, o (d) não tem requisito correspondente no PRD/techspec (fast-follow recomendado) e o (e) presponde "chat permanece inalterado". Testes passam com `-race` e `-count=3`; cobertura 88.3% no pacote; nenhum problema bloqueante. Os dois itens de severidade Baixa são melhorias não bloqueantes, registrados para fast-follow.
