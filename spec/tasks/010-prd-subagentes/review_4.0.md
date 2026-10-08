# Relatório de Code Review — Subagentes: renderização aninhada na TUI (Task 4.0)

## Resumo
- Data: 2026-10-05
- Branch: working tree (features 001–010 não commitadas; nenhum commit feito nesta task)
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 3 (`internal/tui/theme.go`, `internal/tui/tui.go`, `internal/tui/tui_test.go`)
- Linhas Adicionadas: ~176 (theme.go +3, tui.go +44, tui_test.go +129)
- Linhas Removidas: 0 (5 linhas realinhadas por gofmt no bloco de campos do Model)

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Receivers por valor na TUI | OK | `handleAgentEvent` mantém value receiver; `handleNestedEvent`/`nestedPrefix` seguem o padrão dos mutators internos (`resetLiveBlock`, `refreshContent` — pointer receiver) |
| Paleta existente (`colSecondary`) | OK | `nestedStyle` definido em `theme.go` reutiliza `colSecondary`; nenhum hex novo |
| Stack Go 1.27 / stdlib + deps TUI existentes | OK | Nenhuma dependência nova |
| Estrutura de pastas | OK | Mudanças confinadas em `internal/tui/` |
| Formatação/lint | OK | `gofmt -l .` vazio, `go vet` limpo |

## Verificação de Segurança
N/A — funcionalidade de TUI local, sem backend/API/endpoints/SQL. Nenhum secret ou key no diff. Inputs de eventos vêm do canal interno do agent (fonte confiável em processo).

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `handleAgentEvent`: `e.Depth > 0` → prefixo de 2 espaços por nível + `colSecondary` (rota, tool, resultado) | SIM | Branch antecipante em `handleAgentEvent` delega a `handleNestedEvent`; `nestedPrefix(depth)` = 2 espaços/nível; rota/tool/resultado em `nestedStyle` (`colSecondary`) |
| Fluxo do principal mantém os estilos atuais | SIM | Switch depth-0 intocado; teste asserta rota principal em `colTextMuted` sem indent; 182 testes pré-existentes passam |
| Linha da tool `task` usa estilo de tool normal (indentação começa nos eventos do subagente) | SIM | `EventToolStart{task}` (depth 0) renderiza com `toolStyle` sem prefixo; teste asserta ANSI `colInfo` |
| Hint bar `subagent running` enquanto `busy` + subagente ativo | SIM | Flag `subagentActive` setada por `EventToolStart{Tool:"task"}`, limpa no `EventToolResult` dela; hint substitui "Thinking" mantendo o spinner |
| `session.Event.Depth` gravado (task 2.0) | SIM (pré-existente) | `json:"depth,omitempty"` já presente; nada a fazer — a TUI consome `agent.Event.Depth` |
| Re-emissão no canal principal com Depth (decisão 3) | SIM (pré-existente) | TUI permanece com consumidor único de eventos — zero mudança no loop de escuta |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 4.1 Tratar `Depth > 0` com indentação 2 espaços/nível e `colSecondary` | COMPLETA | Rota, tool start e tool result aninhados; escalonamento por nível validado com depth 2 |
| 4.2 Flag de subagente ativo + hint bar `subagent running` | COMPLETA | Set/clear por eventos da tool `task`; limpeza defensiva nos eventos terminais do turno |
| 4.3 Testes 10-11 da techspec | COMPLETA | `TestNestedEventsRenderIndented` (10) e `TestHintBarShowsSubagentRunning` (11) + `TestNestedConfirmPausesApproval` (edge) |

## Testes
- Total de Testes: 185
- Passando: 185
- Falhando: 0
- Novos: 3 (`TestNestedEventsRenderIndented`, `TestHintBarShowsSubagentRunning`, `TestNestedConfirmPausesApproval`)
- Coverage: sem ferramenta de coverage configurada no projeto; cobertura comportamental dos novos branches: rota/tool/resultado aninhados, depth 2, fluxo principal inalterado, não-vazamento de deltas aninhados no stream/chat, não-clobber de `currentModel`/`routerInfo`, hint lifecycle (task start → resultado → abort sem resultado), confirm aninhado com aprovação
- Checks: `go build ./...` OK · `go vet ./...` OK · `gofmt -l .` vazio · `go test ./...` 185/185

## Ressalvas (decisões de design para lacunas da spec — nenhuma pendente de correção)
| # | Decisão | Justificativa |
|---|---------|---------------|
| 1 | Eventos aninhados de `TurnDone`/`TurnAborted`/`Error`/`Delta`/`ToolOutput`/`Compaction` são ignorados pela TUI | A techspec especifica renderização aninhada apenas para rota/tool/resultado. `TurnDone`/`TurnAborted` aninhados no switch principal setariam `busy=false` no meio do turno do principal e renderizariam meta de turno do principal com dados do subagente; deltas aninhados em `m.stream` seriam misturados ao markdown final do principal (contaminação de contexto); tool output aninhado renderizaria em `colTextMuted` sem indentação (indistinguível do fluxo principal). A resposta final do subagente chega como tool result da `task` (depth 0) e o transcript registra tudo com `depth: 1` |
| 2 | `EventConfirm` aninhado pausa a UI igual ao fluxo principal | Sem isso o subagente deadlockaria em `<-ApproveCh`. A techspec (Riscos) confirma: "tools mutantes do subagente pausam para aprovação — UX aceitável na v1". Testado |
| 3 | Route aninhado não atualiza `currentModel`/`routerInfo` | `routerInfo` alimenta o `turnMeta` do turno do principal; `currentModel` alimenta o hint bar. Eventos do subagente não podem clobberar estado do fluxo principal. Testado |
| 4 | Flag `subagentActive` limpa também em `TurnDone`/`TurnAborted`/`Error` (depth 0) | Quando Esc aborta durante a task, `abortTurn` emite `EventTurnAborted` **sem** o `EventToolResult` da tool `task` pendente (`runLoop` retorna antes de gravar o result) — a flag vazaria para o turno seguinte e o hint mostraria `subagent running` sem subagente. Testado |
| 5 | `subagent running` substitui "Thinking" no hint bar (spinner mantido) | A techspec exige apenas que o hint mostre `subagent running` sob a condição busy+flag; a substituição dá estado inequívoco (Thinking → subagent running → Thinking) |
| 6 | Constante local `taskTool = "task"` na TUI | O nome é fixado pelo PRD; exportar `taskToolName` do package `agent` alteraria API fora do escopo desta task |

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| — | — | — | Nenhum problema aberto | — |

## Pontos Positivos
- Branch antecipante por depth isola todo o manuseio de estado do fluxo principal: eventos aninhados não podem tocar `busy`, `stream`, `liveLines`, anexos ou meta de turno — correção estrutural, não por checagem dispersa
- `nestedPrefix` reutilizável cobre qualquer profundidade (validado com depth 2), embora a v1 estruture o limite em 1
- Testes assertam ANSI exato (`colSecondary` vs `colTextMuted` vs `colInfo`), prefixo de indentação, ordem dos blocos e não-regressão do fluxo principal
- O leak da flag no abort (ressalva 4) foi identificado na análise e coberto por teste antes do review

## Recomendações
- Task 5.0 (wiring no `main.go`) deve chamar `ag.AttachTaskTool()` para ativar o fluxo E2E; `kspec-qa` deve validar a delegação real visível (indentação `colSecondary`, hint bar durante a execução, Esc cancelando tudo)
- Se o QA julgar perda de informação, a renderização de tool output aninhado (live block em `colSecondary`) pode ser adicionada depois como melhoria incremental — fora do escopo desta task

## Conclusão
Implementação aderente à techspec: eventos com `Depth > 0` renderizam com prefixo de 2 espaços por nível em `colSecondary` (rota, tool, resultado), o fluxo principal permanece visualmente idêntico, a linha da tool `task` usa estilo normal e o hint bar sinaliza `subagent running` com lifecycle correto — inclusive no caminho de abort. As seis ressalvas são decisões de design documentadas para pontos em que a spec é silente, todas exigidas por correção (deadlock de confirm, `busy=false` prematuro, leak de flag) e cobertas por teste; nenhuma exige correção. 185/185 testes passando, build/vet/gofmt limpos. **APROVADO COM RESSALVAS** — task 4.0 considerada completa.
