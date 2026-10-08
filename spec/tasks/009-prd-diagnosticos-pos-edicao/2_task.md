# Tarefa 2.0: Hook `OnGoEdit` no Registry

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Hook pós-edição em arquivos Go

## Dependências

- 1.0 (`GoVetHook` disponível)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

O ponto de chamada do diagnóstico: o `Registry` ganha o campo injetável `OnGoEdit func(path string) string` (nil por default) com o setter `SetOnGoEdit`. Após sucesso de `write`/`edit` em arquivo `.go`, `Execute` extrai o `path` dos args e anexa `r.OnGoEdit(path)` ao output — o hook roda **após** a escrita ter sucesso; a edição nunca é revertida nem falha por causa do diagnóstico. A injeção via setter (não no construtor) mantém o pacote `tools` e seus testes independentes de Go instalado — o `main.go` é o ponto de composição (task 3.0). Registry sem hook → comportamento idêntico ao atual (noop).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa`, `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; hook injetado via setter — pacote testável sem Go instalado.
</skills>

<requirements>
- Campo `OnGoEdit func(path string) string` no `Registry` (nil default) + `SetOnGoEdit(hook)`.
- Após sucesso da tool, se `name == "write" || name == "edit"`, `r.OnGoEdit != nil` e `strings.HasSuffix(path, ".go")` → `out += r.OnGoEdit(path)`.
- O hook roda após a escrita ter sucesso — a edição nunca é revertida nem falha por causa dele.
- `write` em `.txt`, `edit` em `.md`, `read`/`bash` → hook não chamado.
- Registry sem hook (nil) → comportamento idêntico ao atual (noop).
</requirements>

## Subtarefas

- [ ] 2.1 Adicionar o campo `OnGoEdit` e o setter `SetOnGoEdit` ao `Registry`
- [ ] 2.2 Chamar o hook em `Execute` após sucesso de `write`/`edit` em `.go` (extraindo o path dos args)
- [ ] 2.3 Escrever os testes 9-11 da techspec com hook fake (ver Testes da Tarefa)

## Detalhes de Implementação

- Regras do ponto de chamada em `Execute`: techspec, seção **Design de Implementação** (subseção "Regras do `Registry.Execute` (ponto de chamada)").
- Decisão "hook como campo injetável via setter": techspec, seção **Considerações Técnicas → Decisões Principais** (item 1) e **Verificações Técnicas → Arquitetura**.
- Interação com a techspec 002: o hook anexa ao `Output` do `Result` (assinatura pós-002); se 002 ainda não implementado, anexa ao `string` — mudança mecânica (techspec, **Dependências Técnicas**).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: registry com hook fake + `write` em `.go` → output contém o retorno do hook; hook registra o path recebido.
- Teste prova: `write` em `.txt`, `edit` em `.md`, `read`/`bash` → hook não chamado.
- Teste prova: registry sem hook → comportamento idêntico ao atual.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tools/tools_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 9-11):
  - `TestExecuteCallsHookOnGoEdit` — hook fake chamado no `write` em `.go`; output contém o retorno; path registrado.
  - `TestExecuteSkipsHookForNonGoOrOtherTools` — `.txt`/`.md`/`read`/`bash` → hook não chamado.
  - `TestExecuteNilHookNoop` — sem hook → comportamento idêntico ao atual.
- [ ] Testes de integração — fora do escopo da task; o loop de auto-correção é validado na task 3.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/tools.go` — campo `OnGoEdit` + `SetOnGoEdit` + chamada pós `write`/`edit` em `.go`
- `internal/tools/tools_test.go` — hook chamado/não chamado, nil noop
- `internal/tools/diagnostics.go` — `GoVetHook` consumido na task 3.0
