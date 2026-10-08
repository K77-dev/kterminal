# Relatório de Code Review - EventToolOutput com throttle de 50ms no Agent (Task 2.0)

## Resumo
- Data: 2026-10-04
- Branch: 002-010-prds-kterminal
- Status: APROVADO COM RESSALVAS
- Arquivos Modificados: 2 (`internal/agent/agent.go`, `internal/agent/agent_test.go`)
- Linhas Adicionadas: ~50 em `agent.go` (constante, kind, campo `now`, `executeTool`, `toolOutputCollector`), ~319 em `agent_test.go` (5 testes + helpers)
- Linhas Removidas: 1 (chamada `a.Tools.Execute` no loop, substituída por `a.executeTool`)
- Nota: o diff do working tree acumula as features 001–006 e 007/1.0 (já revisadas); este review cobre apenas o delta da task 2.0.

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Zero comentários no pacote `agent` (grep `//` e `/*` sem matches) |
| Apenas stdlib | OK | Nenhuma dependência nova; só `time` e `strings` (já importados) |
| Eventos via canal buffer 512, emit não-bloqueante | OK | Coletor reusa `a.emit` (select/default) — política inalterada |
| `strings.Builder` por ponteiro | OK | N/A no agent — eventos usam `strings.Join` sem estado no Model da TUI |
| Formatação/lint | OK | `gofmt -l .` vazio, `go vet` limpo |
| Rules DDD/TS/Vitest/logging | N/A | Brownfield Go, conforme techspec |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `toolOutputThrottle = 50 * time.Millisecond` | SIM | Const em `agent.go` |
| `EventToolOutput EventKind = "tool_output"` | SIM | Entre `tool_start` e `tool_result` (ordem do ciclo de vida) |
| Loop usa `ExecuteStream` com coletor (linhas acumuladas, emit quando `now()-lastEmit >= throttle`, reset após emit) | SIM | `executeTool` + `toolOutputCollector.onLine` |
| Flush final antes do `EventToolResult` | SIM | `collector.flush()` dentro de `executeTool`, antes do retorno; testes provam a sequência `ToolStart→ToolOutput→ToolResult` |
| `Event` reusa `Tool` + `Text` (linhas com `\n`), sem campos novos | SIM | Struct `Event` intocado |
| `session.Event` sem mudança — `tool_output` não gravado (REQ-005) | SIM | Nenhum `Session.Write` novo; teste lê o JSONL e nega `type=="tool_output"` |
| Campo `now func() time.Time` injetável (default `time.Now`) | SIM | Campo em `Agent`, setado em `New`; fallback nil-safe para `&Agent{}` literal |
| Throttle no agent (não na tool) | SIM | Coletor vive no agent; tool emite linha a linha |
| Contrato do gateway inalterado (LLM vê só `role:"tool"` consolidado) SIM | SIM | Teste captura mensagens da 2ª chamada: 3 mensagens, tool message com output consolidado |
| Snippet do coletor (techspec §Throttle no agent) | RESSALVA | Ver "Problemas Encontrados" — desvio justificado na inicialização de `lastEmit` |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 2.1 `EventToolOutput` no `EventKind` + campo `now` injetável | COMPLETA | |
| 2.2 Loop usa `ExecuteStream` com coletor de throttle | COMPLETA | Único caller de `Registry.Execute` no agent substituído |
| 2.3 Flush final antes do `EventToolResult` | COMPLETA | Flush roda mesmo em erro/abort — nenhuma linha perdida |
| 2.4 Testes 6-7 da techspec | COMPLETA | + 3 testes extras justificados (ver Testes) |

## Testes
- Total de Testes: 132 (127 pré-existentes + 5 novos)
- Passando: 132
- Falhando: 0
- Coverage: N/A (projeto não mede coverage; suite completa passa, incluindo `-race` no pacote agent)
- Novos:
  - `TestToolOutputThrottled` (techspec item 6) — coletor com fake clock: 100 linhas no mesmo instante ficam acumuladas (0 eventos — sem inundar o canal); +60ms e nova linha → 1º evento agrupado com as 100; +120ms → 2º evento; flush final com as restantes; completude das 115 linhas na ordem; flush idempotente.
  - `TestToolOutputFlushedBeforeToolResult` — e2e com mock gateway, 2 tool calls bash e `now` fixo injetável: sequência exata `ToolStart→ToolOutput→ToolResult` ×2; 1 evento agrupado por comando com texto exato (`1..100`, `101..110`).
  - `TestToolOutputNotWrittenToTranscript` (techspec item 7) — e2e com session writer: JSONL sem `tool_output`; `tool_result` presente com output consolidado; gateway recebe apenas a mensagem `role:"tool"` consolidada (contrato).
  - `TestToolOutputPartialTailNotLost` — nota do review 1.0: `printf` sem `\n` final → cauda parcial do lineWriter chega ao coletor antes de `ExecuteStream` retornar e é emitida no flush antes do resultado; resultado consolidado também a contém.
  - `TestSilentToolEmitsNoToolOutput` — edge case: comando silencioso → 0 eventos de output (REQ-04: sem bloco vazio), resultado `(no output)`.
- Cobertura de edge cases: comando silencioso, cauda parcial sem newline, erro/abort durante execução (flush preservado), tools sem `ExecuteStream` (fallback do registry — exercitado pelos testes existentes de write/edit que agora passam por `executeTool`), `&Agent{}` com `now` nil.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/agent/agent.go | 338 | Desvio do snippet da techspec: `lastEmit` inicializado com `clock()` (início da execução) em vez de `var lastEmit time.Time` (zero value). Com o zero value, a 1ª linha emitiria imediatamente um evento de 1 linha — o burst de 100 linhas produziria 2 eventos, violando o Critério de Sucesso "100 linhas no mesmo instante → 1 EventToolOutput agrupado". O snippet também hardcodeia `time.Now`/`time.Since`, que a própria task manda substituir pelo relógio injetável — é ilustrativo, não literal. O invariante do REQ-004 ("no máximo 1 evento a cada 50ms") permanece: janelas medidas do início da execução/último emit. | Manter; se o comportamento de primeira-linha-imediata for desejado no futuro, inicializar `lastEmit` com o zero value e atualizar o critério de sucesso. |

## Pontos Positivos
- Coletor isolado (`toolOutputCollector`) torna o throttle testável de forma 100% determinística (fake clock, zero sleeps), sem acoplar o teste ao tempo real do bash.
- Flush incondicional em `executeTool` garante "nenhuma linha perdida" inclusive nos caminhos de erro e abort.
- Interação com o flush de cauda parcial do lineWriter (review 1.0) coberta ponta a ponta.
- Contrato do gateway verificado por captura de mensagens, não apenas por ausência de erro.
- Nenhum teste existente quebrou (127 prévios passando) — `EventToolOutput` flui pelo canal sem impactar switches existentes da TUI (kind desconhecido é ignorado até a task 3.0).

## Recomendações
- Na task 3.0 (bloco ao vivo na TUI), tratar `EventToolOutput` no switch e descartar o bloco no `EventToolResult`/`EventTurnAborted` — o agent já garante a ordenação flush→result.
- Considerar rate de eventos no pior caso confirmado: 1000 linhas/s → ≤20 eventos/s + 1 flush por execução, dentro do buffer 512.

## Conclusão
Implementação aderente aos requisitos e critérios de sucesso da task 2.0 e às decisões da techspec (throttle no agent, flush antes do resultado, transcript enxuto, relógio injetável, contrato do gateway intacto). Um desvio deliberado e justificado em relação ao snippet ilustrativo da techspec (inicialização de `lastEmit`) é registrado como ressalva não-bloqueante — ele é o que torna o critério de sucesso "100 linhas → 1 evento agrupado" literalmente verdadeiro. Todos os checks passam: `go build ./...`, `go vet ./...`, `gofmt -l .` (vazio), `go test ./...` (132/132). RESSALVA não exige correção — veredito: APROVADO COM RESSALVAS.
