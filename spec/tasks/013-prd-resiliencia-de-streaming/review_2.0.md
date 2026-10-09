# Review 2.0 — llm: timeouts de streaming em camadas

## Status: APROVADO

## Verificação por subtask

- [x] 2.1 `Limits` exportado (`RequestTimeout`, `IdleTimeout`, `FirstByteTimeout`, `MaxRetries`), campo `limits` no `Client`, `SetLimits` que substitui tudo; o 10m fixo virou `defaultLimits.RequestTimeout` (llm.go:156-168, 197-204)
- [x] 2.2 Timer de primeiro byte em `do()`: ctx derivado com `context.WithCancel`, `time.AfterFunc(FirstByteTimeout, cancel)`, `timer.Stop()` no retorno de `Do`, classificação `ErrFirstByte` com `%w` (llm.go:206-238)
- [x] 2.3 `idleBody` com watchdog: `*time.Timer` resetado a cada `Read` bem-sucedido; expiração cancela o ctx do request (desbloqueia `Read` travado) e marca a causa; erro superficia via `scanner.Err()` com `errors.Is(err, ErrStreamIdle) == true`; `Close` faz `Stop` + `cancel` (llm.go:240-312)
- [x] 2.4 Testes com `Limits` curtos (100ms/150ms) via httptest com stalls controlados (llm_test.go)

## Decisões de design

- **Ciclo de vida do cancel**: `do()` retorna `(*http.Response, context.CancelFunc, error)`; `ChatStream` registra `defer cancel()` antes do close do body — LIFO garante que o cancel só roda depois do body consumido/fechado. Path não-200 fecha o body explicitamente antes do return. `idleBody.Close()` também invoca o `cancel` (idempotente), satisfazendo "Stop + cancel no Close".
- **Classificação first-byte race-free**: `expired := timer != nil && !timer.Stop()`. Se `Stop()` retorna false, o callback do timer já foi despachado e o cancel VAI rodar (body morto) — tratar como `ErrFirstByte` na sucesso é correto e sem race. O gate `ctx.Err() == nil` preserva a semântica de cancelamento do chamador (Esc): cancel do pai retorna o erro cru, não o sentinel.
- **Watchdog stale-safe**: `fire()` revalida `time.Since(b.last) < b.idle` sob mutex — callback atrasado (timer expirou, mas um `Read` bem-sucedido chamou `Reset` antes do callback executar) vira no-op; o `Reset` já rearmou a janela. Estado (`last`, `expired`, `closed`, timer) todo sob mutex — `-race` limpo.
- **Campo `limits` unexportado** (techspec nomeia os campos em minúsculas); `SetLimits` é a superfície pública para o wiring da task 5.0. Testes são in-package e acessam diretamente.
- **`FirstByteTimeout == 0` desliga a camada** (simétrico a `IdleTimeout == 0` e `RequestTimeout == 0`), coberto por teste.

## Testes

- `TestLimits` — defaults em `New` (10m/5m/60s/2 + `HTTPClient.Timeout` 10m) e `SetLimits` substituindo tudo (`RequestTimeout: 0` → `Timeout: 0`)
- `TestChatStreamSuccess` — stream SSE saudável: conteúdo, deltas, finish reason, usage, retorno (sanidade sem leak)
- `TestChatStreamFirstByteTimeout` — stall pré-headers → `errors.Is(err, ErrFirstByte)` em ~100ms
- `TestChatStreamIdleTimeout` — headers + 2 chunks e trava → `errors.Is(err, ErrStreamIdle)` em ~150ms, conteúdo parcial preservado ("hello")
- `TestChatStreamHealthySpacedChunks` — chunks espaçados 100ms (< idle 400ms), total 400ms (> firstByte 150ms) → completa com "abcd" (prova que o timer de first-byte não mata o body pós-headers)
- `TestChatStreamRequestTimeoutCap` — `RequestTimeout` 150ms aborta stream lento SEM `ErrStreamIdle`/`ErrFirstByte` (regressão da semântica antiga de capa total)
- `TestChatStreamZeroTimeoutsDisableLayers` — `IdleTimeout: 0` sem watchdog; `FirstByteTimeout: 0` sem timer; só a capa total governa

Todos passam: `go test -race -count=5 ./internal/llm/` → ok (5 execuções consecutivas, ~8s total)

## Build

- `go build ./internal/llm/` → ok
- `go vet ./internal/llm/` → ok
- `gofmt -l internal/llm/` → sem diffs
- `go test -race ./internal/llm/` → ok

## Notas

- **Handlers de teste consomem `r.Body`** (`io.Copy(io.Discard, ...)`) + fallback de 5s em `hold`: o servidor HTTP do Go só detecta disconnect do cliente (background read → cancel de `r.Context()`) depois de consumir o request body; sem isso, `srv.Close()` deadlockava nos aborts pré-headers (comprovado com probe: cancel pré-headers com body não consumido = servidor NUNCA vê o disconnect). Higiene de teste, sem mudança no código sob teste — gateways reais leem o request antes de responder.
- `MaxRetries` armazenado mas não usado nesta task (retry é a task 3.0), conforme especificado.
- Critério "stream > 10m completa com `RequestTimeout: 0`" não é testável em CI; a semântica está coberta por `TestLimits` (0 → `Timeout: 0`) + `TestChatStreamHealthySpacedChunks` (stream excede a janela de first-byte e completa).
- `ListModels` intocado (ctx do chamador governa), parsing SSE inalterado, assinaturas públicas (`New`, `ChatStream`) inalteradas — sem impacto nos call sites de `main.go`/`internal/agent`/`internal/tui`.
