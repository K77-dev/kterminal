# Relatório de Code Review - Subagentes: Tool `task` com `AttachTaskTool` (Task 3.0)

## Resumo
- Data: 2026-10-05
- Branch: 002-010-prds-kterminal
- Status: APROVADO
- Arquivos Modificados: 2 (`internal/agent/agent.go`, `internal/agent/agent_test.go`)
- Linhas Adicionadas: 166
- Linhas Removidas: 40

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Stdlib apenas (Go 1.27, módulo `kterminal`) | OK | Nenhuma dependência nova |
| Tools via registry | OK | Tool registrada via `Registry.Register` exportado |
| `Mutating: true` para herdar `--confirm` | OK | Confirmado por `TestAttachTaskToolDepthGuard` e `TestSubagentConfirmInherited` |
| Formatação/lint | OK | `gofmt -l .` vazio, `go vet ./...` limpo |
| Nomenclatura | OK | `AttachTaskTool` exportado, segue o padrão de `RunSync`/`newSubagent` |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `Register(t Tool)` exportado no `Registry` | SIM | Pré-existente (task 2.0/estado atual) — verificado em `internal/tools/tools.go` |
| `AttachTaskTool()` registra `task` apenas quando `depth == 0` | SIM | Guarda `if a.depth != 0 { return }` |
| Schema: `description` (obrigatória), `guidance` (opcional) | SIM | Idêntico ao snippet da techspec (nome, descrição, properties, required) |
| Descrição "Delegate a focused subtask to an isolated subagent and get its final answer" | SIM | Texto exato |
| `Mutating: true` — herda `--confirm` | SIM | A tela de confirmação mostra `task(description)` como qualquer tool mutante |
| `Execute` chama `RunSync(ctx, description, guidance)` e devolve o texto final | SIM | Texto final em `Result.Output` |
| Subagente não vê `task` nas definições (registry próprio sem `task`) | SIM | `newSubagent` (task 2.0) cria `tools.NewRegistry()` fresco; provado por teste |
| Wiring no `main.go` | NÃO (correto) | Escopo da task 5.0 — não implementado para evitar scope creep |

Adaptações obrigatórias do snippet da techspec (pseudo-código → tipos reais), sem divergência semântica:
- `str(args, "description")`/`optStr(args, "guidance")` são unexported no package `tools`; usadas type assertions `args["description"].(string)`/`args["guidance"].(string)`, semanticamente idênticas ao snippet (que ignora o erro de `str`).
- O snippet mostra `Execute` retornando `(string, error)`; o tipo real `tools.Executor` retorna `(tools.Result, error)` — o texto final vai em `Result.Output`, erro propagado como erro da tool (o `runLoop` do principal converte em tool result `error: ...` e segue vivo).

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 3.1 Exportar `Register(t Tool)` no `Registry` | COMPLETA | Pré-existente no estado atual do working tree |
| 3.2 Implementar `AttachTaskTool` com schema e guarda de `depth == 0` | COMPLETA | `internal/agent/agent.go` |
| 3.3 Escrever os testes 1 e 3 da techspec | COMPLETA | `TestTaskToolRunsSubagent` (mandatório) e `TestSubagentDoesNotSeeTaskTool` |

## Testes
- Total de Testes: 182 (179 pré-existentes + 3 novos)
- Passando: 182
- Falhando: 0
- Coverage: n/a (sem tool de coverage configurado no projeto); suíte de subagente (10 testes) cobre delegação, roteamento independente, depth, invisibilidade da tool, limites de steps, timeout, cancelamento, transcript e confirm
- Race detector: `go test -race ./internal/agent/` limpo

Testes novos:
- `TestTaskToolRunsSubagent` (mandatório): principal chama `task` → subagente roda (3 calls: parent, subagent, parent) → texto final chega como tool result (depth 0) → subagente emite `EventTurnDone` depth 1 com o texto final → principal responde ao usuário → último evento é `EventTurnDone` depth 0.
- `TestSubagentDoesNotSeeTaskTool`: request do subagente capturado pelo mock gateway **não** contém `task` nas tools (mas contém as tools padrão, ex. `bash`); requests do principal (antes e depois da delegação) contêm `task`.
- `TestAttachTaskToolDepthGuard` (adicional, justificado abaixo): guarda de depth — `AttachTaskTool` registra no principal, recusa no subagente; `IsMutating("task")` = true.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| - | - | - | Nenhum problema bloqueante encontrado | - |

## Pontos Positivos
- Limite de profundidade estrutural preservado: a tool só existe no universo do registry do principal — impossível de burlar por prompt injection (decisão 2 da techspec).
- Stand-in de teste `attachTestTaskTool` (task 2.0, `Mutating: false`) removido: os 7 testes da task 2.0 agora exercitam a tool real, eliminando duplicação de schema e risco de drift.
- `TestSubagentConfirmInherited` fortalecido: com a tool real mutante, agora valida também que a delegação pausa para aprovação no `--confirm` (confirm de depth 0 da `task` antes do confirm de depth 1 do `bash`) — requisito REQ-001.
- Erro do `RunSync` propagado como erro da tool, não como `EventError` — o principal segue vivo (decisão 4 da techspec).

## Recomendações
- `TestAttachTaskToolDepthGuard` foi adicionado além dos dois testes listados na task: valida diretamente a guarda de `depth == 0` (subtask 3.2) e a flag `Mutating` — ambos requisitos explícitos da task; mantê-lo na suíte.
- `description` ausente resulta em delegação com description vazia (o snippet da techspec ignora o erro de `str`); o schema marca `description` como `required`, então o enforcement fica no gateway — comportamento conforme spec, sem ação necessária na v1.
- Na task 5.0 (wiring), chamar `ag.AttachTaskTool()` uma única vez após a construção do agent — `Registry.Register` não é idempotente (registro duplicado duplicaria a definição no request).

## Conclusão
Implementação aderente à techspec: `AttachTaskTool` registra a tool `task` (schema exato, `Mutating: true`) apenas no agente principal, com `Execute` ligado a `RunSync` devolvendo o texto final do subagente como tool result. Os critérios de sucesso da task foram atendidos: `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam com 182 testes (179 pré-existentes preservados + 3 novos), o teste mandatório prova a delegação completa com `EventTurnDone` no fim, e o teste de invisibilidade prova que o subagente não vê `task` nas tools. Sem problemas bloqueantes. **APROVADO**.
