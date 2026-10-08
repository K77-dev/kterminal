# Tarefa 1.0: `ExecuteStream` com line writer no bash

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — Stream do output do bash

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Fundação do stream, isolada no pacote de tools: `Registry` ganha `ExecuteStream(ctx, name, argsJSON, onLine)` mantendo `Execute` como wrapper com `onLine = nil` (comportamento idêntico ao atual — nenhuma alocação de line writer). O bash passa a escrever stdout e stderr em `io.MultiWriter(buffer consolidado, lineWriter)`: o line writer acumula bytes até `\n` e chama o callback linha a linha, com flush do restante no fim (linha sem `\n` final). Timeout de 120s, exit status e truncamento de 32k do resultado final permanecem inalterados — o stream é aditivo ao comportamento consolidado. Uma única passada no pipe garante consolidado e stream consistentes por construção (mesma ordem de bytes).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib (`io`, `bytes`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E deferido), `kspec-pr-review` (revisão semântica).
- Padrões do projeto: sem comentários no código; `strings.Builder` por ponteiro; timeout/kill por contexto preservados (techspec 001).
</skills>

<requirements>
- `ExecuteStream(ctx context.Context, name, argsJSON string, onLine func(string)) (Result, error)` no `Registry`.
- `Execute` vira wrapper: `return r.ExecuteStream(ctx, name, argsJSON, nil)` — todos os callers existentes preservados.
- `onLine == nil` → comportamento idêntico ao atual (nenhuma alocação de line writer).
- `newLineWriter(onLine)`: acumula bytes até `\n`, chama `onLine(linha)` sem o `\n`; flush do restante no fim.
- bash: `cmd.Stdout/cmd.Stderr = io.MultiWriter(buf, lineWriter)` quando `onLine != nil`.
- Timeout de 120s, exit status e truncamento de 32k do consolidado: inalterados.
- Outras tools ignoram `onLine` (stream é só do bash — fora de escopo para os demais).
</requirements>

## Subtarefas

- [ ] 1.1 Adicionar `ExecuteStream` ao `Registry` e transformar `Execute` em wrapper com `onLine = nil`
- [ ] 1.2 Implementar `newLineWriter` (acúmulo até `\n`, callback, flush final)
- [ ] 1.3 Ligar o bash ao `io.MultiWriter` (buffer + line writer) preservando timeout/truncamento
- [ ] 1.4 Escrever os testes 1-5 da techspec em `internal/tools/bash_test.go` (ver Testes da Tarefa)

## Detalhes de Implementação

- Assinatura de `ExecuteStream` e snippet do bash com `io.MultiWriter`: techspec, seção **Design de Implementação → Interfaces Principais**.
- Decisão "`io.MultiWriter` (buffer + line writer)": techspec, seção **Considerações Técnicas → Decisões Principais** (item 3).
- Interação com a techspec 002: `ExecuteStream` retorna `Result` (assinatura pós-002); se 002 ainda não implementado, retorna `string` e a mudança é mecânica depois — techspec, seção **Sequenciamento de Desenvolvimento → Dependências Técnicas**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste mandatório prova: `for i in $(seq 1 10); do echo $i; sleep 0.05; done` → callbacks coletados = `[1..10]` na ordem; resultado final contém as 10 linhas.
- Testes provam: stdout+stderr mesclados no stream e no consolidado; `onLine = nil` idêntico ao `Execute`; linha final sem `\n` chega no flush; timeout mata o processo como hoje.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tools/bash_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 1-5):
  - `TestExecuteStreamEmitsLinesInOrder` (mandatório) — callbacks `[1..10]` na ordem; consolidado com as 10 linhas.
  - `TestExecuteStreamMergesStdoutStderr` — writes em ambos → linhas de ambos no stream e no consolidado.
  - `TestExecuteStreamNilCallbackMatchesExecute` — `onLine = nil` → resultado idêntico ao `Execute`.
  - `TestExecuteStreamFinalLineWithoutNewline` — `printf sem-newline` → callback recebe a linha no flush.
  - `TestExecuteStreamTimeoutKillsProcess` — `sleep 300` com timeout curto → processo morto, erro de timeout.
- [ ] Testes de integração — fora do escopo da task; tool→evento→transcript é validado na task 2.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/tools.go` — `ExecuteStream` com `onLine`; `Execute` wrapper
- `internal/tools/bash.go` — `io.MultiWriter` + line writer; timeout/truncamento preservados
- `internal/tools/bash_test.go` — ordem/contagem de linhas, merge stderr, flush, timeout
