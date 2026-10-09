# Tech Spec — Resiliência de streaming (timeouts em camadas + retry)

## Requisitos Atendidos

- REQ-001 — Timeouts de streaming em camadas
- REQ-002 — Retry com backoff para falhas transientes
- REQ-003 — Timeout de subagente stall-based
- REQ-004 — Configuração e diagnóstico
- REQ-005 — Erros acionáveis na TUI

## Resumo Executivo

Substituímos o `http.Client.Timeout` único por três deadlines independentes dentro de `internal/llm`: primeiro byte (timer que cancela o contexto da requisição até os headers chegarem), idle do stream (watchdog que envolve o `resp.Body` e reseta um timer a cada leitura) e capa total (o próprio `http.Client.Timeout`, agora configurável). O retry com backoff vive dentro de `ChatStream`, que é o único ponto que sabe se deltas já foram emitidos ao usuário — retry apenas quando nada foi emitido. O timeout de subagente troca `context.WithTimeout` total por um watchdog resetado a cada evento drenado no `runSubagent`. Todos os knobs chegam via `config.toml`/env e são injetados com um setter, seguindo o padrão de setters existente (`SetPinned`, `SetRouters`). Sem novas dependências — apenas stdlib.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/llm` (modificado)** — `Client` ganha campos de limites (`firstByteTimeout`, `idleTimeout`, `requestTimeout`, `maxRetries`), o watchdog de idle sobre o body, o timer de primeiro byte em `do()`, a classificação de erros transientes e o loop de retry em `ChatStream`. Exporta os sentinels `ErrStreamIdle` e `ErrFirstByte` e o tipo `Limits`.
- **`internal/agent` (modificado)** — `runSubagent` substitui o ctx de parede total por watchdog de stall (timer resetado a cada evento drenado do canal `Events` do subagente); mensagem de resultado dinâmica; `subagentTimeout` default muda de 5m para 10m com nova semântica.
- **`internal/config` (modificado)** — `[llm]` ganha `request_timeout`, `idle_timeout`, `first_byte_timeout` (strings de duração) e `max_retries` (int); nova seção `[agent]` com `subagent_timeout`; env overrides; validação com erro claro.
- **`main.go` (modificado)** — wiring config → `llm.Client.SetLimits` e `agent.SetSubagentTimeout`; `--doctor` imprime valores efetivos.

Fluxo de dados: `config.Load()` → `main` → setters em `llm.Client`/`Agent` → comportamento em runtime. Nenhum dado persistido novo (transcript e telemetry inalterados).

## Design de Implementação

### Interfaces Principais

```go
type Limits struct {
    RequestTimeout time.Duration
    IdleTimeout    time.Duration
    FirstByteTimeout time.Duration
    MaxRetries     int
}

func New(baseURL, apiKey string, skipTLSVerify bool) *Client
func (c *Client) SetLimits(l Limits)

var ErrStreamIdle error
var ErrFirstByte error

func (a *Agent) SetSubagentTimeout(d time.Duration)
```

`New` mantém a assinatura atual e aplica os defaults (60s/5m/10m/2); `SetLimits` sobrescreve. `Agent` já tem campo `subagentTimeout`; ganha o setter público e o default passa a 10m.

### Mecanismos de timeout (em `llm`)

1. **Capa total**: permanece em `http.Client.Timeout` (semântica exata de capa total). `Limits.RequestTimeout == 0` → `Timeout: 0` (sem capa).
2. **Primeiro byte**: em `do()`, contexto derivado com `context.WithCancel(ctx)` + `time.AfterFunc(firstByteTimeout, cancel)`. O request usa esse contexto; quando `Do` retorna, `timer.Stop()`. O cancel é adiado para depois do consumo do body — cancelar o contexto do request aborta a leitura do body, então o cancel só roda após o body fechar. Se o timer disparou antes dos headers, o erro é classificado como `ErrFirstByte` (wrapped).
3. **Idle do stream**: `resp.Body` é envolvido por um `idleBody` que possui um `*time.Timer` resetado a cada `Read` bem-sucedido. Ao expirar, o watchdog cancela o contexto do request (desbloqueia o `Read` travado) e marca a causa; o `Read`/`scanner.Err()` retorna erro wrapped em `ErrStreamIdle`. O `bufio.Scanner` em `ChatStream` lê através do wrapper — nenhuma mudança no parsing SSE.

### Retry (em `ChatStream`)

- Loop de tentativas envolvendo `do()` + leitura do stream. Contador `emitted` incrementado a cada invocação de `onDelta`/conteúdo acumulado.
- **Pré-condição de retry**: `emitted == 0` e erro transiente. Com `emitted > 0`, qualquer erro retorna imediato (turno aborta como hoje).
- **Transiente**: `net.Error` (timeout/refused/reset), `io.EOF`/`io.ErrUnexpectedEOF` antes de `[DONE]`, HTTP 429/500/502/503/504, `ErrStreamIdle`, `ErrFirstByte`, capa total. Não-transiente: 4xx exceto 429 (401/403/404/400…), erros de decode de chunk.
- **Backoff**: exponencial com jitter — base 1s, fator 2, capa 30s, jitter ±20%. `Retry-After` (delta-seconds) prevalece quando presente e maior que o backoff; valor não parseável ignora o header.
- **Erro final**: `chat stream failed after N attempts (model X): <causa amigável>` — a causa amigável mapeia sentinels ("stream stalled for 5m0s without data", "no response headers within 60s — check the gateway").

### Erro estruturado de status

O tratamento atual de não-200 (`fmt.Errorf` com status+body) vira o tipo interno `statusError{code int, retryAfter time.Duration}` para alimentar a classificação e o `Retry-After`.

### Stall de subagente (em `agent.runSubagent`)

- Hoje: `ctx, cancel := context.WithTimeout(ctx, 5m)` total. Novo: watchdog com `time.AfterFunc(subagentTimeout, fire)` onde `fire` marca `stalled` (atomic) e cancela um contexto derivado.
- O drain goroutine existente (que já itera `sub.Events`) reseta o timer a cada evento recebido.
- Resultado: `stalled == true` → `"error: subtask stalled for 10m0s without progress"` (dinâmico com o valor configurado). Cancelamento vindo do pai (Esc) continua pelo caminho de abort existente — o watchdog não interfere.
- Interação com REQ-001/002: um stream saudável do subagente emite deltas → eventos → resets; um LLM travado é morto antes pelo idle/retry do `llm`. Tool `bash` longo (até 120s) não emite eventos, mas cabe folgado na janela de 10m.

### Modelos de Dados

`config.LLM` ganha `RequestTimeout string`, `IdleTimeout string`, `FirstByteTimeout string`, `MaxRetries int`; nova `config.Agent{SubagentTimeout string}`. Strings de duração parseadas com `time.ParseDuration` em `Load()` — valor inválido → erro apontando a chave (ex: `invalid llm.idle_timeout "5min": time: unknown unit "min"`). Env overrides aplicados após o arquivo, mesmo formato. Zero desliga a capa (`"0s"`); para `max_retries`, zero mantém o default (campo não preenchido) e valor negativo é rejeitado.

### Pontos de Integração

- Gateway OpenAI-compatible (SSE): nenhuma exigência nova; `Retry-After` é opcional e tratado como best-effort.
- Jev (`internal/jev`): intocado (SLA próprio de 30s).
- `--doctor`: imprime `llm timeouts: first byte 60s, idle 5m0s, total 10m0s, retries 2` e `agent: subagent stall 10m0s` na seção de config.

## Verificações Técnicas

### Segurança

Sem superfície nova: mesmos endpoints e headers. `Retry-After` parseado com limite superior (capa de 30s também para o header, evitando um gateway malicioso segurar o turno por horas). Nenhum segredo em mensagens de erro (o body de resposta não-transiente continua truncado como hoje).

### Arquitetura

Watchdogs são locais a cada chamada (`ChatStream`/`runSubagent`) — sem estado global, sem goroutines vazando (timers parados no defer). O retry fica no `llm` porque só ele conhece `emitted`; o `agent` não muda seu fluxo de erro. Ponto de falha restante: falha transiente após primeiro delta continua abortando o turno (aceito no PRD; re-render é evolução).

### Infraestrutura

Nenhuma. Binário único, stdlib only. Rollback = reverter o commit; defaults preservam o comportamento de capa total atual (10m).

## Abordagem de Testes

### Testes de Unidade (`internal/llm`)

Gateway mock via `httptest` com `Limits` curtos (ex: firstByte 100ms, idle 150ms):

- handler bloqueia antes de escrever headers → `ErrFirstByte`; com `maxRetries: 1` e handler que libera na 2ª chamada → sucesso com 2 tentativas.
- handler escreve headers + 2 chunks e trava (canal) → `ErrStreamIdle` dentro da janela.
- handler responde 429 com `Retry-After: 1` e depois 200 → retry honra o header (assertar intervalo entre tentativas).
- handler falha (500) após enviar 1 chunk com delta → única tentativa, sem duplicar `onDelta`.
- 401 → sem retry.
- stream saudável lento (chunks espaçados menores que idle) → completa mesmo excedendo firstByte total decorrido.

### Testes de Unidade (`internal/agent`)

- Adaptar `TestSubagentTimeout`: subagente silencioso (mock que bloqueia) → aborta com "stalled for … without progress".
- Subagente com progresso contínuo (chunks espaçados) sobrevive além da janela antiga de parede total.
- Esc durante subagente continua abortando o turno pai (regressão).

### Testes de Unidade (`internal/config`)

- Defaults aplicados sem config; parsing de `"90s"`/`"10m"`; duração inválida → erro nomeando a chave; env override prevalece; `"0s"` desliga capa.

### Testes E2E

Cobertura E2E manual: sessão real contra gateway com `idle_timeout = "5s"` forçando stall e observando a mensagem amigável na TUI. Sem TestSprite (app de terminal sem UI web).

## Sequenciamento de Desenvolvimento

1. **Config** (`internal/config`): structs, parsing, env, defaults, testes — fundação sem tocar runtime.
2. **`llm` timeouts**: `Limits`, `SetLimits`, first-byte e idle em `do()`/`ChatStream`, sentinels, testes de stall.
3. **`llm` retry**: `statusError`, classificação transiente, loop de tentativas com backoff/`Retry-After`, testes.
4. **Agent stall**: watchdog em `runSubagent`, setter, mensagem dinâmica, adaptação de testes.
5. **Wiring + doctor**: `main.go`, saída do doctor, verificação completa `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

Nenhuma bloqueante. Trabalha sobre o working tree atual (branch `013-prd-resiliencia-de-streaming`).

## Monitoramento e Observabilidade

- **Error tracking**: sem ferramenta externa; erros já fluem para a TUI (`emitError`) e o transcript JSONL (`turn_aborted`). As novas mensagens amigáveis caem no mesmo caminho.
- **Logging**: sem logs estruturados no projeto; nada novo exigido.
- **Métricas**: telemetria de TPS existente continua medindo streams completos; retries não distorcem (só a tentativa bem-sucedida registra TPS). Contador de retries por sessão fica como evolução futura.
- **Health checks**: `--doctor` passa a reportar os limites efetivos.

## Considerações Técnicas

### Decisões Principais

- **Watchdog de idle no body (app-level)** em vez de `Transport.ReadIdleTime`: aquele é health-check HTTP/2 com ping, não aborta leitura travada; o wrapper sobre `resp.Body` é o padrão idiomático client-side e funciona em HTTP/1.1 e 2.
- **Retry dentro de `ChatStream`**: único ponto com visibilidade de `emitted`; evita mudar o contrato do agent e duplicar lógica entre agent principal e subagentes.
- **Setter em vez de mudar `New`**: preserva os 4 call sites existentes e o padrão de setters do repo; `Limits` explícito evita construtor com 7 parâmetros.
- **Durações como strings TOML**: `time.ParseDuration` dá `"90s"`/`"10m"` legíveis; ints em ms (estilo Claude Code) foram rejeitados por serem menos legíveis no arquivo.
- **`Retry-After` com capa de 30s**: um header de gateway não deve poder segurar o turno indefinidamente.

### Riscos Conhecidos

- **Timer de idle vs. chunks SSE espaçados**: modelos com thinking longo podem espaçar chunks; default de 5m (igual ao Claude Code) absorve. Mitigação: knob `idle_timeout`.
- **Race no watchdog de stall** (timer dispara enquanto evento chega): resolvido com stop/reset idempotentes e flag atômica; testado com `-race`.
- **`context.Canceled` ambíguo** (stall vs. Esc): flag `stalled` distingue; caminho de abort do pai inalterado.
- **Testes de timing flaky**: janelas de teste ≥100ms e asserts com tolerância (padrão já usado em `bash_test.go`).

### Conformidade com Skills Padrões

Rules de `.agents/rules/` são orientadas a TypeScript/Java; para o Go do kterminal prevalecem os padrões do código-base (AGENTS.md): sem comentários, eventos via canal, receivers por valor na TUI, `strings.Builder` por ponteiro.

### Arquivos relevantes e dependentes

- `internal/llm/llm.go` — timeouts em camadas, sentinels, retry (núcleo da feature)
- `internal/llm/llm_test.go` — testes de stall/retry/backoff
- `internal/agent/agent.go` — `runSubagent` (watchdog stall), `New` (default 10m), setter
- `internal/agent/agent_test.go` — `TestSubagentTimeout` e novos casos
- `internal/config/config.go` + `internal/config/config_test.go` — knobs, parsing, env
- `main.go` — wiring e `--doctor`
- `.docs/13-resiliencia-de-streaming.md`, `spec/tasks/013-prd-resiliencia-de-streaming/prd.md` — brief e PRD (referência)
