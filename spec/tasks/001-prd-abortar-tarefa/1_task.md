# Tarefa 1.0: Propagar `context.Context` pelos tools

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Cancelamento do turno agêntico via Esc (item 2: o registry de tools propaga o contexto; o `bash` o respeita)

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Hoje os tools executam sem contexto: o `Executor` não recebe `context.Context` e o bash deriva seu timeout de `context.Background()`, então não há como cancelar um processo em execução. Esta task é a fundação da feature: muda as assinaturas de `Executor` e `Registry.Execute` para receber `ctx` e faz o bash derivar o timeout do contexto pai, usando `exec.CommandContext` — quando o contexto morre, o processo bash recebe SIGKILL do próprio stdlib. Ao final desta task o pacote `internal/tools` compila, os callers existentes foram adaptados mecanicamente e um comando bash longo pode ser morto via cancelamento de contexto em teste de unidade. O contexto cancelável em si só aparece na task 2.0 (no Agent); aqui ele apenas precisa fluir.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — apenas stdlib (`context`, `os/exec`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E do kill do bash), `kspec-pr-review` (revisão semântica).
- Padrões do projeto (PRD): sem comentários no código; `strings.Builder` sempre por ponteiro.
</skills>

<requirements>
- `Executor` passa a ter a assinatura `func(ctx context.Context, args map[string]any) (string, error)`.
- `Registry.Execute` passa a receber `ctx` como primeiro argumento e o repassa aos tools.
- O bash deriva o timeout do ctx pai (`context.WithTimeout(ctx, bashTimeout)`) e executa via `exec.CommandContext` — cancelamento do pai mata o processo.
- `DeadlineExceeded` continua tratado como timeout próprio (comportamento atual preservado).
- Tools de filesystem (`fs.go`) são atualizados mecanicamente — recebem ctx mas não o usam (falham rápido naturalmente).
- Callers existentes fora de `internal/tools` (execução de tools no Agent) são adaptados mecanicamente para compilar, repassando o ctx disponível hoje; o ctx cancelável chega na task 2.0.
</requirements>

## Subtarefas

- [ ] 1.1 Mudar a assinatura do tipo `Executor` em `internal/tools/tools.go` para receber `context.Context`
- [ ] 1.2 Mudar `Registry.Execute` para receber `ctx` e repassá-lo a cada `Executor` registrado
- [ ] 1.3 Em `internal/tools/bash.go`, derivar o timeout do ctx pai e trocar `exec.Command` por `exec.CommandContext`; manter o tratamento existente de `DeadlineExceeded`
- [ ] 1.4 Atualizar `internal/tools/fs.go` mecanicamente (nova assinatura; ctx não utilizado)
- [ ] 1.5 Adaptar mecanicamente os call sites de `Registry.Execute` fora do pacote para o código compilar
- [ ] 1.6 Escrever testes de unidade do cancelamento do bash (ver Testes da Tarefa)

## Detalhes de Implementação

- Assinaturas-alvo e o snippet do bash tool estão na techspec, seção **Design de Implementação → Interfaces Principais** e subseção **bash tool**.
- A única mudança semântica do bash é a origem do contexto (`ctx` pai em vez de `context.Background()`); o resto do arquivo permanece inalterado.
- `Canceled` resulta em processo morto (`cmd.Run` retorna erro de signal) — o retorno é descartado pelo caminho de abort do Agent (task 2.0), que é o ponto único de classificação. Não trate `Canceled` dentro do tool.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam com as novas assinaturas.
- Teste de unidade prova: `Execute` de um comando longo (ex.: `sleep 60`) com ctx cancelado retorna em < 1s (processo morto), sem esperar o timeout do bash.
- Teste de unidade prova: timeout próprio do bash (`DeadlineExceeded`) continua funcionando como hoje.
- Nenhuma mudança de comportamento para os tools de filesystem.

## Testes da Tarefa

- [ ] Testes de unidade
  - Bash com ctx cancelado retorna imediatamente (processo morto via `exec.CommandContext`).
  - Bash com timeout próprio ainda retorna erro de timeout como antes.
  - Registry repassa o mesmo ctx recebido ao `Executor` (pode ser verificado com um tool de teste que inspeciona `ctx.Err()`/`ctx.Done()`).
- [ ] Testes de integração — fora do escopo conforme techspec (seção **Abordagem de Testes → Testes de Integração**); o kill real do processo é validado no QA.
- [ ] Testes E2E — deferidos para `kspec-qa` (Esc durante `sleep 60`).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/tools.go` — tipos `Executor` e `Registry.Execute` com ctx
- `internal/tools/bash.go` — timeout derivado do ctx pai; `exec.CommandContext`
- `internal/tools/fs.go` — atualização mecânica de assinatura
- `internal/tools/*_test.go` — testes de unidade do cancelamento
- `internal/agent/agent.go` — call site de `Registry.Execute` (adaptação mecânica apenas)
