# Review 5.0 — Wiring de config + doctor

## Status: APROVADO COM RESSALVAS

## Verificação por subtask

- [x] 5.1 `main()` injeta `llmClient.SetLimits(llm.Limits{RequestTimeout, IdleTimeout, FirstByteTimeout, MaxRetries})` a partir dos campos resolvidos do cfg, dentro do `if cfg.Ready()` (cliente só existe quando configurado) (main.go:74-81); `ag.SetSubagentTimeout(cfg.Agent.SubagentTimeoutDuration)` aplicado junto aos setters de config existentes (`SetSquadPins`, `SetSquadLimits`), antes da TUI subir (main.go:101)
- [x] 5.2 `runDoctor` imprime `llm timeouts: first byte <FB>, idle <IDLE>, total <TOTAL>, retries <N>` e `agent: subagent stall <STALL>` na seção de config, após "skip tls verify" (main.go:194), via helper `doctorLimitsLines(cfg)` (main.go:267-272) usando os valores efetivos (`*Duration` resolvidos no `Load()` — o doctor não recalcula defaults); o cliente do check de gateway recebe os mesmos `SetLimits` (main.go:211-216)
- [x] 5.3 Verificação completa verde (ver seção Checks)

## Fix obrigatório: ressalva 1 da review 3.0 (contagem de chamadas defasada)

- `TestTruncationRescuesGiantToolResult`: `want 2` → `want 4 (3 retried summary attempts + rescued main call)`; `main := calls[1]` → `calls[3]` — exatamente como prescrito (agent_test.go:1569-1571)
- `TestSummaryFailureAbortsCompactionSilently`: **fix adaptado** (divergência deliberada da lista literal, ver Ressalvas): o teste é table-driven e o subcase "empty summary" falha em nível agent (stream HTTP válido com conteúdo vazio), sem retry HTTP — continua com 2 calls e main em `calls[1]`. O fix literal (2→4 global + `calls[3]`) quebraria esse subcase que hoje passa. A contagem virou por-caso (`wantCalls` no struct de casos): "gateway error" → 4 (main = `calls[3]`), "empty summary" → 2 (main = `calls[1]`) (agent_test.go:1482-1490, 1511-1521). A própria review 3.0 registra na nota da ressalva que "empty summary" passa com a contagem atual — a adaptação preserva a intenção do fix (o caminho de HTTP 500 do summary retenta 3× per REQ-002) sem regredir o caso sem retry
- Semântica verificação inalterada: compaction aborta silenciosamente, main call prossegue com tool result truncado, `turn_done` emitido, nenhum `EventError`/`EventCompaction`

## Testes

- `TestDoctorLimitsLines` (novo, main_test.go:234-263): config hermético via `XDG_CONFIG_HOME` em temp dir com `idle_timeout = "90s"` e `subagent_timeout = "2m"` → `config.Load()` → asserta saída exata `  llm timeouts: first byte 1m0s, idle 1m30s, total 10m0s, retries 2` + `  agent: subagent stall 2m0s` — cobre o critério de sucesso da task ("90s" → `idle 1m30s`) e os defaults no mesmo caminho real (TOML → duração → render)
- Testes existentes de `main_test.go`: inalterados e verdes — nenhum cobria a seção de config do doctor (os helpers testados eram `doctorTelemetryTable` e `doctorKspecSection`), nenhuma atualização necessária
- Testes corrigidos do agent: `go test -race -count=3` estável (subcase "gateway error" ~3s por causa do backoff default — custo registrado na review 3.0, ressalva 2; "empty summary" 0.01s)
- E2E manual: `go run . --doctor` com config default do usuário → `llm timeouts: first byte 1m0s, idle 5m0s, total 10m0s, retries 2` / `agent: subagent stall 10m0s`, "all checks passed"; com config custom (`idle_timeout = "90s"`, `request_timeout = "0s"`, `max_retries = 5`, `subagent_timeout = "3m"`) → `first byte 1m0s, idle 1m30s, total 0s, retries 5` / `subagent stall 3m0s` (exit 1 esperado: gateway dummy inalcançável)

## Checks

- `go build ./...` → ok
- `go vet ./...` → ok
- `gofmt -l .` → sem output
- `go test ./...` → ok (todos os pacotes)
- `go test -race -count=3 ./internal/agent/ -run 'TestSummaryFailureAbortsCompactionSilently|TestTruncationRescuesGiantToolResult'` → ok
- `go test -race -count=2 . -run 'TestDoctorLimitsLines'` → ok

## Ressalvas

1. **`rebuildAgent` na TUI (tui.go:1185) cria `llm.Client` sem `SetLimits`**: após salvar config pelo `/config`, o cliente reconstruído usa `defaultLimits` (60s/5m/10m/2) em vez dos valores configurados, até reiniciar. Fora do escopo desta task (requisitos exatos: `main()` + `runDoctor`; PRD/techspec/tasks não mencionam a TUI) — mantido como está, flagrado para follow-up do dono da feature. O formulário `/config` não edita os knobs de timeout, então o cfg mantém as durações resolvidas no boot; aplicar `SetLimits` ali seria um fix de consistência de ~4 linhas.
   - **CORRIGIDO pós-review (orquestrador)**: `rebuildAgent` agora aplica `SetLimits` com os campos resolvidos de `m.cfg` (tui.go:1182-1192) — o cliente pós-`/config` preserva os timeouts configurados.
2. **Renderização "first byte 1m0s" vs "60s" do exemplo da techspec**: `time.Duration.String()` renderiza 60s como `1m0s` (mesma duração, forma canônica do Go). A delegação endossou explicitamente `String()` ("renderiza 1m0s/5m0s/10m0s"); o exemplo literal da techspec era ilustrativo.
3. **Teste unitário adicional além do "N/A" da task**: `TestDoctorLimitsLines` + helper extraído `doctorLimitsLines` seguem o padrão existente de `doctorKspecSection`/`doctorTelemetryTable` (helpers extraídos justamente para testabilidade) e dão cobertura automatizada ao critério de aceite REQ-004 ("--doctor imprime os valores efetivos") em vez de depender só de E2E manual.

## Notas

- Wiring centralizado em `main()` no padrão dos setters existentes; sem comentários no código; stdlib only; código em inglês.
- Arquivos tocados: `main.go`, `main_test.go`, `internal/agent/agent_test.go` (fix prescrito da review 3.0). Mudanças pré-existentes do working tree (feature 012 não commitada + tasks 1-4) preservadas — nenhum restore/reset executado.
- Doctor continua com exit 1 quando o gateway está inalcançável (comportamento existente); os limites aplicados ao cliente do doctor não alteram o check na prática (ctx de 15s domina o primeiro byte e a capa), mas mantêm o check fiel à realidade do runtime — inclusive `request_timeout = "0s"` reflete no cliente do doctor (sem capa HTTP).
- Veredito APROVADO COM RESSALVAS: os requisitos da task 5.0 estão 100% atendidos e verificados; as ressalvas são um gap out-of-scope flagrado para follow-up (1) e registros de decisões deliberadas (2, 3) — nada pendente de correção dentro do boundary desta task.
