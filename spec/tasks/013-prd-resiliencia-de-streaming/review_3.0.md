# Review 3.0 — llm: retry com backoff para falhas transientes

## Status: APROVADO COM RESSALVAS

## Verificação por subtask

- [x] 3.1 `statusError` interno (`code`, `status`, `body` truncado em 4096, `retryAfter`) construído por `newStatusError` com parse do header `Retry-After` em delta-seconds (`strconv.Atoi`; não parseável/negativo/http-date → 0); substituiu o `fmt.Errorf` de não-200 no caminho de `ChatStream` (agora `streamAttempt`), mantendo status+body truncado na mensagem (llm.go:346-369, 486-489)
- [x] 3.2 `isTransient` cobrindo a lista exata: `statusError` 429/500/502/503/504 → true, demais status → false (4xx exceto 429); `ErrStreamIdle`/`ErrFirstByte` → true; `context.DeadlineExceeded` → true; `io.EOF`/`io.ErrUnexpectedEOF` → true; qualquer `net.Error` → true (cobre o `url.Error` com `Timeout() == true` da capa total `http.Client.Timeout`); decode de chunk e `context.Canceled` → false (llm.go:371-401)
- [x] 3.3 Loop de tentativas em `ChatStream` envolvendo `do()` + leitura: contador `emitted` no `ChatStream` via closure `countDelta` (onDelta chamado OU conteúdo acumulado — tool call fragments não contam, não são renderizados ao vivo); retry somente com `emitted == 0 && isTransient && attempt <= MaxRetries`; backoff exponencial (base `retryBase` default 1s, fator 2, capa 30s, jitter ±20% via `math/rand`) com `Retry-After` prevalecendo quando maior (capa 30s); espera interrompível pelo ctx do caller (llm.go:403-478)
- [x] 3.4 Erro final `chat stream failed after %d attempt(s) (model %s): %w` — singular para 1 tentativa, plural a partir de 2 (escolha: gramática correta, consistente); causas amigáveis compostas na fonte: `ErrStreamIdle` = "stream stalled" + " for %s without data" → "stream stalled for 5m0s without data"; `ErrFirstByte` = "no response headers" + " within %s — check the gateway"; cadeia `%w` preservada (llm.go:154-157, 280-282, 232-241, 431-437)
- [x] 3.5 Testes de unidade (tabela `isTransient`, parse `Retry-After`, backoff) + cenários httptest (llm_test.go)

## Decisões de design

- **Extração de `streamAttempt`**: o corpo de leitura/accumulação saiu de `ChatStream` para um método por tentativa. O estado parcial (`result`, `calls`, `order`) é local a cada tentativa — o descarte entre tentativas acontece por construção (a tentativa falha é descartada inteira; a próxima começa do zero). Como o retry exige `emitted == 0`, nenhum delta de conteúdo foi entregue à tentativa falha, logo `onDelta` nunca é invocado duas vezes pelo mesmo conteúdo. O `emitted` cumulativo no `ChatStream` é equivalente ao por-tentativa: tentativas após a primeira só existem se as anteriores emitiram nada.
- **Mensagens amigáveis na fonte, não no wrapper final**: os sentinels foram reworded ("stream stalled", "no response headers") para que o wrapping na fonte produza exatamente o texto da spec ("stream stalled for 5m0s without data", "no response headers within 60s — check the gateway") mantendo `errors.Is` funcional através de `%w` — sem tipo de erro customizado. REQ-005 vale para todos os caminhos (não só exaustão): nenhum erro cru das camadas novas chega à TUI.
- **Cancel do caller nunca retentado, dupla guarda**: `ctx.Err() != nil` → retorno imediato de `ctx.Err()` (cobre Esc mesmo quando o erro que superficia é EOF/reset/limpo — o ctx do pai é a autoridade), mais `errors.Is(err, context.Canceled)` como cinto-e-suspensário para cancel próprio que vaze sem sentinel. O `select` do backoff também observa `ctx.Done()`. O `isAbort` do agent (`errors.Is(err, context.Canceled) || ctx.Err() != nil`) continua funcionando com o erro cru retornado.
- **`retryBase` não-exportado** (default 1s em `New`): testes injetam 10-100ms direto (in-package); `SetLimits` não o toca — é knob de teste, não de config.
- **Retry-After vs jitter**: `wait = max(backoff_com_jitter, retryAfter)` com capa dura de 30s no final — nunca espera menos que o header pediu (jitter negativo não derruba o Retry-Below). A capa é aplicada após o jitter, garantindo teto absoluto de 30s.
- **Overflow-safe**: a duplicação exponencial é iterativa com parada antecipada quando `raw >= backoffCap` — sem shift que estoura int64 para tentativas altas.
- **`statusError.Error()`** = "status: body" (sem o prefixo "chat completions" do `fmt.Errorf` antigo): o contexto da operação vem do wrapper final "chat stream failed after … (model X)"; status + body truncado preservados como hoje.

## Testes

Unitários:
- `TestIsTransient` — tabela com 19 casos: nil, 429/500/502/503/504, 401/400/404, sentinels wrapped, EOF/UnexpectedEOF, DeadlineExceeded, net.Error (timeout e não-timeout), Canceled, decode chunk, genérico
- `TestNewStatusError` — parse `Retry-After` ("1" → 1s, "120" → 120s, ausente/não-parseável/negativo/http-date → 0) + mensagem com status e body
- `TestBackoffDuration` — limites do jitter ±20% nas tentativas 1-4 (100 amostras cada), capa 30s (tentativa 10 → [24s, 30s]), Retry-After 5s prevalece exato, Retry-After 10m capado em 30s, Retry-After menor mantém o backoff

httptest (retryBase curto; contagem de chamadas protegida por mutex para `-race`):
- `TestChatStreamRetryTransientThenSuccess` — 500 pré-delta com MaxRetries 1 → exatamente 2 chamadas, intervalo ≥ 50ms (backoff ocorreu), deltas `[hel lo]` sem duplicação, conteúdo "hello"
- `TestChatStreamRetryHonorsRetryAfter` — 429 com `Retry-After: 1` e depois 200 → sucesso na 2ª tentativa, intervalo ≥ 900ms com retryBase 10ms (prova que o header prevaleceu sobre a base curta); ~1s de wall time (delta-seconds é inteiro, inevitável)
- `TestChatStreamRetryFirstByteTimeout` — stall pré-headers (ErrFirstByte, transiente) com MaxRetries 1 → 2 chamadas, sucesso (techspec: "handler que libera na 2ª chamada")
- `TestChatStreamNoRetryAfterDelta` — delta emitido e depois stall (ErrStreamIdle transiente) com MaxRetries 2 → 1 chamada apenas, `onDelta` 1x com "hel", resultado parcial preservado, mensagem final com "after 1 attempt" + "stream stalled for 150ms without data" + modelo
- `TestChatStreamNoRetryOnClientError` — 401 com MaxRetries 2 → 1 chamada, `errors.As` acha `statusError{401}` através do wrapper, mensagem com status/body/tentativa única
- `TestChatStreamRetryExhaustion` — sempre 500 com MaxRetries 2 → 3 chamadas, erro contendo "chat stream failed after 3 attempts", "glm-5.2" e "500 Internal Server Error"; < 2s com retryBase curto
- `TestChatStreamCancelNotRetried` — cancel do caller via canal após 1º delta → 1 chamada, `errors.Is(err, context.Canceled)`, sem retry

Regressão 2.0: os 7 testes existentes passam inalterados (todos com MaxRetries 0 → 1 tentativa; `errors.Is` preservado pela cadeia `%w`; parcial retornado junto ao erro final).

Execuções: `go test -race -count=5 ./internal/llm/` → ok (~15s, 5 execuções consecutivas).

## Build

- `go build ./internal/llm/` → ok
- `go vet ./internal/llm/` → ok
- `gofmt -l internal/llm/` → sem diffs
- `go test -race ./internal/llm/` → ok
- `go build ./... && go vet ./... && gofmt -l .` → ok

## Ressalvas

1. **2 testes de `internal/agent` falham na contagem de chamadas (fora do escopo desta task — instrução explícita de não tocar `internal/agent`).** Causa raiz: o default `MaxRetries: 2` (PRD REQ-002/REQ-004) agora retenta o 500 do summary path de `compactionGateway` (`summaryChunks == nil` → HTTP 500) 3× antes de abortar a compaction. O comportamento é o especificado; os testes embutem o mundo pré-retry. Semântica preservada (compaction aborta silenciosamente, main call prossegue com tool result truncado) — só a contagem muda. Fix exato para o dono do boundary (task 5.0 ou fix dedicado):
   - `agent_test.go:1512` — `want 2 (failed summary + main call)` → `want 4 (3 retried summary attempts + main call)`
   - `agent_test.go:1514` e `:1518` — `calls[1]` → `calls[3]` (main call é a 4ª chamada)
   - `agent_test.go:1568` — `want 2 (failed summary + rescued main call)` → `want 4`
   - `agent_test.go:1570` — `main := calls[1]` → `main := calls[3]`
   - Nota: `TestSummaryFailureAbortsCompactionSilently/empty summary` e `TestSummaryFailureFallsBackToTruncation` passam (summary falha com stream válido vazio — erro em nível agent, sem retry HTTP).
2. **Testes do agent desacelerados mas estáveis**: `TestEstimateUnchangedAfterStreamError` e `TestSnapshotWrittenAfterStreamError` (failingChatGateway, 500 → 3 tentativas, backoff ~2.4-3.6s) agora levam ~3s cada, dentro do timeout de 5s do `waitForErrorEvent`; validados com `-race -count=3` sem flake. Custo inerente ao default especificado.
   - **CORRIGIDO pós-review (orquestrador)**: `failingChatGateway` passou a responder 401 (não-transiente, 1 tentativa, sem backoff) — os dois testes voltaram a ~instantâneos. Nenhum dos dois afirma sobre o status do erro; o que verificam (estimate zerado / snapshot preservado após erro de stream) é intacto. O caminho transiente-500-com-retry permanece coberto pelos testes dedicados do `internal/llm`.
3. **"500 APÓS 1 delta" (subtarefa 3.5)** é impossível numa única resposta HTTP (status é comprometido nos headers). O cenário equivalente — falha transiente após delta emitido — é coberto por `TestChatStreamNoRetryAfterDelta` (ErrStreamIdle pós-delta), que exercita o mesmo guarda de `emitted` com falha comprovadamente transiente.
4. **`io.EOF` pré-`[DONE]` na prática**: `bufio.Scanner` engole EOF limpo (fim de stream sem `[DONE]` não gera erro); os EOFs que superficiam como erro vêm do body HTTP (ex.: chunked interrompido → `io.ErrUnexpectedEOF`, reset de conexão → `net.Error`) — ambos classificados como transientes e cobertos na tabela do `TestIsTransient`. Não há teste httptest dedicado a EOF abrupto porque o tipo de erro variável o tornaria flaky; a classificação está coberta em unidade.

## Notas

- Erro de exaustão e erro de tentativa única compartilham o formato `chat stream failed after N attempt(s) (model X): causa` (contagem correta, singular para 1); exceção: cancelamento do caller, que retorna `ctx.Err()` cru (o abort é intencional e do conhecimento do chamador — o agent o trata via `isAbort`).
- `Retry-After` é armazenado sem capa no parse (ex.: "300" → 300s) e capado em 30s apenas no uso — o header nunca segura o turno além de 30s (techspec: "limite superior").
- `time.After` no backoff: timer abandonado no cancel não vaza goroutine (runtime desde Go 1.23); loop tem no máximo MaxRetries+1 iterações.
- Sem comentários no código, stdlib only (`math/rand`, `net`, `strconv` novos), código em inglês. `do()` inalterado no contrato; `ListModels` intocado; assinaturas públicas (`New`, `SetLimits`, `ChatStream`) inalteradas.
