# Tech Spec — Input multi-linha + stream do output do bash

## Requisitos Atendidos

- REQ-001 — Prompt multi-linha
- REQ-002 — Histórico de prompts
- REQ-003 — Stream do output do bash
- REQ-004 — Bloco de output ao vivo na TUI
- REQ-005 — Transcript enxuto

## Resumo Executivo

Duas mudanças independentes num PRD só. **Multi-linha**: o `textinput` single-line é trocado por `bubbles/textarea` (mesma família de dependências já no `go.mod`) com keymap invertido explicitamente — `enter` envia, `shift+enter` quebra linha — altura automática de 1 a 8 linhas e viewport nunca menor que 3; histórico dos últimos 20 prompts navegável com ↑/↓ com input vazio. **Stream do bash**: `Registry` ganha `ExecuteStream(ctx, name, argsJSON, onLine)` mantendo `Execute` como wrapper; o bash escreve stdout+stderr em `io.MultiWriter` (buffer consolidado + line writer que emite cada linha via callback). O agent emite `EventToolOutput` com throttle de 50ms (linhas agrupadas), a TUI acumula um bloco ao vivo (últimas 15 linhas + `… +N lines`) sob a linha da tool em `colTextMuted`, e o `EventToolResult` final substitui o bloco pelo resultado consolidado. O transcript grava só o resultado final — o JSONL não cresce com eventos por linha.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/tui/tui.go`** (modificado — multi-linha): `input textinput.Model` → `textarea.Model`; keymap customizado (enter envia / shift+enter quebra); recálculo de alturas (prompt 1-8, viewport ≥ 3); `promptHistory []string` (máx. 20) + `histIdx`.
- **`internal/tools/bash.go`** (modificado): execução com `cmd.Stdout/cmd.Stderr = io.MultiWriter(buf, lineWriter)`; `lineWriter` divide em linhas e chama o callback; timeout de 120s e truncamento de 32k do resultado final preservados.
- **`internal/tools/tools.go`** (modificado): `ExecuteStream(ctx, name, argsJSON, onLine func(string)) (Result, error)`; `Execute` vira wrapper com `onLine = nil`.
- **`internal/agent/agent.go`** (modificado): novo `EventKind` `EventToolOutput`; coletor de linhas com throttle de 50ms; flush final antes do `EventToolResult`.
- **`internal/tui`** (modificado — bloco ao vivo): `liveLines []string` no Model; `EventToolOutput` appenda; `EventToolResult` substitui o bloco.

Fluxo de dados:

1. **Multi-linha**: `shift+enter` → `textarea.InsertString("\n")` → altura recalculada → viewport encolhe (mín. 3). `enter` → trim → histórico push → envio com quebras preservadas.
2. **Stream**: bash produz linha → `onLine` → coletor do agent → (≥ 50ms desde último emit) → `EventToolOutput{Tool, Text: linhas agrupadas}` → TUI appenda no bloco ao vivo → `EventToolResult` substitui o bloco.

## Design de Implementação

### Interfaces Principais

```go
func (r *Registry) ExecuteStream(ctx context.Context, name, argsJSON string, onLine func(string)) (Result, error)
```

- `Execute` mantém a assinatura atual como wrapper: `return r.ExecuteStream(ctx, name, argsJSON, nil)`.
- `onLine == nil` → comportamento idêntico ao atual (nenhuma alocação de line writer).

bash com stream:

```go
buf := &bytes.Buffer{}
var lineWriter io.Writer
if onLine != nil {
	lineWriter = newLineWriter(onLine)
}
cmd.Stdout = io.MultiWriter(buf, lineWriter)
cmd.Stderr = io.MultiWriter(buf, lineWriter)
```

- `newLineWriter(onLine)`: acumula bytes até `\n`, chama `onLine(linha)` sem o `\n`; flush do restante no fim (linha sem `\n` final).
- Timeout/exit status/truncamento de 32k: inalterados — o stream é aditivo ao comportamento consolidado.

Throttle no agent:

```go
const toolOutputThrottle = 50 * time.Millisecond

lines := make([]string, 0, 16)
var lastEmit time.Time
onLine := func(line string) {
	lines = append(lines, line)
	if time.Since(lastEmit) >= toolOutputThrottle {
		a.emit(Event{Kind: EventToolOutput, Tool: name, Text: strings.Join(lines, "\n")})
		lines = lines[:0]
		lastEmit = time.Now()
	}
}
```

- Flush final (`EventToolOutput` com as linhas restantes) antes do `EventToolResult` — nenhuma linha perdida entre o último throttle e o resultado.
- `time.Now` injetável no Agent para teste do throttle (campo `now func() time.Time`, default `time.Now`).

Multi-linha na TUI:

- `textarea.New()` com `SetMaxWidth`, `Prompt = ""`, `ShowLineNumbers = false`, `TextStyle` com `Background(colBgElement)` — visual idêntico ao atual.
- Keymap: interceptar em `handleChatKey` **antes** de delegar ao textarea: `tea.KeyEnter` sem shift → envia; `shift+enter` → `m.input.InsertString("\n")` e consumir a tecla (o default do textarea é o inverso — enter quebra — por isso a interceptação explícita).
- Altura: após cada mudança de conteúdo, `m.input.SetHeight(clamp(1, linhas, 8))`; viewport = `altura - promptHeight - chrome` com mínimo 3 (constante `minViewportRows` já implícita no `WindowSizeMsg` — extrair para reuso).
- Histórico: `promptHistory` append no envio (máx. 20, FIFO); ↑ com input vazio → `histIdx--` e carrega; ↓ → avança até voltar ao vazio; digitar qualquer coisa reseta a navegação.

### Modelos de Dados

```go
const EventToolOutput EventKind = "tool_output"
```

- `agent.Event` reusa `Tool` + `Text` (linhas agrupadas com `\n`) — sem campos novos.
- `session.Event`: **sem mudança** — `tool_output` não é gravado (REQ-005); só o `tool_result` final, como hoje.
- Model da TUI: `liveLines []string` + `liveOmitted int` — estado por valor, sem `strings.Builder` por valor (restrição do PRD).

### Renderização do bloco ao vivo

- Sob a linha `● bash(...)`, linhas em `colTextMuted`, indentadas como o resultado atual.
- Janela deslizante: manter últimas 15 linhas; excedentes contabilizados em `liveOmitted` → prefixo `… +N lines` no topo do bloco.
- Comando silencioso: nenhuma linha → nenhum bloco (spinner segue normal — REQ-004).
- `EventToolResult` → bloco ao vivo descartado, resultado consolidado renderizado como hoje (truncamento de 160 chars na linha).

## Pontos de Integração

- **Nenhuma externa** — stdlib + bubbles (já dependência).
- Contrato com o gateway inalterado: o LLM continua vendo só o resultado consolidado (`role: "tool"`); o stream é experiência de usuário, invisível ao modelo.

## Verificações Técnicas

### Segurança

- O stream não muda o modelo de execução do bash: mesmo timeout, mesmo truncamento, mesmo kill por contexto (techspec 001).
- Output do comando é renderizado como texto plano (sem interpretação ANSI — fora de escopo do PRD) — sem risco de escape de terminal.

### Arquitetura

- `ExecuteStream` é aditivo: `Execute` wrapper preserva todos os callers existentes (agent usará `ExecuteStream` sempre; outros tools ignoram `onLine`).
- Throttle no agent (não na tool): a tool emite linha a linha (barato), o agent decide o ritmo do canal — ponto único de política.
- Buffer do canal (512) + throttle de 50ms: pior caso de comando verboso (1000 linhas/s) → 20 eventos/s — margem folgada.
- Multi-linha não muda o contrato do agent: `Run(texto)` recebe `\n` — o gateway aceita conteúdo multi-linha como hoje (transcript já grava `content` string).

### Infraestrutura

- Sem novos requisitos — `bubbles/textarea` já está no `go.mod` (v1.0.0). Verificação: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

## Abordagem de Testes

### Testes Unidade

**`internal/tools/bash_test.go`**:

1. `TestExecuteStreamEmitsLinesInOrder` (mandatório): comando `for i in $(seq 1 10); do echo $i; sleep 0.05; done` → callbacks coletados = `[1..10]` na ordem; resultado final contém as 10 linhas.
2. `TestExecuteStreamMergesStdoutStderr`: comando com writes em stdout e stderr → linhas de ambos no stream e no consolidado.
3. `TestExecuteStreamNilCallbackMatchesExecute`: mesmo comando com `onLine = nil` → resultado idêntico ao `Execute`.
4. `TestExecuteStreamFinalLineWithoutNewline`: `printf sem-newline` → callback recebe a linha no flush.
5. `TestExecuteStreamTimeoutKillsProcess`: comando `sleep 300` com timeout curto → processo morto, erro de timeout (comportamento atual preservado).

**`internal/agent/agent_test.go`**:

6. `TestToolOutputThrottled`: `now` injetável — 100 linhas no mesmo instante → 1 `EventToolOutput` agrupado; avançar 60ms + mais linhas → 2º evento; flush final antes do `EventToolResult` com as linhas restantes.
7. `TestToolOutputNotWrittenToTranscript`: turno com bash verboso → JSONL sem eventos `tool_output`; `tool_result` presente (REQ-005).

**`internal/tui/tui_test.go`**:

8. `TestShiftEnterCreatesNewline`: `shift+enter` → valor do input contém `\n`; altura do prompt = 2.
9. `TestEnterSendsMultiline`: input com 2 linhas + `enter` → `agent.Run` recebe o texto com `\n` preservado.
10. `TestPromptHeightClamped`: 12 linhas digitadas → altura do prompt = 8; viewport ≥ 3 linhas.
11. `TestHistoryNavigation`: enviar 3 prompts → ↑ com input vazio carrega o 3º, de novo o 2º; ↓ volta; digitar cancela a navegação; 21º prompt descarta o mais antigo.
12. `TestLiveBlockAccumulatesAndReplaces`: `EventToolOutput` ×3 → bloco ao vivo contém as linhas em muted; 20 linhas → últimas 15 + `… +N lines`; `EventToolResult` → bloco substituído pelo resultado consolidado.
13. `TestSilentCommandNoLiveBlock`: `EventToolStart` + `EventToolResult` sem `EventToolOutput` → nenhum bloco ao vivo; spinner inalterado.

### Testes de Integração

Cobertos pelos testes de agent (6, 7) — tool→evento→transcript ponta a ponta com mock.

### Testes de E2E

Deferidos para `kspec-qa`: `for i in $(seq 1 10); do echo $i; sleep 0.3; done` mostra números um a um; `go test ./...` demorado com spinner + linhas ao vivo; `shift+enter` em prompt longo; ↑ recupera prompt anterior.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. **`internal/tools`**: `ExecuteStream` + line writer + testes de bash — fundação isolada.
2. **`internal/agent`**: `EventToolOutput` + coletor com throttle + flush + `now` injetável.
3. **`internal/tui`**: bloco ao vivo (acumulação, janela 15, substituição pelo resultado).
4. **`internal/tui`**: troca `textinput` → `textarea`, keymap, alturas, histórico.
5. **Verificação final**: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

- `bubbles/textarea` — já no `go.mod`; nenhuma dependência nova.
- Interação com techspec 002: `ExecuteStream` retorna `Result` (assinatura pós-002); se 002 ainda não implementado, retorna `string` e a mudança é mecânica depois.
- Interação com techspec 005: `mentionPrefix` opera sobre o texto multi-linha do textarea — compatível por receber o texto corrente.

## Monitoramento e Observabilidade

### Error Tracking

Erros do bash (exit status, timeout) seguem o fluxo atual — visíveis ao modelo no resultado e à TUI; o stream não introduz tipo de erro novo.

### Logging Estruturado

Transcript JSONL inalterado em shape: só `tool_result` final — o requisito "histórico enxuto" é a própria observabilidade (JSONL não infla).

### Health Checks / Métricas / Alertas

Não aplicável — TUI local.

## Considerações Técnicas

### Decisões Principais

1. **Interceptação de teclas na TUI em vez de remontar o keymap do textarea**: o default do componente é o inverso do desejado; interceptar `enter`/`shift+enter` em `handleChatKey` antes de delegar é explícito, testável e não luta contra a lib (PRD: "configurado explicitamente").
2. **Throttle no agent com agrupamento de linhas**: 1 evento a cada 50ms no pior caso — protege o loop da TUI sem perder linha (flush final garante completude).
3. **`io.MultiWriter` (buffer + line writer)**: uma única passada no pipe — o consolidado e o stream são consistentes por construção (mesma ordem de bytes).
4. **Histórico em memória (20)**: PRD explicita não persistir entre sessões — zero I/O.
5. **Bloco ao vivo descartado no resultado**: o histórico final fica idêntico ao de hoje — o live é efêmero por design (PRD: "o histórico final fica idêntico").

### Riscos Conhecidos

- **Bug conhecido do projeto**: `strings.Builder` por valor no Model do Bubble Tea — o bloco ao vivo usa `[]string` e o stream continua `*strings.Builder` por ponteiro (restrição documentada no PRD).
- **Textarea e largura**: quebra visual de linhas longas no prompt pode divergir da contagem lógica — altura calculada por linhas lógicas (`\n`), não visuais; aceitável.
- **Paste multi-linha**: tratamento especial de paste fora de escopo (PRD) — o textarea lida com `\n` do paste nativamente (quebra linhas), envio preserva.
- **Comando com output binário gigante**: truncamento de 32k no consolidado preservado; o stream emite linhas até o limite do buffer do processo — comportamento aceitável.

### Conformidade com Skills Padrões

- `architecture-ddd.md`: não aplicável — brownfield Go com packages `internal/`; a rule determina não impor DDD. Seção Bounded Context removida conforme o template.
- Rules TS/tests.md (Vitest)/logging.md: não aplicáveis — stack Go; padrões do repositório (`testing`, AAA, relógio injetável, harness `newTestModel`/`step`).
- Padrões do projeto: sem comentários no código, eventos via canal (buffer 512, emit não-bloqueante), receivers por valor na TUI, `strings.Builder` por ponteiro.
- Skills kspec aplicáveis: `kspec-tasks`, `kspec-implement`, `kspec-qa` (stream visível linha a linha, multiline, histórico), `kspec-pr-review`.

### Arquivos relevantes e dependentes

| Arquivo | Papel na feature |
| --- | --- |
| `internal/tools/tools.go` | `ExecuteStream` com `onLine`; `Execute` wrapper |
| `internal/tools/bash.go` | `io.MultiWriter` + line writer; timeout/truncamento preservados |
| `internal/tools/bash_test.go` | Ordem/contagem de linhas, merge stderr, flush, timeout |
| `internal/agent/agent.go` | `EventToolOutput`, coletor com throttle 50ms, flush final, `now` injetável |
| `internal/agent/agent_test.go` | Throttle, agrupamento, transcript enxuto |
| `internal/tui/tui.go` | textarea multi-linha, keymap, alturas, histórico, bloco ao vivo |
| `internal/tui/tui_test.go` | Shift+enter, envio multi-linha, clamp de altura, histórico, live block |
| `internal/session/session.go` | Sem mudança — só `tool_result` gravado |
