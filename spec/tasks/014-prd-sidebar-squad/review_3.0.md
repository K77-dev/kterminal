# Relatório de Code Review - Sidebar de agentes do squad (Task 3.0: Agent — `Event.TurnTokens`, ownership da Mesa, espelhos, transições, drain e snapshot)

## Resumo
- Data: 2026-10-09
- Branch: 014-prd-sidebar-squad
- Status: APROVADO
- Arquivos Modificados (escopo 3.0): 4 (`internal/agent/agent.go`, `internal/agent/agent_test.go`, `internal/squad/mesa.go`, `internal/squad/mesa_test.go`)
- Linhas Adicionadas: ~877 (agent.go +99, agent_test.go +715, `Clone()` em mesa.go +15, teste do Clone em mesa_test.go +48)
- Linhas Removidas: 5 (agent.go)
- Revisão independente do auto-review `review_3.md` — conclusões validadas por análise própria do código e execução dos checks

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Zero comentários adicionados em agent.go/mesa.go/testes (verificado por grep no diff) |
| Código em inglês | OK | Identificadores, mensagens e nomes de teste em inglês |
| Eventos via canal existente | OK | Nenhum event-kind, canal ou loop novo; `TurnTokens` é campo aditivo em `Event`; drain reutiliza o loop de eventos existente de `runSubagent` |
| Espelhos colados às mutações existentes | OK | Todos os 5 pontos de mutação de `turnTokens`/`turnConvocations` têm espelho adjacente (tabela abaixo) |
| Receivers por valor em View/render | N/A | Sem mudança de TUI nesta task |
| Padrões de erro do código-base | OK | `fmt.Errorf` com `%w`/`%q` conforme o restante do pacote |
| Sem dependências novas | OK | `squad` já era importado por `agent` |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `Event.TurnTokens int64` aditivo, carimbado com `SessionCost` em `EventToolStart`/`EventTurnDone` pós-acumulação | SIM | agent.go:79, agent.go:827, agent.go:882; `runLoop` compartilhado entre depth 0 e subagentes → eventos aninhados carregam os cumulativos do próprio sub por construção; TUI ignora os campos em depth 0 (só lê `SessionCost` no `turn_done` — tui.go:457), zero regressão |
| Campo `mesa *squad.Mesa` + `Mesa()` (cópia sob lock) / `RestoreMesa()` | SIM | agent.go:125, agent.go:268-283; ponteiro protegido por `a.mu`, estado pelo mutex interno da Mesa; `RestoreMesa(nil)` no-op; `initMesa` init-if-nil na ativação squad preserva Mesa no toggle de modos |
| Espelhos nos mesmos pontos de mutação | SIM | Ver tabela de espelhos abaixo — igualdade por construção |
| Transições: `RegisterKickoff`→`Reset`, `convokePersona`→`StartDeliberation`, fim da contribuição→`FinishDeliberation`, `endTurn`→`FinishTurn` | SIM | agent.go:223-237, agent.go:631-643, agent.go:348-357; `FinishDeliberation` no retorno com sucesso **e** erro (garante `done` no abort) |
| Drain de `runSubagent` → `ObservePersona` | SIM | agent.go:459-463; apenas `tool_start`/`turn_done` de subagente persona (`agentName != ""`); task subagentes comuns não observam |
| `writeSnapshot` passa a Mesa | SIM | agent.go:328 — passa `a.Mesa()` (Clone), não o ponteiro vivo; fecha o contrato da review_2.0 |
| Defasagem mid-convocation preservada | SIM | Tokens do sub em voo só aterrissam no retorno de `runSubagent` (agent.go:480-483), como hoje; assertada positivamente no teste (57 vs 15 em voo no gate 5) |
| Nenhum novo loop/canal/event-kind | SIM | Confirmado no diff |

**Tabela de espelhos** (todos os pontos de mutação de `turnTokens`/`turnConvocations` em agent.go):

| Mutação existente | Espelho |
| --- | --- |
| `runLoop` depth 0: zera ambos (agent.go:731-732) | `ResetTurn()` (agent.go:733-735) |
| `runLoop`: `turnTokens += used` (agent.go:803) | `AddTokens(used)` (agent.go:804-806) — subagentes têm `mesa == nil`, no-op natural |
| `runSubagent`: `turnTokens += sub.turnTokens` (agent.go:480) | `AddTokens(sub.turnTokens)` (agent.go:481-483) |
| `convokePersona`: `turnConvocations++` (agent.go:631) | `AddConvocation()` + `StartDeliberation(name)` (agent.go:632-636) |
| `Agent.Reset()` (`/clear`): zera ambos (agent.go:165-166) | `ResetTurn()` (agent.go:167-169) |

## Veredito sobre os 4 desvios declarados

1. **`Clone()` em `mesa.go` — CORRETO E JUSTIFICADO.** A techspec exige "cópia rasa sob lock" de `Mesa()`, mas o mutex é interno ao pacote `squad`: cópia por valor de fora dispararia `go vet` copylocks e leitura direta dos campos seria racy. `Clone()` sob o mutex interno é a única implementação sã; a adição foi sancionada pela review_1.0. O ajuste de assinatura para retorno `*Mesa` (em vez do retorno por valor sugerido) é correto — retorno por valor reprovaria copylocks. `Roles`/`Entries` ganham backing arrays próprios; `MesaEntry` é struct de valores (strings/números), sem aliasing. `baselines` não copiado é decisão documentada e segura (unexported, transitório, invisível ao JSON; restore mid-deliberation é impossível no fluxo projetado — restore só ocorre no resume). Teste `TestMesaCloneIsolatesState` cobre nil-receiver, cópia campo a campo, não-aliasing e Mesa zerada.

2. **Restore de contadores pós-`Reset` no `RegisterKickoff` — CORRETO E NECESSÁRIO.** `Reset` zera `mesa.Tokens`/`Convocations`, mas `turnTokens`/`turnConvocations` sobrevivem ao kickoff — os tokens da própria chamada de kickoff contam contra o orçamento (o enforcement em `convokePersona` mede `a.turnTokens`, agent.go:624). Sem o restore, `mesa.Tokens == turnTokens − tokensDoKickoff` ao fim do turno, quebrando a igualdade por construção e o princípio "o rodapé mostra o que o enforcement mede". Cobre também re-kickoff mid-turn. A leitura de `turnTokens` fora de `a.mu` é same-goroutine (tool execution roda no goroutine do turno, único escritor) — ver Observação 1 abaixo.

3. **`AddConvocation` em `convokePersona`, não no retorno de `runSubagent` — CORRETO; o texto da subtask 3.3 era impreciso.** `runSubagent` nunca muta `turnConvocations` — a única mutação é `a.turnConvocations++` em `convokePersona`. Espelhar no retorno de `runSubagent` violaria "mesmos pontos de mutação" **e** quebraria a igualdade no outro sentido: `runSubagent` também é chamado por `RunSync` para task subagentes comuns (sem persona), que nunca incrementam `turnConvocations` — o espelho ali contaria convocações fantasmas. A própria techspec diz "convokePersona marca deliberando e conta". `AddTokens` está corretamente no retorno de `runSubagent`, espelhando `a.turnTokens += sub.turnTokens`. Única leitura consistente com a spec.

4. **Espelho `ResetTurn` em `Agent.Reset()` — CORRETO.** `Reset()` zera ambos os campos (ponto de mutação pré-existente, acionado pelo `/clear`). Sem o espelho, o rodapé divergiria do enforcement até o próximo turno. Uma linha, aderente ao requisito "mesmos pontos de mutação". Seguro: `/clear` é bloqueado quando `m.busy` (tui.go:1004), não corre concorrente com turno ativo.

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 3.1 `Event.TurnTokens` + carimbo pós-acumulação | COMPLETA | Inclusive em depth de subagente (runLoop compartilhado); `Tokens` do `turn_done` preserva semântica via `used` (valor idêntico à expressão anterior — zero regressão) |
| 3.2 campo `mesa` + getters + init na ativação | COMPLETA | `initMesa` init-if-nil preserva Mesa restaurada/prévia no toggle |
| 3.3 espelhos de contadores | COMPLETA | 5/5 pontos de mutação espelhados (inclui `Reset()` — desvio #4) |
| 3.4 transições de status | COMPLETA | Kickoff→Reset (com disciplinas resolvidas via store, I/O fora do lock), convoke→Start/FinishDeliberation, endTurn→FinishTurn (depth 0) |
| 3.5 drain `ObservePersona` | COMPLETA | Só personas; cumulativos do sub; baseline da task 1.0 |
| 3.6 `writeSnapshot` passa a Mesa | COMPLETA | Call site da task 2.0 deixa de passar `nil` |
| 3.7 testes | COMPLETA | 7 testes novos no pacote agent + 1 no squad; cenários mandatórios 9/9 (tabela abaixo) |

## Testes
- Total de Testes: 111 (internal/agent, 7 novos) + 36 (internal/squad, 1 novo)
- Passando: todos
- Falhando: 0
- Coverage: suíte completa `go test ./...` verde (12 pacotes com testes, inclui `internal/tui` — zero regressão de render)

**Mapeamento contra a lista mandatória da task:**

| Cenário obrigatório | Teste |
| --- | --- |
| Ciclo completo com checkpoints por etapa | `TestSquadMesaFullCycle` (gateway gated — checkpoints determinísticos; statuses conferidos após kickoff, cada convocation, contribuição e convergência) |
| Eventos aninhados com cumulativos > 0 | `TestNestedEventsCarryCumulativeMetrics` (também confere carimbo em depth 0) |
| Igualdade `Mesa` × `turnTokens`/`turnConvocations` pós-turno | Assertada em 4 testes (full cycle 105/2, baseline 90/2, concorrência 90/2, abort — inclusive em turno abortado) |
| Abort mid-convocation → persona `done` | `TestSquadMesaAbortMarksDeliberatingDone` (determinístico via gateway bloqueante; confere também snapshot com entry `done`) |
| `Mesa()`/`RestoreMesa` round-trip (cópia, não ponteiro) | `TestMesaRestoreRoundTrip` (isolamento nos dois sentidos, `nil` no-op, toggle preserva) |
| Concorrência drain × runLoop × leitor sob `-race` | `TestSquadMesaConcurrentReadWrite` (leitor hammando `Mesa()` durante fluxo completo) |
| Integração `writeSnapshot`→JSONL→`Load` | `TestSnapshotCarriesMesa` (linha `snapshot` com `mesa` no JSONL, `Load` devolve deep equal campo a campo) |
| Acumulação sobre baseline (2ª convocation) | `TestSecondConvocationAccumulatesOverBaseline` (24 tokens, custo dobrado) |
| Defasagem mid-convocation | Assertada positivamente no gate 5 do full cycle (mesa 57 == turnTokens 57 enquanto o sub tem 15 em voo) |

Testes são significativos: valores exatos (15/30/42/57/84/105), custos calculados do catálogo, não apenas "executou sem erro". O gateway gated elimina flakiness das asserções mid-flow sem tocar em relógios.

## Verificação de Segurança
N/A — funcionalidade integralmente local à TUI/agent, sem backend/API. Justificativa: nenhum endpoint, nenhum input de cliente, sem secrets (a Mesa carrega apenas nomes de modelo/tokens/custos que já constam no JSONL), sem HTML/SQL/renderização. Snapshot aditivo retrocompatível (`omitempty`; snapshots antigos carregam com `Mesa == nil`).

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/agent/agent.go | 231 | `RegisterKickoff` lê `turnTokens`/`turnConvocations` fora de `a.mu`. Hoje é same-goroutine (tool execution no goroutine do turno, único escritor) e segue o padrão pré-existente de acesso sem lock desses campos (idem `convokePersona` agent.go:621-624) — sem race real. Risco latente apenas se `RegisterKickoff` passar a ser chamado de outro goroutine. | Manter como está; se esses campos ganharem lock no futuro, incluir esta leitura |
| Baixa | internal/agent/agent.go | 232-236 | Janela transitória entre `m.Reset` e o restore de contadores: um leitor concorrente (TUI) pode ver a Mesa zerada por um instante. Mesma classe de qualquer update multi-passo; invariantes de fim de turno não são afetados. | Aceitável; nenhuma ação |

## Pontos Positivos
- Igualdade por construção travada por teste em 4 cenários distintos — a mitigação "divergência futura" da techspec fica blindada
- Espelhos visivelmente adjacentes às mutações; discipline de lock uniforme (ponteiro sob `a.mu`, estado sob mutex da Mesa, I/O de `Resolve` fora do lock)
- Defasagem mid-convocation assertada positivamente, não apenas preservada por omissão
- `FinishDeliberation` no caminho de erro torna o `done` do abort determinístico antes do `FinishTurn`
- Gateway gated (`callGate`) transforma asserções mid-flow em checkpoints determinísticos — padrão reutilizável para tasks 4.0-6.0
- Zero comentários, zero novos event-kinds/canais/loops, `go vet` limpo (Clone retorna ponteiro, sem copylocks)

## Recomendações
- Observação 1 (leitura fora de lock em `RegisterKickoff`): documentar a invariant "RegisterKickoff roda apenas no goroutine do turno" se o campo ganhar outros call sites futuros
- Cenário exótico não bloqueante: re-kickoff durante convocation em andamento zeraria `baselines` enquanto o drain ainda observa a persona antiga (métricas em voo aterrissariam com baseline zerado). Fora do fluxo projetado (kickoff é sequencial às convocations); rever apenas se o squad ganhar paralelismo (fast-follow da feature 12)
- Task 5.0: o wiring `ag.RestoreMesa(resumed.Mesa)` já está coberto pelo comportamento testado em `TestMesaRestoreRoundTrip`

## Conclusão
A implementação atende integralmente os requisitos da task 3.0 e às decisões da techspec: `Event.TurnTokens` carimbado pós-acumulação em depth 0 e aninhado, ownership da Mesa no Agent com getters de cópia sob lock, espelhos em 5/5 pontos de mutação (igualdade por construção), transições de status nos 4 pontos do loop, drain de personas no `runSubagent` e Mesa no snapshot. Os 4 desvios declarados são todos corretos e justificados — dois são correções de imprecisões do texto da task contra o princípio superior da spec ("mesmos pontos de mutação"/igualdade), um foi sancionado pela review_1.0 e um é extensão natural do princípio dos espelhos. Todos os checks passam (`build`, `vet`, `gofmt`, `-race` no agent e squad, suíte completa), a cobertura de testes excede a lista mandatória com asserções de valores exatos e determinismo por gates. Os dois apontamentos de severidade baixa são informativos e não exigem alteração. **APROVADO.**
