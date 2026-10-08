# Relatório de Code Review — 010-prd-subagentes — Task 1.0: Extrair `runLoop` do loop do Agent (refactor puro)

## Resumo
- Data: 2026-10-04
- Branch: 002-010-prds-kterminal
- Status: APROVADO
- Arquivos Modificados: 1 (`internal/agent/agent.go`)
- Linhas Adicionadas: 17 (referentes à task 1.0, sobre o working tree pré-existente das features 001–009)
- Linhas Removidas: 6

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Go 1.27, módulo `kterminal`, apenas stdlib | OK | Nenhum import novo (context/errors/fmt/strings já existiam) |
| Eventos via canal (buffer 512, emit não-bloqueante) | OK | `emit` intocado; mesmo canal, mesmo buffer, mesmo comportamento de drop |
| Formatação (gofmt) / vet | OK | `gofmt -l .` vazio; `go vet ./...` limpo |
| Nomenclatura | OK | `runLoop` conforme spec; `turnMessage` segue o estilo de campos privados existentes (`lastPromptTokens`, `sessionCost`) |
| DDD/TS/Vitest | N/A | Brownfield Go stdlib, conforme seção "Conformidade com Skills Padrões" da techspec |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `runLoop(ctx) (finalText string, err error)` extraído do `loop` (Decisão 5: extraído, não duplicado) | SIM | Assinatura exata conforme task e techspec; motor de steps/roteamento/abort/tool calls transplantado verbatim |
| `loop` vira wrapper com goroutine delegando o motor a `runLoop` | SIM | Goroutine movida para dentro do wrapper (`RunWithAttachments` chama `loop` sincronamente; `loop` spawn a goroutine do turno) |
| Eventos de turno no modo async | SIM | `EventTurnDone`/`EventTurnAborted` preservados idênticos (mesmo conteúdo, mesma ordem), emitidos pelo motor que o wrapper delega |
| Nenhuma mudança de comportamento observável | SIM | Mesmos eventos, mesma ordem, mesmo conteúdo; transcript idêntico; 172 testes existentes verdes sem modificação |
| Nenhum evento novo | SIM | Ver interpretação nº 1 abaixo sobre `EventTurnStart` |
| Nenhuma mudança de assinatura pública além da interna `runLoop` | SIM | `runLoop` e `turnMessage` são unexported; API exportada intocada |
| Preparação para `RunSync` (task 2.0) | SIM | Motor não anexa a mensagem do usuário (wrapper anexa) — exatamente o que `newSubagent` com `messages = [system, user]` pré-definidas exige; retornos `(finalText, err)` disponíveis para coleta sync |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 1.1 Extrair o corpo do loop de turnos para `runLoop(ctx) (finalText, error)` | COMPLETA | Loop de steps + epílogo de max-steps transplantados com retornos: `(result.Content, nil)` no sucesso, `ctx.Err()` no abort, `err` em erros, erro de max-steps no esgotamento |
| 1.2 Reduzir o `loop` a wrapper (goroutine + eventos de turno) chamando `runLoop` | COMPLETA | Wrapper: goroutine + setup de turno (ensureCandidates → ctx check → vision check → append user message → session write) + delegação |
| 1.3 Suíte completa verde sem mudanças nos testes | COMPLETA | `agent_test.go` intocado; 172/172 passando |

## Testes
- Total de Testes: 172
- Passando: 172
- Falhando: 0
- Coverage: não medida por ferramenta (stdlib apenas, sem comando de coverage no projeto); cobertura comportamental da suíte existente é o critério do refactor puro, conforme a seção Testes da Tarefa
- Extra: `go test -race ./internal/agent/ ./internal/tui/` limpo (diligência — a estrutura de goroutine foi tocada)

## Verificação de Refactor Puro (nenhum comportamento mudou)
- Ordem do setup preservada: `ensureCandidates` → ctx check → vision check → append da user message → session write. `TestNoVisionModelPreservesAttachments` (exige `messages` vazio quando vision falha) passa.
- `decide` recebe a mensagem original do usuário em todos os steps, inclusive após compação removê-la de `a.messages` — garantido pelo campo `turnMessage` (derivá-la de `a.messages` mudaria o roteamento pós-compação).
- Todos os caminhos de abort chamam `abortTurn` com os mesmos argumentos nas mesmas condições; o `partial` do setup é vazio em ambos os mundos (nenhum delta antes do loop).
- Ordem snapshot→erro preservada nos caminhos de erro (`TestSnapshotWrittenAfterStreamError`).
- Sequência exata de eventos de tool preservada (`TestToolOutputFlushedBeforeToolResult`).
- Retornos de `runLoop` são ignorados pelo wrapper (`_, _ =`) — efeito observável zero na task 1.0; existem para o `RunSync` da task 2.0.

## Interpretações Documentadas (contradições internas da task, resolvidas a favor das restrições explícitas)
1. **`EventTurnStart` não foi criado.** A task o menciona na descrição do wrapper, mas o mesmo documento exige "Nenhum evento novo" e "mesmos eventos, na mesma ordem, com o mesmo conteúdo" — e `EventTurnStart` não existe no codebase. Criá-lo seria mudança de comportamento observável, violando o contrato do refactor puro. `EventTurnDone`/`EventTurnAborted` continuam fluindo no modo async exatamente como antes. Se a task 2.0+ quiser `EventTurnStart`, será adição explícita dela.
2. **Eventos de turno emitidos pelo motor, não movidos para o wrapper.** Mover a emissão de `EventTurnDone` para o wrapper é impossível sem mudar seu conteúdo (`Model`/`TPS`/`Tokens` são locais do step) ou enriquecer o retorno além do `(finalText, err)` especificado. Manter a emissão no motor também é o design correto para a 2.0: o `turn_done` do subagente será re-emitido aninhado com `Depth: 1`.
3. **Campo interno `turnMessage`.** Necessário para honrar a assinatura literal `runLoop(ctx)` da spec: o motor precisa da mensagem original do turno para o `decide` de cada step. O wrapper a define imediatamente antes da delegação; escrita e leitura ocorrem na mesma goroutine do turno (sem nova superfície de corrida — o agent já assume um turno por vez sobre `a.messages`). Na 2.0, `newSubagent` a define na construção.
4. **Setup de turno permanece no wrapper** (não no motor): exigido pela ordem observável (mensagem do usuário não pode ser anexada quando candidates/vision falham) e pela 2.0 (mensagens do subagente são pré-definidas; o motor não pode anexar).

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| — | — | — | Nenhum problema bloqueante ou não-bloqueante encontrado | — |

## Pontos Positivos
- Motor de turnos com fonte única: steps, roteamento, abort, tool calls, compação e truncamento idênticos para os dois acionamentos futuros (async/sync) — menos drift, conforme a Decisão 5 da techspec.
- Diff mínimo e cirúrgico (+17/−6): o corpo do motor foi transplantado verbatim, facilitando a auditoria de pureza do refactor.
- Retornos de `runLoop` já no contrato que a task 2.0 precisa (`finalText` para o tool result; `ctx.Err()` para propagação de Esc; erro de steps para o resultado `error: ...`).
- Suíte existente (172 testes, incluindo sequências exatas de eventos, transcript, snapshots, cancelamento e compação) valida o refactor sem nenhuma alteração — o critério de conclusão da task foi atendido por construção.

## Recomendações
- Na task 2.0, `newSubagent` deve definir `turnMessage` na construção (mensagem user = description + guidance) e `RunSync` deve chamar `ensureCandidates` antes de `runLoop` (o setup ficou no wrapper por contrato da 1.0).
- O limite de 10 steps do subagente (`subagentMaxSteps`) continuará sendo responsabilidade da task 2.0 ("com re-emissão e limites") — `runLoop` mantém `maxSteps` (25) nesta task, sem antecipação de escopo.

## Conclusão
Refactor puro executado conforme a task e a techspec: o loop de turnos foi extraído para `runLoop(ctx) (finalText, err)` reusável, o `loop` foi reduzido a wrapper (goroutine + setup de turno + delegação), nenhum comportamento observável mudou (mesmos eventos, mesma ordem, mesmo conteúdo; nenhum evento novo; nenhuma assinatura pública alterada) e a suíte completa de 172 testes passa sem modificação. Build, vet, gofmt e testes verdes; race detector limpo. As quatro interpretações documentadas decorrem de contradições internas do texto da task e foram resolvidas a favor das restrições explícitas de refactor puro. **APROVADO**.

## Checks Executados
| Check | Resultado |
|-------|-----------|
| `go build ./...` | OK |
| `go vet ./...` | OK |
| `gofmt -l .` | OK (nenhum arquivo listado) |
| `go test ./... -count=1` | OK — 172/172 passando, 0 falhas |
| `go test -race ./internal/agent/ ./internal/tui/` | OK (diligência extra) |
