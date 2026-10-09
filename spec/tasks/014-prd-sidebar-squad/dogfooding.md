# Dogfooding — Sidebar de agentes do squad (Task 6.0)

E2E da feature 014 conforme techspec ("Sem TestSprite (TUI local, precedente da feature 012). E2E = dogfooding documentado"). Este relatório registra a validação de dois tipos:

1. **Validação programática** (executada por esta task): os cenários de dogfooding que podem ser exercitados sem terminal interativo foram validados por testes de integração com mock gateway — incluindo a conferência sidebar × transcript JSONL com valores exatos.
2. **Protocolo manual** (pendente de execução pelo mantenedor): passos exatos e valores esperados para a mesa real rodando no próprio kterminal, em terminal real, com resize ao vivo e interrupção com Esc.

## 1. Validação programática (executada)

### 1.1 Resume `--continue` reconstrói o painel (REQ-006)

| Teste | Camada | O que valida | Valores conferidos |
| --- | --- | --- | --- |
| `TestSquadResumeLoadLatestRestoresMesa` | agent | Sessão squad com mesa encerrada → `LoadLatest` (o caminho exato do `--continue` em `main.go`) → `AppendWriter` → `RestoreMesa` → turno de follow-up | Mesa viva: architect `done`, 63 tokens, 1 convocation; snapshot recarregado deep equal; follow-up preserva entries/roles/tetos e zera contadores do novo turno (0 convocations / 11 tokens) |
| `TestResumeContinueRebuildsSquadPanel` | tui | Fluxo TUI completo com mock gateway + session writer real → `LoadLatest` → painel reconstruído na ordem do wiring do `main.go` (`ActivateMode` → `RestoreMesa` → `New` + `WithResumed`) | Painel: `architect · done`, modelo, `12 tok`, custo, `1/4 convocations`, `63/100.0k tokens`; chat com o histórico persistido; sem `mesa not started` |
| `TestResumeLoadLatestPreFeatureSnapshotWithoutMesa` | session | Snapshot pré-feature (JSONL sem campo `mesa`, `mode: squad`) no diretório real de sessões → `LoadLatest` | Carrega sem erro; `Mesa == nil`; modo e mensagens preservados |
| `TestResumePreFeatureSnapshotRendersMesaNotStarted` | tui | Mesmo snapshot pré-feature → resume completo → render | Painel exibe `mesa not started` com header `maestro`; **sem** rodapé de orçamento (nenhum `convocations`/`/`); chat legado intacto; largura do view correta |

### 1.2 Fluxo completo com mock gateway, sem vazamento entre modos (REQ-001/002/003/004)

| Teste | O que valida | Valores conferidos |
| --- | --- | --- |
| `TestSquadFullFlowModeTogglePreservesMesa` | Sequência exata da subtarefa 6.2: kickoff → convocações → convergência → `/mode sdd` (sidebar some) → `/mode squad` (mesa preservada) → novo kickoff reconstrói a Mesa | Convergência: `architect · done`, `backend · done`, `2/4 convocations`, `90/100.0k tokens`; em sdd: `maestro` ausente do view, viewport em largura total, mesa intacta no agent; de volta ao squad: painel preservado; rekickoff com `qa` sozinho: `qa · waiting`, `0/2 convocations`, `36/50.0k tokens`, **nenhuma persona antiga vaza**, tetos 2/50000, contadores zerados |
| `TestSquadEscMidConvocationPanelReflectsFinalState` | Esc durante convocation (gateway bloqueia o stream da persona) | Mid-convocation: `architect · deliberating`, `1/4 convocations`; pós-Esc: persona vira `done`, painel reflete o estado final (`1/4 convocations`, `30/100.0k tokens`), `busy=false` |
| `TestSquadFlowSidebarIntegration` (task 5, citado) | Fluxo ao vivo com gates congelando o agent nos pontos de asserção | Pós-kickoff: `waiting`, `0/4 convocations`, `15/100.0k tokens`; mid-convocation: `deliberating`, `1/4`, `30/100.0k`; pós contribuição: `12 tok` + custo da persona; convergência: `done`, `1/4`, `63/100.0k`; largura do view conferida em **todos** os frames |

### 1.3 Conferência sidebar × transcript JSONL (REQ-004)

O critério "valores de tokens/custo do sidebar batem com o transcript JSONL" é garantido por construção (mesma escrita espelhada nos pontos de mutação — decisão #2 da techspec) e **verificado programaticamente**:

- `TestResumeContinueRebuildsSquadPanel` gera uma sessão real com mock gateway, extrai a última linha `snapshot` do JSONL e confere o campo `mesa` contra o `Snapshot` carregado (`tokens`, `convocations`, `max_convocations`, `token_budget`, `entries[].name/status`) **e** contra o painel renderizado (modelo, `N tok`, custo `$X.XXXX`, `n/N convocations`, `k/B tokens` — todos derivados dos valores do JSONL).
- `TestSnapshotCarriesMesa` (task 3) confere a linha `snapshot` do JSONL contra a Mesa viva do agent campo a campo (counters, entries com model/tokens/cost).
- `TestSquadMesaFullCycle` (task 3) confere `mesa.Tokens == turnTokens` e `mesa.Convocations == turnConvocations` em cada checkpoint do fluxo — o rodapé mostra exatamente o que o enforcement de tetos mede, incluindo a defasagem mid-convocation documentada (tokens do subagente em voo só aterrissam no retorno da convocation).

### 1.4 Regressão completa (subtarefa 6.3)

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ ok |
| `go vet ./...` | ✅ ok |
| `gofmt -l .` | ✅ vazio |
| `go test ./... -race -count=1` | ✅ ok (14 pacotes) |
| `go test ./internal/tui/ -race -count=3` (testes de integração novos) | ✅ ok (checagem de flakiness) |

## 2. Protocolo manual de verificação (mesa real no kterminal)

Protocolo para execução em terminal real pelo mantenedor. Cada cenário lista passos exatos e valores esperados. Recomenda-se sessão de teste isolada para não misturar com sessões reais: `XDG_DATA_HOME=$(mktemp -d) ./kterminal`.

### Preparação

1. `go build -o kterminal .`
2. `./kterminal --doctor` — gateway e jev configurados (a mesa real faz chamadas LLM)
3. Terminal com ≥ 100 colunas (janela full em monitor comum dá 200+)

### Cenário A — Mesa real + resize ao vivo (80/120/200 cols) [REQ-005]

1. `./kterminal` → `/mode` → seta para `squad` → Enter
   - **Esperado**: sidebar à direita com header `squad`, linha `maestro · waiting` e o texto `mesa not started`; hint bar com `mode squad`; nenhuma tecla deixa de funcionar
2. Redimensionar a janela para ~200 cols
   - **Esperado**: painel com ~40 cols de largura (teto do clamp); chat na coluna restante; nenhuma linha excede o terminal
3. Redimensionar para ~120 cols
   - **Esperado**: painel com 29 cols; conteúdo truncado com `…`, sem quebrar o layout
4. Redimensionar para ~80 cols
   - **Esperado**: painel some; chat ocupa a largura total (idêntico ao modo sdd)
5. Voltar para ~200 cols
   - **Esperado**: painel reaparece com o mesmo estado; chat e painel íntegros
6. Enviar um pedido de deliberação (ex.: `design a review checklist for this repo`) e aguardar o kickoff (linha `● squad_kickoff(...)` no chat)
   - **Esperado**: painel popula as personas na ordem do kickoff, todas `waiting`; rodapé `0/N convocations` e `0/B tokens` (B = orçamento do kickoff, ex.: `100.0k`)
7. Durante as convocações, redimensionar 200 → 120 → 80 → 200
   - **Esperado**: status (`deliberating`) e contadores preservados em todas as larguras; a 80 cols o painel some e volta ao voltar; sem corrupção de chat ou painel em nenhum passo

### Cenário B — Interrupção com Esc mid-convocation [REQ-002/003]

1. Com a mesa ativa, aguardar uma convocation iniciar
   - **Esperado**: persona `deliberating` no painel; atividade corrente `tool(args)` quando a persona executa tools (ou `—` se contribui em texto puro); rodapé incrementa convocações
2. Pressionar `Esc` durante a deliberação
   - **Esperado**: turno abortado (bloco `⊘ interrompido · <modelo>` no chat); a persona que deliberava vira `done` no painel; rodapé mantém convocações/tokens consumidos até o abort
3. Conferir o JSONL (ver Cenário D): última linha `snapshot` com a entry da persona `status: "done"` e os contadores do turno abortado

### Cenário C — Resume com `--continue` [REQ-006]

1. Após a convergência (todas as personas `done`), sair (`/exit`)
2. `./kterminal --continue`
   - **Esperado**: modo squad restaurado; hint bar `mode squad` + `resumed · N mensagens`; painel reconstrói as personas `done` com modelo, `N tok` e custo por persona, e o rodapé com os totais persistidos — valores idênticos aos da sessão anterior
3. Enviar uma nova pergunta
   - **Esperado**: rodapé zera convocações/tokens para o novo turno (`0/N convocations`); personas do kickoff anterior permanecem no painel até que um novo kickoff reconstrua a mesa
4. (Opcional) Resume de sessão pré-feature (sem campo `mesa` no snapshot): `./kterminal --session <caminho do JSONL antigo>` com `mode: squad`
   - **Esperado**: carrega sem erro; painel exibe `mesa not started` sem rodapé de orçamento

### Cenário D — Conferência sidebar × transcript JSONL [REQ-004]

1. Localizar a sessão: `ls -t ~/.local/share/kterminal/sessions/*.jsonl | head -1` (ou `$XDG_DATA_HOME/kterminal/sessions/` se definido)
2. Extrair a última linha de snapshot: `grep '"type":"snapshot"' <arquivo> | tail -1` e inspecionar o campo `mesa`
3. Conferir campo a campo contra o painel exibido:

| Campo no JSONL | Elemento do painel | Formato exibido |
| --- | --- | --- |
| `entries[].name` / `entries[].status` | `▸ <nome> · <status>` | textual (cor nunca é o único indicador) |
| `entries[].model` | linha de métricas da persona | `<modelo> · …` |
| `entries[].tokens` | métricas da persona | `<N> tok` (≥1000 → `N.Nk`) |
| `entries[].cost` | métricas da persona | `$0.XXXX` (4 decimais) |
| `convocations` / `max_convocations` | rodapé | `<n>/<N> convocations` |
| `tokens` / `token_budget` | rodapé | `<k>/<B> tokens` (ex.: `63/100.0k`) |

   - **Esperado**: valores idênticos — o painel e o JSONL saem da mesma escrita (espelho nos pontos de mutação)
   - **Nota**: mid-convocation o rodapé pode defasar em relação ao que a persona já consumiu (tokens do subagente em voo aterrissam no retorno da convocation) — comportamento documentado na techspec; o rodapé mostra exatamente o que o enforcement de tetos mede

### Cenário E — Toggle de modos com mesa ativa + resize vertical [REQ-001]

1. Com a mesa ativa (personas `done`/`deliberating`), `/mode` → `sdd`
   - **Esperado**: sidebar some na mesma sessão, sem restart; chat volta à largura total; hint bar `mode sdd`
2. `/mode` → `squad`
   - **Esperado**: sidebar reaparece com a mesa preservada — mesmas personas, status e métricas
3. Redimensionar a altura para bem poucas linhas (ex.: 10–15) com mesa cheia (3+ personas)
   - **Esperado**: largura permanece íntegra, sem overflow horizontal; o painel pode ficar mais alto que o chat (limitação conhecida sem clamp de altura, registrada na review 5.0 como fast-follow — sem requisito de altura no PRD)

## 3. Valores de referência

Largura do sidebar (`sidebarWidth()`): piso 24, teto 40, proporcional entre 100 e 200+ cols; abaixo de 100 cols o painel é omitido.

| Largura do terminal | Largura do sidebar |
| --- | --- |
| < 100 | 0 (oculto; chat full-width) |
| 100 | 24 |
| 120 | 29 |
| 140 | 35 |
| 200 | 40 (bruto 51, clampado no teto) |

Textos canônicos do painel (inglês, conforme regra do código-base): header `squad`; maestro `▸ maestro · waiting|deliberating` + modelo corrente; estado ocioso `mesa not started`; entrada `▸ <nome> · <waiting|deliberating|done>` / `<modelo> · <N> tok · $<custo>` / atividade `tool(args)` ou `—`; rodapé `<n>/<N> convocations` e `<k>/<B> tokens`.

## 4. Status

| Cenário | Validação programática | Protocolo manual |
| --- | --- | --- |
| Resume `--continue` (mesa encerrada) | ✅ `TestSquadResumeLoadLatestRestoresMesa`, `TestResumeContinueRebuildsSquadPanel` | ☐ pendente — Cenário C |
| Snapshot pré-feature → `mesa not started` | ✅ `TestResumeLoadLatestPreFeatureSnapshotWithoutMesa`, `TestResumePreFeatureSnapshotRendersMesaNotStarted` | ☐ pendente — Cenário C.4 |
| Fluxo completo + toggle de modos + rekickoff sem vazamento | ✅ `TestSquadFullFlowModeTogglePreservesMesa` | ☐ pendente — Cenário E |
| Esc mid-convocation → estado final | ✅ `TestSquadEscMidConvocationPanelReflectsFinalState` (+ `TestSquadMesaAbortMarksDeliberatingDone`, task 3) | ☐ pendente — Cenário B |
| Sidebar × transcript JSONL | ✅ `TestResumeContinueRebuildsSquadPanel` (+ `TestSnapshotCarriesMesa`, `TestSquadMesaFullCycle`, tasks 3) | ☐ pendente — Cenário D |
| Resize ao vivo 80/120/200 | ✅ `TestSidebarResizeKeepsPanelsIntact`, `TestSquadFlowSidebarIntegration` (largura em todos os frames — tasks 4/5) | ☐ pendente — Cenário A |
| Mesa real com LLM de verdade (Jev roteando) | — (exige terminal interativo e gateway real) | ☐ pendente — Cenários A–E |

A parte programática está completa e verde (seção 1). O protocolo manual fica registrado aqui para execução pelo mantenedor — os valores esperados derivam do código implementado e dos valores exatos assertados nos testes de integração. Todos os cenários manuais estão **pendentes** (☐ na tabela acima); ao executar cada cenário em terminal real, o mantenedor deve assinalar o checkbox correspondente (☐ → ☑).
