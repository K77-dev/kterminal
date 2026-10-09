# Tarefa 3.0: Catalog tags + Agent core — `Event.Agent`, `buildState` por papel, ativação de modo, fila de mensagens, acumuladores de turno, wiring no `main.go`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Maestro prompt-driven
- REQ-004 — Convocação dinâmica e subset de rules
- REQ-005 — Roteamento por papel e pin manual
- REQ-007 — Fila de mensagens do usuário
- REQ-008 — Eventos, transcript e identidade visual por papel

## Dependências

- 1.0 (pacote `internal/squad` — `Store`, `Persona`, `MaestroPrompt`)
- 2.0 (config `[squad]` — defaults, pins, `default_mode`)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Modificar o núcleo do agente para suportar squad mode: (1) `Event.Agent` para rotular contribuições por papel, (2) `buildState` por papel com model-tags, (3) ativação de modo (system prompt do maestro), (4) fila de mensagens do usuário drenada entre steps, (5) acumuladores de convocations e tokens do turno, (6) `newSubagent` aceita system prompt de persona, pin de modelo e registry read-only, (7) `Criteria()` do catalog inclui tags, (8) wiring no `main.go`.

## Conformidade com Skills Padrões

- Go 1.27, sem comentários, eventos via canal
- Reuso de `newSubagent()`, `decide()` e da tool `task` — nenhum novo loop agêntico em Go
- `strings.Builder` por ponteiro onde aplicável
- Assets via `go:embed` (prompt de maestro vem do `squad.Store`)

## Requisitos

- `agent.Event` ganha campo `Agent string` (aditivo, não serializado — TUI lê direto)
- `buildState` do subagente persona inclui: `You are the <discipline> persona "<name>" in a engineering squad. Preferred model tags: <tags>.`
- `catalog.Model.Criteria()` termina com `Tags: <tags>.` integrando as tags à decisão do Jev
- `ActivateMode(mode string) error` aplica o system prompt do maestro quando `mode == "squad"`
- `Mode() string` retorna o modo ativo
- `EnqueueUserMessage(text string)` appenda em slice protegido pelo mutex existente; drenada no início de cada step do `runLoop` do maestro (depth 0, modo squad)
- Acumuladores de turno: contador de convocations e soma de tokens do turno (lê `Usage` do `ChatStream`)
- `newSubagent` aceita system prompt de persona, `SetPinned(cfg.Squad.Pins[persona])` quando houver pin, registry read-only e `Agent = persona` nos eventos emitidos
- `main.go` carrega `squad.Load()` e passa ao agent

## Subtarefas

- [ ] 3.1 Adicionar campo `Agent string` em `agent.Event` (aditivo)
- [ ] 3.2 Modificar `catalog.Model.Criteria()` para incluir `Tags: <tags>.` no final
- [ ] 3.3 Implementar `ActivateMode(mode string) error` e `Mode() string` em `agent.go` — aplica system prompt do maestro (`squad.Store.MaestroPrompt()`) quando `squad`
- [ ] 3.4 Modificar `buildState` para incluir papel e model-tags quando o subagente é uma persona
- [ ] 3.5 Implementar `EnqueueUserMessage(text string)` — fila protegida pelo mutex existente; drenar no início de cada step do `runLoop` (depth 0, modo squad), injetando como `llm.Message{Role: "user"}` e `session.Event{Type: "user"}`
- [ ] 3.6 Adicionar acumuladores de turno: `turnConvocations int` e `turnTokens int64` — incrementados a cada convocation e a cada `ChatStream` result; reset no início do turno
- [ ] 3.7 Modificar `newSubagent` para aceitar system prompt de persona, pin de modelo e registry read-only; setar `Agent = persona` nos eventos emitidos pelo subagente
- [ ] 3.8 Wiring no `main.go`: carregar `squad.Load()`, passar ao agent, aplicar `cfg.Squad.DefaultMode` na abertura

## Detalhes de Implementação

Consulte "Design de Implementação → Interfaces Principais" e "Pontos de Integração" na `techspec.md`:

- `ActivateMode` / `Mode` / `EnqueueUserMessage` / `RegisterKickoff` — assinaturas na techspec
- Roteamento por papel: `buildState` inclui `You are the <discipline> persona "<name>"...`; `Criteria()` inclui tags; sem pin, Jev decide com fallback heurístico
- Fila: `EnqueueUserMessage` appenda em slice protegido pelo mutex existente; no início de cada step do `runLoop` (depth 0, modo squad), a fila é drenada e injetada como `llm.Message{Role: "user"}` no contexto
- Acumuladores: `turnConvocations` incrementado a cada `task(persona=...)`; `turnTokens` soma `result.Usage.PromptTokens + result.Usage.CompletionTokens` de todas as chamadas LLM do turno
- `newSubagent` hoje (`agent.go:261`) cria subagente com `subagentSystemPrompt` fixo. Modificar para aceitar system prompt custom (persona), pin e registry

## Critérios de Sucesso

- `Event.Agent` carrega o nome da persona em eventos de subagente persona
- `Criteria()` inclui tags do modelo
- `ActivateMode("squad")` aplica o prompt de maestro; `Mode()` retorna `"squad"`
- `ActivateMode("sdd")` restaura o prompt base; `Mode()` retorna `"sdd"`
- Mensagem enfileirada mid-mesa aparece como user message entre steps sem interromper convocation em curso
- Acumuladores resetam a cada turno e contam corretamente
- `newSubagent` com persona seta `Agent` nos eventos e usa o system prompt da persona
- `main.go` carrega `squad.Store` e aplica `default_mode` do config

## Testes da Tarefa

- [ ] Testes de unidade:
  - `Criteria()` inclui tags (catalog_test.go)
  - `ActivateMode("squad")` / `Mode()` (agent_test.go)
  - `EnqueueUserMessage` — fila drenada entre steps (agent_test.go com mock gateway)
  - Acumuladores de turno resetam e contam (agent_test.go)
- [ ] Testes de integração (seguir padrão de mocks de `agent_test.go` — `mockGateway`, `mockJev`, `stateCapturingJev`):
  - Persona com `model-tags: [reasoning]` roteia para modelo diferente do maestro (verificar `state` capturado contém o papel e as tags)
  - Pin no config fixa o modelo do papel (route event com `Router: "pin"`)
  - Fila: mensagem enviada mid-mesa aparece como user message entre steps
  - Resume: snapshot com `Mode: "squad"` restaura o modo e o prompt de maestro

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` (modificado — Event.Agent, fila, kickoff/tetos, newSubagent com persona, buildState por papel)
- `internal/agent/prompt.go` (modificado — system prompt de maestro por modo)
- `internal/agent/agent_test.go` (modificado)
- `internal/catalog/catalog.go` (modificado — Criteria() com tags)
- `internal/catalog/catalog_test.go` (modificado)
- `main.go` (modificado — wiring do squad.Store)
