# Tarefa 2.0: Diff nas tools `write`/`edit`, `Result{Output, Diff}` e `PendingDiff`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Diff pós-execução
- REQ-002 — Diff antes da aprovação (modo `--confirm`)

## Dependências

- 1.0 (algoritmo `LineDiff` disponível em `internal/tools`)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Esta task integra o algoritmo da task 1.0 às tools: `write` lê o conteúdo anterior antes de sobrescrever e `edit` captura o `data` que já lê hoje; ambos calculam o diff antes↔depois. `Registry.Execute` muda o retorno de `string` para o struct `Result{Output, Diff}` (extensível — decisão 1 da techspec) e ganha o novo método `PendingDiff`, que simula a edição **em memória** para o modo `--confirm` — o disco nunca é mutado antes da aprovação. As demais tools (`bash`, `read`, `glob`, `grep`) são atualizadas mecanicamente com diff `nil`. O único caller de `Execute` é o agent, adaptado mecanicamente para compilar (a propagação semântica é a task 3.0).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — apenas stdlib (`strings`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa`, `kspec-pr-review`.
- Padrões do projeto: sem comentários no código.
</skills>

<requirements>
- `write` em arquivo existente: lê o conteúdo anterior antes de sobrescrever e retorna diff com `-` e `+`.
- `write` em arquivo novo: todas as linhas como `+`.
- `edit`: diff entre o conteúdo anterior e o editado; edição sem mudança → diff vazio.
- `Registry.Execute(ctx, name, argsJSON) (Result, error)` com `Result{Output string, Diff []DiffLine}`.
- `Registry.PendingDiff(ctx, name, argsJSON) ([]DiffLine, error)`: `edit` aplica `strings.Replace` em memória (mesmas validações de `old_string` único da tool); `write` lê o existente se houver (senão `""`); outras tools → `nil, nil`.
- `PendingDiff` **nunca escreve no disco** (requisito do fluxo `--confirm`).
- `bash`/`read`/`glob`/`grep` retornam `Result` com `Diff: nil` (mudança mecânica).
</requirements>

## Subtarefas

- [x] 2.1 Mudar `Registry.Execute` para retornar `Result{Output, Diff}` e adaptar os executors das tools mecanicamente
- [x] 2.2 Em `internal/tools/fs.go`, capturar o conteúdo anterior no `write` e calcular o diff antes↔depois no `write`/`edit`
- [x] 2.3 Implementar `Registry.PendingDiff` com simulação em memória para `write`/`edit` (validações idênticas às da tool)
- [x] 2.4 Adaptar mecanicamente o call site do agent para a nova assinatura (propagação semântica fica na task 3.0)
- [x] 2.5 Escrever os testes de unidade 7-11 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Struct `Result` e regras de `PendingDiff` por tool: techspec, seção **Design de Implementação → Interfaces Principais** (subseção "PendingDiff por tool").
- Decisão "Result struct em vez de segundo retorno": techspec, seção **Considerações Técnicas → Decisões Principais** (item 1).
- Decisão "simulação em memória" (alternativa rejeitada: aplicar + reverter com backup): techspec, seção **Considerações Técnicas → Decisões Principais** (item 2).
- Segurança de `PendingDiff` (nunca escreve): techspec, seção **Verificações Técnicas → Segurança**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam com a nova assinatura.
- Testes provam: write em existente retorna diff com `-`/`+`; write em novo retorna tudo `+`; edit retorna diff antigo↔novo.
- Teste prova que `PendingDiff` de edit/write não toca o disco (conteúdo idêntico antes/depois) e valida `old_string` inexistente/duplicado como a tool.
- Comportamento das tools não-editoras permanece idêntico (diff `nil`).

## Testes da Tarefa

- [x] Testes de unidade (`internal/tools`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 7-11):
  - `TestWriteToolDiffOnExistingFile` — write em arquivo existente retorna diff com `-` e `+`.
  - `TestWriteToolDiffNewFile` — arquivo novo → todas `+`.
  - `TestEditToolDiff` — edit retorna diff antigo↔novo.
  - `TestPendingDiffDoesNotTouchDisk` — conteúdo em disco idêntico antes/depois; diff correto.
  - `TestPendingEditValidation` — `old_string` inexistente/duplicado retorna erro igual à tool.
- [x] Testes de integração — fora do escopo da task; fluxo tool→evento→transcript é validado na task 3.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/tools.go` — `Result{Output, Diff}`, `PendingDiff`
- `internal/tools/fs.go` — captura do conteúdo anterior; diff em `write`/`edit`
- `internal/tools/bash.go`, `internal/tools/glob.go`, `internal/tools/grep.go` — adaptação mecânica (diff `nil`)
- `internal/tools/*_test.go` — testes 7-11
- `internal/agent/agent.go` — call site adaptado mecanicamente
