# Tech Spec — Subagentes (tool `task`)

## Requisitos Atendidos

- REQ-001 — Tool `task`
- REQ-002 — Subagente isolado
- REQ-003 — Eventos aninhados
- REQ-004 — Limites e timeout
- REQ-005 — Cancelamento compartilhado
- REQ-006 — TUI e transcript

## Resumo Executivo

O `Agent` ganha um campo `depth` e uma fábrica `newSubagent()` que compartilha a infraestrutura (LLM, routers, catálogo, sessão, telemetria, configuração de confirmação) mas cria estado próprio: `messages` zeradas iniciadas com prompt de sistema específico + description + guidance, canal `Events` próprio, `depth = parent+1` e limite de 10 steps. A tool `task` é registrada **apenas no agente principal** (`depth == 0` — subagentes não a veem nas definições) e executa `RunSync`: uma variante do loop que roda sincronamente (sem goroutine — requisito não negociável da v1), re-emite os eventos do subagente no canal principal com `Depth: 1` e `ParentTool: "task"`, e devolve o texto final como resultado da tool. Timeout de 5 minutos via contexto derivado do turno; estouro de steps ou timeout devolve erro **como resultado** — o agente principal segue vivo. Esc cancela o turno inteiro (contexto compartilhado). A TUI renderiza eventos aninhados com indentação de 2 espaços em `colSecondary` e o hint bar mostra `subagent running`; o transcript grava os eventos com campo `depth`.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/agent/agent.go`** (modificado — núcleo): campo `depth int`; `newSubagent(description, guidance string) *Agent`; `RunSync(ctx, description, guidance) (string, error)`; registro da tool `task` no registry do agente principal; re-emissão de eventos aninhados; constantes `subagentMaxSteps = 10`, `subagentTimeout = 5 * time.Minute`.
- **`internal/tools/tools.go`** (modificado): método exportado `Register(t Tool)` (hoje `register` é unexported) para o agent registrar `task` no wiring.
- **`internal/agent`** (modificado): `Event` ganha `Depth int` e `ParentTool string`; `session.Event` ganha `Depth int`.
- **`internal/tui/tui.go`** (modificado): eventos com `Depth > 0` renderizam indentados em `colSecondary`; hint bar `subagent running` enquanto há subagente ativo.
- **`main.go`** (modificado): `ag.AttachTaskTool()` após a construção do agent.

Fluxo de dados:

1. Agente principal recebe tool call `task` → (confirm se `--confirm`) → `RunSync(ctx, desc, guidance)`.
2. `RunSync` cria o subagente (`newSubagent`), deriva ctx com timeout de 5min, roda o loop **sincronamente**; cada evento do subagente é re-emitido no canal principal com `Depth: 1`.
3. Texto final → resultado da tool → o principal continua o loop dele (re-roteia pelo Jev).
4. Steps > 10 ou timeout → resultado é `error: ...` — o principal decide o próximo passo.

## Design de Implementação

### Interfaces Principais

```go
const subagentMaxSteps = 10
const subagentTimeout = 5 * time.Minute

func (a *Agent) AttachTaskTool()
func (a *Agent) RunSync(ctx context.Context, description, guidance string) (string, error)
func (a *Agent) newSubagent(description, guidance string) *Agent
```

Tool `task` (registrada por `AttachTaskTool` quando `depth == 0`):

```go
reg.Register(Tool{
	Name:     "task",
	Mutating: true,
	Schema: llm.Tool{
		Type: "function",
		Function: llm.Function{
			Name:        "task",
			Description: "Delegate a focused subtask to an isolated subagent and get its final answer",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"description": map[string]any{"type": "string", "description": "What the subagent must accomplish"},
					"guidance":    map[string]any{"type": "string", "description": "Optional context, constraints or hints"},
				},
				"required": []string{"description"},
			},
		},
	},
	Execute: func(ctx context.Context, args map[string]any) (string, error) {
		description, _ := str(args, "description")
		guidance, _ := optStr(args, "guidance")
		return parent.RunSync(ctx, description, guidance)
	},
})
```

- `Mutating: true` — herda as regras de `--confirm` (REQ-001); a tela de confirmação mostra `task(description)` como qualquer tool mutante.

`newSubagent`:

- Compartilha por referência: `LLM`, `Router`, `Fallback`, `Catalog`, `Session`, `Telemetry`, `Confirm`.
- Próprio: `Events` (buffer 512), `messages = [system, user]`, `depth = parent.depth+1`, `Tools = tools.NewRegistry()` **sem** `task` (limite de profundidade 1 — REQ-001), `pinned` vazio.
- Mensagens iniciais: `{Role: "system", Content: "You are a subagent handling a focused subtask for a parent agent. Be concise; return only the final result."}` + `{Role: "user", Content: description + "\n\n" + guidance}` (guidance omitida se vazia).

`RunSync`:

- `ctx, cancel := context.WithTimeout(ctx, subagentTimeout); defer cancel()`.
- Roda o loop do subagente **na goroutine do passo da tool** (síncrono — sem `go`): o loop existente é extraído para `runLoop(ctx) (finalText string, err error)` reusável por `loop` (wrapper com goroutine + eventos de turno) e `RunSync`.
- Re-emissão: o subagente emite no canal próprio; `RunSync` drena em loop (`for ev := range sub.Events`) até o fim, re-emite `ev.Depth = sub.depth; ev.ParentTool = "task"` no canal do principal, e grava `session.Event{..., Depth: ev.Depth}` no transcript compartilhado.
- Fim do subagente → fechar o canal próprio → `RunSync` retorna o texto final.
- Steps esgotados (10) → retorna `"error: subtask exceeded max steps (10)"` **como resultado, sem error** — o principal segue vivo (REQ-004).
- Timeout → `"error: subtask timed out after 5m"` idem; o cancelamento mata HTTP/processos via ctx (mesma máquina da techspec 001).

### Modelos de Dados

```go
type Event struct {
	// campos existentes...
	Depth      int
	ParentTool string
}
```

- `session.Event` ganha `Depth int` (`json:"depth,omitempty"`) — eventos do fluxo principal permanecem idênticos (REQ-006).
- O subagente compartilha o `Session` writer: eventos aninhados appended no mesmo JSONL com `depth: 1`.

### Renderização na TUI

- `handleAgentEvent`: `e.Depth > 0` → prefixo de 2 espaços por nível + estilos em `colSecondary` (linha de rota, tool, resultado); o fluxo do principal mantém os estilos atuais.
- Hint bar: `subagent running` enquanto `busy` e houver subagente ativo (flag no Model setada por `EventToolStart` da tool `task`, limpa no `EventToolResult` dela).
- A linha da tool `task` usa o estilo de tool normal — a indentação começa nos eventos **do** subagente.

## Pontos de Integração

- **Gateway LLM**: o subagente faz suas próprias chamadas `ChatStream` — o gateway vê clientes independentes com históricos separados; nenhum formato novo.
- **Jev (Typesafe)**: o subagente chama o router por conta própria a cada passo — roteamento independente (REQ-002); o state do subagente menciona a description da subtarefa.
- **Telemetria (techspec 006)**: chamadas do subagente gravam `Record(model, tps)` no store compartilhado — amostras de subagente alimentam as estatísticas do modelo (decisão documentada na 006).
- **Cancelamento (techspec 001)**: o ctx do subagente deriva do ctx do turno do principal — `Cancel()`/Esc propaga e mata tudo (REQ-005); o `EventTurnAborted` do principal fecha o turno.

## Verificações Técnicas

### Segurança

- Limites rígidos como defesa: profundidade 1 (sem `task` no registry do subagente), 10 steps, timeout de 5min — um subagente problemático nunca derruba o principal (REQ de segurança operacional do PRD).
- `task` é mutante e passa pelo `--confirm` — delegação é decisão visível do usuário quando o modo está ativo.
- Sem execução paralela: um subagente por vez, na goroutine do passo — sem corrida de UI nem de transcript.

### Arquitetura

- Extração de `runLoop` do `loop` existente: um único motor de turnos, dois modos de acionamento (async com eventos de turno para o principal; sync com coleta para o subagente) — sem duplicar a lógica de steps/roteamento/abort.
- Re-emissão com enriquecimento (`Depth`, `ParentTool`) no canal do principal: a TUI continua com um único consumidor de eventos — zero mudança no loop de escuta.
- Registry do subagente sem `task`: o limite de profundidade é estrutural (a tool não existe no universo do subagente), não uma checagem em runtime.
- `emit` não-bloqueante preservado nos dois níveis; drenagem do canal do subagente é síncrona no passo (sem goroutine extra).

### Infraestrutura

- Sem novos requisitos — stdlib. Verificação: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

## Abordagem de Testes

### Testes Unidade

**`internal/agent/agent_test.go`** (mock gateway com respostas sequenciais + mock Jev):

1. `TestTaskToolRunsSubagent` (mandatório): principal chama `task` → subagente roda (resposta do mock para a chamada do subagente) → texto final chega como tool result → o principal responde ao usuário com o resultado; `EventTurnDone` no fim.
2. `TestSubagentRoutesIndependently`: mock Jev registra as decisões — a chamada do subagente usa modelo diferente do escolhido para o passo do principal (REQ-002).
3. `TestSubagentDoesNotSeeTaskTool`: mock gateway captura o request do subagente → `tools` do request **não** contém `task`; o request do principal contém.
4. `TestNestedEventsCarryDepth`: eventos do subagente chegam ao canal do principal com `Depth: 1` e `ParentTool: "task"`; eventos do principal com `Depth: 0`.
5. `TestSubagentStepsLimit`: mock faz o subagente chamar tools 11 vezes → resultado `error: subtask exceeded max steps (10)`; o principal completa o turno vivo.
6. `TestSubagentTimeout`: mock bloqueia a resposta do subagente (`<-r.Context().Done()`) → resultado `error: subtask timed out after 5m` (timeout encurtado via constante injetável no teste); principal vivo.
7. `TestEscCancelsSubagent`: `Run` do principal → `task` em execução → `Cancel()` → subagente morto (ctx derivado), `EventTurnAborted` do principal, sem goroutine vazando (turno seguinte completa).
8. `TestSubagentWritesTranscriptWithDepth`: JSONL contém eventos do subagente com `"depth": 1` e do principal sem o campo.
9. `TestSubagentConfirmInherited`: `Confirm = true` → tool mutante do subagente emite `EventConfirm` (com `Depth: 1`) — herança da configuração.

**`internal/tui/tui_test.go`**:

10. `TestNestedEventsRenderIndented`: `EventRoute`/`EventToolStart` com `Depth: 1` → blocos com prefixo de 2 espaços em `colSecondary`.
11. `TestHintBarShowsSubagentRunning`: `EventToolStart{Tool: "task"}` → hint contém `subagent running`; `EventToolResult` da task → some.

### Testes de Integração

Cobertos pelos testes de agent (1-9) — principal→subagente→gateway→eventos→transcript com mocks.

### Testes de E2E

Deferidos para `kspec-qa`: delegação real visível indentada no chat; subagente que estoura steps devolve erro e o principal segue; Esc cancela tudo; hint bar durante a execução.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. **`internal/agent`**: extrair `runLoop` de `loop` (refactor puro, testes existentes verdes) — fundação reusável.
2. **`internal/agent`**: campo `depth`, `newSubagent`, `RunSync` com re-emissão e limites.
3. **`internal/tools`**: `Register` exportado; `internal/agent`: `AttachTaskTool` + schema.
4. **`internal/session`**: campo `Depth`.
5. **`internal/tui`**: renderização indentada + hint bar.
6. **`main.go`**: `AttachTaskTool` no wiring.
7. **Verificação final**: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

- Nenhuma externa — stdlib.
- Interação com techspec 001: o cancelamento reusa a máquina de ctx existente — o subagente deriva do ctx do turno.
- Interação com techspec 006: store de telemetria compartilhado por referência.
- Interação com techspec 003: o subagente tem `messages` próprias e curtas — compação raramente dispara; se disparar, funciona igual (janela do modelo escolhido pelo roteamento do subagente).

## Monitoramento e Observabilidade

### Error Tracking

Estouro de steps/timeout do subagente **não é** `EventError` — é resultado da tool (texto `error: ...`): o principal decide; o usuário vê no resultado indentado. Falhas do principal seguem o fluxo existente.

### Logging Estruturado

Transcript JSONL: eventos do subagente com `depth: 1` no mesmo arquivo — auditoria completa da delegação (rotas, tools, resultados) distinguível do fluxo principal (REQ-006).

### Health Checks / Métricas / Alertas

Não aplicável — TUI local. O hint bar `subagent running` é o indicador de estado ativo.

## Considerações Técnicas

### Decisões Principais

1. **Sequencial na v1** (requisito não negociável do PRD): `RunSync` roda na goroutine do passo — paralelismo exigiria cancelamento múltiplo e UI de progresso paralela primeiro; a arquitetura (ctx derivado, eventos com Depth) deixa a evolução natural.
2. **Limite de profundidade estrutural**: o registry do subagente simplesmente não tem `task` — impossível de burlar por prompt injection no modelo; checagem em runtime seria segunda linha desnecessária.
3. **Re-emissão no canal principal com `Depth`**: um consumidor na TUI (simplicidade) em vez de multiplexação de canais; o custo é copiar eventos — barato.
4. **Erro como resultado, não como `error`/`EventError`**: o contrato da tool é texto para o modelo — o principal reage e decide; `EventError` abortaria o turno do principal, violando "o principal segue vivo".
5. **`runLoop` extraído, não duplicado**: um motor, dois acionamentos — steps, roteamento, abort e tool calls idênticos nos dois níveis; menos código, menos drift.

### Riscos Conhecidos

- **Custo de tokens dobrado no passo**: a chamada do subagente é um request cheio (com tools) — inerente à delegação; o limite de 10 steps e o timeout de 5min contêm o teto.
- **Deadlock de canais**: re-emissão usa o mesmo `emit` não-bloqueante (drop sob buffer cheio) — sem bloqueio do subagente esperando a TUI; risco residual de descarte igual ao fluxo atual.
- **Prompt de sistema do subagente**: primeira system message do projeto junto com a compação (techspec 003) — gateways OpenAI-compatible aceitam; validado pelo mock nos testes.
- **Confirm dentro do subagente**: com `--confirm`, tools mutantes do subagente pausam para aprovação — UX aceitável na v1 (o usuário aprova sabendo que é da delegação, eventos com `Depth: 1`).

### Conformidade com Skills Padrões

- `architecture-ddd.md`: não aplicável — brownfield Go com packages `internal/`; a rule determina não impor DDD. Seção Bounded Context removida conforme o template.
- Rules TS/tests.md (Vitest)/logging.md: não aplicáveis — stack Go; padrões do repositório (`testing` + `httptest`, mocks sequenciais, AAA).
- Padrões do projeto: sem comentários no código, eventos via canal (buffer 512, emit não-bloqueante), receivers por valor na TUI, `strings.Builder` por ponteiro.
- Skills kspec aplicáveis: `kspec-tasks`, `kspec-implement`, `kspec-qa` (delegação E2E, limites, Esc), `kspec-pr-review`.

### Arquivos relevantes e dependentes

| Arquivo | Papel na feature |
| --- | --- |
| `internal/agent/agent.go` | `depth`, `runLoop` extraído, `newSubagent`, `RunSync`, `AttachTaskTool`, re-emissão, limites |
| `internal/agent/agent_test.go` | 9 cenários: delegação, roteamento independente, depth, limites, timeout, cancelamento, transcript |
| `internal/tools/tools.go` | `Register` exportado |
| `internal/session/session.go` | Campo `Depth` no `Event` |
| `internal/tui/tui.go` | Renderização indentada `colSecondary`, hint bar `subagent running` |
| `internal/tui/tui_test.go` | Indentação, hint bar |
| `main.go` | `AttachTaskTool` no wiring |
| `internal/router/router.go` / `internal/jev/jev.go` | Sem mudança — reusados por referência pelo subagente |
