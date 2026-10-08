# Relatório de Code Review — 003 Gestão de janela de contexto (compação) — Task 5.0 Verificação final integrada

## Resumo
- Data: 2026-10-04
- Branch: 002-010-prds-kterminal
- Status: APROVADO
- Arquivos Modificados (working tree completo, features 001+002+003): 11
- Linhas Adicionadas: 2183
- Linhas Removidas: 96
- Escopo da feature 003: `internal/agent/agent.go` (estimativa, compação, truncamento, evento), `internal/session/session.go` (TokensBefore/TokensAfter), `internal/tui/tui.go` + `internal/tui/theme.go` (linha ⚡ muted), `internal/agent/agent_test.go` (16 testes de compação/estimativa/truncamento), `internal/tui/tui_test.go` (2 testes de renderização)

## Verificação Encadeada (subtask 5.1)

`go build ./... && go vet ./... && gofmt -l . && go test ./... -count=1` — execução única encadeada, exit 0:

| Check | Resultado |
|-------|-----------|
| `go build ./...` | OK |
| `go vet ./...` | OK |
| `gofmt -l .` | OK (saída vazia) |
| `go test ./... -count=1` | OK — 66 testes passando, 0 falhando (agent, llm, tools, tui) |

## Checklist de Critérios de Aceite do PRD (subtask 5.2)

| Critério de Aceite (PRD) | Status | Evidência |
|--------------------------|--------|-----------|
| REQ-001: contagem usa medição real após a primeira resposta | OK | `TestEstimateUsesRealUsageAfterFirstCall` (lastPromptTokens=100 do mock; estimativa = 100 + delta/4); `TestCompactionTokensWithRealMeasurement` (before=1516, after=21 com medição real) |
| REQ-001: sem medição disponível, heurística usada | OK | `TestEstimateUsesRealUsageAfterFirstCall` (fase pré-primeira-chamada: len/4); `TestEstimateFallsBackWhenUsageUnreported` (usage=0 → heurística); `TestEstimateUnchangedAfterStreamError` (stream falho não corrompe medição) |
| REQ-002: sessão com janela pequena (2k) compacta antes de estourar e continua | OK | `TestCompactionBeforeOverflow` — catálogo de teste com `context_window: 2000`, histórico grande, `EventTurnDone` no fim |
| REQ-002: resumo preserva as últimas 4 mensagens intactas | OK | `TestCompactionPreservesLastFour` — cauda byte-idêntica via `reflect.DeepEqual`, tanto no estado do agent quanto nas mensagens enviadas ao gateway; inclui tool calls/ToolCallID |
| REQ-002: chamada de resumo antes da chamada que estouraria | OK | `TestCompactionBeforeOverflow` — calls[0] = resumo (`deepseek-v4.1-flash`, sem tools, single user message), calls[1] = principal (`glm-5.3`); `lastPromptTokens` final = 30 (medição não atualizada pela chamada de resumo) |
| REQ-002 (complementar): fallback do modelo de resumo | OK | `TestCompactionUsesFallbackModel` — flash ausente nos candidatos → default do catálogo (`glm-5.2`) |
| REQ-003: tool result gigante sozinho não causa estouro após truncamento | OK | `TestTruncationRescuesGiantToolResult` — 50k chars em janela 2k truncado para 2000 + `… (truncated)`; estimativa pós-truncamento dentro da janela; turno completa |
| REQ-004: TUI renderiza `⚡ context compacted (12.4k → 3.1k tokens)` em muted | OK | `TestCompactionLineRendersMuted` — ANSI `\x1b[38;2;128;128;128m` (colTextMuted #808080), texto exato, não reseta stream, não muda state/busy/pendingConfirm; `TestFormatTokensK` cobre a formatação k |
| REQ-004: evento `compaction` gravado no transcript JSONL | OK | `TestCompactionEmitsEventAndTranscript` — `EventCompaction` com TokensBefore(1516) > TokensAfter(20); evento `compaction` no JSONL com `tokens_before`/`tokens_after` |

Cobertura adicional além dos critérios de aceite (robustez):

- `TestNeedsCompactionThreshold` — limiar 70% estritamente maior (borderline 5596/5600 chars).
- `TestCompactionSkipsWhenHistoryFitsTail` — ≤ 4 mensagens não dispara compação nem evento.
- `TestSummaryFailureAbortsCompactionSilently` (2 subtests: gateway error, empty summary) — falha silenciosa, sem `EventError`, sem `EventCompaction`.
- `TestSummaryFailureFallsBackToTruncation` — degrada para truncamento; loop para ao caber (segundo tool result intacto).
- `TestTruncationPreservesTail` — tool result gigante dentro da cauda preservada nunca truncado; compação o mantém intacto.
- `TestTruncateOldToolResultsBoundaries` — boundary exato de 2000 intacto; uma truncagem por chamada; idempotência (já truncado não re-trunca — loop termina); cauda intocada.

## Conformidade com Rules (subtask 5.3)

| Rule | Status | Observações |
|------|--------|-------------|
| Rules `.agents/rules/*.md` (TS/Java/React/etc.) | N/A | Stack Go — techspec declara não aplicabilidade; nenhuma rule Go existe no repositório |
| Sem comentários no código | OK | grep em agent.go, session.go, tui.go, theme.go: nenhum comentário |
| Eventos via canal (buffer 512, emit não-bloqueante) | OK | `make(chan Event, 512)`; `emit` com `select`/`default`; `EventCompaction` segue o mesmo canal e caminho |
| Receivers por valor na TUI | OK | `handleAgentEvent`, `handleChatKey`, `handleConfirmKey` por valor; receivers por ponteiro (`refreshContent`, `scrollUp`, etc.) pré-existentes do baseline — padrão do projeto não alterado pela feature |
| `strings.Builder` sempre por ponteiro | OK | `partial` passada como `&partial` a `abortTurn`; `stream *strings.Builder`; `writeDiffLines(b *strings.Builder)`; nenhum builder copiado |
| Sem dependências não autorizadas | OK | go.mod: apenas promoção de `muesli/termenv` de indireta para direta (feature 002, já revisada); stdlib na feature 003 |
| Go 1.27, módulo `kterminal` | OK | go.mod inalterado quanto a versão |

## Aderência à TechSpec

| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `estimateTokens` = medição + delta/4; fallback len/4 | SIM | Campos `lastPromptTokens`/`lastEstimateChars`; atualizados só no loop principal (não na chamada de resumo) |
| Constantes `compactionThreshold=0.7`, `preservedTailMessages=4`, `summaryModel="deepseek-v4.1-flash"`, `toolResultTruncateChars=2000` | SIM | agent.go:20-24 |
| Ordem no loop: decide → needsCompaction → compact → re-estimar → truncate → ChatStream | SIM | agent.go:190-198; compação após roteamento (janela do modelo decidido) |
| `compact`: prompt de resumo com `messages[:len-4]`, ChatStream sem tools, substitui por `[system resumo] + últimas 4`, emite evento + transcript | SIM | Role `system` para o resumo conforme decisão confirmada; fallback para `Catalog.DefaultModel` |
| Falha do resumo → compação aborta silenciosamente → truncamento | SIM | `_ = a.compact(ctx)` + loop de truncamento; sem `EventError` |
| `truncateOldToolResults`: mais antiga primeiro, 2000 chars + `… (truncated)`, re-estima após cada truncagem, para ao caber ou nada a truncar, nunca toca a cauda | SIM | `limit := len - preservedTailMessages`; guarda de idempotência |
| `EventCompaction` via canal existente; `session.Event` com `TokensBefore/TokensAfter` `omitempty` | SIM | Eventos existentes permanecem byte-idênticos |
| TUI: linha `⚡ context compacted (Xk → Yk tokens)` em `colTextMuted` | SIM | `compactionStyle` = Foreground(colTextMuted); `formatTokensK` |
| Invariantes: últimas 4 mensagens nunca compactadas nem truncadas | SIM | Verificado por `TestCompactionPreservesLastFour` e `TestTruncationPreservesTail` |
| `buildSummaryPrompt`: role + content truncado a 2000 chars por mensagem, pede decisões/arquivos/estado | SIM | `summaryTurnChars=2000` |

## Escopo

| Item | Status |
|------|--------|
| Compação manual (slash/flag) | NÃO implementado (correto — fora de escopo) |
| Modelo de resumo escolhido pelo usuário | NÃO implementado — `summaryModel` é const (correto) |
| RAG / memória de longo prazo | NÃO implementado (correto) |
| Limiar configurável | NÃO implementado — `compactionThreshold` const 0.7 (correto) |
| Compação seletiva por importância | NÃO implementado — mecânica (correto) |
| Interação com usuário na compação | NENHUMA — mecânica, sem prompts (correto) |

## Tasks Verificadas

| Task | Status | Observações |
|------|--------|-------------|
| 1.0 Estimativa de tokens | COMPLETA | review_1.0.md existente; `estimateTokens`/`promptChars` + 3 testes de estimativa |
| 2.0 Compação automática com evento e transcript | COMPLETA | review_2.0.md existente; `compact`/`needsCompaction` + session fields + 7 testes |
| 3.0 Truncamento como garantia dura | COMPLETA | review_3.0.md existente; `truncateOldToolResults` + loop pós-compação + 4 testes |
| 4.0 Linha de compação na TUI | COMPLETA | review_4.0.md existente; `EventCompaction` case + `compactionStyle` + `formatTokensK` + 2 testes |
| 5.0 Verificação final integrada | COMPLETA | Esta review — checks verdes, checklist de aceite com evidência, diff conforme padrões, cenários E2E listados |

## Testes
- Total de Testes: 66 (execução `-count=1`, inclui subtests)
- Passando: 66
- Falhando: 0
- Coverage: não medida — projeto não usa cobertura como gate; cobertura funcional da feature 003: 16 testes de agent + 2 de TUI cobrem os 8 cenários mandatórios da techspec + 6 cenários extras de robustez
- Testes de integração: cobertos pelos testes de agent com mock gateway (fluxo completo decide→compact→truncate→call), conforme techspec
- Testes E2E: nenhum novo nesta task (conforme definição); execução deferida ao `kspec-qa`

## Segurança

N/A — funcionalidade local de TUI, sem endpoints/CORS/SQL/renderização de HTML. Sem secrets no código (chaves em testes são mocks). O prompt de resumo envia a conversa ao mesmo gateway que já recebe o histórico inteiro — nenhum dado novo sai do ambiente (techspec).

## Problemas Encontrados

| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| — | — | — | Nenhum problema bloqueante encontrado | — |

Observações não-bloqueantes (registro apenas):

1. `estimateTokens` com delta negativo grande (pós-compação com medição real) pode subestimar o próximo prompt até a próxima medição — heurística documentada na techspec (risco conhecido, mitigado pela folga de 30% do limiar). Nenhuma ação necessária na v1.
2. `needsCompaction` com `ContextWindow=0` (modelo fora do catálogo) compactaria sempre — cenário impossível na prática (`decide` só retorna modelos do catálogo; pin é validado). Registro documental.

## Pontos Positivos
- Garantia dura em camadas: compação (LLM) → truncamento (mecânico) → a não-estouro nunca depende do modelo de resumo funcionar; falha do resumo é silenciosa por design e testada em 2 variantes.
- Invariante da cauda de 4 mensagens verificado byte-a-byte com `reflect.DeepEqual` em três cenários distintos (preservação, truncamento, falha de resumo).
- Guarda de idempotência em `truncateOldToolResults` elimina loop infinito — coberto por teste dedicado de boundaries.
- Medição real (`lastPromptTokens`) atualizada apenas na chamada principal, nunca na de resumo — assertado explicitamente.
- TUI consome `EventCompaction` sem tocar stream/state/busy — linha sutil que não interrompe a leitura, exatamente como o PRD pede.

## Prontidão para `kspec-qa` (subtask 5.4)

Cenários E2E a executar (techspec — Abordagem de Testes → Testes de E2E):

1. **Sessão longa real compacta e continua**: sessão com gateway real ultrapassa 70% da janela do modelo em uso → compação automática ocorre antes da chamada que estouraria e a conversa continua sem falha, sem pergunta e sem pausa.
2. **Linha ⚡ visível em muted**: ao compactar, a linha `⚡ context compacted (Xk → Yk tokens)` aparece no chat em cor de texto muted, sem interromper a leitura do stream corrente.
3. **Coesão pós-compação**: após a compação, o agente mantém o fio da meada da tarefa corrente (resumo denso + últimas 4 mensagens preservadas) — pedir referência a decisão/arquivo antigo e validar continuidade.
4. **Transcript real**: evento `compaction` gravado no JSONL da sessão com `ts`, `tokens_before`, `tokens_after`.

## Recomendações
- Prosseguir com `kspec-qa` com os 4 cenários acima; depois `kspec-pr-review` para revisão semântica da entrega completa.
- Interação com techspec 004 (resume): snapshot serializa `messages` já compactadas — compatível por construção; manter o evento `compaction` no transcript como métrica de eficácia.

## Conclusão

APROVADO. A verificação encadeada completa passa (build, vet, gofmt vazio, 66/66 testes). Todos os 9 critérios de aceite do PRD (REQ-001 a REQ-004) têm evidência de teste direta; os 8 cenários mandatórios da techspec estão implementados e cobertos, com 6 testes extras de robustez além do especificado. O diff da feature 003 está conforme os padrões do projeto (sem comentários, eventos via canal com buffer 512 e emit não-bloqueante, receivers por valor na TUI, `strings.Builder` por ponteiro, sem dependências novas). Nenhum item fora de escopo foi implementado. A feature está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.
