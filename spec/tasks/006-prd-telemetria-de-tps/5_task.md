# Tarefa 5.0: Verificação final integrada e checagem de critérios de aceite

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Coleta de TPS por modelo
- REQ-002 — Persistência local
- REQ-003 — Integração com o catálogo e o router
- REQ-004 — Visibilidade no `--doctor`

## Dependências

- 4.0 (feature completa: pacote, catálogo, agent e wiring implementados e testados)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechamento da feature: rodar a verificação obrigatória do projeto completa, checar cruzadamente os critérios de aceite do PRD contra o que foi implementado e confirmar a conformidade com os padrões do código. Esta task não escreve código novo — se qualquer verificação falhar, o ajuste volta para a task responsável (1.0, 2.0, 3.0 ou 4.0). Ao final, a funcionalidade está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.

<skills>
### Conformidade com Skills Padrões

- Verificação obrigatória do projeto (PRD): `go build ./... && go vet ./... && gofmt -l . && go test ./...`.
- Skills do fluxo kspec aplicáveis: `kspec-qa` (próximo passo — E2E: 5+ chamadas citam medido; reinício preserva estatísticas; `--doctor` mostra tabela), `kspec-pr-review` (revisão semântica contra PRD/Tech Spec/tasks).
- Padrões do projeto a conferir: sem comentários no código; eventos via canal; sem dependências externas; YAML intocado como fallback.
</skills>

<requirements>
- `go build ./...`, `go vet ./...`, `gofmt -l .` (saída vazia) e `go test ./...` todos verdes em uma única execução encadeada.
- Checklist dos critérios de aceite do PRD percorrido item a item, com evidência (teste que o cobre ou deferimento explícito ao QA).
- Conformidade com os padrões de código do projeto confirmada por inspeção do diff.
- Nenhum requisito do PRD deixado de lado; itens fora de escopo do PRD não implementados (telemetria remota, gráficos na TUI, medição de latência separada, métricas além de TPS, α configurável).
</requirements>

## Subtarefas

- [ ] 5.1 Executar `go build ./... && go vet ./... && gofmt -l . && go test ./...` e confirmar tudo verde
- [ ] 5.2 Percorrer o checklist de critérios de aceite do PRD (ver Critérios de Sucesso) e registrar a evidência de cada item
- [ ] 5.3 Inspecionar o diff completo contra os padrões do projeto (sem comentários, sem dependências externas, YAML intocado)
- [ ] 5.4 Confirmar prontidão para `kspec-qa`: listar os cenários E2E da techspec que o QA deve executar

## Detalhes de Implementação

- Sem código novo. Verificação obrigatória definida no PRD (seção **Restrições Técnicas de Alto Nível**) e na techspec (seção **Infraestrutura**).
- Cenários E2E para o QA estão na techspec, seção **Abordagem de Testes → Testes de E2E**.
- Se um critério de aceite falhar, não corrigir nesta task — reabrir a task responsável (1.0 pacote, 2.0 catálogo, 3.0 agent, 4.0 wiring/doctor) e corrigir lá.

## Critérios de Sucesso

- Verificação encadeada completa passa sem erros.
- Checklist de aceite com evidência por item:
  - Amostras absurdas (0, negativas, > 10000) não contaminam a média (`TestOutliersDiscarded`).
  - Valor exposto prioriza o EWMA com ≥ 5 amostras (`TestGetReturnsZeroBelowFiveSamples`).
  - Reiniciar o app não perde as estatísticas (`TestPersistenceRoundtrip`; E2E — QA).
  - Após 5+ chamadas, os critérios enviados ao Jev citam o TPS medido (`TestCandidatesCarryMeasuredTPS`).
  - Sem amostras suficientes, o texto usa o estimate atual (`TestCriteriaUsesEstimateByDefault`).
  - A tabela do `--doctor` mostra medido e estimado por modelo (`TestDoctorPrintsTelemetryTable`).
- Diff conforme os padrões do projeto.

## Testes da Tarefa

- [ ] Testes de unidade — todos os suites das tasks 1.0, 2.0, 3.0 e 4.0 passando (`go test ./...`).
- [ ] Testes de integração — cobertos pelos testes de agent com mock (task 3.0).
- [ ] Testes E2E — nenhum novo nesta task; execução deferida ao `kspec-qa` com a lista de cenários montada na subtarefa 5.4.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/telemetry/telemetry.go`, `internal/telemetry/telemetry_test.go` — diff da task 1.0
- `internal/catalog/catalog.go`, `internal/catalog/catalog_test.go` — diff da task 2.0
- `internal/agent/agent.go`, `internal/agent/agent_test.go` — diff da task 3.0
- `main.go` — diff da task 4.0
- `spec/tasks/006-prd-telemetria-de-tps/prd.md` — critérios de aceite (fonte do checklist)
- `spec/tasks/006-prd-telemetria-de-tps/techspec.md` — cenários E2E para o QA
