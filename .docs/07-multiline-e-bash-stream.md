# 07 — Input multi-linha + stream do output do bash

## Contexto

kterminal é um terminal agêntico em Go (Bubble Tea). Duas fricções de UX: o prompt é single-line (prompts longos ficam ilegíveis) e o tool `bash` bloqueia até o comando terminar — um `go test ./...` demorado mostra nada até o fim.

Arquitetura relevante:

- `internal/tui/tui.go` — input é `textinput.Model`; o prompt box renderiza `m.input.View()`.
- `internal/tools/bash.go` — `exec.CommandContext` com buffers; captura tudo e retorna no fim.
- `internal/agent/agent.go` — `Registry.Execute` síncrono; eventos `EventToolStart`/`EventToolResult`.

## Objetivo

1. Prompt multi-linha: `shift+enter` quebra linha, `enter` envia.
2. Output do bash streamando ao vivo no chat enquanto o comando roda.

## Especificação

### Multi-linha

1. Trocar `textinput` por `bubbles/textarea` (mesma família de dependências): altura auto de 1 a 8 linhas, placeholder vazio, cursor estático com o mesmo estilo atual (`Background(colBg).Foreground(colText)` lembrando que o `cursor.View()` aplica `Reverse`), `TextStyle` com `Background(colBgElement)`.
2. `enter` envia, `shift+enter` quebra linha (binding padrão do textarea é o inverso — configurar `tea.KeyMap` interno se necessário).
3. O prompt box cresce com o conteúdo (recalcular altura do viewport quando o textarea muda de altura; viewport nunca menor que 3 linhas).
4. Histórico de prompts: seta ↑/↓ com o input vazio navega os últimos 20 prompts enviados (guardar em memória na TUI).

### Stream do bash

5. `Registry` ganha `ExecuteStream(ctx, name, argsJSON, onLine func(string)) (string, error)` — mantém `Execute` como wrapper. No `bash`: `cmd.StdoutPipe()` + `bufio.Scanner` (e StderrPipe mesclado via `io.MultiReader`... na prática: `cmd.Stdout = io.MultiWriter(buf, lineWriter)` e idem Stderr), emitindo cada linha via `onLine`.
6. `Agent` emite novo evento `EventToolOutput{Tool, Line}` a cada linha (throttle: no máx 1 evento a cada 50ms, agrupando linhas — o canal tem buffer 512).
7. TUI: linhas de output acumulam num bloco ao vivo sob a linha `● bash(...)` em `colTextMuted`, truncado nas últimas 15 linhas visíveis com contador (`… +N lines`); o `EventToolResult` final substitui o bloco pelo resultado consolidado (que continua truncado como hoje).
8. Transcript: gravar só o resultado final (não cada linha) para não inflar o JSONL.

## Critérios de aceitação

- `shift+enter` cria segunda linha no prompt; envio preserva as quebras.
- `bash` rodando `for i in $(seq 1 10); do echo $i; sleep 0.3; done` mostra os números aparecendo um a um.
- Comando silencioso longo mostra o spinner normalmente.
- ↑ com input vazio recupera o penúltimo prompt.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: `ExecuteStream` com comando que produz linhas com pausas — coletar callbacks e assertar ordem/contagem; throttle testável com relógio injetável ou contador de eventos; teste de TUI: bloco de output ao vivo contém as linhas e o resultado final o substitui.

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Cuidado com o bug conhecido: `strings.Builder` nunca por valor dentro do Model do Bubble Tea.
