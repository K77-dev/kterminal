# Review 1.0 — Config: knobs de resiliência (durações, env, validação)

## Status: APROVADO

## Verificação por subtask

- [x] 1.1 Campos em `config.LLM` (`RequestTimeout`/`IdleTimeout`/`FirstByteTimeout` string, `MaxRetries` int, tags `request_timeout`/`idle_timeout`/`first_byte_timeout`/`max_retries`) e nova `config.Agent{SubagentTimeout string}` com tag `subagent_timeout`; `Agent Agent toml:"agent"` em `Config`
- [x] 1.2 Helper `parseDurationField(key, value)` — `time.ParseDuration` com erro no formato `invalid <chave> "<valor>": <causa>` (ex: `invalid llm.idle_timeout "5min": time: unknown unit "min" in duration "5min"`)
- [x] 1.3 Defaults em constantes (10m/5m/60s/2/10m) aplicados em `Load()`; env overrides aplicados após o arquivo (`KTERMINAL_LLM_REQUEST_TIMEOUT`, `KTERMINAL_LLM_IDLE_TIMEOUT`, `KTERMINAL_LLM_FIRST_BYTE_TIMEOUT`, `KTERMINAL_LLM_MAX_RETRIES`, `KTERMINAL_AGENT_SUBAGENT_TIMEOUT`); `max_retries` 0/ausente → default 2, negativo → erro
- [x] 1.4 Testes: defaults sem arquivo; parsing `"90s"`/`"10m"`/`"0s"`; duração inválida citando a chave; env prevalece sobre arquivo; `max_retries` negativo rejeitado

## Campos resolvidos para o wiring (task 5.0)

- `cfg.LLM.RequestTimeoutDuration`, `cfg.LLM.IdleTimeoutDuration`, `cfg.LLM.FirstByteTimeoutDuration` (`time.Duration`), `cfg.LLM.MaxRetries` (int já resolvido) e `cfg.Agent.SubagentTimeoutDuration` — preenchidos em `Load()`, prontos para `SetLimits`/`SetSubagentTimeout`
- `"0s"` parseia para `0` (capa total desligada em runtime — semântica definida pelo `llm` na 2.0)

## Desvios e decisões

- **`toml:"-"` nos campos resolvidos** (em vez de literalmente sem tag): verificado no fonte do BurntSushi v1.6.0 que campos exportados sem tag **são** serializados por nome de campo — campos sem tag vazariam no `Save()` como `RequestTimeoutDuration = 600000000000`, poluindo o `config.toml` do usuário. `toml:"-"` exclui do encode e do decode (encode.go:649, type_fields.go:95), preservando a intenção "fora da superfície TOML, preenchido em Load()". Guardado por `TestSaveOmitsResolvedDurationFields`.
- **Env `max_retries` inválido (não-numérico) → erro nomeando `llm.max_retries`**: mesma política de erro claro da techspec para durações; parse via `strconv.Atoi` no ponto do env.
- Testes extras além dos 5 exigidos: env negativo, env não-numérico, env com duração inválida (erro cita a chave TOML, não o nome do env) e guard de Save.

## Testes

- `TestLoadResilienceDefaults` — sem arquivo: 10m/5m/60s/2/10m resolvidos
- `TestLoadResilienceDurationsParsed` — `"90s"`/`"10m"`/`"0s"` parseados; strings brutas preservadas
- `TestLoadInvalidDuration` — `idle_timeout = "5min"` → erro citando `llm.idle_timeout` e `"5min"`
- `TestLoadEnvOverridesFile` — os 5 env prevalecem sobre o arquivo
- `TestLoadNegativeMaxRetriesRejected` — `max_retries = -1` no arquivo → erro citando `llm.max_retries`
- `TestLoadEnvNegativeMaxRetriesRejected` / `TestLoadEnvInvalidMaxRetriesRejected` / `TestLoadEnvInvalidDurationRejected` — variantes via env
- `TestSaveOmitsResolvedDurationFields` — `Save()` não escreve campos resolvidos

Todos passam: `go test ./internal/config/` → ok (18 tests, também com `-race -count=1`)

## Checks

- `go build ./internal/config/` → ok
- `go vet ./internal/config/` → ok
- `gofmt -l internal/config/` → vazio
- `go test ./internal/config/` → ok

Escopo da verificação restrito a `internal/config` conforme combinado — `internal/llm`, `internal/agent` e `main.go` estão sendo editados em paralelo por outros agents (tasks 2.0-5.0) e não foram tocados.

## Critérios de sucesso da task

- `config.Load()` sem arquivo retorna todos os defaults resolvidos ✅
- Arquivo com `idle_timeout = "5min"` falha o boot citando `llm.idle_timeout` ✅
- Env `KTERMINAL_LLM_IDLE_TIMEOUT=90s` prevalece sobre o arquivo ✅
- `go test ./internal/config/` verde ✅
