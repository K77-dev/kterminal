# Relatório de Code Review - Estimativa de tokens por chamada no Agent (Task 1.0)

## Resumo
- Data: 2026-10-03
- Branch: 002-010-prds-kterminal
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 2 (`internal/agent/agent.go`, `internal/agent/agent_test.go`)
- Linhas Adicionadas: ~120 (25 em agent.go, 95 em agent_test.go; 9 linhas de agent.go são apenas realinhamento gofmt do bloco de campos)
- Linhas Removidas: 0 (substantivas)

> Nota: o working tree contém mudanças não commitadas das features 001/002 (completas e revisadas). Este review analisa exclusivamente o delta da task 1.0.

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Apenas stdlib | OK | Nenhum import novo |
| Eventos via canal buffer 512, emit não-bloqueante | OK | Mecanismo de eventos inalterado |
| gofmt / go vet | OK | `gofmt -l .` vazio; vet limpo |
| Rules DDD/TS/Vitest | N/A | Brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec |
| Padrões de teste (testing + httptest, AAA, independência) | OK | Testes isolados com mocks próprios, Arrange-Act-Assert |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| Campos `lastPromptTokens int64` e `lastEstimateChars int` no `Agent` | SIM | Subtask 1.1 |
| `estimateTokens() int64` = `lastPromptTokens + (charsAtuais - lastEstimateChars)/4` com medição | SIM | Fórmula literal, divisão inteira |
| Fallback heurístico `len(mensagens concatenadas)/4` sem medição | SIM | Ativo enquanto `lastPromptTokens == 0` |
| `lastPromptTokens`/`lastEstimateChars` atualizados a cada `StreamResult` no loop principal | SIM | Imediatamente após `ChatStream` bem-sucedido, antes do append da resposta — captura exatamente o prompt medido; não atualiza em falha de stream |
| Estimativa por chars usa o concatenado que vira prompt | SIM | `promptChars()` percorre role + content + tool_call_id + tool_calls (id, name, arguments) — o payload serializado no prompt (Ressalva 2) |
| `internal/llm` sem mudança | SIM | `Usage.PromptTokens` apenas consumido |
| `internal/catalog` sem mudança | SIM | `ContextWindow` consumido apenas na task 2.0 |
| Fora do escopo 1.0 não implementado (constantes de compação, `needsCompaction`, `compact`, `truncateOldToolResults`, `EventCompaction`, campos session/TUI) | SIM | Corretamente deferidos às tasks 2.0–4.0; nenhum código morto adicionado |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 1.1 Campos `lastPromptTokens` e `lastEstimateChars` no `Agent` | COMPLETA | |
| 1.2 `estimateTokens()` com medição real + delta e fallback heurístico | COMPLETA | |
| 1.3 Atualização a cada `StreamResult` no loop principal | COMPLETA | |
| 1.4 `TestEstimateUsesRealUsageAfterFirstCall` | COMPLETA | Cobrem-se também edge cases e cenário de erro |

## Testes
- Total de Testes: 52 (49 pré-existentes + 3 novos)
- Passando: 52
- Falhando: 0
- Coverage: n/a (sem toolchain de coverage configurado no repo; verificação via `go test -race` limpa)

Novos:
- `TestEstimateUsesRealUsageAfterFirstCall` (mandatório) — antes da 1ª chamada: heurística `/4` (400 chars → 100; sem mensagens → 0); após resposta do mock com `prompt_tokens=100`: `lastPromptTokens=100`, `lastEstimateChars=400` capturado no momento da chamada; com follow-up appended, estimativa = `100 + 47/4 = 111` (medição + delta).
- `TestEstimateFallsBackWhenUsageUnreported` (edge) — gateway reporta `prompt_tokens=0` → sem medição → heurística `/4` sobre as mensagens correntes.
- `TestEstimateUnchangedAfterStreamError` (erro) — gateway falha com HTTP 500 → `EventError`; nenhum campo de medição atualizado; estimativa permanece heurística.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/agent/agent.go | 100-104 | `Reset()` limpa os novos campos — extensão não literal à techspec | Aceita (Ressalva 1): sem isso, medição obsoleta pós-reset geraria estimativa negativa na task 2.0 |
| Baixa | internal/agent/agent.go | 378-387 | `promptChars()` inclui campos de tool calls além de role+content | Aceita (Ressalva 2): tool call arguments são parte do prompt; ignorá-los subestimaria deltas pós-tool-call e atrasaria a compação |
| Baixa | internal/agent/agent.go | 194-195 | Atualização incondicional mesmo quando `Usage.PromptTokens == 0` | Aceita (Ressalva 3): literal à techspec ("atualizado a cada StreamResult"); usage=0 mantém o fallback heurístico, consistente com o PRD |

## Verificação de Segurança
N/A — funcionalidade local passiva (estimativa em memória); sem endpoints, sem input externo novo, sem secrets, sem dados sensíveis em logs. O prompt de resumo (task 2.0) é o único fluxo que envia conteúdo novo ao gateway.

## Pontos Positivos
- Fórmula e semântica de atualização implementadas literalmente à techspec, com ponto de captura correto (antes do append da resposta).
- Estimativa 100% passiva: nenhum comportamento visível alterado — os 49 testes existentes passam intocados.
- Testes pinam os três regimes (sem medição, medição real + delta, falha de stream) com valores exatos calculados a partir do mock, não tautológicos.
- Sem scope creep: nada das tasks 2.0–4.0 foi antecipado; nenhum código morto.

## Recomendações
- Na task 2.0, ao implementar `compact()`, validar o comportamento da fórmula com delta negativo (pós-substituição do histórico) para `TokensAfter` — a fórmula atual é literal à techspec, mas o valor reportado no evento merece asserção explícita.
- Na task 2.0, garantir que a chamada de resumo NÃO atualize `lastPromptTokens`/`lastEstimateChars` (requisito explícito da techspec).

## Conclusão
APROVADO COM RESSALVAS. A task 1.0 implementa exatamente o escopo de REQ-001 definido na techspec (fundação observável: campos, `estimateTokens()`, atualização por `StreamResult`), com fórmula literal, sem consumidores antecipados e sem mudança de comportamento visível. As três ressalvas são decisões de detalhe justificadas e não-bloqueantes, documentadas para rastreabilidade; nenhuma requer ação corretiva. Todos os checks passam: build, vet, gofmt e 52/52 testes (race detector limpo).
