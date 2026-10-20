# 18 — Prompt enxuto e contexto de projeto

## Contexto

Tarefas simples no kterminal ("rode o projeto") demoram demais e leem muitos arquivos. Medição em sessões reais aponta três causas:

1. **Zero contexto do projeto no system prompt** — o kterminal nunca carrega `AGENTS.md`/`CLAUDE.md`/`README.md` (nenhuma referência no código Go). Para um "rode o projeto", o modelo parte cego e descobre a estrutura via glob/read exploratório: turn com 38 tool calls quase todos de leitura; turn medido de 573s com 43 tool calls. Cada step LLM custa ~6-8s (mediana medida) — step count é o multiplicador dominante.
2. **Rules infladas** — `activeRules()` injeta todos os `.agents/rules/*.md` no system prompt. Neste repo: 82KB (~21k tokens) de rules Angular/Spring/React/Java num projeto Go, pagos em cada step (TTFT mais lento, custo maior, atenção diluída).
3. **Convenção que incentiva steps extras** — "Verify your work with the project's build, lint and test commands" faz o modelo rodar build/teste até em requests não-mutantes.

Fatores menores: decisão Jev por step (mediana 0,3s, máx 1,9s — dependência de rede por step) e `ListModels` (timeout 15s) no primeiro turn de cada sessão.

Descoberta que habilita o filtro de rules: as rules do kspec já declaram `paths:` no frontmatter (ex.: `typescript.md` → `**/*.ts`), e o repo já tem um matcher de glob `**` em `internal/tools/fs.go`.

Arquitetura relevante:

- `internal/agent/prompt.go` — `baseSystemPrompt`, `buildSystemPrompt`, `activeRules`, `rulesSection`.
- `internal/agent/agent.go` — `runLoop` (decide por step), `ensureCandidates` (ListModels por sessão), `newSubagent`.
- `internal/kspec/` — `Rule`, `readRules`, frontmatter (`disciplines`).
- `internal/tools/fs.go` — `expandGlob`/`matchGlobSegments` (matcher `**`).
- `internal/config/config.go` — seção `[llm]` com durations.
- `main.go` — wiring config → agent; `telemetryPath()` (XDG data).

## Objetivo

Tarefa simples vira 1-2 steps: o modelo recebe o contexto do projeto no system prompt, carrega só as rules aderentes ao projeto, e age direto em requests operacionais. System prompt neste repo cai de ~83KB para ≤ ~12KB. Decisão de rota é tomada uma vez por turn; lista de modelos do gateway é cacheada em disco.

## Especificação

1. **Pacote `internal/globx`** — extração do matcher `**` de `tools/fs.go`: `Match(pattern, path)` e `MatchAny(patterns, path)` sobre paths slash-separated. `expandGlob` passa a usar `globx.Match`.
2. **Pacote `internal/project`** —
   - `Context(dir)`: primeiro entre `AGENTS.md` → `CLAUDE.md` → `README.md`, cap 16KB com marcador de truncamento; retorna nome + conteúdo.
   - `Files(dir)`: walk limitado (skip `.git`, `node_modules`, `vendor`, `graphify-out`, `dist`, `build`, `target`; cap 10k arquivos), paths relativos slash-separated.
   - `MatchesAny(patterns, files)`: via globx.
3. **kspec: `paths` nas rules** — `ruleFrontmatter` ganha `Paths []string`; `Rule.Paths` exposto; `readRules` popula (rules sem frontmatter/paths = genéricas).
4. **Fix A — contexto no prompt** — `Agent.AttachProject(dir)` carrega contexto + files e injeta seção `# Project context (<file>)` no `buildSystemPrompt` (entre base e rules); recarregado no início de cada turn (depth 0); subagentes `task` herdam o contexto no system prompt. Personas do squad não recebem (o maestro passa guidance).
5. **Fix B — rules filtradas + cap** — `activeRules()` mantém apenas rules sem `paths` (genéricas) ou com algum pattern batendo em `Files(dir)`; `rulesSection` respeita orçamento de 12KB e lista rules omitidas por tamanho.
6. **Fix C — convenção de eficiência** — nova convenção no `baseSystemPrompt`: requests operacionais simples (run/build/test) usam o contexto do projeto e agem direto no menor número de steps; sem verificação build/lint/teste em requests não-mutantes.
7. **Fix D — cache de decisão por turn** — `decide()` reutiliza a decisão do step 0 nos steps > 0 do mesmo turn (reason `turn cache`), desde que o modelo não esteja bloqueado pelo model health e seja compatível com visão; a decisão cacheada acompanha o modelo efetivo após fallbacks de rate-limit/unavailable. Pin continua short-circuitando. Evento `route` continua emitido por step.
8. **Fix E — cache de `ListModels`** — `internal/llm/cache.go`: load/save de `{ts, models}` em JSON (0600) no XDG data (`models-cache.json`). `ensureCandidates` usa cache fresco (TTL `[llm] models_cache_ttl`, default 15m, env `KTERMINAL_LLM_MODELS_CACHE_TTL`, `0` desativa); em erro do gateway, cache stale vira fallback silencioso; fetch bem-sucedido persiste o cache.
9. **Wiring** — `main.go`: `AttachProject(cwd)` após `AttachKspec`; `SetModelsCache(dataPath("models-cache.json"), ttl)`; `telemetryPath()` refatorado para `dataPath(name)` compartilhado.

## Critérios de aceitação

- Com `AGENTS.md` presente, o system prompt contém `# Project context (AGENTS.md)` com o conteúdo (cap 16KB); fallback `CLAUDE.md` → `README.md`; sem nenhum, sem seção.
- Neste repo (Go), rules `typescript`, `angular`, `react`, `java`, `spring-boot`, `hono`, `database`, `logging`, `java-tests`, `tests` ficam de fora; `code-standards`, `architecture-ddd`, `graphify` permanecem.
- Rules sem `paths` sempre permanecem; projeto TS mantém as rules TS.
- Orçamento de rules: conteúdo além de 12KB é omitido com nota nomeando as rules omitidas.
- Turn com 2+ steps chama Jev uma única vez; steps seguintes emitem `route` com reason `turn cache`; modelo bloqueado ou incompatível com imagens força nova rota.
- `ensureCandidates` com cache fresco não chama o gateway; com gateway fora do ar e cache stale, usa o cache sem erro; fetch bem-sucedido escreve o cache.
- Subagente `task` recebe o contexto do projeto no system prompt.
- `models_cache_ttl = "0s"` desativa cache (sempre fetch, sem leitura/escrita).

## Verificação

```bash
go build ./... && go vet ./... && gofmt -l . && go test ./...
go test ./internal/globx/ ./internal/project/ -v
go test ./internal/agent/ -run 'Project|Rules|TurnCache|ModelsCache' -v
go test ./internal/config/ -run ModelsCache -v
```

## Restrições

- Sem comentários no código; eventos via canal; walk de arquivos limitado e barato (1x por turn).
- O filtro de rules nunca remove rules genéricas (sem `paths`) — só rules claramente não aderentes.
- O cache de decisão é por turn e por agente (subagentes têm instância própria); nunca cruza turns.
- O cache de modelos nunca retorna lista vazia como válida (cache vazio/corrompido = sem cache).
- Personas do squad não recebem o contexto do projeto (o maestro concentra os fatos no guidance).
