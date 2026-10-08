# Tarefa 1.0: Estimativa de tokens por chamada no Agent

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Estimativa de tokens por chamada

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Fundação observável da feature: o `Agent` passa a rastrear `lastPromptTokens` (medição real de `StreamResult.Usage.PromptTokens` de cada chamada principal) e `lastEstimateChars`, e expõe `estimateTokens() int64` — medição real somada ao delta aproximado de caracteres desde a última chamada (`/4`), com fallback heurístico (`len(mensagens concatenadas)/4`) quando ainda não há medição. Nesta task a estimativa existe e é testável, mas ninguém a consome ainda — o limiar de compação chega na task 2.0.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — apenas stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E deferido), `kspec-pr-review` (revisão semântica).
- Padrões do projeto: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante).
</skills>

<requirements>
- Campos `lastPromptTokens int64` e `lastEstimateChars int` no `Agent`.
- `estimateTokens() int64`: com medição → `lastPromptTokens + (charsAtuais - lastEstimateChars)/4`; sem medição → `len(mensagens concatenadas)/4`.
- `lastPromptTokens` atualizado a cada `StreamResult` recebido no loop principal (não na chamada de resumo da task 2.0).
- A estimativa por chars usa o mesmo concatenado que vira prompt (aproximação `/4` documentada como heurística).
</requirements>

## Subtarefas

- [ ] 1.1 Adicionar os campos `lastPromptTokens` e `lastEstimateChars` ao `Agent`
- [ ] 1.2 Implementar `estimateTokens()` com medição real + delta e fallback heurístico
- [ ] 1.3 Atualizar `lastPromptTokens`/`lastEstimateChars` a cada `StreamResult` no loop principal
- [ ] 1.4 Escrever `TestEstimateUsesRealUsageAfterFirstCall` (ver Testes da Tarefa)

## Detalhes de Implementação

- Assinatura de `estimateTokens` e regras de cálculo: techspec, seção **Design de Implementação → Interfaces Principais** e **Arquitetura do Sistema → Fluxo de dados** (item 1).
- Racional da heurística `/4` como fallback: techspec, seção **Considerações Técnicas → Decisões Principais** (item 5).
- `internal/llm` e `internal/catalog` não mudam: `Usage.PromptTokens` e `ContextWindow` já existem (techspec, seção **Arquitetura do Sistema**).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: após a primeira resposta do mock (usage conhecido), a estimativa reflete `lastPromptTokens + delta`; antes da primeira, usa heurística `/4`.
- Nenhuma mudança de comportamento visível ao usuário (estimativa é observação passiva).

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`):
  - `TestEstimateUsesRealUsageAfterFirstCall` — após primeira resposta do mock (usage conhecido), a estimativa reflete `lastPromptTokens + delta`; antes da primeira, usa heurística `/4` (techspec, item 5).
- [ ] Testes de integração — fora do escopo da task; o fluxo decide→compact→call é validado na task 2.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — `lastPromptTokens`, `lastEstimateChars`, `estimateTokens`
- `internal/agent/agent_test.go` — teste de estimativa com mock gateway
- `internal/llm/llm.go` — sem mudança (`Usage.PromptTokens` consumido)
- `internal/catalog/catalog.go` — sem mudança (`ContextWindow` consumido na task 2.0)
