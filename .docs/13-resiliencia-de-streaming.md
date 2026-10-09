# 13 — Resiliência de streaming (timeouts em camadas + retry)

## Contexto

Usuários do kterminal relatam `context deadline exceeded (Client.Timeout or context cancellation while reading body)` durante sessões agênticas. Causa raiz: `http.Client.Timeout = 10 * time.Minute` fixo em `internal/llm/llm.go` é um deadline *total* (conexão + TLS + headers + corpo inteiro) que não distingue stream travado de stream longo-saudável — ambos morrem aos 10 min. Agravante: `subagentTimeout = 5 * time.Minute` em `internal/agent/agent.go` é relógio de parede total para o subtask inteiro (todas as chamadas de LLM + tools), então subtasks agênticas legítimas morrem no meio. Não há retry: o turno aborta e o usuário recomeça do snapshot.

Prior art (Claude Code): timeout total por requisição (`API_TIMEOUT_MS`, default 10 min) + dois watchdogs de idle em streaming (nível-evento ≥5 min, nível-byte 10s–30 min) + deadline de primeiro byte + retry com backoff exponencial (default 10 tentativas) + timeout de subagente *stall-based* (`CLAUDE_ASYNC_AGENT_STALL_TIMEOUT_MS`, default 10 min, timer reseta a cada evento de progresso). Stream travado morre em minutos; stream longo-saudável sobrevive.

Arquitetura relevante:

- `internal/llm/llm.go` — `Client` com `http.Client`, `do()` e `ChatStream()` (SSE via `bufio.Scanner`).
- `internal/agent/agent.go` — `runSubagent()` aplica `context.WithTimeout` total; `runLoop()` trata erro de `ChatStream` com `emitError` + aborto de turno.
- `internal/config/config.go` — TOML `[llm]` com `base_url`, `api_key`, `skip_tls_verify`; sem knobs de timeout.
- `main.go` — wiring `config → llm.New()`; `--doctor` imprime configuração.

## Objetivo

Substituir o deadline único por timeouts em camadas (primeiro byte, idle do stream, capa total), adicionar retry com backoff para falhas transientes antes do primeiro delta, e mudar o timeout de subagente para stall-based — tudo configurável via `config.toml`.

## Especificação

1. **Timeouts em camadas no `llm.Client`**:
   - `first_byte_timeout` (default 60s): cobre conexão + TLS + headers. Timer `time.AfterFunc` cancela o ctx da requisição se a resposta não chegar; timer parado quando `do()` retorna. O ctx permanece vivo durante a leitura do corpo.
   - `idle_timeout` (default 5m): watchdog sobre o `resp.Body` — cada `Read` bem-sucedido reseta o timer; ao expirar sem bytes, cancela o ctx e retorna o sentinel `ErrStreamIdle`.
   - `request_timeout` (default 10m, `0` = sem capa): permanece como `http.Client.Timeout` (semântica de capa total, retro-compatível).
2. **Retry com backoff dentro de `ChatStream`**: erros transientes (net errors, `io.EOF`/`ErrUnexpectedEOF`, HTTP 429/500/502/503/504, deadlines dos timers) são retentados **apenas se nenhum delta foi emitido ao usuário** (callback `onDelta` nunca chamado e conteúdo acumulado vazio). Backoff exponencial com jitter, honrando o header `Retry-After` quando presente. `max_retries` default 2. Esgotadas as tentativas, erro final envolvido com mensagem acionável (modelo, tentativas, causa).
3. **Subagente stall-based**: `runSubagent()` troca `context.WithTimeout` total por watchdog — timer de `subagent_timeout` (default 10m) resetado a cada evento drenado do canal `Events` do subagente; silêncio total pela janela → cancela. Mensagem de resultado muda de `error: subtask timed out after 5m` para dinâmica (`error: subtask stalled for 10m without progress`).
4. **Config**: seção `[llm]` ganha `request_timeout`, `idle_timeout`, `first_byte_timeout` (strings de duração `"10m"`/`"90s"` via `time.ParseDuration`) e `max_retries` (int); nova seção `[agent]` com `subagent_timeout`. Env overrides `KTERMINAL_LLM_REQUEST_TIMEOUT`, `KTERMINAL_LLM_IDLE_TIMEOUT`, `KTERMINAL_LLM_FIRST_BYTE_TIMEOUT`, `KTERMINAL_LLM_MAX_RETRIES`, `KTERMINAL_AGENT_SUBAGENT_TIMEOUT`. Defaults aplicados em `Load()`.
5. **Wiring e doctor**: `main.go` passa os knobs ao `llm.Client`; `--doctor` imprime os valores efetivos.
6. **Erros distinguíveis**: `llm` exporta `ErrStreamIdle` e `ErrFirstByteTimeout`; o agente renderiza mensagens amigáveis em vez do erro cru do Go.

## Critérios de aceitação

- Stream que entrega headers e para de enviar bytes é abortado em `idle_timeout` com `ErrStreamIdle` — sem esperar a capa total.
- Gateway que não responde headers em `first_byte_timeout` falha rápido e é retentado.
- 429/5xx/conexão recusada antes do primeiro delta → retry com backoff; a TUI não mostra texto duplicado.
- Falha após o primeiro delta → sem retry; turno aborta como hoje (snapshot preservado).
- Stream saudável mais longo que o antigo 10min sobrevive quando `request_timeout = 0` ou maior.
- Subagente que emite progresso (deltas, tools) continuamente sobrevive além de 10 min de relógio de parede; subagente silencioso é abortado em `subagent_timeout` com a nova mensagem.
- Todos os knobs funcionam via `config.toml` e env; `--doctor` os exibe.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: gateway mock via `httptest` — (a) bloqueia antes dos headers → `ErrFirstByteTimeout` + retry; (b) envia 2 chunks e trava → `ErrStreamIdle` em `idle_timeout` curto de teste; (c) 429 com `Retry-After` e depois sucesso → retry honra o header; (d) falha após delta emitido → única tentativa; (e) stream longo saudável com timeouts curtos → completa. Agente: subagente com progresso lento contínuo sobrevive; subagente silencioso aborta com mensagem de stall. Config: defaults, parsing de durações inválidas (erro claro), env overrides.

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Retry não re-renderiza parcial na TUI (fora de escopo; evolução natural com evento de re-render).
- Sem mudança no cliente Jev (`internal/jev`, 30s) — roteamento tem SLA próprio.
- Defaults conservadores alinhados ao Claude Code; valores menores só via config.
