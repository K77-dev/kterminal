# Tarefa 1.0: Pacote `internal/telemetry` — estatísticas e persistência

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Coleta de TPS por modelo
- REQ-002 — Persistência local

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Pacote folha, isolado e com zero dependências do resto do projeto: `internal/telemetry` mantém estatísticas rolling de TPS por modelo — média incremental estilo Welford (sem guardar amostras, memória O(1)) + janela EWMA (α=0.3) — com outliers grosseiros (≤ 0 ou > 10000) descartados na porta. O `Store` persiste em `~/.local/share/kterminal/telemetry.json` com save atômico (temp + rename, 0600), debounce de 10s e save incondicional no `Close()`. O relógio é injetável (`now func() time.Time`) para testar o debounce sem dormir. `Get` retorna `(EWMA, Samples)` apenas com ≥ 5 amostras (senão `(0, Samples)`) — médias de 1-2 amostras são ruído; `GetMean` expõe a média Welford para o `--doctor`.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib (`sync`, `encoding/json`, `os`, `time`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E deferido), `kspec-pr-review` (revisão semântica).
- Padrões do projeto: sem comentários no código; sem dependências externas; relógio injetável para tempo.
</skills>

<requirements>
- Constantes: `ewmaAlpha = 0.3`, `measuredMinSamples = 5`, `saveDebounce = 10 * time.Second`, `outlierMaxTPS = 10000`.
- `Stats{Samples, Mean, M2, EWMA}`; `Store` com mutex, `models map[string]*Stats`, `path`, `lastSave`, `now` injetável.
- `New(path) *Store`, `Record(model, tps)`, `Get(model) (float64, int)`, `GetMean(model) (float64, int)`, `Close() error`.
- `Record`: outlier (≤ 0 ou > 10000) → descarta; válido → Welford (`n++; delta; mean += delta/n; m2 += delta*(tps-mean)`) e EWMA (`ewma = ewma*(1-α) + tps*α`; primeira amostra inicializa); save se `now() - lastSave >= saveDebounce`.
- `Get`: `(EWMA, Samples)` com `Samples >= 5`; senão `(0, Samples)` — nunca piora a informação do Jev (fallback para o estimate).
- Persistência: `telemetry.json` com `{"models": {...}}`; save atômico (temp + rename, 0600); `Close()` salva incondicionalmente.
- `Load` de caminho inexistente → store vazio funcional; JSON corrompido → store fresh (best-effort, telemetria nunca derruba a sessão).
- Concorrência: mutex protege o map (`Record` na goroutine do loop; `Close`/doctor concurrent-safe).
</requirements>

## Subtarefas

- [ ] 1.1 Criar `internal/telemetry/telemetry.go` com `Stats`, `Store`, `New` e `Record` (Welford + EWMA + descarte de outliers)
- [ ] 1.2 Implementar `Get`/`GetMean` com o limiar de 5 amostras
- [ ] 1.3 Implementar persistência: save atômico com debounce, `Close`, `Load` com fallback fresh
- [ ] 1.4 Escrever `internal/telemetry/telemetry_test.go` com os 7 testes da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Structs, constantes e regras de `Record`/`Get`/`GetMean`: techspec, seção **Design de Implementação → Interfaces Principais** (subseção "Regras").
- Decisões "Welford + EWMA sem histórico", "Get retorna 0 abaixo de 5", "save atômico + debounce": techspec, seção **Considerações Técnicas → Decisões Principais** (itens 1, 2, 5).
- Segurança (0600, sem PII, save atômico): techspec, seção **Verificações Técnicas → Segurança**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Testes provam: média Welford de sequência conhecida; outliers descartados; EWMA com α=0.3 verificado à mão; limiar de 5 amostras; roundtrip de persistência; debounce com relógio injetável; load de arquivo inexistente.
- O pacote é folha: importa só stdlib.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/telemetry/telemetry_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 1-7):
  - `TestWelfordMeanKnownSequence` — `[10, 20, 30]` → `GetMean` = 20, Samples = 3.
  - `TestOutliersDiscarded` — `0`, `-5`, `20000` → Samples = 0; válidas seguintes não contaminadas.
  - `TestEWMAConverges` — sequência constante 100 → converge ~100; α=0.3 verificado para 2 amostras.
  - `TestGetReturnsZeroBelowFiveSamples` — 4 amostras → (0, 4); 5ª → (ewma, 5).
  - `TestPersistenceRoundtrip` — `Record` ×3 → `Close` → `Load` → stats idênticos (tolerância de float).
  - `TestSaveDebounce` — `now` injetável: dois Records no mesmo instante → 1 save; +11s → 2º save; `Close` sempre salva.
  - `TestLoadMissingFileStartsFresh` — caminho inexistente → store vazio funcional.
- [ ] Testes de integração — fora do escopo da task; coleta→injeção→critérios é validado na task 3.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/telemetry/telemetry.go` — novo: `Stats`, `Store`, Welford/EWMA, persistência com debounce
- `internal/telemetry/telemetry_test.go` — novo: 7 testes (stats, outliers, roundtrip, debounce)
