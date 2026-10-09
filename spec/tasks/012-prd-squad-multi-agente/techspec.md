# Tech Spec — Squad mode (loop de engenharia multi-agente)

## Requisitos Atendidos

- REQ-001 — Seletor de modo `/mode`
- REQ-002 — Maestro prompt-driven
- REQ-003 — Catálogo de personas
- REQ-004 — Convocação dinâmica e subset de rules
- REQ-005 — Roteamento por papel e pin manual
- REQ-006 — Kickoff, tetos e critério de saída
- REQ-007 — Fila de mensagens do usuário
- REQ-008 — Eventos, transcript e identidade visual por papel
- REQ-009 — Ponte SDD e execução do plano

## Resumo Executivo

O squad mode reusa o loop agêntico existente: o agente raiz vira **maestro** por prompt (asset embutido), convoca personas como subagentes via tool `task` estendida com argumento `persona`, e a execução do plano convergido desce pela mesma maquinaria. Nenhum novo loop em Go. O código novo vive num pacote `internal/squad` (loader de personas, prompt de maestro, kickoff e tetos), seguindo o precedente de `internal/kspec` (embed + frontmatter + override de projeto).

Decisões-chave alinhadas: (1) kickoff via tool estruturada `squad_kickoff`, validada e enforceada mecanicamente pelo Go — tetos de convocações e tokens nunca dependem do LLM se policiar; (2) teto de tokens medido pelo usage real somado de todas as chamadas LLM do turno; (3) personas deliberam com registry somente-leitura; (4) fila de mensagens do usuário no Agent, drenada entre steps do maestro; (5) pin por papel reutiliza o campo `pinned` existente do subagente.

## Arquitetura do Sistema

### Visão Geral dos Componentes

**Novos:**

- `internal/squad` — pacote do domínio squad: `Store` (loader de personas com `go:embed` de 7 personas default + override de `.agents/agents/<nome>/AGENT.md`), prompt de maestro embutido, tipos `Kickoff`/`Budget` e validação de tetos.
- `tools.NewReadOnlyRegistry()` — registry com read/glob/grep apenas, usado em convocations de persona.
- Tool `squad_kickoff` — registra a mesa proposta (papéis, tetos, critério de saída) no Agent raiz antes de qualquer convocation.
- Assets embutidos: `internal/squad/embed/personas/<nome>/AGENT.md` (7) e `internal/squad/embed/maestro.md`.

**Modificados:**

- `internal/agent` — campo `Agent string` em `Event`; ativação de modo (system prompt do maestro); fila de mensagens (`EnqueueUserMessage`) drenada entre steps do `runLoop`; contagem de convocations e acumulação de tokens do turno; `newSubagent` aceita system prompt de persona, pin de modelo e registry read-only; `buildState` inclui papel e model-tags.
- `internal/tools` — tool `task` ganha argumento opcional `persona`; registro da tool `squad_kickoff`.
- `internal/catalog` — `Model.Criteria()` passa a incluir `Tags`.
- `internal/config` — seção `[squad]` (`default_mode`, `max_convocations`, `token_budget`, `[squad.pins]`).
- `internal/session` — campo `Agent` em `Event` (`agent,omitempty`); campo `Mode` em `Snapshot`.
- `internal/tui` — popup `/mode` (estado dedicado, navegação por setas), modo ativo no hint bar, render `▸ <papel>:` com cor por disciplina, enfileiramento de mensagens quando busy em squad.
- `.agents/rules/` — rules Go mínimas com frontmatter declarando disciplinas (dogfooding).

**Fluxo de dados**: `/mode` → `squad` → pedido do usuário → system prompt do maestro aplicado → maestro chama `squad_kickoff` (papéis, tetos, critério de saída) → Go valida contra config e anuncia → maestro chama `task(persona=...)` por rodada → subagente persona (system prompt = persona + subset de rules; pin ou rota por papel; tools read-only) → contribuição rotulada (`Agent` no evento) → maestro sintetiza entre rodadas, drena fila do usuário → convergência → plano executado via `task` (registry completo) → `turn_done`.

## Design de Implementação

### Interfaces Principais

```go
type Persona struct {
    Name       string
    Discipline string
    ModelTags  []string
    Rules      []string
    Prompt     string
}

type Store struct{}
func Load() *Store
func (s *Store) List() []Persona
func (s *Store) Resolve(name string) (Persona, error)
func (s *Store) MaestroPrompt() string
```

```go
type Kickoff struct {
    Roles           []string
    MaxConvocations int
    TokenBudget     int64
    ExitCriterion   string
}
```

```go
func (a *Agent) ActivateMode(mode string) error
func (a *Agent) Mode() string
func (a *Agent) EnqueueUserMessage(text string)
func (a *Agent) RegisterKickoff(k Kickoff) error
```

Convocation — extensão da tool `task` (schema): propriedade opcional `persona` (string). No `Execute`, quando `persona` está presente: resolver a persona no `squad.Store`, montar system prompt (prompt da persona + rules do subset), criar subagente com `SetPinned(cfg.Squad.Pins[persona])` quando houver pin, registry read-only e `Agent = persona` nos eventos emitidos.

Kickoff — tool `squad_kickoff` com args `roles []string`, `max_convocations int`, `token_budget int`, `exit_criterion string`. O Go valida: papéis existem no catálogo de personas; tetos ≤ defaults do config (override declarado no kickoff prevalece sobre o default, nunca o amplia); sem kickoff registrado, `task(persona=...)` falha com instrução para convocar o kickoff primeiro.

Fila — `EnqueueUserMessage` appenda em slice protegido pelo mutex existente; no início de cada step do `runLoop` do maestro (depth 0, modo squad), a fila é drenada e injetada como `llm.Message{Role: "user"}` no contexto, com `session.Event{Type: "user"}` correspondente. Convocation em curso nunca é interrompida.

Roteamento por papel — `buildState` do subagente persona inclui: `You are the <discipline> persona "<name>" in a engineering squad. Preferred model tags: <tags>.` O `Criteria()` do catálogo passa a terminar com `Tags: reasoning, code.`, integrando as tags à decisão do Jev. Sem pin, o Jev decide com fallback heurístico (keywords existentes já cobrem `architect`, `debug` etc.).

### Modelos de Dados

Frontmatter da persona (`.agents/agents/<nome>/AGENT.md` e assets embutidos):

```yaml
---
name: architect
discipline: architecture
model-tags: [reasoning]
rules: [go, code-standards]
---
<prompt do papel>
```

Frontmatter de rule (mapeamento rule → disciplina; rules sem frontmatter ficam fora do subset de persona):

```yaml
---
disciplines: [backend, architecture]
---
```

Config TOML:

```toml
[squad]
default_mode = "sdd"
max_convocations = 8
token_budget = 200000

[squad.pins]
architect = "glm-5.3"
```

`session.Event` ganha `Agent string \`json:"agent,omitempty"\``; `Snapshot` ganha `Mode string`. `agent.Event` ganha `Agent string` (aditivo, não serializado — TUI lê direto).

### Endpoints de API

Não aplicável — funcionalidade integralmente local à TUI.

## Pontos de Integração

- **Jev (Typesafe systemone)**: `state` da rota agora carrega papel e model-tags; `criteria` inclui tags. Sem mudança de contrato HTTP — mudanças apenas no conteúdo dos campos existentes. Modos de falha inalterados: erro do Jev → fallback heurístico; timeout 30s existente.
- **Gateway LLM**: nenhuma mudança de contrato; o acumulador de tokens do turno lê o `Usage` já retornado pelo `ChatStream`.

## Verificações Técnicas

### Segurança

- Personas deliberam com registry somente-leitura: sem `write`, `edit`, `bash`, `kspec_bootstrap`. Escrita só na fase de execução pós-convergência, via `task` com registry completo (sujeita ao `Confirm` existente quando ativo).
- Frontmatter de persona é parseado com o `splitFrontmatter` existente (delimitadores estritos); nomes de persona validados por `validName` (sem path traversal).
- Tetos validados no Go: `max_convocations` e `token_budget` do kickoff são clampados aos máximos do config; valores ausentes caem nos defaults (8 / 200k).

### Arquitetura

- Nenhum novo loop agêntico: maestro é o `runLoop` existente com system prompt diferente; convocations são `RunSync`/`newSubagent` existentes.
- Limite de steps: maestro em squad usa `skillMaxSteps` (100) — mesa de até 8 convocações com sínteses cabe no limite existente.
- Ponto de falha: maestro descumprir o protocolo (convocar sem kickoff). Mitigado por enforcement mecânico (tool falha) e pelo prompt do maestro.
- Compaction de contexto do maestro durante mesa longa: comportamento existente preservado (threshold 0.7); contribuições truncadas não perdem o rótulo `agent` no transcript.

### Infraestrutura

- Binário auto-contido: personas e prompt de maestro via `go:embed` (precedentes: `models.yaml`, `markdown.json`, `internal/kspec/embed`).
- Sem dependências externas novas; `gopkg.in/yaml.v3` e `BurntSushi/toml` já usadas.
- Rollback: feature isolada por modo; `/mode sdd` (default) restaura o fluxo atual sem código condicional residual.

## Abordagem de Testes

### Testes Unidade

- `internal/squad`: parse de frontmatter (name/discipline/model-tags/rules), override de persona de projeto sobre embutida, persona inédita de projeto, rules subset por disciplina, `MaestroPrompt` não vazio, validação de kickoff (clamp de tetos, papéis inexistentes).
- `internal/config`: defaults (8/200k/`sdd`), pins parseados, `default_mode` inválido rejeitado.
- `internal/catalog`: `Criteria()` inclui tags.
- `internal/tools`: `NewReadOnlyRegistry` expõe apenas read/glob/grep; `task` com `persona` inexistente retorna erro claro.

### Testes de Integração

Seguir o padrão de mocks de `agent_test.go` (`mockGateway`, `mockJev`, `stateCapturingJev`):

- Mesa completa com mock gateway: kickoff → 2 convocations → convergência; eventos carregam `Agent`; JSONL registra `agent`.
- Persona com `model-tags: [reasoning]` roteia para modelo diferente do maestro (verificar `state` capturado contém o papel e as tags).
- Pin no config fixa o modelo do papel (route event com `Router: "pin"`).
- Teto de convocations: 9ª convocation falha com sinal de fim; maestro encerra com plano parcial.
- Teto de tokens: gateway reporta usage alto; convocation seguinte bloqueada.
- Fila: mensagem enviada mid-mesa aparece como user message entre steps; Esc aborta tudo (`turn_aborted`).
- Resume: snapshot com `Mode: "squad"` restaura o modo e o prompt de maestro.

### Testes de E2E

Sem TestSprite (aplicação TUI local). E2E = dogfooding documentado: primeira mesa real (arquiteto + backend + qa) resolvendo um problema do próprio kterminal com as rules Go, verificando: mesa anunciada antes do gasto, contribuições rotuladas por papel, convergência dentro dos tetos, fluxo SDD intacto após `/mode sdd`.

## Sequenciamento de Desenvolvimento

### Ordem de Construção

1. `internal/squad` — loader de personas, assets embutidos (7 personas + maestro), tipos e validação de kickoff. Fundação sem dependências do resto.
2. `internal/config` — seção `[squad]` com defaults e pins.
3. `internal/catalog` + `internal/agent` — tags no `Criteria()`, campo `Agent` em `Event`, `buildState` por papel, ativação de modo, fila de mensagens, acumuladores de turno.
4. `internal/tools` — `NewReadOnlyRegistry`, `task` com `persona`, tool `squad_kickoff`.
5. `internal/session` — campo `Agent`, `Mode` no snapshot, restore no resume.
6. `internal/tui` — popup `/mode`, hint bar, render `▸ <papel>:` com cor por disciplina, feedback de enfileiramento.
7. Rules Go mínimas com frontmatter + dogfooding da primeira mesa real.

### Dependências Técnicas

- Nenhuma externa bloqueante; Jev e gateway seguem com fallback heurístico quando indisponíveis.
- Passos 3–4 dependem do 1–2; o 5–6 dependem do 3; o 7 fecha o ciclo.

## Monitoramento e Observabilidade

### Error Tracking

Erros de mesa (persona inexistente, kickoff inválido, teto estourado) fluem pelo `EventError` existente e ficam no JSONL (`type: "error"`). Sem ferramenta externa (CLI local).

### Logging Estruturado

Transcript JSONL é o log: cada evento de persona carrega `agent` (papel), `depth`, `model`, `router`, `cost`, `tps` — rastreabilidade completa de quem disse o quê, com qual modelo e a quanto custou. O kickoff fica registrado como `tool_call` de `squad_kickoff` com os tetos nos args. Dados sensíveis (API keys) nunca aparecem no transcript — inalterado.

### Health Checks

Não aplicável (CLI). O `--doctor` existente pode listar personas carregadas e fonte (embedded/project), análogo ao que faz com skills.

### Métricas de Negócio

- Custo e tokens da mesa vs. budget declarado (derivável do JSONL: soma de `cost`/usage dos eventos com `depth ≥ 0` do turno).
- Convocações usadas vs. `max_convocations`.
- TPS por modelo já medido pelo `telemetry.Store` — permite avaliar se o roteamento por papel está acertando (reasoning lento e bom para arquiteto, fast para qa).

### Alertas

Não aplicável (CLI local). O equivalente em-app: linha de anúncio do kickoff mostra orçamento estimado; hint bar mostra custo acumulado da sessão — já existente.

## Considerações Técnicas

### Decisões Principais

- **Pacote `internal/squad` novo** em vez de estender `internal/kspec`: squad é domínio distinto (personas ≠ skills); o loader espelha o padrão do kspec sem acoplar os dois. Alternativa rejeitada: personas como asset do kspec aumentaria a superfície de um pacote já responsável por SDD.
- **`task` estendida com `persona`** em vez de tool nova: reusa `RunSync`/`newSubagent`/eventos aninhados; o maestro já conhece a tool. Alternativa rejeitada: tool `convene` duplicaria a maquinaria.
- **Kickoff via tool estruturada** em vez de parse de texto do stream: enforcement mecânico dos tetos, sem depender do LLM formatar corretamente. O anúncio visual vem do tool result + texto do maestro.
- **Usage real** para o teto de tokens em vez de estimativa chars/4: o gateway já reporta usage; estimativa subestimaria completion tokens de modelos verbosos.
- **Pin por papel via `pinned` existente**: `decide()` já dá precedência absoluta ao pin — zero código novo no caminho da decisão.
- **Fila no Agent** drenada entre steps: cumpre "consumida entre convocações" sem novo canal ou timer.

### Riscos Conhecidos

- **Maestro descumpre o protocolo** (pula kickoff, convoca persona irrelevante): mitigado por enforcement mecânico (tools falham) e pelo prompt; restante é aceito como qualidade de modelo.
- **`subagentTimeout` de 5min** pode ser curto para modelos reasoning em convocations densas: avaliar na primeira mesa real; se necessário, timeout próprio para convocations de persona.
- **Rules sem frontmatter** ficam fora do subset de persona: comportamento intencional (fim do bundle-all), mas requer escrever as rules Go mínimas com frontmatter desde o início — parte do dogfooding.
- **Compaction do maestro** pode sumarizar contribuições no meio da mesa: aceitável (o transcript preserva tudo); o prompt do maestro instrui a manter síntese própria do estado da mesa.

### Conformidade com Skills Padrões

`.agents/rules/architecture-ddd.md` orienta projetos TypeScript/Java com `src/modules/`; o kterminal é Go flat com `internal/` — prevalecem os padrões do código-base (AGENTS.md). A seção Bounded Context do template não se aplica. Não há tabela "Stack e skills recomendadas" no CLAUDE.md; skills de fluxo aplicáveis ao próximo passo: `kspec-tasks` (decomposição) e `kspec-implement` (execução). Rules Go do próprio código-base: sem comentários, eventos via canal, TUI com receivers por valor, `strings.Builder` por ponteiro, tools registradas no construtor, assets via `go:embed`.

### Arquivos relevantes e dependentes

**Novos:**
- `internal/squad/squad.go`, `internal/squad/frontmatter.go`, `internal/squad/embed/maestro.md`, `internal/squad/embed/personas/{architect,backend,frontend,database,ux,qa,test}/AGENT.md`, `internal/squad/squad_test.go`
- `.agents/rules/go.md` (rules Go mínimas com frontmatter)

**Modificados:**
- `internal/agent/agent.go` — Event.Agent, fila, kickoff/tetos, newSubagent com persona, buildState por papel (agent.go:58,261,592,658)
- `internal/agent/prompt.go` — system prompt de maestro por modo (prompt.go:107)
- `internal/tools/tools.go` — NewReadOnlyRegistry (tools.go:38)
- `internal/catalog/catalog.go` — Criteria() com tags (catalog.go:92)
- `internal/config/config.go` — seção [squad] (config.go:21)
- `internal/session/session.go` — Event.Agent, Snapshot.Mode (session.go:21,90)
- `internal/tui/tui.go` — /mode, hint bar, render por papel, fila (tui.go:616,1325)
- `internal/tui/theme.go` — cores por disciplina (theme.go:9)
- `main.go` — wiring do squad.Store
