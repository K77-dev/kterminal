# Relatório de Code Review — Task 3.0: Coleta pós-chamada e injeção de TPS medido no Agent

## Resumo

- **Data**: 2026-10-04
- **Branch**: `002-010-prds-kterminal`
- **Status**: **APROVADO**
- **Arquivos Modificados**: 2 (`internal/agent/agent.go`, `internal/agent/agent_test.go`)
- **Linhas Adicionadas**: 173 (21 em `agent.go` + 152 em `agent_test.go`)
- **Linhas Removidas**: 9 (7 realinhamentos gofmt do struct `Agent` + 2 chamadas `Route` que passaram a receber os candidatos injetados)
- **Reviewer**: kspec-review-runner (auto-review do task-runner)

## Veredito

**APROVADO** — a implementação fecha o ciclo coleta→consumo exatamente como a Task 3.0 e a Tech Spec prescrevem: o `Agent` ganha o campo `Telemetry *telemetry.Store`, grava `Record(decision.Model, tps)` após cada `ChatStream` do loop com `Usage.CompletionTokens > 0` (guard anti-NaN herdado da review 1.0 preservado), e injeta `MeasuredTPS`/`MeasuredSamples` nos candidatos em `decide()` via snippet literal da techspec — `catalog` e `telemetry` permanecem desacoplados (injeção pull no agent). Os testes 10-11 da techspec existem e provam o que a task exige; um teste adicional cobre o requisito explícito "após **cada** ChatStream" (passos de tool também gravam). Tolerância a store nil é exercitada pela suíte inteira (117 testes pré-existentes rodam com `Telemetry` nil). 120/120 testes passando, build/vet/gofmt limpos, `-race` limpo, estável em 3 execuções. Nenhuma mudança fora de escopo (`main.go` intocado — wiring é da task 4.0).

## Conformidade com Rules

| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código (padrão do projeto) | OK | `grep -n "//"` em `agent.go`: nenhum comentário; blocos novos sem comentários |
| Sem dependências externas (stdlib only) | OK | Nenhum import novo fora de `kterminal/internal/telemetry` (interno, pacote folha da task 1.0) |
| Direção de dependência: agent → telemetry; catalog não importa telemetry | OK | `go list -deps ./internal/agent` inclui telemetry; `go list -deps ./internal/catalog` = apenas o próprio pacote — injeção permanece pull no agent |
| Formatação/lint | OK | `gofmt -l .` sem output; `go vet ./...` sem achados |
| Nomenclatura Go | OK | `Telemetry` exported (setado pelo main na task 4.0 e pelos testes); helpers de teste seguem o padrão existente (`mockGateway`, `capturingJev`, `slowGateway`) |
| Padrões de teste do repositório (`testing` puro, `t.TempDir()`, `t.Helper()`, sem framework) | OK | Mesmo estilo dos helpers pré-existentes; store em `t.TempDir()` — testes nunca tocam `~/.local/share/kterminal/telemetry.json` real |
| Tratamento de erro | OK | `Record` é best-effort por design (spec: telemetria nunca derruba a sessão); nenhum novo caminho de erro no agent |
| Logging | N/A | Techspec: sem logs novos |
| Rules DDD/TS/Vitest/Java/Angular | N/A | Brownfield Go; todas as rules de `.agents/rules/` são de outras stacks |

## Verificação de Segurança

| Item | Status | Observações |
|------|--------|-------------|
| Sem secrets/PII | OK | Agent grava apenas `(nome do modelo, tps)` no Store; nenhum conteúdo de conversa vaza para telemetria |
| Persistência de teste isolada | OK | Todos os stores de teste usam `t.TempDir()` — nenhum teste escreve no path real do usuário |
| Endpoints/CORS/SQL/auth/headers | N/A | Sem backend; o agent só chama `Record`/`Get` no pacote local |
| Inputs malformados | OK | `tps` só é computado com `StreamSeconds > 0 && CompletionTokens > 0`; o gate de outliers da task 1.0 permanece como última linha de defesa |

## Aderência à TechSpec

| Decisão Técnica (techspec/task) | Implementado | Evidência |
|-----------------|--------------|-------------|
| Campo `Telemetry *telemetry.Store` no `Agent` | SIM | `agent.go:66`; campo exported, sem mudança de assinatura de `New` (wiring é da task 4.0) |
| Após cada `ChatStream` com `Usage.CompletionTokens > 0` → `Record(decision.Model, tps)`; sem completion tokens não grava | SIM | `agent.go:236-238`; gate literal em `CompletionTokens > 0`; provado em `TestAgentRecordsTPSAfterCall` (2ª chamada com completion 0 → samples continuam 1) |
| Guard anti-NaN preservado (ressalva herdada da review 1.0 — responsabilidade desta task) | SIM | `tps` só é computado com `StreamSeconds > 0 && CompletionTokens > 0` (`agent.go:233-235`); com `CompletionTokens > 0` e `StreamSeconds == 0`, `tps = 0` é descartado pelo gate de outliers do Store — `NaN` permanece inalcançável (exigiria 0/0) |
| Posição do `Record`: antes do branch de tool calls → cada passo do loop grava | SIM | `agent.go:236-238` precede o branch `len(result.ToolCalls) > 0`; provado em `TestAgentRecordsTPSAfterEachStreamStep` (tool step + resposta final = 2 samples) |
| Injeção em `decide()` — snippet literal da techspec | SIM | `agent.go:353-359` idêntico ao bloco "Injeção no agent": cópia por valor de cada candidato, `if tp, n := a.Telemetry.Get(m.Name); tp > 0` atribui os dois campos juntos (recomendação da review 2.0 atendida) |
| Router **e** Fallback recebem os candidatos injetados | SIM | `agent.go:360` e `agent.go:365` — ambos os `Route` usam a slice injetada |
| `Record` chamado só na goroutine do loop do agent | SIM | Única chamada está em `loop()`; mutex do Store cobre acessos concorrentes do doctor/`Close` (task 4.0) |
| Tolerância a store nil (agent sem telemetria funciona como hoje) | SIM | `Record`/`Get` do Store são nil-safe (task 1.0, `TestNilStoreIsSafe`); os 117 testes pré-existentes rodam com `Telemetry` nil — cada `Run` exercita `Record` nil e cada `decide` exercita `Get` nil |
| `catalog` e `telemetry` desacoplados (injeção é pull no agent) | SIM | `go list -deps ./internal/catalog` não inclui telemetry; nenhum arquivo dos dois pacotes tocado nesta task |
| Chamada de resumo da compação não grava telemetria | SIM | `compact()` não computa `tps` nem chama `Record` — coleta é "fim de `ChatStream` no loop" com tps computado, conforme o fluxo de dados da techspec |
| `Criteria()` cita o medido com ≥ 5 amostras; senão estimate (nunca piora) | SIM (consumo) | `TestCandidatesCarryMeasuredTPS`: glm-5.3 com 5 amostras → critérios contêm `~50 tok/s` + `measured over 5 calls`; glm-5.2 sem amostras → estimate sem "measured" |
| Wiring no `main.go` / `--doctor` | NÃO (correto) | Fora do escopo da 3.0 — task 4.0; `main.go` e `go.mod` intocados por esta task |

### Decisões de interpretação (documentadas)

1. **Gate do `Record` em `CompletionTokens > 0`** (literal da task) em vez de `tps > 0`: quando `CompletionTokens > 0` mas `StreamSeconds == 0`, `Record(model, 0)` é chamado e descartado pelo gate de outliers do Store — comportamento idêntico, regra literal da spec mantida.
2. **Campo em vez de parâmetro em `New()`**: a task pede o campo; o wiring ("injeta no agent") é da task 4.0. Testes e o futuro `main.go` fazem `ag.Telemetry = store` — assinatura de `New` intacta, zero churn nos callers.
3. **Caminho pin retorna antes da injeção**: com modelo pinado não há chamada ao router nem critérios — injeção seria inócua; early-return preservado.
4. **Teste adicional `TestAgentRecordsTPSAfterEachStreamStep`**: o requisito "após **cada** `ChatStream`" inclui passos de tool; sem este teste, um `Record` mal posicionado dentro do branch de resposta final passaria nos testes 10-11. Segue o padrão das reviews 1.0/2.0 (testes extras para requisitos explícitos fora da lista mandatória).
5. **`slowGateway` com `time.Sleep`**: `StreamSeconds` é tempo real medido dentro de `ChatStream` (sem ponto de injeção); num stream sub-milissegundo o `tps` de 5 tokens poderia exceder `outlierMaxTPS` (10000) e ser descartado, flakando o teste. O delay de 25ms antes do `[DONE]` garante `tps ≈ 200`, longe do gate. Distingue-se do princípio "zero sleep" da review 1.0: lá o sleep seria para testar lógica de tempo (debounce — resolvido com relógio injetável); aqui controla a duração física do stream, única alavanca disponível.

## Tasks Verificadas

| Task | Status | Observações |
|------|--------|-------------|
| 3.1 Campo `Telemetry` no `Agent` com guarda para nil | COMPLETA | `agent.go:66`; guarda = métodos nil-safe do Store, exercitados por toda a suíte existente |
| 3.2 Gravar `Record(decision.Model, tps)` após cada chamada com completion tokens | COMPLETA | `agent.go:236-238`; provado nos dois sentidos (grava / não grava) |
| 3.3 Injetar valores medidos nos candidatos em `decide()` | COMPLETA | `agent.go:353-365`; snippet literal; Router e Fallback |
| 3.4 Testes 10-11 da techspec | COMPLETA | 2 mandatórios + 1 adicional para "após cada ChatStream" |

## Testes

- **Total de Testes**: 120 top-level (117 pré-existentes + 3 novos; 122 execuções contando 2 subtests pré-existentes)
- **Passando**: 120
- **Falhando**: 0
- **Coverage**: `internal/agent` 85.4%; todos os blocos novos executados (profile: `agent.go:236-238` e `353-359` com count 1, incluindo o branch TRUE da injeção); `internal/telemetry` 92.5% e `internal/catalog` 57.6% inalterados

### Testes mandatórios (techspec, itens 10-11) — ambos PASS

| Teste | O que prova |
|-------|-------------|
| `TestAgentRecordsTPSAfterCall` | Gateway com usage conhecido (prompt 10, completion 5) → `GetMean("glm-5.3")` = amostra exata (`mean == done.TPS` bit-a-bit: mesmo float flui para `Record` e para o evento `turn_done`; média Welford de 1 amostra é exata) com samples 1; isolamento por modelo (`glm-5.2` → 0/0); 2ª chamada com completion 0 → samples continuam 1 e média inalterada |
| `TestCandidatesCarryMeasuredTPS` | Store com 5 amostras em glm-5.3 → `decide` passa ao mock Jev (que captura os critérios da requisição) critérios contendo `~50 tok/s` e `measured over 5 calls`; glm-5.2 sem amostras → texto estimate sem "measured" (fallback "nunca piora" ponta a ponta) |

### Teste adicional (requisito explícito da task)

| Teste | Requisito que cobre |
|-------|---------------------|
| `TestAgentRecordsTPSAfterEachStreamStep` | "Após **cada** `ChatStream`" — turno com tool call (completion 5) + resposta final (completion 1) → 2 samples; prova que o `Record` está fora do branch de resposta final |

### Verificação obrigatória executada

```
go build ./...                              OK
go vet ./...                                OK (sem achados)
gofmt -l .                                  OK (sem output)
go test ./... -count=1                      ok nos 8 pacotes com testes — 120 PASS, 0 FAIL
go test -race -count=1 ./internal/agent/ ./internal/telemetry/   OK (sem data races)
go test ./internal/agent/ -count=3          OK (estável — sem flakes de timing)
```

Os 117 testes pré-existentes continuam passando (nada quebrou).

## Problemas Encontrados

| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | `agent.go` | 236-238 | Com `CompletionTokens > 0` e `StreamSeconds == 0`, `Record(model, 0)` é chamado e descartado pelo gate de outliers — chamada redundante inofensiva | Alternativa: gate composto `if tps > 0`. Comportamento idêntico; regra literal da task (`CompletionTokens > 0`) mantida. Sem ação |
| Baixa | `agent.go` | 353-359 | Injeção refaz `Get` por candidato a cada `decide` (cada step do loop) | Custo desprezível (map lookup sob mutex, ≤ 7 candidatos por chamada); cache fora de escopo e adicionaria invalidação. Aceito como está |
| Baixa | `agent_test.go` | 93-119 | `slowGateway` introduz `time.Sleep(25ms)` — primeiro sleep real na suíte de agent | Necessário para determinismo do `StreamSeconds` (ver interpretação 5); custo total ~100ms em 3 chamadas. Aceito como está |

## Pontos Positivos

- Ciclo coleta→consumo fechado com o snippet literal da techspec — zero desvio nas fórmulas de integração.
- Guard anti-NaN (ressalva da review 1.0) endereçado exatamente como recomendado: o gate de tokens permanece a única porta de entrada do `Record`.
- Assert de igualdade bit-a-bit `GetMean == done.TPS`: prova que a amostra gravada é a mesma exibida no turno, sem depender do valor absoluto do `tps` (não-determinístico por natureza).
- `capturingJev` prova o contrato ponta a ponta: os critérios que chegam ao Jev de fato contêm o medido — não apenas que o store tem os dados.
- Controle negativo forte: glm-5.2 sem amostras recebe estimate sem "measured" na mesma requisição — o fallback "nunca piora" verificado dentro de um único decide.
- Determinismo de timing resolvido na infraestrutura de teste (delay no gateway), não com margens frágeis ou asserts condicionais; estável em 3 execuções e `-race` limpo.
- Escopo cirúrgico: 12 linhas líquidas em produção, nenhum pacote fora de `internal/agent` tocado, nada da task 4.0 (wiring/doctor) antecipado.

## Recomendações

1. **Task 4.0**: usar `telemetry.Load(path)` no wiring, `ag.Telemetry = store` e `defer store.Close()` (recomendação herdada das reviews 1.0/2.0).
2. **Task 4.0**: o `--doctor` consome via `GetMean` — sem necessidade de expor o map do Store.
3. **Techspec 010 (subagentes)**: quando os subagentes compartilharem o `Store`, reutilizar o campo `Telemetry` do agent principal — o design pull desta task já suporta (Store mutex-protected).

## Conclusão

A Task 3.0 fecha o ciclo coleta→consumo conforme a Tech Spec: coleta pós-chamada com o guard de tokens preservado (mantendo `NaN` fora do alcance do gate de outliers), gravação best-effort na goroutine do loop, e injeção pull dos valores medidos nos candidatos do router com o snippet literal da spec — `Criteria()` cita o medido com ≥ 5 amostras e o estimate permanece como fallback, nunca piorando a informação do Jev. Dependências limpas (agent → telemetry; catalog intocado), tolerância a nil provada por toda a suíte existente, testes mandatórios 10-11 provando coleta e injeção ponta a ponta, mais um teste para o requisito "após cada ChatStream". 120/120 testes, build/vet/gofmt/`-race` limpos, sem flakes. **APROVADO** — base pronta para a task 4.0 (wiring no `main.go` e tabela no `--doctor`).
