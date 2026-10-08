# Tarefa 3.0: Coleta pós-chamada e injeção de TPS medido no Agent

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Coleta de TPS por modelo
- REQ-003 — Integração com o catálogo e o router

## Dependências

- 1.0 (`telemetry.Store` disponível)
- 2.0 (`MeasuredTPS`/`MeasuredSamples` no `catalog.Model`)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral`

Esta task fecha o ciclo coleta→consumo: o `Agent` ganha o campo `Telemetry *telemetry.Store` e, após cada `ChatStream` com `Usage.CompletionTokens > 0`, grava `Record(decision.Model, tps)` (o TPS já computado pelo loop hoje). Ao montar candidatos para o router em `decide()`, o agent copia cada `catalog.Model` e injeta `MeasuredTPS`/`MeasuredSamples` via `Store.Get` — o `Criteria()` da task 2.0 cita o medido quando há confiança (≥ 5 amostras), senão mantém o estimate. Injeção é pull no agent: `catalog` e `telemetry` permanecem desacoplados.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa`, `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; eventos via canal; `telemetry` é folha (agent depende dele, não o contrário).
</skills>

<requirements>
- Campo `Telemetry *telemetry.Store` no `Agent`.
- Após cada `ChatStream` com `Usage.CompletionTokens > 0` → `Record(decision.Model, tps)`; chamada sem completion tokens não grava.
- Em `decide()`: copiar cada candidato e injetar `MeasuredTPS`/`MeasuredSamples` quando `Get` retorna `tp > 0`.
- `Record` chamado só na goroutine do loop do agent (mutex do store cobre acessos concorrentes do doctor/Close).
- Tolerância a store nil (agent sem telemetria funciona como hoje — wiring chega na task 4.0).
</requirements>

## Subtarefas

- [ ] 3.1 Adicionar o campo `Telemetry` ao `Agent` com guarda para nil
- [ ] 3.2 Gravar `Record(decision.Model, tps)` após cada chamada com completion tokens
- [ ] 3.3 Injetar valores medidos nos candidatos ao montar a escolha do router em `decide()`
- [ ] 3.4 Escrever os testes 10-11 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Snippet da injeção em `decide`: techspec, seção **Design de Implementação → Interfaces Principais** (bloco "Injeção no agent").
- Fluxo de dados coleta→consumo: techspec, seção **Arquitetura do Sistema → Fluxo de dados** (itens 1 e 2).
- Decisão "injeção pull no agent" (catalog não conhece telemetry): techspec, seção **Considerações Técnicas → Decisões Principais** (item 4) e **Verificações Técnicas → Arquitetura**.
- Concorrência: techspec, seção **Verificações Técnicas → Arquitetura**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: mock gateway com usage conhecido → `Store.GetMean(model)` reflete a amostra; chamada sem completion tokens não grava.
- Teste prova: store com 5+ amostras num modelo → `decide` passa ao mock Jev critérios contendo "measured".

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 10-11):
  - `TestAgentRecordsTPSAfterCall` — usage conhecido → `GetMean` reflete a amostra; sem completion tokens não grava.
  - `TestCandidatesCarryMeasuredTPS` — 5+ amostras → critérios ao mock Jev contendo "measured".
- [ ] Testes de integração — cobertos pelos testes de agent com mock (coleta→injeção→critérios ponta a ponta, conforme techspec **Abordagem de Testes → Testes de Integração**).
- [ ] Testes E2E — deferidos para `kspec-qa` (5+ chamadas → critérios citam medido).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — campo `Telemetry`, `Record` pós-chamada, injeção nos candidatos
- `internal/agent/agent_test.go` — coleta pós-chamada; critérios com "measured"
- `internal/telemetry/telemetry.go` — `Store` consumido (task 1.0)
- `internal/catalog/catalog.go` — campos calculados consumidos (task 2.0)
