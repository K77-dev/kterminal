# 10 — Subagentes (tool `task`)

## Contexto

kterminal é um terminal agêntico em Go onde o Jev escolhe o LLM a cada chamada. Tarefas grandes serializam o agente principal; delegar subtarefas a subagentes (cada um com roteamento Jev próprio) permite paralelismo e contexto limpo — o padrão `task` do Claude Code/opencode.

Arquitetura relevante:

- `internal/agent/agent.go` — `Agent` com `loop()`, canal `Events`, routers, catálogo, tools. O loop é auto-contido e reutilizável.
- `internal/tools/tools.go` — `Registry` com `Tool{Name, Mutating, Schema, Execute}`.
- `internal/tui/tui.go` — renderiza `EventToolStart`/`EventToolResult` como blocos.

## Objetivo

Tool `task(description)` que roda um subagente isolado (próprias mensagens, próprio roteamento Jev por chamada, próprios tools) e devolve a resposta final como resultado do tool.

## Especificação

1. **Fábrica**: `Agent` ganha `newSubagent() *Agent` — compartilha `LLM`, `Router`, `Fallback`, `Catalog`, `Session` e configuração de `Confirm`, mas tem `Events` próprio (buffered), `messages` zerado e `depth` = parent+1.
2. **Tool `task`**: registrada só quando `depth == 0` (subagentes não spawnam subagentes — limite de profundidade 1). Schema: `{"description": string, "guidance": string opcional}`. `Mutating: true` (herda as regras de confirmação do modo `--confirm`).
3. **Execução síncrona com eventos**: `RunSync(ctx, description) (finalText string, err error)` — variante do loop que, em vez de emitir para a TUI, coleta eventos internamente. Os eventos do subagente são re-emitidos no canal principal como eventos aninhados: novo campo `Depth int` e `ParentTool string` em `agent.Event`; `EventRoute`/`EventToolStart`/`EventToolResult` do subagente chegam à TUI marcados com `Depth: 1`.
4. **Contexto do subagente**: prompt de sistema inicial: "You are a subagent handling a focused subtask for a parent agent. Be concise; return only the final result." + a description + guidance. Máximo de steps do subagente: 10 (metade do principal).
5. **Timeout**: 5 minutos por subagente via ctx; estourou → resultado é `error: subtask timed out after 5m`.
6. **TUI**: eventos `Depth: 1` renderizam com indentação de 2 espaços e cor `colSecondary` (azul) para diferenciar do fluxo principal; a linha do `task` usa o estilo de tool normal. O hint bar mostra `subagent running` enquanto há subagente ativo.
7. **Transcript**: eventos aninhados gravados com campo `depth` no JSONL.
8. **Cancelamento**: `Agent.Cancel()` cancela também subagentes ativos (ctx compartilhado do turno).

## Critérios de aceitação

- Agente principal chama `task` → subagente roda com roteamento Jev próprio (eventos de rota aninhados visíveis), devolve texto final como tool result, o principal continua com o resultado.
- Subagente não pode chamar `task` (não vê a tool).
- Subagente que estoura 10 steps ou 5min devolve erro como resultado — o principal segue vivo.
- Esc cancela o turno inteiro, incluindo subagentes.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: mock de gateway com respostas sequenciais — a chamada do subagente usa modelo diferente do principal (assertar roteamento independente); subagente com tool call interno (loop de 2 níveis); timeout com mock que bloqueia; `task` ausente nas definições enviadas ao subagente.

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Sem goroutines paralelas na v1 — `task` é sequencial (paralelismo é evolução natural, mas exige cancelamento e UI de progresso múltiplo primeiro).
