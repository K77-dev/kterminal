# Review — Task 1.0: Propagar `context.Context` pelos tools

- **Task**: 1.0 (`spec/tasks/001-prd-abortar-tarefa/1_task.md`)
- **Feature**: abortar tarefa em execução (Esc) — REQ-001, item 2
- **Reviewer**: kspec-review-runner
- **Data**: 2026-10-03
- **Veredito**: **APROVADO**

## Veredito

**APROVADO** — a implementação está em conformidade com o PRD, a Tech Spec e a Task 1.0. Todos os critérios de sucesso foram verificados com evidência; nenhuma regra crítica foi violada; nenhuma mudança fora de escopo foi encontrada.

## Checklist percorrido com evidência

### 1. Conformidade com a spec

| Requisito da spec | Evidência | Status |
| --- | --- | --- |
| `Executor` com assinatura `func(ctx context.Context, args map[string]any) (string, error)` | `internal/tools/tools.go:11` — idêntico à seção *Interfaces Principais* da techspec | OK |
| `Registry.Execute` recebe `ctx` como primeiro argumento e repassa aos tools | `internal/tools/tools.go:54` (assinatura) e `tools.go:65` (`t.Execute(ctx, args)`) | OK |
| Bash deriva o timeout do ctx pai via `context.WithTimeout(ctx, bashTimeout)` | `internal/tools/bash.go:39` — era `context.WithTimeout(context.Background(), bashTimeout)` | OK |
| Bash executa via `exec.CommandContext` | `internal/tools/bash.go:41` | OK |
| Tratamento de `DeadlineExceeded` preservado | `internal/tools/bash.go:51-53` — branch inalterado no diff; mesmo formato de erro (`"command timed out after %s; output so far:\n%s"`) | OK |
| fs.go atualizado mecanicamente (ctx recebido, não usado) | Diff de `internal/tools/fs.go`: apenas assinatura dos 5 tools (`read`, `glob`, `grep`, `write`, `edit`) + import de `context`; zero mudança de corpo | OK |
| Call sites fora do pacote adaptados mecanicamente | Único call site: `internal/agent/agent.go:180` — `a.Tools.Execute(ctx, ...)` repassando o ctx disponível hoje (`ctx := context.Background()`, `agent.go:118`, pré-existente) | OK |

### 2. Regras críticas

- **Nenhum tratamento de `context.Canceled` dentro do tool bash**: confirmado — `bash.go` só inspeciona `ctx.Err() == context.DeadlineExceeded` (linha 51). Cancelamento do pai → `cmd.Run` retorna erro de signal → cai no branch `*exec.ExitError` (linha 54) ou retorna `runErr`; a classificação permanece delegada ao ponto único no Agent (task 2.0), exatamente como a techspec determina.
- **Sem comentários no código**: o diff não introduz nenhum comentário; os dois arquivos de teste novos não contêm comentários.
- **Única mudança semântica do bash é a origem do ctx**: o diff de `bash.go` tem exatamente 2 linhas efetivas (assinatura do closure + origem do ctx em `WithTimeout`); o restante do arquivo é byte a byte inalterado.

### 3. Testes exigidos

Os 3 testes mandatórios da task existem e provem o que ela pede. Executados com `go test -count=1 -race -v ./internal/tools/` — todos PASS:

| Teste | O que prova | Evidência |
| --- | --- | --- |
| `TestBashCanceledContextKillsLongProcess` (`bash_test.go:10`) | `Execute` de `sleep 60` com ctx cancelado retorna em < 1s (processo morto), sem esperar o timeout de 120s do bash | Verifica que `Execute` ainda está bloqueado após 200ms, cancela, e asserta retorno dentro de 1s. Passou em 0.20s — só é possível se o processo foi morto via `CommandContext` |
| `TestBashDeadlineExceededReturnsTimeoutError` (`bash_test.go:36`) | Caminho `DeadlineExceeded` continua funcionando como hoje (erro de timeout com mensagem formatada com `bashTimeout`, falha rápida) | Deadline de 250ms no ctx pai expira o ctx derivado; asserta erro contendo `"timed out"` + `bashTimeout.String()` e duração < 2s. Passou em 0.25s |
| `TestRegistryPassesContextToExecutor` (`tools_test.go:8`) | Registry repassa o **mesmo** ctx recebido ao `Executor` | Tool de teste `probe` captura o ctx; asserta identidade (`seen != ctx` falharia), ctx vivo antes do cancel e `seen.Err() == context.Canceled` após. Passou em 0.00s |

### 4. Verificação obrigatória

`go build ./... && go vet ./... && go test ./...` executado no working tree — tudo verde:

```
?   kterminal        [no test files]
ok  kterminal/internal/agent
?   kterminal/internal/catalog    [no test files]
?   kterminal/internal/clipboard [no test files]
?   kterminal/internal/config     [no test files]
?   kterminal/internal/jev        [no test files]
ok  kterminal/internal/llm
?   kterminal/internal/router     [no test files]
?   kterminal/internal/session    [no test files]
ok  kterminal/internal/tools
ok  kterminal/internal/tui
```

- `go vet ./...`: sem achados.
- `gofmt -l .`: sem output (nenhum arquivo fora de formatação).
- Suite completa re-executada com `-count=1` (sem cache) e pacote `internal/tools` adicionalmente com `-race`: sem data races.

### 5. Escopo

- Mudanças limitadas exatamente aos arquivos listados na task: `internal/tools/tools.go`, `internal/tools/bash.go`, `internal/tools/fs.go`, `internal/agent/agent.go` (1 linha) + 2 arquivos de teste novos.
- **Nenhum** mecanismo cancelável introduzido no Agent (isso é task 2.0): `agent.go:118` permanece `ctx := context.Background()` — comportamento atual preservado, como esperado nesta fase.
- Sem mudanças em `internal/tui`, `internal/session`, `internal/llm`, `internal/router`, `internal/jev` (grep por call sites de `Execute` confirmou que `agent.go` é o único caller fora do pacote).

## Problemas encontrados

Nenhum problema bloqueante ou corretivo.

## Recomendações (não-bloqueantes)

1. **Teste de timeout usa deadline do ctx pai (250ms) em vez do timeout próprio de 120s**: o caminho de código exercitado é idêntico (`ctx.Err() == DeadlineExceeded` no ctx derivado — o deadline menor vence), e testar os 120s reais seria impraticável em unidade. A derivação do pai já está pinada pelo teste de cancelamento; a constante `bashTimeout = 120s` (`bash.go:13`) é inalterada no diff. Aceito como está; registrar apenas para futura referência.
2. **Housekeeping de spec**: existe um `review_1.md` não-rastreado (relatório de uma review anterior desta mesma task) na pasta da spec. Este review oficial está em `review_1.0.md`; considerar remover o arquivo antigo para evitar ambiguidade.

## Conclusão

A task 1.0 cumpre seu papel de fundação: o `context.Context` agora flui de `Registry.Execute` até o `exec.CommandContext` do bash, o kill de processo por cancelamento é provado em unidade (< 1s), o comportamento de timeout próprio é preservado e os tools de filesystem não mudam de comportamento. A base está pronta para a task 2.0 (ctx cancelável no Agent).
