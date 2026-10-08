# Tarefa 3.0: Propagação do diff no Agent e no transcript

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Diff antes da aprovação (modo `--confirm`)
- REQ-003 — Diff no transcript

## Dependências

- 2.0 (`Result{Output, Diff}` e `PendingDiff` disponíveis no registry)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Esta task liga o diff ao fluxo do agente: `agent.Event` ganha o campo `Diff []tools.DiffLine`; no fluxo de confirmação, o agent chama `PendingDiff` **antes** de emitir o `EventConfirm` (o diff existe antes da aprovação, sem tocar o disco); após a execução, `Result.Diff` é propagado no `EventToolResult` e gravado no transcript. `session.Event` ganha `Diff []string` (linhas já formatadas com prefixo `+`/`-`/espaço, `omitempty`) — a formatação acontece na gravação; `session` não importa `tools` (recebe strings do agent). Erro de `PendingDiff` é engolido (diff `nil`) — a confirmação nunca deixa de aparecer por falha na simulação.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — apenas stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa`, `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante).
</skills>

<requirements>
- `agent.Event` ganha campo `Diff []tools.DiffLine`.
- Fluxo de confirmação: `Confirm && IsMutating(name)` → `PendingDiff` → `EventConfirm` carrega o diff antes do y/n.
- Erro de `PendingDiff` é engolido (diff `nil`) — confirmação sempre aparece.
- `EventToolResult` propaga `Result.Diff`.
- `session.Event` ganha `Diff []string` com `json:"diff,omitempty"` — linhas formatadas (`+foo`, `-bar`, ` baz`).
- Eventos de tools não-editoras permanecem byte-idênticos no JSONL (`omitempty`).
- Diferença de tipos: `agent.Event.Diff` é `[]tools.DiffLine` (tipado); `session.Event.Diff` é `[]string` formatado pelo agent — `session` não importa `tools`.
</requirements>

## Subtarefas

- [ ] 3.1 Adicionar `Diff []tools.DiffLine` ao `agent.Event` e `Diff []string` (com `omitempty`) ao `session.Event`
- [ ] 3.2 No fluxo de confirmação, chamar `PendingDiff` antes de emitir `EventConfirm` (engolir erro)
- [ ] 3.3 Propagar `Result.Diff` no `EventToolResult` e gravar as linhas formatadas no transcript
- [ ] 3.4 Escrever `TestToolResultEventCarriesDiff` com mock gateway (ver Testes da Tarefa)

## Detalhes de Implementação

- Snippet do fluxo de confirmação (`if a.Confirm && a.Tools.IsMutating(name)`) e regra do erro engolido: techspec, seção **Design de Implementação → Interfaces Principais**.
- Modelos de dados (`Event` com `Diff`, tags JSON): techspec, seção **Design de Implementação → Modelos de Dados**.
- Fluxo de dados pós-execução e pré-aprovação: techspec, seção **Arquitetura do Sistema → Fluxo de dados**.
- Direção de dependência (`session` não importa `tools`): techspec, seção **Verificações Técnicas → Arquitetura**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste com mock gateway prova: tool call `write` → `EventToolResult` carrega o diff; transcript gravado contém o campo `diff`.
- No modo confirm, o `EventConfirm` carrega o diff calculado antes da aprovação (validado na task 4.0 via `confirmView`; aqui o evento carrega o campo).
- Eventos de tools não-editoras no JSONL permanecem idênticos aos atuais.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`):
  - `TestToolResultEventCarriesDiff` — mock gateway responde com tool call `write` → `EventToolResult` carrega o diff; transcript gravado contém campo `diff` (techspec, item 16).
- [ ] Testes de integração — cobertos pelo teste acima (fluxo ponta a ponta tool→evento→transcript com mock gateway, conforme techspec **Abordagem de Testes → Testes de Integração**).
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — campo `Diff` no `Event`; `PendingDiff` no confirm; propagação no `EventToolResult`
- `internal/agent/agent_test.go` — teste de propagação evento/transcript
- `internal/session/session.go` — campo `Diff []string` no `Event`
