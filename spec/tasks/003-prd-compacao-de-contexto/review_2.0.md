# Relatório de Code Review - Compação automática em 70% da janela com evento e transcript (Task 2.0)

## Resumo
- Data: 2026-10-03
- Branch: 002-010-prds-kterminal
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 3 (`internal/agent/agent.go`, `internal/agent/agent_test.go`, `internal/session/session.go`)
- Linhas Adicionadas: ~496 (64 em agent.go, 2 em session.go, 430 em agent_test.go)
- Linhas Removidas: 0 (substantivas)

> Nota: o working tree contém mudanças não commitadas das features 001/002/003-1.0 (completas e revisadas). Este review analisa exclusivamente o delta da task 2.0: constantes de compação, `EventCompaction`, campos `TokensBefore/TokensAfter` em `agent.Event` e `session.Event`, métodos `needsCompaction`/`buildSummaryPrompt`/`modelAvailable`/`compact`, inserção no loop e 8 testes novos.

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Apenas stdlib | OK | Nenhum import novo (errors/sync já presentes das features anteriores) |
| Eventos via canal buffer 512, emit não-bloqueante | OK | `EventCompaction` usa o mesmo `a.emit` (select/default) |
| gofmt / go vet | OK | `gofmt -l .` vazio; vet limpo |
| Rules DDD/TS/Vitest | N/A | Brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec |
| Padrões de teste (testing + httptest, AAA, independência) | OK | Mocks próprios por teste, catálogo de janela pequena isolado via `catalog.Parse`, sem estado compartilhado entre testes |
| `internal/llm` e `internal/catalog` sem mudança | OK | `ChatStream` sem tools e `ContextWindow`/`DefaultModel` apenas consumidos |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| Constantes `compactionThreshold = 0.7`, `preservedTailMessages = 4`, `summaryModel = "deepseek-v4.1-flash"` | SIM | Literais, nomes exatos da techspec |
| `needsCompaction(window int) bool` — estimativa > 0.7 × janela | SIM | Desigualdade estrita; fronteira pinada por teste (1400 exato → false; 1401 → true) |
| `compact(ctx) error` como método privado do `Agent` | SIM | Estrutura literal ao snippet da techspec |
| Ordem no loop: `decide` → `needsCompaction(window do modelo decidido)` → `compact` → `ChatStream` | SIM | Inserido após `Catalog.Get(decision.Model)` e evento de rota, antes do `ChatStream` — compação pós-roteamento, janela do modelo certo |
| `buildSummaryPrompt`: role + content truncado a 2000 chars por mensagem, pede resumo denso (decisões, arquivos tocados, estado da tarefa) | SIM | Função livre como no snippet; truncamento por bytes (`len`) |
| Resumo entra como mensagem `role: "system"` + últimas 4 byte-idênticas | SIM | Cauda copiada defensivamente; byte-identidade provada via JSON round-trip + `reflect.DeepEqual` incluindo tool_calls e tool_call_id |
| Chamada de resumo sem tools; modelo fixo não-roteado | SIM | `ChatStream(ctx, model, [user prompt], nil, nil)`; não passa pelo router |
| Fallback para `Catalog.DefaultModel` se ausente/indisponível | SIM | `!ok \|\| !modelAvailable` — cobre ausência no catálogo e nos candidatos |
| `EventCompaction` emitido com `TokensBefore > TokensAfter` | SIM | Valores exatos pinados nos dois regimes (heurístico 1516/20; medição real 1516/21) |
| Evento `compaction` no transcript com `tokens_before`/`tokens_after` | SIM | `session.Event{Type: "compaction", ...}`; `ts` preenchido pelo Writer |
| `session.Event` com `TokensBefore/TokensAfter int64` `omitempty` — eventos existentes byte-idênticos | SIM | Campos zero são omitidos; 52 testes pré-existentes intocados |
| Falha da chamada de resumo → aborta silenciosamente, sem `EventError` | SIM | `_ = a.compact(ctx)` no loop; testado com erro HTTP 500 |
| `lastPromptTokens`/`lastEstimateChars` NÃO atualizados na chamada de resumo | SIM | `compact` não toca os campos; provado por asserção (medição = 30 da chamada principal, não 5 da de resumo) e pelo regime medido |
| Últimas 4 mensagens nunca compactadas | SIM | Invariante do slice de cauda; guard adicional cobre histórico que cabe na cauda |
| Fora do escopo 2.0 não implementado (`truncateOldToolResults`, linha ⚡ na TUI, re-estimação pós-compação) | SIM | Corretamente deferidos às tasks 3.0/4.0; nenhum código morto |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 2.1 `EventCompaction` no `EventKind` + campos em `agent.Event` e `session.Event` | COMPLETA | |
| 2.2 `needsCompaction` e `buildSummaryPrompt` | COMPLETA | Com teste de fronteira de limiar |
| 2.3 `compact` (resumo sem tools, system message, substituição, evento + transcript) | COMPLETA | |
| 2.4 Checagem no loop após `decide` e antes do `ChatStream` | COMPLETA | |
| 2.5 Testes 1-4 da techspec com catálogo de janela pequena e mock gateway | COMPLETA | 4 mandatórios + 4 adicionais de edge/erro |

## Testes
- Total de Testes: 60 (52 pré-existentes + 8 novos)
- Passando: 60
- Falhando: 0
- Coverage: n/a (sem toolchain de coverage configurado no repo; verificação via `go test -race` limpa)

Novos (catálogo de teste `ContextWindow: 2000`, gateway gravador de chamadas com dispatch por ausência de tools):
- `TestCompactionBeforeOverflow` (mandatório) — histórico de 6 mensagens (>70% de 2k) → chamada de resumo (`deepseek-v4.1-flash`, sem tools, 1 mensagem user contendo turno antigo) ocorre ANTES da chamada principal (`glm-5.3`); `EventCompaction` + `EventTurnDone`; medição reflete a chamada principal (30), não a de resumo (5).
- `TestCompactionPreservesLastFour` — chamada principal recebe `[system resumo] + últimas 4` byte-idênticas (incluindo tool_calls/tool_call_id via DeepEqual pós-JSON); estado do agent preserva a cauda.
- `TestCompactionUsesFallbackModel` — `deepseek-v4.1-flash` ausente no `/v1/models` → resumo usa o default do catálogo (`glm-5.2`); principal segue `glm-5.3`.
- `TestCompactionEmitsEventAndTranscript` — `EventCompaction` com `TokensBefore=1516 > TokensAfter=20` (heurística `/4`, valores exatos); transcript JSONL contém `compaction` com `tokens_before`/`tokens_after` idênticos ao evento.
- `TestNeedsCompactionThreshold` (edge) — exatamente 70% da janela (1400 tokens) → false; 1401 → true.
- `TestCompactionTokensWithRealMeasurement` (edge) — medição real (100 tokens/400 chars) + delta: before=1516, after=21 (delta negativo pós-substituição); atende a recomendação pendente do review_1.0.
- `TestCompactionSkipsWhenHistoryFitsTail` (edge) — 4 mensagens acima do limiar: nenhuma chamada de resumo, histórico completo enviado, sem evento (guard da Ressalva 1).
- `TestSummaryFailureAbortsCompactionSilently` (erro, 2 subcasos) — erro HTTP 500 e resumo vazio: 2 chamadas, histórico inalterado (7 mensagens), sem `EventError`, sem `EventCompaction`, `EventTurnDone` no fim.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/agent/agent.go | 429-431 | Guard `len(a.messages) <= preservedTailMessages` ausente no snippet da techspec | Aceita (Ressalva 1): sem ela, `messages[:len-4]` entra em pânico com len < 4 e chamaria o resumo com slice vazio com len == 4; o invariante "últimas 4 nunca compactadas" implica não haver o que compactar; testada |
| Baixa | internal/agent/agent.go | 441-443 | Guard `res.Content == ""` → aborta (snippet não prevê) | Aceita (Ressalva 2): resumo vazio substituiria turnos antigos por nada — perda silenciosa de contexto; abortar preserva o histórico completo e degrada para o truncamento (task 3.0); testada |
| Baixa | internal/agent/agent.go | 419-426 | Helper `modelAvailable` (método) em vez de `available` (função do snippet) | Aceita (Ressalva 3): evita shadowing com a variável local `available` existente em `decide()`; mesma semântica |
| Baixa | internal/agent/agent.go | 21 | Constante extra `summaryTurnChars = 2000` | Aceita (Ressalva 4): a techspec exige truncamento por mensagem no prompt de resumo, mas `toolResultTruncateChars` é constante da task 3.0; constante nomeada em vez de magic number (regra: sem comentários) |

## Verificação de Segurança
N/A — aplicação local (TUI). O prompt de resumo envia o histórico da conversa ao mesmo gateway que já recebe o histórico inteiro a cada chamada hoje (nenhum dado novo sai do ambiente, conforme techspec). Sem endpoints, sem input externo novo, sem secrets, sem dados sensíveis novos em logs (transcript grava apenas `ts`/`tokens_before`/`tokens_after`).

## Pontos Positivos
- `compact` segue o snippet da techspec linha a linha (ordem before/resumo/substituição/after/emit/transcript preservada), com as duas guards defensivas isoladas e testadas.
- A checagem no loop fica exatamente no ponto especificado — após o roteamento (janela do modelo decidido), antes do `ChatStream` — e é silenciosa por construção (`_ =`), mantendo `EventError` reservado à chamada principal.
- A cauda preservada é byte-idêntica por construção (cópia defensiva + append em slice novo), e o teste prova isso no payload real enviado ao gateway, não apenas no estado interno.
- Os testes pinam valores exatos calculados a partir dos mocks (1516/20 heurístico, 1516/21 com medição real, fronteira 1400/1401) — não tautológicos.
- Recomendações pendentes do review_1.0 ambas atendidas: asserção explícita de `TokensAfter` com delta negativo, e prova de que a chamada de resumo não atualiza a medição.
- Sem scope creep: truncamento (3.0) e linha ⚡ da TUI (4.0) não foram antecipados; TUI ignora `EventCompaction` até a task 4.0 (switch sem default).

## Recomendações
- Na task 3.0, ao implementar `truncateOldToolResults`, atualizar `TestSummaryFailureAbortsCompactionSilently` — o histórico não permanecerá inalterado quando o truncamento passar a rodar após o aborto do resumo (comportamento esperado da nova garantia dura).
- Na task 3.0, o loop de re-estimativa pós-compação deve parar antes da cauda preservada (invariante já respeitado por `compact`).
- Edge conhecida (literal ao snippet, sem ação): se o `Catalog.DefaultModel` também estiver indisponível no gateway, a chamada de resumo falha e o aborto silencioso degrada para o truncamento — a garantia de não-estouro nunca depende do LLM de resumo.

## Conclusão
APROVADO COM RESSALVAS. A task 2.0 implementa o núcleo de REQ-002/REQ-004 exatamente como especificado: disparo mecânico em 70% da janela do modelo decidido, resumo denso via modelo barato fixo (com fallback), substituição do histórico por `[system] + últimas 4` byte-idênticas, `EventCompaction` no canal e evento `compaction` no transcript, com falha do resumo abortando silenciosamente. As quatro ressalvas são decisões de detalhe justificadas (duas guards defensivas, um rename anti-shadowing, uma constante nomeada), todas cobertas por testes dedicados e não-bloqueantes. Todos os checks passam: build, vet, gofmt e 60/60 testes (race detector limpo), incluindo os 52 pré-existentes intocados.
