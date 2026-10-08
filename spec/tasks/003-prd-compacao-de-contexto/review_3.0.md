# Relatório de Code Review - Truncamento de tool results gigantes como garantia dura (Task 3.0)

## Resumo
- Data: 2026-10-04
- Branch: 002-010-prds-kterminal
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 2 (`internal/agent/agent.go`, `internal/agent/agent_test.go`)
- Linhas Adicionadas: ~273 (23 em agent.go, 250 em agent_test.go)
- Linhas Removidas: 0 (substantivas)

> Nota: o working tree contém mudanças não commitadas das features 001/002/003-1.0+2.0 (completas e revisadas). Este review analisa exclusivamente o delta da task 3.0: constante `toolResultTruncateChars`, método `truncateOldToolResults`, loop pós-compação no fluxo principal, 3 testes mandatórios da techspec, 1 teste extra de fronteiras e a atualização de `TestSummaryFailureAbortsCompactionSilently` (recomendação pendente do review_2.0).

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Apenas stdlib | OK | Nenhum import novo |
| Compação e truncamento como métodos privados do `Agent` | OK | `truncateOldToolResults` segue o padrão; TUI/session não tocados |
| gofmt / go vet | OK | `gofmt -l .` vazio; vet limpo |
| Rules DDD/TS/Vitest | N/A | Brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec |
| Padrões de teste (testing + httptest, AAA, independência) | OK | Testes de integração reutilizam os mocks do 2.0 (gateway gravador, catálogo de janela 2k); teste de fronteiras é unitário direto, sem infraestrutura |
| `internal/llm`, `internal/catalog`, `internal/session`, `internal/tui` sem mudança | OK | Apenas consumidos; nenhum arquivo fora de `internal/agent` tocado |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| Constante `toolResultTruncateChars = 2000` | SIM | Nome e valor exatos da techspec |
| `truncateOldToolResults() bool` — assinatura sem parâmetros | SIM | Literal ao snippet da techspec |
| Percorre `messages` da mais antiga para a mais recente | SIM | Scan `0..limit`; uma truncagem por chamada, sempre a mais antiga elegível (Ressalva 2) |
| `role: "tool"` com `len(Content) > 2000` → `Content[:2000] + "… (truncated)"` | SIM | Fórmula literal, desigualdade estrita; fronteira 2000/2001 pinada por teste |
| Re-estimar após cada truncamento | SIM | Condição do `for` chama `needsCompaction` (→ `estimateTokens`) a cada iteração |
| Parar quando a estimativa cabe ou não truncou nada | SIM | `break` quando o scan não trunca; guarda de igualdade implementa "não truncou nada" para conteúdo já truncado (Ressalva 1) |
| Últimas 4 mensagens (cauda preservada) nunca truncadas | SIM | `limit = len - preservedTailMessages`; provado em 2 testes (integração + unitário) |
| Falha da chamada de resumo → fluxo cai no truncamento; turno completa sem `EventError` | SIM | `_ = a.compact(ctx)` preservado do 2.0; truncamento roda pós-aborto; provado nos 3 modos de falha (HTTP 500, resumo vazio) |
| Ordem no loop: `decide` → `compact` → re-estimar → `truncate` → `ChatStream` | SIM | Inserido entre o bloco de compação e o `ChatStream` (agent.go linhas 190-200) |
| REQ-003: tool result gigante sozinho não causa estouro pós-truncamento | SIM | Payload pós-truncamento estimado em ~534 tokens numa janela de 2000 (assinção explícita no teste) |
| Fora do escopo 3.0 não implementado (linha ⚡ na TUI, verificação final) | SIM | Deferidos às tasks 4.0/5.0; nenhum código morto |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 3.1 `truncateOldToolResults` com limite de 2000 chars + sufixo | COMPLETA | Método em agent.go; sufixo `… (truncated)` literal |
| 3.2 Loop pós-compação (re-estimar → truncar → repetir) antes do `ChatStream` | COMPLETA | `for needsCompaction { if !truncate { break } }` |
| 3.3 Falha de resumo da task 2.0 degrada para o truncamento | COMPLETA | Fluxo + `TestSummaryFailureFallsBackToTruncation` + silence test atualizado |
| 3.4 Testes 6-8 da techspec | COMPLETA | 3 mandatórios + 1 extra de fronteiras |

## Testes
- Total de Testes: 64 (60 pré-existentes + 4 novos)
- Passando: 64
- Falhando: 0
- Coverage: n/a (sem toolchain de coverage configurado no repo; verificação via `go test -race` limpa)

Novos:
- `TestTruncationRescuesGiantToolResult` (mandatório) — tool result de 50k chars em janela de 2k com falha de resumo (HTTP 500): truncado para exatamente 2000 + sufixo (2015 bytes) no payload real; estimativa do prompt pós-truncamento dentro da janela; `EventTurnDone`.
- `TestTruncationPreservesTail` (mandatório) — compação bem-sucedida com tool result gigante (50k) dentro das últimas 4: cauda byte-idêntica no payload (DeepEqual), `EventCompaction` emitido, `EventTurnDone`; a estimativa pós-resumo permanece acima do limiar, forçando o truncamento a rodar e provar que não toca a cauda (Ressalva 3).
- `TestSummaryFailureFallsBackToTruncation` (mandatório) — resumo vazio → aborto silencioso → truncamento executa: tool result antigo (50k) truncado, segundo tool result (2500 bytes, ainda fora da cauda) permanece intacto porque o loop para assim que a estimativa cabe; sem `EventError`/`EventCompaction`; `EventTurnDone`.
- `TestTruncateOldToolResultsBoundaries` (edge, unitário direto) — fronteira exata de 2000 chars intocada; 2001 bytes truncado; uma truncagem por chamada na ordem mais antiga→mais recente; segunda chamada sobre conteúdo já truncado retorna false (prova de terminação); tool results gigantes dentro da cauda nunca tocados.

Atualizado (recomendação do review_2.0):
- `TestSummaryFailureAbortsCompactionSilently` — histórico agora contém tool result gigante; asserções refletem o novo contrato: o truncamento roda pós-aborto e o payload traz o conteúdo truncado (o histórico não permanece inalterado), a contagem de mensagens é preservada (7), o aborto continua silencioso (sem `EventError`/`EventCompaction`) e o turno completa. Ambos subcasos (gateway error, empty summary) mantidos.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/agent/agent.go | 466-469 | Guarda `truncated == m.Content` ausente no texto da techspec | Aceita (Ressalva 1): o conteúdo truncado tem 2015 bytes e re-matcha o predicado `> 2000`; sem a guarda, o loop re-truncaria a mesma mensagem para sempre quando a estimativa nunca cabe (ex.: cauda gigante). A guarda implementa literalmente "não truncou nada" (nenhuma mudança de conteúdo) sem alterar a fórmula do spec; testada |
| Baixa | internal/agent/agent.go | 459-474 | Uma truncagem por chamada em vez de truncar tudo num único passe | Aceita (Ressalva 2): única leitura consistente com a assinatura da techspec (`() bool`, sem acesso à janela) e com "re-estima após cada truncamento; para quando a estimativa cabe" — truncar tudo num passe violaria o early-stop e over-truncaria; semântica pinnada por 2 testes |
| Baixa | internal/agent/agent_test.go | TestTruncationPreservesTail | Cenário adaptado em relação ao sugerido pela techspec ("janela grande o suficiente pós-resumo") | Aceita (Ressalva 3): com janela que comporta o pós-resumo, `needsCompaction` seria falso e o truncamento nunca rodaria — o teste não provaria o invariante da cauda. Usada janela de 2000 com estimativa pós-resumo ainda acima do limiar, forçando o truncamento a rodar; o turno completa (`EventTurnDone`), que é o contrato observável do cenário |

## Verificação de Segurança
N/A — aplicação local (TUI). O truncamento apenas REDUZ o volume de dados enviado ao gateway (tool results antigos encolhem para 2015 bytes); nenhum dado novo sai do ambiente, nenhum input externo novo, sem secrets, sem mudança em logs (o evento `compaction` no transcript permanece o da task 2.0 — truncamento é silencioso por design).

## Pontos Positivos
- A garantia de não-estouro nunca depende do LLM de resumo: os três modos de falha da compação (erro HTTP, resumo vazio, guarda de histórico curto) terminam todos no truncamento mecânico, que sempre cabe.
- Early-stop provado por teste: o loop para assim que a estimativa cabe — o segundo tool result (2500 bytes) permanece intacto quando truncar o primeiro (50k) já basta; não over-trunca.
- Terminação garantida e provada: cada mensagem é truncada no máximo uma vez (guarda de igualdade); o teste de fronteiras mostra a chamada seguinte retornando false sobre conteúdo já truncado — sem essa prova, um loop infinito seria possível com cauda gigante.
- O teste de fronteiras cobre os limites do requisito (2000 exato intocado, 2001 truncado, ordem, re-truncação, cauda) diretamente no método, sem depender de infraestrutura de gateway.
- Recomendação pendente do review_2.0 atendida: o teste de silêncio evoluiu para o novo contrato em vez de simplesmente continuar passando por ausência de tool results no histórico.
- Sem scope creep: linha ⚡ da TUI (4.0) e verificação final (5.0) não antecipados; `EventCompaction` segue ignorado pela TUI até a task 4.0.

## Recomendações
- Task 4.0: consumir `EventCompaction` na TUI (linha `⚡ context compacted (Xk → Yk tokens)` em cor muted) — o evento já trafega no canal com `TokensBefore/TokensAfter` preenchidos.
- Task 5.0 / kspec-qa: edge conhecida sem ação — uma cauda que sozinha exceda 70% da janela não pode ser salva nem pela compação nem pelo truncamento (invariante da cauda é não negociável); a chamada segue best-effort. Vale documentar no QA com sessão longa real.
- Edge conhecida (literal ao spec, sem ação): tool result de 2001-2015 bytes cresce para 2015 bytes pós-truncamento (`Content[:2000] + sufixo`); impacto na estimativa é desprezível.

## Conclusão
APROVADO COM RESSALVAS. A task 3.0 implementa o "assoalho" de REQ-003 exatamente como especificado: `truncateOldToolResults` percorre o histórico da mais antiga para a mais recente fora da cauda preservada, trunca tool results acima de 2000 chars com o sufixo literal, e o loop pós-compação re-estima após cada truncamento e para quando a estimativa cabe ou nada foi truncado — na ordem `decide → compact → re-estimar → truncate → ChatStream`. A falha do LLM de resumo degrada silenciosamente para o truncamento em todos os modos testados, sem `EventError`. As três ressalvas são decisões de detalhe justificadas (guarda de terminação, semântica de uma truncagem por chamada exigida pelo early-stop do spec, adaptação do cenário do teste de cauda para que ele exercite o truncamento), todas cobertas por testes dedicados e não-bloqueantes. Todos os checks passam: build, vet, gofmt e 64/64 testes (race detector limpo), incluindo os 60 pré-existentes.
