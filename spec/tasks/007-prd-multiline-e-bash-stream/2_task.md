# Tarefa 2.0: `EventToolOutput` com throttle de 50ms no Agent

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — Stream do output do bash
- REQ-005 — Transcript enxuto

## Dependências

- 1.0 (`ExecuteStream` disponível no registry)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

O agent vira o ponto único de política do stream: o loop passa a usar `ExecuteStream` com um coletor que agrupa linhas e emite `EventToolOutput` no máximo 1× a cada 50ms (throttle) — protegendo o canal (buffer 512) e o loop da TUI sem perder linha: o flush final antes do `EventToolResult` garante completude. O `EventToolOutput` reusa os campos `Tool` + `Text` (linhas agrupadas com `\n`) — sem campos novos. O transcript **não** grava `tool_output` (REQ-005): só o `tool_result` final, como hoje — o JSONL não cresce com eventos por linha. O relógio é injetável (`now func() time.Time`) para testar o throttle sem dormir.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib (`time`, `strings`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa`, `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante); `strings.Builder` por ponteiro.
</skills>

<requirements>
- Constante `toolOutputThrottle = 50 * time.Millisecond`; novo `EventKind` `EventToolOutput = "tool_output"`.
- O loop do agent usa `ExecuteStream` com callback coletor: linhas acumuladas; emit quando `now() - lastEmit >= throttle`; `lines` resetadas após o emit.
- Flush final (`EventToolOutput` com as linhas restantes) antes do `EventToolResult` — nenhuma linha perdida.
- `EventToolOutput` reusa `Tool` + `Text` (linhas agrupadas com `\n`) — sem campos novos no `Event`.
- `session.Event` sem mudança — `tool_output` não é gravado no transcript (REQ-005).
- Campo `now func() time.Time` injetável no `Agent` (default `time.Now`) para teste do throttle.
- Contrato com o gateway inalterado: o LLM continua vendo só o resultado consolidado (`role: "tool"`).
</requirements>

## Subtarefas

- [ ] 2.1 Adicionar `EventToolOutput` ao `EventKind` e o campo `now` injetável ao `Agent`
- [ ] 2.2 Trocar a execução de tools no loop para `ExecuteStream` com o coletor de throttle
- [ ] 2.3 Implementar o flush final antes do `EventToolResult`
- [ ] 2.4 Escrever os testes 6-7 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Snippet do coletor com throttle e constantes: techspec, seção **Design de Implementação → Interfaces Principais** (subseção "Throttle no agent").
- Decisão "throttle no agent (não na tool)": techspec, seção **Verificações Técnicas → Arquitetura** e **Considerações Técnicas → Decisões Principais** (item 2).
- Buffer do canal + throttle no pior caso (1000 linhas/s → 20 eventos/s): techspec, seção **Verificações Técnicas → Arquitetura**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova (com `now` injetável): 100 linhas no mesmo instante → 1 `EventToolOutput` agrupado; +60ms e mais linhas → 2º evento; flush final com as restantes antes do `EventToolResult`.
- Teste prova: turno com bash verboso → JSONL sem eventos `tool_output`; `tool_result` presente (REQ-005).

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 6-7):
  - `TestToolOutputThrottled` — agrupamento por janela de 50ms; flush final antes do resultado.
  - `TestToolOutputNotWrittenToTranscript` — JSONL sem `tool_output`; `tool_result` presente.
- [ ] Testes de integração — cobertos pelos testes de agent (tool→evento→transcript ponta a ponta com mock).
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — `EventToolOutput`, coletor com throttle 50ms, flush final, `now` injetável
- `internal/agent/agent_test.go` — throttle, agrupamento, transcript enxuto
- `internal/tools/tools.go` — `ExecuteStream` consumido (task 1.0)
- `internal/session/session.go` — sem mudança (só `tool_result` gravado)
