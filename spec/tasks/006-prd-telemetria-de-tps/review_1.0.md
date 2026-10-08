# Relatório de Code Review — Task 1.0: Pacote `internal/telemetry` — estatísticas e persistência

## Resumo

- **Data**: 2026-10-04
- **Branch**: `002-010-prds-kterminal`
- **Status**: **APROVADO**
- **Arquivos Modificados**: 2 (novos: `internal/telemetry/telemetry.go`, `internal/telemetry/telemetry_test.go`)
- **Linhas Adicionadas**: 462
- **Linhas Removidas**: 0
- **Reviewer**: kspec-review-runner (auto-review do task-runner)

## Veredito

**APROVADO** — a implementação atende integralmente a Task 1.0, a REQ-001 e a REQ-002, e segue a Tech Spec (Interfaces Principais, regras de `Record`/`Get`/`GetMean`, persistência atômica com debounce, `Load` best-effort). Os 7 testes mandatórios existem e provam o que a task pede; 4 testes adicionais cobrem requisitos explícitos não listados na seção de testes (JSON corrompido, concorrência, nil-safety, falha de persistência). Todos os checks passam. Nenhuma mudança fora de escopo.

## Conformidade com Rules

| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código (padrão do projeto) | OK | `grep -n "//"` nos dois arquivos: nenhum comentário |
| Sem dependências externas (stdlib only) | OK | `go list -deps ./internal/telemetry/`: apenas pacotes stdlib (`sync`, `encoding/json`, `os`, `path/filepath`, `time`) — pacote folha confirmado |
| Formatação/lint | OK | `gofmt -l .` sem output; `go vet ./...` sem achados |
| Nomenclatura Go | OK | Exportados: `Stats`, `Store`, `New`, `Load`, `Record`, `Get`, `GetMean`, `Save`, `Close`; unexported: `statsFor`, `persist`, `writeAtomic`, `diskStore`, constantes |
| Padrões de teste do repositório (`testing`, AAA, `t.TempDir()`, relógio injetável) | OK | Mesmo padrão de `internal/session/session_test.go` (helpers, temp dirs, sem framework externo) |
| Tratamento de erro | OK | Falhas de save silenciosas em `Record` (spec: best-effort); `Close`/`Save` retornam `error`; `Load` nunca erroa (spec: telemetria nunca derruba a sessão) |
| Logging | N/A | Techspec: "Sem logs novos" |
| Rules DDD/TS/Vitest | N/A | Brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec |

## Verificação de Segurança

| Item | Status | Observações |
|------|--------|-------------|
| Arquivo persistido com 0600 | OK | `os.CreateTemp` (0600 por definição) + rename; provado em `TestPersistenceRoundtrip` (`Mode().Perm() == 0o600`) |
| Save atômico (temp + rename) | OK | `writeAtomic`: temp no mesmo diretório → write → close → rename; temp residual removido em todos os caminhos |
| Sem PII / secrets no arquivo | OK | `telemetry.json` contém apenas `{"models": {nome: {samples, mean, m2, ewma}}}` — sem conteúdo de conversa, sem chaves |
| Dados malformados tolerados | OK | JSON corrompido → store fresh (`TestLoadCorruptJSONStartsFresh`); entradas `null` descartadas no `Load` |
| Endpoints/CORS/SQL/auth | N/A | Pacote local sem rede nem backend |

## Aderência à TechSpec

| Decisão Técnica (techspec) | Implementado | Evidência |
|-----------------|--------------|-------------|
| `Stats{Samples, Mean, M2, EWMA}` | SIM | `telemetry.go:16-21`, idêntico ao bloco Interfaces Principais |
| Constantes `ewmaAlpha=0.3`, `measuredMinSamples=5`, `saveDebounce=10s`, `outlierMaxTPS=10000` | SIM | `telemetry.go:10-14`; todas usadas (`ewmaAlpha`/`outlierMaxTPS` em `Record`, `measuredMinSamples` em `Get`, `saveDebounce` em `Save`) |
| `Store` com mutex, `models`, `path`, `lastSave`, `now` injetável | SIM | `telemetry.go:27-33`; default `time.Now` em `New`; testes injetam `s.now` sem dormir |
| `New(path) *Store` | SIM | `telemetry.go:35-41` |
| `Record`: outlier `tps <= 0 \|\| tps > 10000` → descarta | SIM | `telemetry.go:58`; gate antes de qualquer estado |
| `Record`: Welford `n++; delta; mean += delta/n; m2 += delta*(tps-mean)` | SIM | `telemetry.go:63-66`, fórmula literal da spec; provado com `[10,20,30]` → mean 20, M2 200 |
| `Record`: EWMA `ewma*(1-α) + tps*α`, primeira amostra inicializa | SIM | `telemetry.go:67-71`; α=0.3 verificado à mão (2 amostras → 13; cadeia de 5 → 32.269) |
| `Record`: save se `now() - lastSave >= saveDebounce` | SIM | `Record` → `Save()` com o check exato (`telemetry.go:118-120`) |
| `Get`: `(EWMA, Samples)` com `Samples >= 5`; senão `(0, Samples)` | SIM | `telemetry.go:79-92`; fallback "nunca piora" preservado (0 → estimate do YAML) |
| `GetMean`: média + amostras, sem limiar (para o `--doctor`) | SIM | `telemetry.go:94-104` |
| `Save()` com debounce (visão de componentes) | SIM | `telemetry.go:106-115` |
| `Close()` salva incondicionalmente | SIM | `telemetry.go:117-123` → `persist` sem check de debounce; provado no debounce test |
| `Load(path)`: inexistente → store vazio funcional; corrompido → fresh | SIM | `telemetry.go:43-56`; ambos provados em teste |
| Persistência `{"models": {...}}`, temp+rename, 0600 | SIM | `diskStore` + `persist`/`writeAtomic`; chave `models` e perms verificadas no roundtrip |
| Concorrência: mutex protege o map; `Close`/doctor concurrent-safe | SIM | Todas as operações de map sob `mu`; `TestConcurrentAccess` (8 goroutines × 25 records + Gets) limpo com `-race` |
| Pacote folha (importa só stdlib) | SIM | `go list -deps` confirma |

### Decisões de interpretação (documentadas)

1. **`New` × `Load`**: o bloco Interfaces Principais lista `New(path)`; a visão de componentes e os testes da própria techspec exigem `Load(path)` retornando store. Resolução: `New` é o construtor (store vazio vinculado ao path) e `Load` = `New` + hidratação best-effort. O wiring da task 4.0 usará `Load`, consistente com "cria o `Store` (load de `~/.local/share/kterminal/telemetry.json`)".
2. **Tags JSON em `Stats`** (`samples`/`mean`/`m2`/`ewma`): o formato de arquivo `{"models": {...}}` exige tags; chaves minúsculas, consistentes com `models`. O bloco de interfaces omite tags por ser esquemático.
3. **`Save()` exportado**: incluído porque a visão de componentes lista "`Save()` com debounce"; semântica: no-op silencioso (retorna `nil`) dentro da janela. `Record` o utiliza como caminho único do debounce.
4. **`models` unexported**: conforme o comentário estrutural do bloco de interfaces (`models map[string]*Stats`). O `--doctor` (task 4.0) consome via `GetMean`, sem necessidade de map exportado.
5. **`lastSave` parte do zero**: primeiro `Record` persiste imediatamente (elapsed desde zero >> debounce) — casa com o teste da techspec ("dois Records no mesmo instante → 1 save") e evita perder a primeira amostra num crash.

## Tasks Verificadas

| Task | Status | Observações |
|------|--------|-------------|
| 1.1 `Stats`, `Store`, `New`, `Record` (Welford + EWMA + outliers) | COMPLETA | `telemetry.go` |
| 1.2 `Get`/`GetMean` com limiar de 5 amostras | COMPLETA | limiar só em `Get`; `GetMean` sem limiar (doctor) |
| 1.3 Persistência: save atômico com debounce, `Close`, `Load` com fallback fresh | COMPLETA | temp+rename 0600, debounce 10s, `Close` incondicional, `Load` best-effort |
| 1.4 `telemetry_test.go` com os 7 testes da techspec | COMPLETA | 7 testes mandatórios + 4 adicionais (ver abaixo) |

## Testes

- **Total de Testes**: 114 (103 pré-existentes + 11 novos)
- **Passando**: 114
- **Falhando**: 0
- **Coverage**: 92.5% (`go test -cover ./internal/telemetry/`); restante = guards defensivos de marshal e caminhos de erro de I/O não injetáveis sem abstrair o filesystem (fora do escopo da spec)

### Testes mandatórios (techspec, itens 1-7) — todos PASS

| Teste | O que prova |
|-------|-------------|
| `TestWelfordMeanKnownSequence` | `[10,20,30]` → mean 20, samples 3; M2 = 200 (prova o algoritmo, não só a média) |
| `TestOutliersDiscarded` | `0`, `-5`, `20000` → samples 0; fronteira `10000` aceita e `10000.0001` descartada; válidas seguintes não contaminadas (mean 5025 exato) |
| `TestEWMAConverges` | α=0.3 verificado à mão com 2 amostras (10\*0.7+20\*0.3 = 13, via estado interno — `Get` é gated em 5 por design); sequência constante 100 → EWMA ~100 via API pública |
| `TestGetReturnsZeroBelowFiveSamples` | modelo desconhecido → (0,0); 4 amostras → (0,4); 5ª → (32.269, 5) — cadeia EWMA completa calculada à mão via API pública |
| `TestPersistenceRoundtrip` | 2 modelos; perms 0600; chave top-level `models`; stats idênticos pós-reload (tolerância float); Welford continua corretamente pós-reload (25, 4) |
| `TestSaveDebounce` | relógio injetável: 2 records no mesmo instante → 1 save (arquivo com 1 amostra); +11s → 2º save (3); dentro da janela → sem save; `Close` salva com 0s decorrido (4). Contagem via conteúdo persistido (comportamental, sem mocks) |
| `TestLoadMissingFileStartsFresh` | caminho inexistente (dir aninhado) → store vazio funcional: registra, responde e persiste |

### Testes adicionais (requisitos explícitos da task/techspec fora da lista 1-7)

| Teste | Requisito que cobre |
|-------|-------------|
| `TestLoadCorruptJSONStartsFresh` | "JSON corrompido → store fresh" (requirements da task) + recuperação: save sobrescreve com JSON válido |
| `TestConcurrentAccess` | "Concorrência: mutex protege o map; Close/doctor concurrent-safe" — 8 goroutines × 25 records + Gets concorrentes, 200 amostras exatas, `-race` limpo |
| `TestNilStoreIsSafe` | Contrato defensivo nil-safe, padrão do projeto (cf. `session.Writer`) |
| `TestPersistFailureIsBestEffort` | "Falha de save é silenciosa com fallback na memória — telemetria nunca derruba a sessão" + contrato de erro do `Close() error` (path sem escrita → Record não pânico, stats na memória, Close retorna erro) |

### Verificação obrigatória executada

```
go build ./...            OK
go vet ./...              OK (sem achados)
gofmt -l .                OK (sem output)
go test ./... -count=1    ok kterminal, internal/agent, internal/llm, internal/session,
                          internal/telemetry, internal/tools, internal/tui — 114 PASS, 0 FAIL
go test -race -count=1 ./internal/telemetry/   OK (sem data races)
```

Os 103 testes pré-existentes continuam passando (nada quebrou).

## Problemas Encontrados

| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | `telemetry.go` | 58 | `NaN` atravessa o gate de outliers (`NaN <= 0` e `NaN > 10000` são ambos falsos) | Inalcançável na prática: o agent grava só com `CompletionTokens > 0` (techspec), logo `tps ∈ (0, +Inf]`; `+Inf` é descartado por `> 10000` e `StreamSeconds < 0` produz `tps ≤ 0` descartado. Regra literal da spec mantida; task 3.0 deve preservar o guard de tokens |
| Baixa | `telemetry.go` | 106-115 | `Save()` retorna `nil` quando dentro da janela de debounce (skip silencioso) — caller não distingue "salvo" de "debounced" | Semântica best-effort da spec; `Close` é o save garantido. Nenhum caller atual depende da distinção; registrar para a task 4.0 |
| Baixa | `telemetry.go` | 143-150 | `lastSave` só avança em save bem-sucedido → path permanentemente sem escrita tenta I/O a cada `Record` | Custo desprezível (1 tentativa por chamada de LLM, falha rápida no `MkdirAll`) e habilita retry quando a falha é transitória. Aceito como está |

## Pontos Positivos

- Fórmulas provadas com valores calculados à mão (mean 20/M2 200; EWMA 13 e 32.269), não apenas "executa sem erro" — um regresso de α ou da fórmula de Welford quebra teste.
- Debounce 100% determinístico: relógio injetável, zero `time.Sleep` nos testes.
- Roundtrip valida o contrato completo de persistência: formato `{"models": ...}`, perms 0600, igualdade de stats e continuidade do Welford após reload.
- Comportamento best-effort provado nos dois sentidos: falha de load (corrompido → fresh) e falha de save (memória preservada, `Close` reporta).
- Concorrência exercitada com `-race` limpo.
- Escopo cirúrgico: 2 arquivos novos, nenhum arquivo existente tocado, nada das tasks 2.0–5.0 antecipado.

## Recomendações

1. **Task 3.0**: preservar o guard `Usage.CompletionTokens > 0` antes de `Record` (já previsto na techspec) — mantém `NaN`/`Inf` fora do alcance do gate de outliers.
2. **Task 4.0**: usar `telemetry.Load(path)` no wiring (semântica de `New` × `Load` documentada acima) e `defer store.Close()` para o save final.
3. Considerar, no futuro, `math.IsNaN` no gate se a origem do `tps` mudar — hoje é desnecessário.

## Conclusão

A Task 1.0 entrega o pacote folha `internal/telemetry` exatamente como desenhado na Tech Spec: estatísticas O(1) por modelo (Welford + EWMA α=0.3) com descarte de outliers na porta, `Get` com limiar de 5 amostras preservando o fallback para o estimate, e persistência crash-safe (temp+rename, 0600) com debounce de 10s testável sem dormir e `Close` incondicional. Os 7 testes mandatórios provam o que a task exige; os 4 adicionais cobrem requisitos explícitos (corrupção, concorrência, best-effort). 114/114 testes passando, build/vet/gofmt limpos, `-race` limpo. **APROVADO** — base pronta para a task 2.0 (campos calculados no catálogo).
