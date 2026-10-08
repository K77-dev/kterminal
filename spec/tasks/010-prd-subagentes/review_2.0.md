# Relatório de Code Review — 010-prd-subagentes — Task 2.0: `depth`, `newSubagent` e `RunSync` com re-emissão e limites

## Resumo
- Data: 2026-10-05
- Branch: 002-010-prds-kterminal
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 5 (`internal/agent/agent.go`, `internal/agent/agent_test.go`, `internal/session/session.go`, `internal/tools/tools.go`, `internal/tools/tools_test.go`)
- Linhas Adicionadas: ~620 (referentes à task 2.0, sobre o working tree pré-existente das features 001–009 e 010/1.0)
- Linhas Removidas: ~20

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Go 1.27, módulo `kterminal` | OK | Nenhum import novo em agent.go (context/errors/fmt/strings/sync/time pré-existentes); stdlib apenas |
| Eventos via canal (buffer 512, emit não-bloqueante) | OK | Subagente com canal próprio buffer 512; `emit` intocado; re-emissão usa o mesmo `emit` não-bloqueante do principal (select/default) — sem bloqueio nos dois níveis |
| Formatação (gofmt) / vet | OK | `gofmt -l .` vazio; `go vet ./...` limpo |
| Nomenclatura | OK | `subagentMaxSteps`/`subagentTimeout`/`taskToolName` conforme techspec; `stepLimit`/`writeSnapshot`/`newSubagent`/`RunSync` seguem o estilo do pacote |
| DDD/TS/Vitest | N/A | Brownfield Go stdlib, conforme seção "Conformidade com Skills Padrões" da techspec |
| Padrões de teste (`testing` + `httptest`, mocks sequenciais, AAA) | OK | 7 testes seguem os harnesses existentes (mock gateway com respostas sequenciais, mock Jev, captura de chamadas) |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| Campo `depth int` no `Agent` | SIM | Zero value = agente principal; `newSubagent` define `parent+1` |
| `Event` ganha `Depth int` e `ParentTool string` | SIM | Campos no fim do struct; eventos do principal permanecem `Depth: 0`/`ParentTool: ""` |
| `session.Event` ganha `Depth int` (`json:"depth,omitempty"`) | SIM | Eventos do principal byte-idênticos (0 omitido); do subagente gravados com `"depth": 1` |
| Constantes `subagentMaxSteps = 10`, `subagentTimeout = 5 * time.Minute` | SIM | Timeout encurtável/injetável via campo unexported `subagentTimeout` (ver Interpretação 6) |
| `newSubagent`: infra compartilhada por referência | SIM | LLM/Router/Fallback/Catalog/Session/Telemetry/Confirm compartilhados |
| `newSubagent`: estado próprio | SIM | `Events` próprio (512), `messages = [system, user]`, `depth = parent+1`, `Tools = tools.NewRegistry()` **sem** `task`, `pinned` vazio, `turnMessage` = mensagem user |
| Mensagens iniciais (system prompt + description + guidance) | SIM | Prompt exato da techspec; guidance omitida se vazia; `description + "\n\n" + guidance` (verificado no gateway capturado) |
| `RunSync`: ctx derivado com timeout 5min | SIM | `context.WithTimeout(ctx, timeout)` derivado do ctx do passo (= ctx do turno) |
| `RunSync`: loop síncrono na goroutine do passo (sem `go` para o loop) | SIM | `sub.runLoop(ctx)` roda na goroutine do passo da tool — requisito não negociável honrado (ver Interpretação 1 sobre a drenagem) |
| Re-emissão com `Depth`/`ParentTool` no canal do principal | SIM | Drenagem enriquece cada evento e re-emite via `a.emit`; TUI continua com um único consumidor |
| Gravação no transcript compartilhado com depth | SIM | Via writes de sessão do motor com `Depth: a.depth` (Session compartilhado) — ver Interpretação 2 |
| Fecha o canal do subagente no fim | SIM | `close(sub.Events)` após `runLoop` (e no caminho de erro de `ensureCandidates`), com join `<-drained` antes do retorno |
| Steps esgotados → `"error: subtask exceeded max steps (10)"` sem error | SIM | Sentinel `errMaxSteps` no epílogo do motor; `RunSync` mapeia para resultado com `nil` error; `EventError` suprimido para depth>0 (Decisão 4 da techspec) |
| Timeout → `"error: subtask timed out after 5m"` idem | SIM | `ctx.Err() == context.DeadlineExceeded` no RunSync; mensagem literal conforme spec (ver Interpretação 5) |
| Cancelamento compartilhado (Esc) | SIM | ctx do subagente deriva do ctx do turno; `Cancel()` mata o subagente e o principal aborta com `EventTurnAborted` (Depth 0); ordering determinístico (abort do sub Depth 1 re-emitido antes do abort do principal) |
| Limite de 10 steps no subagente (metade do principal) | SIM | `stepLimit()` por depth — um motor, dois limites (Decisão 5) |
| Limite de profundidade estrutural (registry sem `task`) | SIM | `tools.NewRegistry()` fresco no subagente — a tool não existe no universo dele, imune a prompt injection |
| Telemetria compartilhada (techspec 006) | SIM | `Telemetry.Record` do motor com store compartilhado por referência — amostras do subagente alimentam as estatísticas |
| Compação no subagente (techspec 003) | SIM | Motor idêntico; mensagens curtas raramente disparam; evento de compação gravado com depth |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 2.1 `depth` no Agent + `Depth`/`ParentTool` no `agent.Event` + `Depth` no `session.Event` | COMPLETA | Campos adicionados; transcript do principal inalterado (omitempty) |
| 2.2 `newSubagent` (infra compartilhada, estado próprio, registry sem task, mensagens iniciais) | COMPLETA | Verificado por `TestSubagentRoutesIndependently` (mensagens system/user capturadas no gateway) |
| 2.3 `RunSync` (timeout derivado, loop síncrono, drenagem com re-emissão, gravação no transcript) | COMPLETA | Verificado por `TestNestedEventsCarryDepth` e `TestSubagentWritesTranscriptWithDepth` |
| 2.4 Limites: steps (10) e timeout (5min) como resultado, sem derrubar o principal | COMPLETA | Verificado por `TestSubagentStepsLimit` (exatamente 10 chamadas, resultado de erro, turno do principal completa) e `TestSubagentTimeout` (timeout injetado 50ms, principal vivo) |
| 2.5 Testes 2, 4-9 da techspec | COMPLETA | 7 testes: `TestSubagentRoutesIndependently`, `TestNestedEventsCarryDepth`, `TestSubagentStepsLimit`, `TestSubagentTimeout`, `TestEscCancelsSubagent`, `TestSubagentWritesTranscriptWithDepth`, `TestSubagentConfirmInherited` |

## Testes
- Total de Testes: 179 (172 pré-existentes + 7 novos)
- Passando: 179
- Falhando: 0
- Coverage: não medida por ferramenta (sem comando de coverage no projeto); cobertura comportamental: roteamento independente, mensagens iniciais, partição de depth em todos os eventos, limite de steps com contagem exata de chamadas ao gateway, timeout com mock bloqueante e timeout injetado, cancelamento com ordering de aborts e turno seguinte sem vazamento, transcript com depth e snapshot único do principal, confirm herdado com aprovação concorrente
- Extra: `go test -race ./...` limpo na suíte completa (diligência — goroutine de drenagem nova)

## Verificação de Segurança
- N/A para backend/API/CORS/SQL/HTML/rate-limiting (TUI local, sem endpoints).
- Aplicável: sem secrets/keys no código (apenas mocks de teste pré-existentes); limite de profundidade estrutural (impossível burlar por prompt injection — a tool `task` não existe no registry do subagente); limites rígidos (10 steps/5min) contêm o teto de custo; transcript não vaza dados de classe diferente dos eventos do fluxo principal.

## Interpretações Documentadas (contradições/lacunas da techspec, resolvidas a favor dos requisitos explícitos)
1. **Goroutine de drenagem (ressalva principal).** A techspec exige simultaneamente (a) "roda o loop do subagente na goroutine do passo (sem `go`)" e (b) "drenagem do canal do subagente é síncrona no passo (sem goroutine extra)". Ambas são mutuamente exclusivas com o fluxo de confirm (teste 9 obrigatório): o `EventConfirm` do subagente precisa alcançar o canal do principal enquanto o `runLoop` está bloqueado em `<-ch` — a re-emissão TEM que ser concorrente ao loop, senão deadlock permanente. Resolução: o loop roda síncrono na goroutine do passo (requisito não negociável da task) e a drenagem é uma goroutine forwarder que re-emite via `a.emit` não-bloqueante e é **joined** (`<-drained`) antes do retorno de RunSync. Propriedades preservadas: nenhuma goroutine sobrevive ao passo; o passo bloqueia até a delegação terminar (sequencial da v1 intacto — sem subagentes paralelos, sem progresso do principal durante a delegação); emit não-bloqueante nos dois níveis; TUI com um único consumidor; sem corrida de transcript (writes de sessão ficam na goroutine do passo). A alternativa (drain síncrono + loop em goroutine) violaria o "sem `go`" do loop, que a task marca como não negociável, e deixaria panics do motor fora da goroutine do passo.
2. **Gravação no transcript acontece no motor, não no drain.** A techspec diz que RunSync "grava `session.Event{..., Depth}` no transcript compartilhado". Implementado adicionando `Depth: a.depth` aos writes de sessão do motor (Session compartilhado por referência). Gravar no drain duplicaria eventos (o motor já grava route/tool_call/tool_result/assistant/turn_aborted) e escreveria deltas/tool_outputs no JSONL, violando o contrato existente (`TestToolOutputNotWrittenToTranscript`). Efeito observável idêntico ao especificado: eventos do subagente no mesmo JSONL com `depth: 1` (REQ-006, testado).
3. **Snapshots do subagente suprimidos.** `WriteSnapshot` não tem parâmetro depth; snapshots do subagente ficariam sem marcação (violando o "distinguível" do REQ-006) e poluiriam o stream de resume da feature 004 (`Load` pega o último snapshot; subagentes não são persistidos por PRD — fora de escopo). Helper `writeSnapshot()` pula depth>0; o último snapshot do JSONL continua sendo o do principal (assertado no teste 8). A auditoria da delegação permanece completa via eventos de fluxo (rotas, tools, resultados — exatamente o que a techspec enumera).
4. **`tools.Register` exportado antecipado (pull-forward da task 3.0).** A techspec coloca a exportação no passo 3, mas o teste obrigatório `TestEscCancelsSubagent` exige que a engine do principal execute uma tool que chama `RunSync` ("Run do principal → task em execução"), o que requer registrar uma tool stand-in a partir do package `agent` — impossível com `register` unexported. Renome mecânico (register→Register, 6 call-sites internos + 1 call-site de teste), zero mudança de comportamento. A task 3.0 encontrará o método pronto.
5. **Mensagem de timeout fixa.** A techspec e o critério de sucesso exigem a string literal `"error: subtask timed out after 5m"` mesmo com timeout encurtado injetado no teste — hardcoded conforme spec (`fmt.Sprintf` com `time.Duration` produziria "5m0s", divergindo do contrato).
6. **Campo `subagentTimeout` no Agent.** Constants não são injetáveis; o campo unexported (setado em `New` a partir da const, herdado por `newSubagent`, fallback para a const se ≤0) implementa o "timeout encurtável/injetável para teste" exigido pela task. Mesmo padrão do seam `now` existente.
7. **Stand-in task tool nos testes com `Mutating: false`.** O foco do teste 9 é a herança do Confirm no SUBAGENTE (tool mutante do subagente emite `EventConfirm` com `Depth: 1`); a confirmação da própria tool `task` pertence à task 3.0 (`Mutating: true` no `AttachTaskTool` real). A stand-in espelha o `Execute` da techspec (chamada a `RunSync` com description/guidance), com a assinatura real de `tools.Executor` (`Result, error`) — o snippet da techspec usa `(string, error)`, que não corresponde ao tipo `Executor` do código; seguido o contrato real.
8. **SessionCost do principal não inclui custo do subagente.** Techspec silenciosa. Os custos do subagente ficam gravados por-evento no transcript (assistant events com `Cost` e `depth: 1`) e o TPS alimenta a telemetria compartilhada; o `turn_done` do principal reporta o custo do fluxo principal. Não consolidação de total na v1.

## Deferrals por sequenciamento (não são ressalvas)
- Renderização aninhada na TUI (indentação 2 espaços, `colSecondary`, hint bar `subagent running`): task 4.0. Os eventos aninhados já chegam ao canal do principal com `Depth`/`ParentTool`, mas a TUI atual não os distingue.
- `AttachTaskTool` + schema real + `Mutating: true`: task 3.0. O wiring no `main.go` (task 5.0) torna a feature inerte no app real até lá — nenhum comportamento aninhado é produzido sem a tool registrada.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/agent/agent.go | 189-226 | Goroutine de drenagem diverge da letra de "drenagem síncrona sem goroutine extra" (techspec, Arquitetura) — exigida pelo fluxo de confirm (ver Interpretação 1) | Documentado; se a techspec for revisada, alinhar o texto à implementação (drain forwarder joined) |
| Baixa | internal/agent/agent.go | 417-422 | `EventError` de max-steps suprimido para depth>0 ramifica o motor por depth (ver Interpretação 2/Decisão 4) | Aceitável — alternativa seria filtrar pós-hoc no drain, impossível com re-emissão em streaming |
| Baixa | internal/tools/tools.go | 49 | `Register` exportado antes da task 3.0 (ver Interpretação 4) | Nenhuma — renome mecânico, zero comportamento |

## Pontos Positivos
- Um único motor de turnos (Decisão 5): `stepLimit()` por depth elimina qualquer duplicação de lógica de steps/roteamento/abort/tool calls entre os dois níveis.
- Limite de profundidade estrutural: o subagente recebe um registry fresco — a tool `task` simplesmente não existe no universo dele; imune a prompt injection por construção.
- Drenagem com join determinístico: `close(sub.Events)` → `range` termina → `<-drained` garante que nenhuma goroutine sobrevive ao passo e que todo evento emitido antes do fechamento é re-emitido.
- Cancelamento com ordering determinístico: o abort do subagente (Depth 1) é sempre re-emitido antes do abort do principal (Depth 0), porque RunSync só retorna após o join — assertado no teste 7.
- Sentinel `errMaxSteps` preserva a mensagem exata do fluxo principal ("reached max tool steps (25) without a final answer") enquanto permite `errors.Is` no RunSync — mapeamento robusto, sem parsing de string.
- Transcript byte-idêntico para o fluxo principal (`Depth: 0` + `omitempty`): os 172 testes existentes passam sem nenhuma mudança de asserção (apenas 1 rename de call-site em `tools_test.go`).
- Testes exercitam o fluxo real de ponta a ponta (principal→tool→RunSync→subagente→gateway→eventos→transcript) com a tool stand-in — a task 3.0 só substitui a stand-in pelo `AttachTaskTool` real.

## Recomendações
- Task 3.0: `AttachTaskTool` deve registrar a tool com `Mutating: true` e o `Execute` exato da stand-in (chamando `parent.RunSync`); reutilizar `taskToolName`.
- Task 4.0: a TUI deve ignorar `EventTurnDone`/`EventTurnAborted` com `Depth > 0` para o controle de estado do turno (busy/abort) e renderizar eventos aninhados com indentação; o hint bar usa `EventToolStart`/`EventToolResult` da tool `task` (Depth 0).
- Task 5.0: no wiring, chamar `AttachTaskTool` apenas no agente principal (a checagem `depth == 0` pertence à 3.0).

## Conclusão
Implementação aderente à task 2.0 e à techspec: campo `depth`, `newSubagent` com infra compartilhada/estado próprio/registry estruturalmente sem `task`, e `RunSync` síncrono com timeout derivado, re-emissão enriquecida (`Depth: 1`/`ParentTool: "task"`), gravação no transcript compartilhado com depth e limites de steps/timeout devolvidos como resultado sem derrubar o principal. Cancelamento compartilhado com ordering determinístico e sem vazamento de goroutines. Os 7 testes obrigatórios provam todos os critérios de sucesso (roteamento independente, partição de depth, limite de 10 steps com contagem exata de chamadas, timeout injetável, Esc cancelando ambos os níveis, JSONL com depth, confirm herdado); os 172 testes existentes permanecem verdes sem mudança de asserção e o race detector está limpo na suíte completa. As interpretações 1-8 decorrem de contradições internas da techspec (goroutine da drenagem vs confirm concorrente; gravação de transcript no drain vs motor) e de lacunas (snapshots do subagente, injetabilidade do timeout), resolvidas a favor dos requisitos explícitos e não negociáveis da task e documentadas acima. **APROVADO COM RESSALVAS** — ressalvas são de severidade baixa, todas documentadas e nenhuma bloqueante para as tasks subsequentes.

## Checks Executados
| Check | Resultado |
|-------|-----------|
| `go build ./...` | OK |
| `go vet ./...` | OK |
| `gofmt -l .` | OK (nenhum arquivo listado) |
| `go test ./... -count=1` | OK — 179/179 passando (172 pré-existentes + 7 novos), 0 falhas |
| `go test -race ./... -count=1` | OK (diligência extra — suíte completa limpa) |
