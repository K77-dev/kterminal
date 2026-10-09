# 12 — Squad mode (loop de engenharia multi-agente)

## Contexto

kterminal é um terminal agêntico em Go onde o Jev escolhe o LLM a cada chamada. O desenvolvimento de features segue o fluxo SDD kspec (brief → PRD → techspec → tasks → implement) via skills — ideal para features novas, pesado para spikes, refactors e bugs difíceis. Falta um segundo modo de trabalho: um loop agêntico onde um maestro convoca agentes de disciplina (arquiteto, backend, frontend, banco, ux, qa, teste) que deliberam entre si seguindo as rules, convergem num plano, e a execução desce para a maquinaria de subagentes existente.

Arquitetura relevante:

- `internal/agent/agent.go` — `newSubagent()` já é a fábrica de participantes (compartilha `LLM`/`Router`/`Session`, `messages` próprios); `task` é síncrono, depth limitado a 1; `decide()` roda a cada step com `state` livre.
- `internal/agent/prompt.go` — `buildSystemPrompt()` injeta rules (bundle-all) + skill ativa; `activeRules()` lê do `kspec.Store`.
- `internal/kspec/` — loader de skills/rules com precedência projeto-first (`.agents/` vence embutido); parser de frontmatter existe (`frontmatter.go`); `.agents/agents/*/AGENT.md` é o formato de persona dos 3 runners, mas não é lido pelo Go.
- `internal/router/` — `Router.Route(ctx, state, candidates)`; HeuristicRouter já matchea keywords de papel; `Model.Tags` existe no catálogo e não é consumido por nada.
- `internal/session/` — JSONL com `depth` (sem campo de agente); snapshot carrega `Messages` + `Skill` (precedente de estado de modo persistido).
- `internal/tui/` — `askWizard` é o widget modal navegável mais completo; popup de comandos é o modelo de lista; `busy` é flag única; eventos `Depth > 0` renderizam aninhados.

## Objetivo

Comando `/mode` que alterna entre o fluxo SDD (comportamento atual, default) e o **squad mode**: o agente principal vira maestro, anuncia a mesa (papéis, plano, orçamento), convoca personas via tool `task` — cada uma com system prompt da disciplina, subset de rules, roteamento Jev por papel — sintetiza a conversa até o critério de saída e entrega o plano para execução.

## Especificação

1. **Seletor `/mode`**: popup na TUI (padrão do popup de comandos / `askWizard`) com duas opções — `sdd` (default, fluxo de skills atual) e `squad`. Modo ativo aparece no hint bar, persiste no snapshot (precedente: `Skill`) e é restaurado no resume. Default configurável no `config.toml`.
2. **Maestro prompt-driven**: ao ativar squad, o agente principal recebe o prompt de maestro (asset embutido, estilo skill): convocar personas relevantes via `task`, passar o contexto acumulado da mesa, sintetizar contribuições, declarar convergência. Sem novo loop em Go.
3. **Personas**: `.agents/agents/<nome>/AGENT.md` no projeto vence; 7 defaults embutidas via `go:embed` (arquiteto, backend, frontend, banco, ux, qa, teste). Frontmatter estruturado: `name`, `discipline`, `model-tags` (ex.: `reasoning`, `fast`), `rules` (quais rules injetar), além do corpo com o prompt do papel. Loader novo no `kspec.Store` (estender o parser de frontmatter existente).
4. **Convocação dinâmica**: o maestro monta a mesa por relevância ao pedido — num projeto Go, frontend/ux não são convocados. Cada convocation = subagente com system prompt = persona + subset de rules da disciplina.
5. **Subset de rules**: interpretar o frontmatter (`paths`/`description`) das rules para mapear rule → disciplina; injetar só o subset no prompt da persona (fim do bundle-all para subagentes de squad). Rules Go mínimas (build/vet/test/sem comentários) como parte do dogfooding.
6. **Roteamento por papel**: papel entra no `state` do `decide()` da persona; `Model.Tags` passa a integrar `Model.Criteria()` enviado ao Jev (ex.: arquiteto → tag `reasoning`, qa → tag `fast`). Pin manual por papel como override.
7. **Kickoff e tetos**: antes da primeira convocation, o maestro anuncia mesa proposta, plano de rodadas e orçamento estimado. Limites no `config.toml` (`max_convocations`, `token_budget`) com override no kickoff. Fim da mesa: critério de saída declarado no kickoff, teto estourado, ou interrupção do usuário.
8. **Fila do usuário**: mensagens digitadas durante a mesa são enfileiradas e consumidas pelo maestro entre convocações (gancho no `runLoop`, que já verifica ctx a cada iteração). Esc continua abortando o turno inteiro.
9. **Eventos e transcript**: campo `Agent string` (aditivo, `omitempty`) em `agent.Event` e `session.Event`. TUI renderiza cada contribuição com rótulo `▸ <papel>:` e cor por disciplina (paleta do tema). Transcript JSONL registra o campo para a conversa completa.
10. **Ponte SDD**: a mesa lê/grav artefatos `spec/` via tools fs quando existirem. Após convergência, o plano desce para execução via `task` (maquinaria existente).

## Critérios de aceitação

- `/mode` oferece `sdd` (default) e `squad`; squad ativa o maestro; modo persiste no resume.
- Maestro anuncia mesa + orçamento antes de convocar; convoca só papéis relevantes ao pedido.
- Persona convocada recebe system prompt da disciplina + subset de rules; decisão de rota usa o papel no `state` e `Tags` no critério do Jev.
- Contribuições aparecem como `▸ <papel>:` com cor da disciplina; JSONL registra `agent`.
- Mensagem do usuário enfileirada mid-mesa é consumida entre convocações.
- Mesa encerra por critério de saída, teto ou esc; o plano segue para execução via `task`.
- Persona definida no projeto sobrescreve a embutida de mesmo nome.
- Dogfooding: a primeira mesa real (arquiteto + backend + qa) resolve um problema do próprio kterminal usando as rules Go.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: mock de gateway — persona com `model-tags: [reasoning]` roteia para modelo diferente do maestro; subset de rules injetado no system prompt da persona (e não o bundle); fila de mensagens drenada entre steps; snapshot/restauração do modo; campo `agent` no JSONL; kickoff anuncia orçamento antes da primeira convocation.

## Restrições

- v1 estritamente sequencial — paralelismo read-only (convocações sem tools de escrita) é fast-follow, junto com a UI de progresso múltiplo prevista em `10-subagentes.md`.
- Fora de escopo: pipeline SDD gerenciado (`/mode sdd` automatizado), enforcement hard de rules, personas editando código simultaneamente.
- Go 1.27, módulo `kterminal`. Sem comentários no código. Código em inglês, specs em pt-BR. Assets via `go:embed`. Tools registradas no construtor do `Registry`.
