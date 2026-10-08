# Tarefa 2.0: Campos calculados e `Criteria()` alternável no catálogo

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — Integração com o catálogo e o router

## Dependências

- Nenhuma (independente da task 1.0 — os campos calculados são populados em memória pelo agent na task 3.0)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

O catálogo aprende a expressar TPS medido: `catalog.Model` ganha os campos calculados `MeasuredTPS float64` e `MeasuredSamples int` — **sem tags YAML**, nunca parseados do `models.yaml` (o `tps_estimate` permanece como fallback, intocado). O `Criteria()` alterna o texto de speed: `MeasuredTPS > 0` → `Speed: ~X tok/s (measured over N calls)`; senão o texto atual do estimate. O contrato com o Jev não muda — continua sendo um string por candidato; o Jev apenas vê texto mais preciso quando há amostras. Nunca piorar a informação do Jev é requisito não negociável do PRD.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib + YAML existente.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa`, `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; parse do YAML intocado; `catalog` não importa `telemetry` (injeção é pull no agent).
</skills>

<requirements>
- `Model` ganha `MeasuredTPS float64` e `MeasuredSamples int` — sem tags YAML (ignorados no parse).
- `Criteria()`: `MeasuredTPS > 0` → `Speed: ~%.0f tok/s (measured over %d calls)`; senão o texto atual (`~%.0f tok/s` do estimate).
- `models.yaml` sem mudança — `tps_estimate` permanece como fallback.
- `catalog` não importa `telemetry` (direção de dependência limpa — injeção é pull no agent, task 3.0).
- Contrato com o Jev inalterado: `Criteria()` continua sendo um string por candidato.
</requirements>

## Subtarefas

- [ ] 2.1 Adicionar `MeasuredTPS` e `MeasuredSamples` ao `catalog.Model` (sem tags YAML)
- [ ] 2.2 Alternar o texto de speed no `Criteria()` conforme `MeasuredTPS > 0`
- [ ] 2.3 Escrever os testes 8-9 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Struct `Model` com campos calculados: techspec, seção **Design de Implementação → Modelos de Dados**.
- Texto alternável do `Criteria()`: techspec, seção **Design de Implementação** (subseção "Regras", item final).
- Decisão "campos calculados no `catalog.Model` em vez de map paralelo no agent": techspec, seção **Considerações Técnicas → Decisões Principais** (item 3).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: modelo sem `MeasuredTPS` → texto contém o estimate (ex.: `~70 tok/s`), sem "measured".
- Teste prova: `MeasuredTPS: 92.4, MeasuredSamples: 7` → texto contém `measured over 7 calls`.
- O parse do `models.yaml` permanece idêntico (campos calculados ignorados).

## Testes da Tarefa

- [ ] Testes de unidade (`internal/catalog/catalog_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 8-9):
  - `TestCriteriaUsesEstimateByDefault` — sem `MeasuredTPS` → estimate, sem "measured".
  - `TestCriteriaUsesMeasuredWhenAvailable` — `MeasuredTPS: 92.4, MeasuredSamples: 7` → `measured over 7 calls`.
- [ ] Testes de integração — fora do escopo da task; a injeção pelo agent é validada na task 3.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/catalog/catalog.go` — `MeasuredTPS`/`MeasuredSamples` calculados; `Criteria()` alternável
- `internal/catalog/catalog_test.go` — critérios estimate × measured
- `internal/catalog/models.yaml` — sem mudança (`tps_estimate` permanece como fallback)
