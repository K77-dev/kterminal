# Tech Spec — Interromper tarefa em execução (Esc)

## Requisitos Atendidos

- REQ-001 — Cancelamento do turno agêntico via Esc
- REQ-002 — Tecla Esc na TUI e estados
- REQ-003 — Renderização do turno abortado e feedback visual

## Resumo Executivo

O cancelamento é implementado com `context.Context`: cada `Run()` cria um contexto cancelável derivado de `context.Background()`, guardado no `Agent` e disparado pelo novo método público `Cancel()`. O contexto cobre todo o ciclo do turno — `ensureCandidates`, `decide` (Jev), `ChatStream` (o HTTP do gateway aborta nativamente via `http.NewRequestWithContext`) e execução de tools, cujas assinaturas (`Executor`, `Registry.Execute`) passam a receber `ctx`; o bash deriva seu timeout do contexto pai, matando o processo via `exec.CommandContext`. O loop do Agent é o ponto único de classificação: qualquer retorno com `ctx.Err() != nil` (ou `errors.Is(err, context.Canceled)`) segue o caminho de abort — emite `EventTurnAborted` com o texto parcial acumulado, grava `turn_aborted` no transcript e encerra — nunca `EventError`. Para preservar o contrato OpenAI, tool calls sem resultado no momento do abort recebem tool result sintético `"user aborted this turn"`. Na TUI, `Esc` com agente ocupado chama `Cancel()`; no prompt de confirmação, recusa o tool pendente e cancela o turno; o turno abortado renderiza o parcial como markdown com meta line `⊘ interrompido`, e a hint bar exibe `esc to interrupt` enquanto ocupado.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/agent`** (modificado — núcleo da feature): campos `mu sync.Mutex` + `cancel context.CancelFunc`; método público `Cancel()`; `loop` passa a receber `ctx`; novo `EventKind` `EventTurnAborted`; acumulação do texto parcial do stream; tool results sintéticos; classificação central de cancelamento; evento `turn_aborted` no transcript.
- **`internal/tools`** (modificado): `Executor` e `Registry.Execute` recebem `context.Context`; `bash` deriva o timeout do contexto pai em vez de `context.Background()` — o kill do processo acontece pelo próprio `exec.CommandContext`.
- **`internal/tui`** (modificado): `Esc` no estado chat (cancela se `busy`, no-op se ocioso) e no estado confirm (recusa + cancela o turno); caso `agent.EventTurnAborted` no `handleAgentEvent`; meta line `⊘ interrompido`; hint bar com `esc to interrupt`.
- **`internal/session`** (sem mudança estrutural): novo valor de `Type` — `"turn_aborted"` — usando os campos existentes (`Content` carrega o parcial).
- **`internal/llm`**, **`internal/router`**, **`internal/jev`** (sem mudança): `ChatStream`, `Route` e `Decide` já aceitam `ctx` e propagam o cancelamento ao HTTP.

Fluxo de dados do abort:

1. TUI recebe `Esc` → `agent.Cancel()` → cancelFunc do turno corrente (idempotente).
2. Contexto cancelado → conexão HTTP do `ChatStream`/`Route` fechada pelo `net/http`; processo bash morto (SIGKILL via `CommandContext`).
3. A operação bloqueada retorna; o loop detecta `ctx.Err() != nil` → caminho de abort.
4. O loop anexa tool results sintéticos pendentes, emite `EventTurnAborted{Text: parcial}` e grava `session.Event{Type: "turn_aborted", Content: parcial}`.
5. A TUI renderiza o parcial como markdown + meta `⊘ interrompido`, `busy = false`, input liberado.

## Design de Implementação

### Interfaces Principais

```go
type Executor func(ctx context.Context, args map[string]any) (string, error)

func (r *Registry) Execute(ctx context.Context, name, argsJSON string) (string, error)

func (a *Agent) Cancel()
```

Estrutura do cancelamento no Agent:

```go
type Agent struct {
	// campos existentes...
	mu     sync.Mutex
	cancel context.CancelFunc
}

func (a *Agent) Run(userInput string) {
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
	go a.loop(ctx, userInput)
}

func (a *Agent) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}
```

Regras do `loop(ctx, userInput)`:

- Verificar `ctx.Err() != nil` imediatamente após cada operação bloqueante: `ensureCandidates`, `decide`, `ChatStream`, `<-ch` (confirmação de tool) e `Tools.Execute`. Cancelado → caminho de abort.
- Classificação central: erro com `errors.Is(err, context.Canceled) || ctx.Err() != nil` → abort; qualquer outro erro → `emitError` (comportamento atual).
- Parcial: `var partial strings.Builder` acumulado no callback de deltas do `ChatStream` (além do `EventDelta` emitido).
- Caminho de abort, em ordem: (1) para cada tool call do passo corrente ainda sem resultado anexado, `a.messages = append(a.messages, llm.Message{Role: "tool", Content: "user aborted this turn", ToolCallID: tc.ID})`; (2) `Session.Write(session.Event{Type: "turn_aborted", Model: ..., Content: partial.String()})`; (3) `emit(Event{Kind: EventTurnAborted, Model: ..., Text: partial.String()})`; (4) return.
- Borda: `ChatStream` retorna normalmente mas o ctx foi cancelado no mesmo instante — com tool calls, o abort vence e nada é anexado ao histórico; sem tool calls, o turno é concluído (`EventTurnDone`) — o trabalho já terminou.

bash tool — única mudança é a origem do contexto:

```go
Execute: func(ctx context.Context, args map[string]any) (string, error) {
	// validação do comando...
	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	// resto inalterado
}
```

`DeadlineExceeded` continua tratado como timeout próprio; `Canceled` resulta em processo morto (`cmd.Run` retorna erro de signal) — o retorno é descartado pelo caminho de abort do Agent, que é o ponto único de classificação.

### Modelos de Dados

```go
const EventTurnAborted EventKind = "turn_aborted"
```

- `agent.Event`: reutiliza campos existentes — `Kind: EventTurnAborted`, `Text` (parcial acumulado), `Model`.
- `session.Event{Type: "turn_aborted", Model: ..., Content: parcial}` — permite ao revisor do transcript distinguir turno abortado de concluído; sem mudança estrutural no struct.
- Mensagem sintética: `llm.Message{Role: "tool", Content: "user aborted this turn", ToolCallID: tc.ID}` — mantém o par `tool_calls` ↔ `tool results` exigido pelo gateway OpenAI-compatible.

## Pontos de Integração

- **Gateway LiteLLM**: `ChatStream` e `ListModels` já usam `http.NewRequestWithContext` — o cancelamento fecha a conexão e interrompe o stream SSE nativamente, sem mudança de protocolo. Modo de falha residual (servidor lento para fechar) é irrelevante: o loop já retornou; o request morre em background.
- **Jev (Typesafe)**: `Route(ctx, ...)` → `Decide(ctx, ...)` — cancelamento no meio do roteamento aborta a chamada HTTP.
- **Idempotência**: `Cancel()` é seguro para chamadas múltiplas (cancelFunc é idempotente); `Esc` repetido no mesmo turno não causa erro.

## Verificações Técnicas

### Segurança

- Abortar não desfaz efeitos de tools mutantes já executados (fora de escopo, documentado no PRD). O kill do bash usa o `exec.CommandContext` do stdlib — sem superfície nova.
- `Cancel()` não expõe estado interno; mutex protege o cancelFunc contra acesso concorrente.

### Arquitetura

- Ponto único de classificação de cancelamento no Agent — tools e LLM apenas propagam; a TUI apenas chama `Cancel()`. Direção de dependência inalterada (`tui → agent → llm/tools/router/session`).
- Padrões do projeto preservados: eventos via canal (buffer 512, emit não-bloqueante), receivers por valor na TUI, `strings.Builder` sempre por ponteiro, sem comentários no código.
- `EventTurnAborted` usa o mesmo emit não-bloqueante; descarte só é possível com buffer saturado — cenário irreal com a drenagem contínua da TUI (aceito como risco residual).

### Infraestrutura

- Sem novos requisitos — apenas stdlib (`context`, `sync`, `errors`). Verificação obrigatória: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

## Abordagem de Testes

### Testes Unidade

**Agent** (`internal/agent/agent_test.go`), estendendo os harnesses existentes (`mockGateway`, `mockJev`, `collectEvents`):

1. `TestAgentCancelAbortsStream` (mandatório do PRD): handler do gateway envia um chunk de conteúdo e bloqueia em `<-r.Context().Done()`; `Run` → aguardar `EventDelta` → `Cancel` → assertar `EventTurnAborted` com o parcial, ausência de `EventError` e ausência de goroutine vazando (um novo `Run` contra gateway normal completa dentro de timeout — prova que o loop anterior terminou e o estado está limpo).
2. `TestAgentCancelDuringConfirmAbortsTurn`: `Confirm = true`, gateway responde com tool call mutante (bash); aguardar `EventConfirm`; simular o Esc da TUI (enviar `false` no `ApproveCh` + `Cancel`); assertar `EventTurnAborted` sem `EventError`; um segundo `Run` com resposta normal completa — o mock captura o request e confirma que a última mensagem do histórico é `role: tool` com o conteúdo sintético (contrato OpenAI preservado).
3. `TestAgentCancelWhenIdleIsNoOp`: `Cancel()` antes de qualquer `Run` — sem panic; `Run` posterior funciona.
4. `TestAgentDoubleCancelIsIdempotent`: dois `Cancel` no mesmo turno — sem erro, um único `EventTurnAborted`.

**TUI** (`internal/tui/tui_test.go`), seguindo o padrão `newTestModel`/`step`; para os casos de cancelamento, construir o agent com o gateway bloqueante (mesmo harness dos testes do agent) e dirigir o estado via `agentEventMsg`:

5. `TestEscCancelsWhenBusy`: `busy = true` + `tea.KeyEsc` → `EventTurnAborted` chega no canal de eventos; `busy = false` ao final.
6. `TestEscNoOpWhenIdle`: `busy = false` + `tea.KeyEsc` → nenhum estado alterado, sem panic.
7. `TestEscInConfirmDeclinesAndCancels`: estado confirm com `pendingConfirm` + `tea.KeyEsc` → `ApproveCh` recebe `false` e o turno aborta (`EventTurnAborted`); `n` sozinho apenas recusa o tool (comportamento atual preservado).
8. `TestHintBarShowsEscToInterruptWhenBusy`: render com `busy = true` contém `esc to interrupt`; ocioso não contém.
9. `TestTurnAbortedRendersMarker`: deltas + `EventTurnAborted` → conteúdo renderizado contém o parcial e `interrompido`; stream resetado; `busy = false`.

### Testes de Integração

Fora do escopo escolhido. A morte do processo bash via contexto (critério REQ-001) é garantida pelo design (`exec.CommandContext` com ctx pai) e validada no QA.

### Testes de E2E

Deferidos para `kspec-qa`: Esc durante stream longo, Esc durante `sleep 60` (processo morto), nova mensagem pós-abort sem resíduos, latência percebida ≤ 100ms, marcador textual legível no conteúdo plano copiável.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. **`internal/tools`**: mudar assinaturas (`Executor`, `Registry.Execute`) e derivar o timeout do bash do ctx pai — fundação sem a qual o cancelamento não alcança os processos; atualizar `fs.go` mecanicamente.
2. **`internal/agent`**: contexto cancelável em `Run`, `Cancel()`, `EventTurnAborted`, classificação central, parcial, tool results sintéticos, transcript.
3. **`internal/tui`**: Esc nos estados chat/confirm, caso `EventTurnAborted`, meta `⊘ interrompido`, hint bar `esc to interrupt`.
4. **Testes**: agent (mandatório do PRD + confirm + no-op + idempotência) e TUI (key-handling + renderização).
5. **Verificação final**: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

- Nenhuma externa — apenas stdlib (`context`, `sync`, `errors`).
- `ChatStream`/`Route`/`Decide` já aceitam ctx — sem dependências bloqueantes.

## Monitoramento e Observabilidade

### Error Tracking

Cancelamentos nunca são rastreados como erro: não geram `EventError` nem `session.Event{Type: "error"}`. O abort é evento de primeira classe (`turn_aborted`), distinguível no transcript.

### Logging Estruturado

O transcript JSONL é o mecanismo de observabilidade da sessão: `turn_aborted` carrega `ts`, `model` e `content` (parcial). Nenhum dado sensível além do que já é gravado hoje (conteúdo da conversa — sensibilidade inerente ao transcript existente).

### Health Checks / Métricas / Alertas

Não aplicável — aplicação local TUI, sem servidor. Custo de sessão e tok/s continuam computados como hoje; turnos abortados não contabilizam custo (o usage do stream cancelado não é contabilizado — comportamento natural do fluxo existente, preservado).

## Considerações Técnicas

### Decisões Principais

1. **Contexto cancelável por turno** em vez de canal de stop dedicado: reutiliza a propagação nativa do `context` até o socket HTTP e o `exec` — nenhuma máquina nova de sinalização, sem polling.
2. **Mudança de assinaturas** em `Executor`/`Registry.Execute` (confirmado): explícito e idiomático Go. Alternativa de campo de contexto no Registry rejeitada — mutação compartilhada race-prone.
3. **Tool result sintético** `"user aborted this turn"` (confirmado): preserva o contrato OpenAI (todo tool_call exige tool result) e informa o modelo do estado — evita re-executar tool mutante já aplicado. Truncar o turno foi rejeitado: perder tool calls concluídos arrisca re-execução; perder a user message descartava o contexto do pedido.
4. **Meta line `⊘ interrompido · <model>`** (confirmado): espelha o meta de turno concluído (`▣`), diferenciação imediata e textualmente legível no conteúdo plano copiável (acessibilidade do PRD).
5. **Classificação centralizada no Agent**: tools podem retornar qualquer erro; só o loop decide abort vs `EventError` — ponto único da verdade, facilita o requisito "nenhum erro espúrio".

### Riscos Conhecidos

- **Race Esc ↔ conclusão natural**: se o turno conclui entre o Esc e o processamento, `EventTurnDone` chega e o `Cancel()` vira no-op sobre contexto morto. Benigno — sem erro exibido.
- **Esc durante `ensureCandidates`** (timeout de 15s): cancelamento propaga; abort emitido. Coberto pela classificação central.
- **Deltas em race pós-cancelamento**: chunks já bufferados podem ser entregues antes do retorno do scanner — ficam no parcial acumulado (desejado: nada se perde do que o usuário viu).
- **Emit não-bloqueante** pode descartar `EventTurnAborted` com buffer 512 cheio — irreal com drenagem contínua; risco residual aceito.
- **`ctrl+c` durante turno ocupado** encerra o app sem abort gracioso — comportamento atual preservado (fora de escopo).

### Conformidade com Skills Padrões

- `architecture-ddd.md`: não aplicável — a própria rule determina não impor DDD em brownfield com arquitetura consistente; o kterminal usa packages `internal/` em Go, padrão que prevalece. Seção Bounded Context do template removida conforme instrução do template.
- Rules TS/Java/Angular/React/tests.md (Vitest): não aplicáveis — stack Go; o padrão de testes é o do repositório (`testing` + `httptest`, AAA, independência entre testes).
- Skills do fluxo kspec aplicáveis: `kspec-tasks` (decomposição), `kspec-implement` (execução), `kspec-qa` (E2E: Esc em stream, Esc em bash longo, latência ≤ 100ms), `kspec-pr-review` (revisão semântica contra PRD/Tech Spec).
- Padrões do projeto (PRD): sem comentários no código, eventos via canal, receivers por valor na TUI, `strings.Builder` por ponteiro.

### Arquivos relevantes e dependentes

| Arquivo | Papel na feature |
| --- | --- |
| `internal/agent/agent.go` | Núcleo: ctx cancelável, `Cancel()`, `EventTurnAborted`, classificação, sintéticos, transcript |
| `internal/agent/agent_test.go` | Teste mandatório do PRD + confirm + no-op + idempotência |
| `internal/tools/tools.go` | Assinaturas com ctx (`Executor`, `Registry.Execute`) |
| `internal/tools/bash.go` | Timeout derivado do ctx pai; kill via `CommandContext` |
| `internal/tools/fs.go` | Assinatura atualizada mecanicamente (ctx não usado — tools rápidos) |
| `internal/tui/tui.go` | Esc chat/confirm, `EventTurnAborted`, meta `⊘`, hint bar |
| `internal/tui/tui_test.go` | Testes de key-handling e renderização |
| `internal/session/session.go` | Sem mudança estrutural; novo `Type` `"turn_aborted"` |
| `internal/llm/llm.go` | Sem mudança — `ChatStream` já aborta com ctx |
| `internal/router/router.go` / `internal/jev/jev.go` | Sem mudança — `Route`/`Decide` já aceitam ctx |
