# Tech Spec — Telemetria real de TPS alimentando o catálogo

## Requisitos Atendidos

- REQ-001 — Coleta de TPS por modelo
- REQ-002 — Persistência local
- REQ-003 — Integração com o catálogo e o router
- REQ-004 — Visibilidade no `--doctor`

## Resumo Executivo

Novo pacote `internal/telemetry` mantém estatísticas rolling de TPS por modelo: média incremental estilo Welford (sem guardar amostras) + janela EWMA (α=0.3); outliers grosseiros (≤ 0 ou > 10000) são descartados na porta. O `Agent` já computa `tps = CompletionTokens / StreamSeconds` após cada chamada — passa a gravar `telemetry.Record(model, tps)` quando há tokens de output. O `Store` persiste em `~/.local/share/kterminal/telemetry.json` com debounce (máx. 1 save a cada 10s) e save no encerramento. O `catalog.Model` ganha campos calculados `MeasuredTPS`/`MeasuredSamples` (não vêm do YAML); o agent os injeta ao montar candidatos para o router, e `Criteria()` cita `Speed: ~X tok/s (measured over N calls)` quando `MeasuredTPS > 0` (≥ 5 amostras) — senão mantém o estimate do YAML, nunca piorando a informação do Jev. O `--doctor` imprime a tabela medido × estimado.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/telemetry/telemetry.go`** (novo): `Stats{Samples, Mean, M2, EWMA}`, `Store{Models map[string]*Stats}` com `Record(model, tps)`, `Get(model) (ewma float64, n int)`, `GetMean(model) (mean float64, n int)`, `Load(path)`, `Save()` com debounce, `Close()`.
- **`internal/agent/agent.go`** (modificado): campo `Telemetry *telemetry.Store`; após cada `ChatStream` com `Usage.CompletionTokens > 0` → `Record(decision.Model, tps)`; ao montar candidatos para o router, copia cada `catalog.Model` e injeta `MeasuredTPS`/`MeasuredSamples`.
- **`internal/catalog/catalog.go`** (modificado): `Model` ganha `MeasuredTPS float64`, `MeasuredSamples int` (calculados, ignorados no parse do YAML); `Criteria()` alterna o texto de speed conforme `MeasuredTPS > 0`.
- **`main.go`** (modificado): cria o `Store` (load de `~/.local/share/kterminal/telemetry.json`), injeta no agent, `defer store.Close()`; `--doctor` imprime a tabela medido/estimado.
- **`internal/tui`** (sem mudança): o TPS do hint bar já é o medido por resposta.

Fluxo de dados:

1. **Coleta**: fim de `ChatStream` no loop → `tps` já computado → `Record(model, tps)` → Welford + EWMA atualizados → save com debounce.
2. **Consumo**: `decide()` monta candidatos com valores medidos injetados → `Criteria()` do Jev cita o medido quando há ≥ 5 amostras.
3. **Transparência**: `--doctor` → `Store.GetMean` por modelo × `TPSEstimate` do YAML.

## Design de Implementação

### Interfaces Principais

```go
const ewmaAlpha = 0.3
const measuredMinSamples = 5
const saveDebounce = 10 * time.Second
const outlierMaxTPS = 10000

type Stats struct {
	Samples int
	Mean    float64
	M2      float64
	EWMA    float64
}

type Store struct {
	// mu sync.Mutex; models map[string]*Stats; path string; lastSave time.Time; now func() time.Time
}

func New(path string) *Store
func (s *Store) Record(model string, tps float64)
func (s *Store) Get(model string) (float64, int)
func (s *Store) GetMean(model string) (float64, int)
func (s *Store) Close() error
```

Regras:

- `Record`: `tps <= 0 || tps > outlierMaxTPS` → descarta (REQ-001). Válido → Welford (`n++; delta = tps - mean; mean += delta/n; m2 += delta*(tps-mean)`) e EWMA (`ewma = ewma*(1-α) + tps*α`; primeira amostra inicializa `ewma = tps`). Após atualizar, `save` se `now() - lastSave >= saveDebounce`.
- `Get`: retorna `(EWMA, Samples)` quando `Samples >= measuredMinSamples`; senão `(0, Samples)` — `MeasuredTPS = 0` faz o `Criteria()` cair no estimate (nunca piora).
- `GetMean`: média aritmética Welford + amostras — usada só pelo `--doctor`.
- Persistência: `telemetry.json` com `{"models": {"<name>": {stats...}}}`; save atômico (escreve temp + rename, 0600); `Close()` salva incondicionalmente.
- `now func() time.Time` injetável (default `time.Now`) — debounce testável sem dormir.
- Injeção no agent (`decide`):

```go
candidates := make([]catalog.Model, len(a.candidates))
for i, m := range a.candidates {
	if tp, n := a.Telemetry.Get(m.Name); tp > 0 {
		m.MeasuredTPS, m.MeasuredSamples = tp, n
	}
	candidates[i] = m
}
```

- `Criteria()`: `MeasuredTPS > 0` → `Speed: ~%.0f tok/s (measured over %d calls)`; senão o texto atual (`~%.0f tok/s` do estimate).

### Modelos de Dados

```go
type Model struct {
	// campos YAML existentes...
	MeasuredTPS     float64
	MeasuredSamples int
}
```

- `MeasuredTPS`/`MeasuredSamples` sem tags YAML — nunca parseados do `models.yaml`; populados em memória pelo agent.
- `telemetry.json`: schema plano versionável por campo (sem versão explícita na v1; campos novos são adicionais).

## Pontos de Integração

- **Nenhuma externa** — arquivo local, stdlib.
- **Jev (Typesafe)**: sem mudança de contrato — o `Criteria()` continua sendo um string por candidato; o Jev apenas vê texto mais preciso quando há amostras.
- **`--doctor`**: após as checagens existentes, imprime tabela `model | measured (mean tok/s, samples) | estimate (tok/s)` — dados do `Store` carregado + catálogo.

## Verificações Técnicas

### Segurança

- `telemetry.json` em `~/.local/share/kterminal/` com 0600 (padrão do projeto); contém apenas nomes de modelo e números — sem PII, sem conteúdo de conversa.
- Save atômico (temp + rename) evita arquivo corrompido em crash.

### Arquitetura

- `telemetry` é folha (importa só stdlib) — agent e main dependem dele; catalog não importa telemetry (injeção é pull no agent, mantendo `catalog` puro).
- Concorrência: `Record` chamado só na goroutine do loop do agent; mutex no `Store` protege o map para o `--doctor`/`Close` concurrent-safe.
- Fallback garantido: `Get` retorna 0 com < 5 amostras → estimate do YAML — requisito não negociável "nunca piorar a informação do Jev".

### Infraestrutura

- Sem novos requisitos — stdlib. Verificação: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

## Abordagem de Testes

### Testes Unidade

**`internal/telemetry/telemetry_test.go`**:

1. `TestWelfordMeanKnownSequence` — amostras `[10, 20, 30]` → `GetMean` = 20, Samples = 3.
2. `TestOutliersDiscarded` — `Record(model, 0)`, `-5`, `20000` → Samples = 0; amostras válidas seguintes não contaminadas.
3. `TestEWMAConverges` — sequência constante 100 → EWMA converge para ~100; α=0.3 verificado com valor esperado calculado à mão para 2 amostras.
4. `TestGetReturnsZeroBelowFiveSamples` — 4 amostras → `Get` = (0, 4); 5ª amostra → `Get` = (ewma, 5).
5. `TestPersistenceRoundtrip` — `Record` ×3 → `Close()` → `Load(path)` → stats idênticos (mean, samples, ewma com tolerância de float).
6. `TestSaveDebounce` — `now` injetável: dois `Record` no mesmo instante → 1 save; avançar 11s + `Record` → 2º save; `Close` sempre salva.
7. `TestLoadMissingFileStartsFresh` — `Load` de caminho inexistente → store vazio funcional.

**`internal/catalog/catalog_test.go`**:

8. `TestCriteriaUsesEstimateByDefault` — modelo sem `MeasuredTPS` → texto contém `~70 tok/s` (estimate), sem "measured".
9. `TestCriteriaUsesMeasuredWhenAvailable` — `MeasuredTPS: 92.4, MeasuredSamples: 7` → texto contém `measured over 7 calls`.

**`internal/agent/agent_test.go`**:

10. `TestAgentRecordsTPSAfterCall` — mock gateway com usage conhecido → `Store.GetMean(model)` reflete a amostra; chamada sem completion tokens não grava.
11. `TestCandidatesCarryMeasuredTPS` — store com 5+ amostras num modelo → `decide` passa ao mock Jev critérios contendo "measured".

**`main.go`/doctor**:

12. `TestDoctorPrintsTelemetryTable` — captura da saída de `--doctor` contém colunas medido/estimado (função extraída testável `doctorTelemetryTable(store, cat) string`).

### Testes de Integração

Cobertos pelos testes de agent com mock (10, 11) — coleta→injeção→critérios ponta a ponta.

### Testes de E2E

Deferidos para `kspec-qa`: 5+ chamadas num modelo → critérios do Jev citam medido; reinício do app preserva estatísticas; `--doctor` mostra a tabela.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. **`internal/telemetry`** + testes de stats/persistência — pacote isolado, zero dependências do resto.
2. **`internal/catalog`**: campos calculados + `Criteria()` alternável + testes.
3. **`internal/agent`**: `Record` pós-chamada + injeção nos candidatos.
4. **`main.go`**: wiring do store (load/debounce/Close) + tabela no `--doctor`.
5. **Verificação final**: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.

### Dependências Técnicas

- Nenhuma externa — stdlib (`sync`, `encoding/json`, `os`, `time`).
- Interação com techspec 010 (subagentes): subagentes compartilham o `Store` do agent principal — chamadas de subagente também alimentam a telemetria do modelo usado (comportamento desejado, anotar na 010).

## Monitoramento e Observabilidade

### Error Tracking

Falha de load/save de `telemetry.json` é silenciosa com fallback para store vazio/na memória — telemetria nunca derruba a sessão (best-effort por natureza).

### Logging Estruturado

Sem logs novos — o `--doctor` é a janela de transparência; o JSONL existente já grava `tps` por turno (evento `assistant`), permitindo reconstruir amostras históricas se necessário.

### Health Checks / Métricas / Alertas

Não aplicável — TUI local. A própria telemetria é a métrica de negócio: TPS medido por modelo é o KPI do pitch do produto.

## Considerações Técnicas

### Decisões Principais

1. **Welford + EWMA, sem histórico de amostras** (PRD): memória O(1) por modelo; EWMA prioriza janela recente (rede/mudança de load do gateway); média Welford mantida para o `--doctor`.
2. **`Get` retorna 0 abaixo de 5 amostras** em vez da média parcial: médias de 1-2 amostras são ruído — o estimate do YAML é melhor informação; alterna só com confiança estatística mínima.
3. **Campos calculados no `catalog.Model`** em vez de map paralelo no agent: `Criteria()` fica coesa (o texto lê o modelo completo); parse do YAML intocado.
4. **Injeção pull no agent** (`Get` ao montar candidatos): catalog não conhece telemetry — direção de dependência limpa.
5. **Save atômico + debounce 10s**: crash-safe e sem I/O em cada token; `Close` garante a última amostra.

### Riscos Conhecidos

- **TPS distorcido por rede lenta na primeira chamada**: EWMA α=0.3 recupera em poucas amostras; outliers > 10000 descartados; lentidão real é dado válido (é o ambiente do usuário).
- **Concorrência doctor×loop**: mutex do `Store` cobre; `--doctor` roda antes da TUI na prática.
- **Corrupção do JSON**: `Load` falha → store fresh (estatísticas recomeçam) — degradação aceitável para dados de telemetria.
- **Modelo renomeado no catálogo**: stats órfãs no JSON — inofensivas (nunca consultadas); limpeza fora de escopo.

### Conformidade com Skills Padrões

- `architecture-ddd.md`: não aplicável — brownfield Go com packages `internal/`; a rule determina não impor DDD. Seção Bounded Context removida conforme o template.
- Rules TS/tests.md (Vitest)/logging.md: não aplicáveis — stack Go; padrões do repositório (`testing`, AAA, temp dirs, relógio injetável para tempo).
- Padrões do projeto: sem comentários no código, eventos via canal, sem dependências externas.
- Skills kspec aplicáveis: `kspec-tasks`, `kspec-implement`, `kspec-qa` (persistência entre reinícios, `--doctor`), `kspec-pr-review`.

### Arquivos relevantes e dependentes

| Arquivo | Papel na feature |
| --- | --- |
| `internal/telemetry/telemetry.go` | Novo — `Stats`, `Store`, Welford/EWMA, persistência com debounce |
| `internal/telemetry/telemetry_test.go` | Novo — 7 testes (stats, outliers, roundtrip, debounce) |
| `internal/catalog/catalog.go` | `MeasuredTPS`/`MeasuredSamples` calculados; `Criteria()` alternável |
| `internal/catalog/catalog_test.go` | Critérios estimate × measured |
| `internal/agent/agent.go` | `Record` pós-chamada; injeção de medidos nos candidatos |
| `internal/agent/agent_test.go` | Coleta pós-chamada; critérios com "measured" |
| `main.go` | Wiring do store; tabela de telemetria no `--doctor` |
| `internal/catalog/models.yaml` | Sem mudança — `tps_estimate` permanece como fallback |
