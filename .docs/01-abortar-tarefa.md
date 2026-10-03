# 01 — Interromper tarefa em execução (Esc)

## Contexto

kterminal é um terminal agêntico em Go (Bubble Tea) onde o Jev (Typesafe) escolhe o LLM a cada chamada, via um gateway LiteLLM OpenAI-compatible. Arquitetura relevante:

- `internal/agent/agent.go` — `Agent.loop()` roda o ciclo agêntico numa goroutine com `context.Background()`; emite eventos (`EventRoute`, `EventDelta`, `EventToolStart`, `EventToolResult`, `EventConfirm`, `EventTurnDone`, `EventError`) num canal `Events`.
- `internal/llm/llm.go` — `ChatStream(ctx, ...)` já aceita contexto; o HTTP aborta se o ctx for cancelado.
- `internal/tools/bash.go` — `exec.CommandContext` já aceita contexto, mas hoje recebe um ctx com timeout próprio criado internamente.
- `internal/tui/tui.go` — `handleChatKey`: `ctrl+c` encerra o app; não existe forma de cancelar uma tarefa em andamento.

## Objetivo

Permitir interromper a tarefa corrente com `Esc` sem sair do app: abortar o stream LLM em curso, abortar o tool em execução (inclusive `bash`), e voltar ao estado idle mantendo a conversa.

## Especificação

1. `Agent` ganha um campo `cancel context.CancelFunc` e um método `Cancel()`. `Run()` cria um ctx cancelável (derivado de `context.Background()`) e o passa para todo o loop: `ChatStream`, execução de tools e chamadas Jev.
2. Novo evento `EventTurnAborted`. Quando o ctx é cancelado: o loop emite `EventTurnAborted` (com o texto parcial que chegou), grava no transcript (`session.Event{Type: "turn_aborted"}`) e retorna. Erros de ctx cancelado NÃO devem virar `EventError`.
3. `tools.Registry.Execute` passa a aceitar `ctx` (propagar para `bash`; tools de filesystem falham rápido naturalmente).
4. TUI: em `handleChatKey`, tecla `esc` quando `m.busy` chama `m.agent.Cancel()`. O texto parcial streamado permanece no chat (renderizar markdown do que chegou, igual ao `EventTurnDone`). Status volta a idle (`busy = false`).
5. `esc` quando não busy não faz nada (hoje também não faz). `ctrl+c` continua saindo do app.
6. Enquanto `busy`, o hint bar mostra `esc to interrupt` no lado direito.

## Critérios de aceitação

- Esc durante stream: stream para, evento de abort emitido, TUI volta a aceitar input, conversa preservada.
- Esc durante `bash` longo (ex.: `sleep 60`): processo morto via ctx.
- Nova mensagem após abort funciona normalmente (o loop reinicia limpo).

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Teste novo em `internal/agent/agent_test.go`: mock de gateway cujo handler bloqueia até o ctx do request morrer; chamar `Run` e depois `Cancel`; assertar `EventTurnAborted` e que nenhuma goroutine vaza (testar com timeout).

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Seguir os padrões existentes: eventos via canal, TUI com receivers por valor, `strings.Builder` sempre por ponteiro.
