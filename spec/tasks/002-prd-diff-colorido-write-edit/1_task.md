# Tarefa 1.0: Algoritmo de diff de linhas (LCS) em `internal/tools/diff.go`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Diff pós-execução (fundação: o algoritmo que as tools consumirão na task 2.0)

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Esta task entrega o coração da feature isolado do resto do sistema: o diff unificado de linhas implementado à mão (LCS clássico com DP matrix — sem dependências, conforme restrição do PRD). O novo arquivo `internal/tools/diff.go` define `DiffLine{Kind, Text}` e `LineDiff(old, new string) []DiffLine`. Nada mais do projeto muda nesta task — ela entrega o algoritmo puro com testes de unidade cobrindo inserção, remoção, mudança no meio, igualdade, arquivo vazio e o guard contra arquivos gigantes.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — apenas stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E deferido), `kspec-pr-review` (revisão semântica).
- Padrões do projeto: sem comentários no código.
</skills>

<requirements>
- `DiffLine` com campos `Kind byte` e `Text string`.
- `LineDiff(old, new string) []DiffLine` produz diff unificado: linhas `+` (adição), `-` (remoção) e contexto (espaço).
- LCS clássico com DP matrix — arquivos de código são pequenos o suficiente para O(n·m).
- Guard: se qualquer lado exceder 10k linhas, o diff degrada para "todas as linhas removidas + adicionadas" (sem DP), evitando alocação gigante.
- Conteúdos iguais → diff vazio (`len == 0`).
</requirements>

## Subtarefas

- [ ] 1.1 Criar `internal/tools/diff.go` com `DiffLine` e `LineDiff` (LCS com DP matrix)
- [ ] 1.2 Implementar o guard de 10k linhas com degradação sem DP
- [ ] 1.3 Escrever `internal/tools/diff_test.go` com os 6 cenários da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Assinaturas e regras do algoritmo: techspec, seção **Design de Implementação → Interfaces Principais**.
- Racional do guard de 10k linhas e do custo O(n·m): techspec, seção **Verificações Técnicas → Arquitetura**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Os 6 testes de unidade do algoritmo passam (inserção, remoção, mudança no meio com contexto, igualdade, vazio→novo, fallback de arquivos grandes).
- Nenhum outro arquivo do projeto é modificado (algoritmo isolado).

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tools/diff_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**):
  - `TestLineDiffInsertion` — linha nova no fim → uma `+`.
  - `TestLineDiffRemoval` — linha removida → uma `-`.
  - `TestLineDiffMiddleChange` — mudança no meio → `-` e `+` adjacentes com contexto preservado.
  - `TestLineDiffEqual` — conteúdos iguais → diff vazio.
  - `TestLineDiffEmpty` — arquivo vazio → novo → tudo `+`.
  - `TestLineDiffLargeFallback` — > 10k linhas → degrada sem DP.
- [ ] Testes de integração — fora do escopo da task (algoritmo puro); o fluxo ponta a ponta é validado na task 3.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/diff.go` — novo: `DiffLine`, `LineDiff` (LCS)
- `internal/tools/diff_test.go` — novo: testes do algoritmo
