# Review — Task 3.0: Agent — `Event.TurnTokens`, ownership da Mesa, espelhos de contadores, transições de status, drain de `runSubagent` e passagem ao snapshot

**Veredito: APROVADO** (com 4 divergências documentadas e justificadas — ver seção "Notas e divergências")

## Escopo implementado

- `internal/agent/agent.go` (modificado, +104/−5): `Event.TurnTokens int64` (aditivo); campo `mesa *squad.Mesa` + getters `Mesa()`/`RestoreMesa()` + helpers `currentMesa()`/`initMesa()`; espelhos `ResetTurn`/`AddTokens`/`AddConvocation` colados às mutações existentes de `turnTokens`/`turnConvocations`; transições `RegisterKickoff`→`Reset` (com disciplinas resolvidas), `convokePersona`→`StartDeliberation`/`FinishDeliberation`, `endTurn`→`FinishTurn`; drain de `runSubagent` chamando `ObservePersona` com os cumulativos dos eventos aninhados enriquecidos; `writeSnapshot` passa `a.Mesa()` (cópia sob lock — fecha o contrato anotado na review_2.0).
- `internal/agent/agent_test.go` (modificado, +715): 7 testes novos + helpers (`gatedTaskFlowGateway` com `callGate` para checkpoints determinísticos, `newSquadAgent`, `waitForEventMatch`, `kickoffArgs`, `personaTaskArgs`, `glmCost`, `mesaEntryByName`).
- `internal/squad/mesa.go` (dependência task 1.0 — **adição**: `Clone() *Mesa`, cópia sob lock; ver divergência #1).
- `internal/squad/mesa_test.go` (**adição**: `TestMesaCloneIsolatesState`).
- Nada além do escopo: nenhum render de sidebar, nenhum wiring de resume no `main.go`, nenhum campo novo em `session.Event` — tasks 4.0–6.0 intocadas.

## Evidências do diff

### Frente 1 — `Event.TurnTokens` (REQ-003)

- `TurnTokens int64` adicionado a `Event` ao lado de `Tokens` — campo aditivo, literais por nome em todo o código, zero quebra.
- Carimbado em `EventToolStart` (`SessionCost: a.sessionCost, TurnTokens: a.turnTokens`) e `EventTurnDone` (`TurnTokens: a.turnTokens` junto ao `SessionCost` existente), **após** a acumulação de usage (`used := …; a.turnTokens += used`), no mesmo `runLoop` compartilhado por depth 0 e subagentes — eventos aninhados carregam os cumulativos do próprio subagente por construção.
- `Tokens` do `turn_done` preserva a semântica existente (tokens da última chamada) via `used`; `session.Event` não ganha campo novo (fora de escopo).
- Depth 0: TUI ignora os campos novos hoje (consumo por nome) — zero regressão, suíte `internal/tui` verde.

### Frente 2 — ownership da Mesa (REQ-002, REQ-004, REQ-006)

- Campo `mesa *squad.Mesa`; ponteiro protegido por `a.mu` (escrita em `initMesa`/`RestoreMesa`/`RegisterKickoff`; leitura via `currentMesa()`); estado do pontoeiro nunca escapa sem lock.
- `Mesa()` devolve `Clone()` (cópia rasa sob o mutex interno da Mesa — `Roles`/`Entries` com backing arrays próprios, sem aliasing); `nil` quando não inicializada ("mesa não iniciada").
- `RestoreMesa(m)` injeta `m.Clone()` — o Agent tem posse exclusiva; `nil` é no-op.
- `initMesa` na ativação do modo squad, init-if-nil: preserva Mesa restaurada/prévia no toggle de modos (fluxo "mesa preservada" da techspec).
- `RegisterKickoff` cria a Mesa se preciso e chama `Reset(k, kickoffDisciplines(k))` — disciplinas resolvidas via `squadStore.Resolve(role)` → `persona.Discipline` (I/O fora do lock; store nil → disciplinas vazias).
- `writeSnapshot` passa `a.Mesa()` — cópia, não o ponteiro vivo (exatamente a mitigação pedida na review_2.0, nota de severidade baixa).

### Espelhos — igualdade por construção (REQ-004)

Todos os pontos de mutação de `turnTokens`/`turnConvocations` têm espelho adjacente:

| Mutação existente | Espelho |
| --- | --- |
| `runLoop` depth 0: `turnConvocations/turnTokens = 0` | `ResetTurn()` |
| `runLoop`: `turnTokens += used` (por chamada) | `AddTokens(used)` — subagentes têm `mesa == nil`, no-op natural |
| `runSubagent`: `turnTokens += sub.turnTokens` | `AddTokens(sub.turnTokens)` — defasagem mid-convocation preservada (tokens do sub em voo só aterrissam aqui, como hoje) |
| `convokePersona`: `turnConvocations++` | `AddConvocation()` + `StartDeliberation(name)` |
| `Agent.Reset()` (`/clear`): zera ambos | `ResetTurn()` (ver divergência #4) |

### Transições de status (REQ-002)

- `RegisterKickoff` → `Reset` (roles + disciplinas resolvidas; contadores do turno restaurados — ver divergência #2).
- `convokePersona` → `StartDeliberation` (baseline capturado antes do drain começar a observar) e `FinishDeliberation` no retorno de `RunSyncPersona` (sucesso **e** erro — contribuição abortada também encerra).
- `endTurn` (depth 0) → `FinishTurn` — persona deliberando em turno abortado (Esc) vira `done`.
- Drain de `runSubagent`: `ObservePersona(agentName, ev.Model, ev.TurnTokens, ev.SessionCost)` em `tool_start`/`turn_done` aninhados de subagente persona (`agentName != ""`; task subagentes comuns não observam) — métricas ao vivo durante a deliberação + acumulação entre convocations via baseline da task 1.0.

## Cobertura de testes (7 novos no pacote agent — 111 no total; 1 novo no squad — 36 no total)

Mapeamento contra a lista mandatória da task:

- **Ciclo completo conferindo a Mesa após cada etapa** → `TestSquadMesaFullCycle`: gateway com gates por chamada HTTP produz checkpoints determinísticos (o handler só responde após o teste liberar), eliminando flakiness das asserções mid-flow; statuses `waiting`→`deliberating`→`done` conferidos após kickoff, cada convocation, cada contribuição e convergência; roles/disciplinas/tetos conferidos; eventos aninhados da persona (tool_start vivo com 15 tokens/custo e turn_done com 27/custo somado) conferidos com valores exatos; **defasagem mid-convocation assertada** (mesa.Tokens == turnTokens == 57 enquanto o sub tem 15 em voo); igualdade final 105/2; preservação no toggle sdd→squad; espelho do `Reset()` (0/0, entries preservadas).
- **Eventos aninhados com cumulativos > 0** → `TestNestedEventsCarryCumulativeMetrics`: task subagente com tool — `tool_start` aninhado TurnTokens 15/SessionCost > 0, `turn_done` aninhado 27/custo somado; carimbo em depth 0 também conferido (parent tool_start 15, parent turn_done 63).
- **Igualdade pós-turno** → assertada em 4 testes: full cycle (105/2), baseline (90/2), concorrência (90/2), abort (30 — igualdade vale inclusive em turno abortado).
- **Abort mid-convocation (Esc)** → `TestSquadMesaAbortMarksDeliberatingDone`: persona `deliberating` conferida com o sub bloqueado (determinístico), `ag.Cancel()` → `done`; snapshot do abort carrega a mesa com a entry `done` e contadores 1/30.
- **`Mesa()`/`RestoreMesa` round-trip** → `TestMesaRestoreRoundTrip`: cópia isolada nos dois sentidos (mutar o original não vaza para o Agent; mutar a cópia devolvida não vaza para o Agent; `Mesa()` nunca devolve ponteiro compartilhado); `RestoreMesa(nil)` preserva; init vazia na ativação squad; preservação no toggle de modos.
- **Concorrência drain × runLoop × leitor** → `TestSquadMesaConcurrentReadWrite`: leitor hammando `Mesa()` (lock do ponteiro + Clone sob mutex interno) durante o fluxo completo com kickoff + 2 convocations, sob `-race`; igualdade conferida ao fim.
- **Integração Agent↔Session** → `TestSnapshotCarriesMesa`: linha `snapshot` do JSONL com `mesa` (convocations 1, tokens 63, entry architect done com model/tokens/custo), `session.Load` devolve `Snapshot.Mesa` deep equal campo a campo com `ag.Mesa()` + modo `squad`.
- **Segunda convocation sobre o baseline** → `TestSecondConvocationAccumulatesOverBaseline`: mesma persona duas vezes — entry 24 tokens (12 baseline + 12) e custo dobrado, convocations 2.
- **`Clone()` unit** → `TestMesaCloneIsolatesState` (squad): nil-receiver, cópia campo a campo, não-aliasing de entries/contadores, Mesa zerada.

## Checks executados

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ ok |
| `go vet ./...` | ✅ ok (sem copylocks — `Clone` retorna `*Mesa`) |
| `gofmt -l .` | ✅ vazio |
| `go test ./internal/agent/ -race -count=1` | ✅ ok (111 testes, 7 novos; também verde com `-count=3` no subconjunto novo) |
| `go test ./internal/squad/ -race -count=1` | ✅ ok (36 testes, 1 novo) |
| `go test ./... -count=1` | ✅ ok (12 pacotes com testes, sem regressão — inclui `internal/tui`) |

## Notas e divergências documentadas

1. **`Clone()` adicionado a `internal/squad/mesa.go`** (arquivo listado como "dependência — task 1.0"): sem um método de cópia sob lock no pacote dono do mutex, "cópia rasa sob lock" é inexequível — cópia por valor reprova `go vet` copylocks e é racy; leitura direta dos campos exportados de fora do pacote é racy. Adição **sancionada explicitamente** pela review_1.0.md (Problemas Encontrados: "Na task 3.0, estender a Mesa com `func (m *Mesa) Clone() Mesa`... documentar o desvio em relação à lista de interfaces"). Assinatura ajustada para `*Mesa` (o retorno por valor sugerido dispararia copylocks — o review_1.0 antecipou a alternativa "garantir sincronização no Agent", que seria mais invasiva e frágil).
2. **`RegisterKickoff` restaura os contadores do turno após o `Reset`**: `Reset` zera `Tokens`/`Convocations` da Mesa, mas `turnTokens`/`turnConvocations` (base do enforcement de tetos) sobrevivem ao kickoff — os tokens da própria chamada de kickoff contam contra o orçamento, exatamente como o `convokePersona` mede. Sem o restore, `mesa.Tokens == turnTokens − tokensDoKickoff` ao fim do turno, violando a igualdade por construção e o princípio "o rodapé mostra exatamente o que o enforcement mede". O restore cobre também re-kickoff mid-turn (loop de `AddConvocation`), previsto no fluxo "novo kickoff reconstrói a Mesa" da techspec.
3. **`AddConvocation` em `convokePersona`, não no retorno de `runSubagent`**: a subtask 3.3 menciona "AddTokens/AddConvocation no retorno de `runSubagent`", mas `runSubagent` nunca muta `turnConvocations` — a mutação existente é unicamente `a.turnConvocations++` em `convokePersona`. Os requisitos "mesmos pontos de mutação" e "igualdade por construção" (e a techspec: "convokePersona marca deliberando e conta") mandam espelhar no ponto da mutação; colocar `AddConvocation` também no retorno duplicaria a contagem (toda convocation passa por `convokePersona` → `RunSyncPersona` → `runSubagent`). `AddTokens` **está** no retorno de `runSubagent`, espelhando `a.turnTokens += sub.turnTokens`.
4. **Espelho `ResetTurn` em `Agent.Reset()`** (fora da lista enumerada de espelhos): `Reset()` zera `turnConvocations`/`turnTokens` — é um ponto de mutação dos mesmos campos, acionado pelo `/clear` da TUI entre turnos. Sem o espelho, o rodapé divergiria do enforcement até o próximo turno. Uma linha, aderente ao requisito "mesmos pontos de mutação".

Decisões de interpretação (sem divergência de spec):

- `initMesa` é init-if-nil: a techspec exige "mesa preservada" no toggle `/mode sdd` → `/mode squad` (teste de integração planejado), então a ativação não pode reconstruir uma Mesa existente.
- `RestoreMesa(nil)` é no-op (não limpa): "injeta Mesa restaurada do snapshot" — snapshot sem mesa significa ausência de estado a injetar; agente recém-resumido tem `mesa == nil` de qualquer forma.
- `Clone` não copia `baselines` (unexported, transitório por convocation, invisível ao JSON): `RestoreMesa` com uma Mesa mid-deliberation perderia o baseline — impossível no fluxo projetado (restore só ocorre no resume, sem deliberação em voo).
- `FinishDeliberation` roda também no retorno com erro da convocation: além de "fim da contribuição", é o que garante `done` no abort mid-convocation de forma determinística (antes do `abortTurn` do pai), complementando o `FinishTurn` do `endTurn`.

## Pontos positivos

- Igualdade por construção testada em 4 cenários distintos (turno completo, baseline, concorrência, abort) — a mitigação "divergência futura se alguém mutar `turnTokens` sem espelhar" da techspec fica travada por teste.
- O gateway gated (`callGate`) transforma asserções mid-flow tradicionalmente flaky em checkpoints determinísticos sem tocar em relógios — o handler só responde quando o teste libera, então o estado da Mesa é estável na inspeção.
- A defasagem mid-convocation existente é assertada positivamente (57 vs 72 consumidos reais no gate 5), não apenas preservada por omissão.
- Zero comentários no código; espelhos visivelmente colados às mutações; lock discipline uniforme (ponteiro sob `a.mu`, estado sob mutex da Mesa, ordem consistente `a.mu` → `mesa.mu`).

## Observação para tasks futuras

- Task 5.0: o wiring do resume (`ag.RestoreMesa(resumed.Mesa)` no `main.go`) usa o getter exatamente como testado em `TestMesaRestoreRoundTrip`; snapshots em modo `sdd` de sessões que passaram pelo squad carregam a Mesa (inofensivo — sidebar oculto), e o resume em `sdd` a injeta sem exibir.
- Task 6.0: o fluxo de integração "kickoff → convocações → convergência → `/mode sdd` → `/mode squad` (mesa preservada) → novo kickoff reconstrói a Mesa" já tem os dois primeiros terços cobertos por `TestSquadMesaFullCycle` (toggle preservado) e pelo restore de contadores do divergência #2 (re-kickoff).
