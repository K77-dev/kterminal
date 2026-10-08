# Tarefa 2.0: `depth`, `newSubagent` e `RunSync` com re-emissão e limites

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Subagente isolado
- REQ-003 — Eventos aninhados
- REQ-004 — Limites e timeout
- REQ-005 — Cancelamento compartilhado

## Dependências

- 1.0 (`runLoop` extraído e reusável)

## Estimativa

- **Tamanho**: GG
- **Horas estimadas**: 8-12h

## Visão Geral

O coração da feature: o `Agent` ganha campo `depth` e a fábrica `newSubagent(description, guidance)` — infraestrutura compartilhada por referência (LLM, Router, Fallback, Catalog, Session, Telemetry, Confirm) mas estado próprio: `messages` zeradas iniciadas com prompt de sistema específico de subagente + description + guidance, canal `Events` próprio (buffer 512), `depth = parent+1` e registry **sem** a tool `task` (limite de profundidade estrutural — impossível de burlar por prompt injection). `RunSync(ctx, description, guidance)` roda o loop do subagente **sincronamente** na goroutine do passo (sem `go` — requisito não negociável da v1), deriva ctx com timeout de 5 minutos, drena o canal do subagente re-emite os eventos no canal do principal com `Depth: 1` e `ParentTool: "task"`, grava-os no transcript compartilhado com `depth`, e devolve o texto final. Estouro de 10 steps ou timeout devolve `error: ...` **como resultado, sem error** — o principal segue vivo. Esc cancela o turno inteiro (ctx derivado do turno do principal).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (delegação E2E, limites, Esc), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; eventos via canal (buffer 512, emit não-bloqueante nos dois níveis); sem goroutines paralelas na v1.
</skills>

<requirements>
- Constantes: `subagentMaxSteps = 10`, `subagentTimeout = 5 * time.Minute` (timeout encurtável/injetável para teste).
- Campo `depth int` no `Agent`; `Event` ganha `Depth int` e `ParentTool string`; `session.Event` ganha `Depth int` (`json:"depth,omitempty"`).
- `newSubagent`: compartilha LLM/Router/Fallback/Catalog/Session/Telemetry/Confirm por referência; próprio `Events` (buffer 512), `messages = [system, user]`, `depth = parent+1`, `Tools = tools.NewRegistry()` sem `task`, `pinned` vazio.
- Mensagens iniciais: system "You are a subagent handling a focused subtask for a parent agent. Be concise; return only the final result." + user `description + "\n\n" + guidance` (guidance omitida se vazia).
- `RunSync`: ctx derivado com timeout 5min; loop síncrono via `runLoop`; drena o canal do subagente e re-emite no canal do principal com `Depth`/`ParentTool`; grava `session.Event{..., Depth}` no transcript compartilhado; fecha o canal do subagente no fim.
- Steps esgotados (10) → resultado `"error: subtask exceeded max steps (10)"` sem error; timeout → `"error: subtask timed out after 5m"` idem — o principal segue vivo.
- Cancelamento: ctx do subagente deriva do ctx do turno — `Cancel()`/Esc mata tudo; `EventTurnAborted` do principal fecha o turno.
- Limite de 10 steps no subagente (metade do principal).
</requirements>

## Subtarefas

- [ ] 2.1 Adicionar `depth` ao `Agent` e os campos `Depth`/`ParentTool` ao `agent.Event` e `Depth` ao `session.Event`
- [ ] 2.2 Implementar `newSubagent` (infra compartilhada, estado próprio, registry sem `task`, mensagens iniciais)
- [ ] 2.3 Implementar `RunSync`: timeout derivado, loop síncrono, drenagem com re-emissão enriquecida, gravação no transcript
- [ ] 2.4 Implementar os limites: steps (10) e timeout (5min) devolvendo erro como resultado, sem derrubar o principal
- [ ] 2.5 Escrever os testes 2, 4-9 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Snippets de `AttachTaskTool`/`newSubagent`/`RunSync` e constantes: techspec, seção **Design de Implementação → Interfaces Principais**.
- Decisões "sequencial na v1", "limite de profundidade estrutural", "re-emissão com Depth", "erro como resultado": techspec, seção **Considerações Técnicas → Decisões Principais** (itens 1-4).
- Interações com techspecs 001 (cancelamento via ctx), 003 (compação no subagente) e 006 (telemetria compartilhada): techspec, seção **Pontos de Integração** e **Sequenciamento → Dependências Técnicas**.
- Risco de deadlock de canais (emit não-bloqueante nos dois níveis): techspec, seção **Considerações Técnicas → Riscos Conhecidos**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: a chamada do subagente usa modelo diferente do passo do principal (roteamento independente — REQ-002).
- Teste prova: eventos do subagente chegam ao canal do principal com `Depth: 1` e `ParentTool: "task"`; eventos do principal com `Depth: 0` (REQ-003).
- Testes provam: 11 tool calls → `error: subtask exceeded max steps (10)` com o principal vivo; resposta bloqueada → `error: subtask timed out after 5m` (timeout encurtável no teste) com o principal vivo (REQ-004).
- Teste prova: `Cancel()` durante `task` → subagente morto, `EventTurnAborted` do principal, sem goroutine vazando (REQ-005).
- Teste prova: JSONL contém eventos do subagente com `"depth": 1` e do principal sem o campo.
- Teste prova: `Confirm = true` → tool mutante do subagente emite `EventConfirm` com `Depth: 1` (herança).

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`, mock gateway com respostas sequenciais + mock Jev — techspec **Abordagem de Testes → Testes Unidade**):
  - `TestSubagentRoutesIndependently` — modelo do subagente diferente do principal (item 2).
  - `TestNestedEventsCarryDepth` — `Depth: 1`/`ParentTool: "task"`; principal `Depth: 0` (item 4).
  - `TestSubagentStepsLimit` — 11 tools → erro como resultado; principal vivo (item 5).
  - `TestSubagentTimeout` — resposta bloqueada → erro de timeout (constante injetável); principal vivo (item 6).
  - `TestEscCancelsSubagent` — `Cancel()` mata o subagente; turno seguinte completa sem vazamento (item 7).
  - `TestSubagentWritesTranscriptWithDepth` — JSONL com `"depth": 1` (item 8).
  - `TestSubagentConfirmInherited` — `EventConfirm` com `Depth: 1` (item 9).
- [ ] Testes de integração — cobertos pelos testes de agent (principal→subagente→gateway→eventos→transcript com mocks).
- [ ] Testes E2E — deferidos para `kspec-qa` (delegação visível indentada; limites; Esc cancela tudo).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — `depth`, `newSubagent`, `RunSync`, re-emissão, limites, campos no `Event`
- `internal/agent/agent_test.go` — 7 cenários: roteamento, depth, limites, timeout, cancelamento, transcript, confirm
- `internal/session/session.go` — campo `Depth` no `Event`
