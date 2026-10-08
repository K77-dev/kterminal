# Relatório de Code Review — 006 Telemetria de TPS: Task 5.0 (Verificação final integrada e checagem de critérios de aceite)

## Resumo

- Data: 2026-10-04
- Branch: 002-010-prds-kterminal
- Status: APROVADO
- Arquivos da feature 006: 8 (`internal/telemetry/telemetry.go`, `internal/telemetry/telemetry_test.go`, `internal/catalog/catalog.go`, `internal/catalog/catalog_test.go`, `internal/agent/agent.go`, `internal/agent/agent_test.go`, `main.go`, `main_test.go`)
- Linhas Adicionadas (escopo 006, somando tasks 1.0–4.0): ~715
- Linhas Removidas (escopo 006): ~7
- Natureza da task: verificação — nenhum código novo escrito, nenhum ajuste necessário

## Conformidade com Rules

| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Único `//` nos arquivos da feature é a diretiva `//go:embed models.yaml` (pré-existente, exigida pelo compilador); nenhum comentário adicionado nos diffs |
| Sem dependências externas | OK | `internal/telemetry` importa apenas stdlib (`encoding/json`, `os`, `path/filepath`, `sync`, `time`); diff do `go.mod` limitado à promoção de `muesli/termenv` de indirect para direct (feature 004, fora do escopo 006) |
| YAML intocado como fallback | OK | `git diff internal/catalog/models.yaml` vazio; `tps_estimate` permanece a fonte de fallback |
| Eventos via canal | OK | Nenhum canal novo; `Record` é síncrono na goroutine do loop do agent, conforme design da techspec |
| Formatação/lint | OK | `gofmt -l .` sem output; `go vet ./...` limpo |
| Estrutura de pastas | OK | Pacote folha `internal/telemetry`; `catalog` não importa `telemetry` (injeção é pull no agent) |
| Segurança (API/CORS/SQL/etc.) | N/A | TUI local sem backend; `telemetry.json` local 0600, save atômico (temp+rename), sem PII — apenas nomes de modelo e números |

## Aderência à TechSpec

| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `Stats{Samples, Mean, M2, EWMA}` + Welford incremental sem histórico de amostras | SIM | `internal/telemetry/telemetry.go` — memória O(1) por modelo |
| EWMA α=0.3, primeira amostra inicializa `ewma = tps` | SIM | `ewmaAlpha` const; verificado com valor calculado à mão em `TestEWMAConverges` |
| Outliers (≤ 0 ou > 10000) descartados na porta | SIM | `Record` retorna cedo; fronteira 10000 é válida (testada) |
| `Get` retorna `(EWMA, Samples)` com ≥ 5 amostras; senão `(0, Samples)` | SIM | `measuredMinSamples = 5`; fallback garantido ao estimate do YAML |
| `GetMean` (média Welford) só para o `--doctor` | SIM | Usado exclusivamente em `doctorTelemetryTable` |
| Persistência `~/.local/share/kterminal/telemetry.json` com `{"models": {...}}`, save atômico 0600, debounce 10s, `Close` incondicional | SIM | `writeAtomic` (CreateTemp 0600 + rename); `now` injetável testado sem dormir |
| `catalog.Model` com `MeasuredTPS`/`MeasuredSamples` sem tags YAML | SIM | Populados em memória pelo agent; `TestParseIgnoresMeasuredFields` |
| `Criteria()` alterna para `Speed: ~%.0f tok/s (measured over %d calls).` quando `MeasuredTPS > 0` | SIM | Texto exato da techspec; senão mantém estimate |
| Injeção pull no `decide()` do agent | SIM | Loop de candidatos idêntico ao snippet da techspec |
| `Record` pós-chamada quando `Usage.CompletionTokens > 0` | SIM | Grava também em passos de tool loop (`TestAgentRecordsTPSAfterEachStreamStep`); chamada sem completion tokens não grava |
| Wiring no `main.go`: Load + injeção + `defer Close` + tabela no `--doctor` | SIM | `telemetryPath()` XDG-aware seguindo o padrão do projeto |
| Arquitetura: `telemetry` folha, mutex no `Store`, `Record` só na goroutine do loop | SIM | `TestConcurrentAccess` (8 goroutines × 200 records) passa com race detector do `go test` |
| Falha de load/save silenciosa (best-effort) | SIM | `TestLoadCorruptJSONStartsFresh`, `TestLoadMissingFileStartsFresh`, `TestPersistFailureIsBestEffort` |

## Checklist de Critérios de Aceite do PRD (subtarefa 5.2)

| Critério de Aceite | Evidência | Status |
|--------------------|-----------|--------|
| REQ-001: Amostras absurdas (0, negativas, >10000) não contaminam a média | `TestOutliersDiscarded` — 0, -5 e 20000 descartados (Samples=0); amostras válidas seguintes não contaminadas; fronteira 10000 aceita, 10000.0001 descartada | OK |
| REQ-001: Valor exposto prioriza o EWMA quando houver ≥ 5 amostras | `TestGetReturnsZeroBelowFiveSamples` — 4 amostras → `(0, 4)`; 5ª amostra → `(ewma, 5)` com valor hand-computado; `TestEWMAConverges` valida α=0.3 | OK |
| REQ-002: Reiniciar o app não perde as estatísticas | `TestPersistenceRoundtrip` — Record ×3 + Close + Load → stats idênticos (mean/m2/ewma), modo 0600, record pós-reload acumula (25, 4). E2E de reinício real: **deferido ao `kspec-qa`**. Verificação comportamental complementar: `--doctor` carregou `telemetry.json` pré-existente e exibiu as stats | OK |
| REQ-003: Após 5+ chamadas num modelo, os critérios enviados ao Jev citam o TPS medido | `TestCandidatesCarryMeasuredTPS` — store com 5 amostras → mock Jev captura critério `glm-5.3` contendo `~50 tok/s` e `measured over 5 calls` | OK |
| REQ-003: Sem amostras suficientes, o texto usa o estimate atual | `TestCriteriaUsesEstimateByDefault` — sem `MeasuredTPS` → `~70 tok/s` sem "measured"; `TestCandidatesCarryMeasuredTPS` confirma o caso glm-5.2 no mesmo fluxo | OK |
| REQ-004: A tabela do `--doctor` mostra medido e estimado por modelo | `TestDoctorPrintsTelemetryTable` — header `model \| measured (mean tok/s, samples) \| estimate (tok/s)` + linha completa por modelo; verificação comportamental executada (ver abaixo) | OK |

### Verificação integrada comportamental (ponta a ponta)

Executada além dos testes de unidade, com binário buildado e `XDG_DATA_HOME` isolado:

1. `telemetry.json` pré-populado (3 modelos com amostras, 4 sem) → `kterminal --doctor` imprimiu a seção `telemetry:` com as 7 linhas do catálogo, medido (mean + samples) ao lado do estimate — ex.: `glm-5.2 | 88.5 tok/s, 12 samples | 70 tok/s`, modelos sem amostras como `no samples`. Exit 0, `all checks passed`.
2. Fluxo de consumo coberto por teste de integração com mocks: chamada → `Record` (TPS real medido com gateway com delay) → `decide()` injeta medido → Jev recebe critérios citando `measured over N calls` (`TestAgentRecordsTPSAfterCall`, `TestCandidatesCarryMeasuredTPS`).

## Verificação de Escopo (fora do PRD — confirmado não implementado)

| Item fora de escopo | Status |
|---------------------|--------|
| Telemetria remota ou compartilhada entre máquinas | OK — nenhum import de rede em `internal/telemetry` |
| Gráficos ou visualização de histórico na TUI | OK — zero referências a telemetry em `internal/tui` |
| Medição separada de latência de rede vs. geração | OK — apenas TPS |
| Coleta de métricas além de TPS (custo, latência total) | OK — `Stats` contém apenas Samples/Mean/M2/EWMA |
| Configuração de α do EWMA pelo usuário | OK — `ewmaAlpha` é const |

## Tasks Verificadas

| Task | Status | Observações |
|------|--------|-------------|
| 1.0 Pacote `internal/telemetry` | COMPLETA | Review 1.0 APROVADO; 12 testes (7 da techspec + corrupt JSON, concorrência, nil store, persist failure) |
| 2.0 Campos calculados e `Criteria()` alternável | COMPLETA | Review 2.0 APROVADO; 3 testes |
| 3.0 Coleta pós-chamada e injeção no Agent | COMPLETA | Review 3.0 APROVADO; 3 testes de integração com mocks |
| 4.0 Wiring no `main.go` e tabela no `--doctor` | COMPLETA | Review 4.0 APROVADO; 2 testes |
| 5.1 Verificação encadeada `go build && go vet && gofmt -l && go test` | COMPLETA | Execução encadeada única, tudo verde (`-count=1`, sem cache) |
| 5.2 Checklist de critérios de aceite com evidência | COMPLETA | Tabela acima — 6/6 critérios com teste + 2 com verificação comportamental |
| 5.3 Inspeção do diff contra os padrões do projeto | COMPLETA | Sem comentários, sem dependências externas novas, YAML intocado |
| 5.4 Prontidão para `kspec-qa` com cenários E2E listados | COMPLETA | Ver seção seguinte |

## Cenários E2E para o `kspec-qa` (subtarefa 5.4)

Da techspec (Abordagem de Testes → Testes de E2E):

1. **5+ chamadas num modelo → critérios do Jev citam o medido**: configurar gateway real, fazer 5+ chamadas a um mesmo modelo, verificar que o router passa a receber `Speed: ~X tok/s (measured over N calls)` (observável via `--doctor` acumulando amostras e pelo comportamento de roteamento).
2. **Reinício do app preserva estatísticas**: fazer chamadas, encerrar o app (Close salva), relançar e rodar `--doctor` — as amostras/mean anteriores permanecem e acumulam.
3. **`--doctor` mostra a tabela**: executar `kterminal --doctor` e conferir a seção `telemetry:` com colunas medido × estimado para todos os modelos do catálogo (incluindo `no samples` para modelos sem dados).

## Testes

- Comando: `go build ./... && go vet ./... && gofmt -l . && go test ./... -count=1` — **tudo verde em execução encadeada única**
- Total de Testes: 122
- Passando: 122
- Falhando: 0
- Coverage (pacotes da feature, sem regressão vs. review 4.0): telemetry 92.5%, agent 85.4%, catalog 57.6%, main 16.2%
- Testes de evidência dos critérios de aceite executados individualmente (`-v`): `TestWelfordMeanKnownSequence`, `TestOutliersDiscarded`, `TestEWMAConverges`, `TestGetReturnsZeroBelowFiveSamples`, `TestPersistenceRoundtrip`, `TestSaveDebounce`, `TestLoadMissingFileStartsFresh`, `TestCriteriaUsesEstimateByDefault`, `TestCriteriaUsesMeasuredWhenAvailable`, `TestParseIgnoresMeasuredFields`, `TestAgentRecordsTPSAfterCall`, `TestAgentRecordsTPSAfterEachStreamStep`, `TestCandidatesCarryMeasuredTPS`, `TestTelemetryPathRespectsXDGDataHome`, `TestDoctorPrintsTelemetryTable` — todos PASS
- E2E: nenhum novo nesta task, conforme definido; execução deferida ao `kspec-qa` com os 3 cenários listados

## Problemas Encontrados

| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| — | — | — | Nenhum problema encontrado na verificação final | — |

## Pontos Positivos

- Os 12 testes planejados na techspec existem e passam, mais 7 testes extras de robustez (JSON corrompido, concorrência, nil store, falha de persistência, fronteira de outlier, gravação por passo de tool loop, path XDG).
- Direção de dependência limpa: `catalog` puro (não conhece telemetry), `telemetry` folha stdlib, agent faz o pull — exatamente a decisão 4 da techspec.
- Fallback não negociável ("nunca piorar a informação do Jev") garantido em camada dupla: `Get` retorna 0 abaixo de 5 amostras e `Criteria()` só alterna com `MeasuredTPS > 0`.
- Verificação comportamental do `--doctor` confirmou o fluxo de reinício (Load de JSON pré-existente) e a tabela medido × estimado para os 7 modelos.
- Nenhum requisito do PRD ficou de lado e nenhum item fora de escopo foi implementado.

## Recomendações

- Executar `kspec-qa` com os 3 cenários E2E listados (subtarefa 5.4) antes do `kspec-pr-review`.
- A techspec 010 (subagentes) deve confirmar o compartilhamento do `Store` do agent principal, conforme anotado na própria techspec ("Interação com techspec 010").

## Conclusão

A verificação final integrada confirmou a feature completa: o fluxo ponta a ponta (chamadas → amostras gravadas com outliers descartados → catálogo com TPS medido injetado → roteamento usa o medido com ≥ 5 amostras → `--doctor` exibe medido × estimado → persistência sobrevive a reinício) está implementado conforme a techspec, os 6 critérios de aceite do PRD têm evidência de teste passando (com os E2E de reinício devidamente deferidos ao `kspec-qa`), os padrões do projeto estão respeitados (sem comentários, stdlib apenas, YAML intocado) e a verificação encadeada completa passa com 122/122 testes. Nenhum ajuste foi necessário. A feature está pronta para o `kspec-qa` e o `kspec-pr-review`. **Veredito: APROVADO.**
