# CURSOR.md

Guia para agentes de IA ao trabalhar com o código deste repositório — plataforma Cursor.

Este projeto é o **kterminal** — um coding agent de terminal (TUI agêntica) em Go, no estilo Claude Code/opencode. O diferencial: o roteador externo **Jev** (Typesafe) escolhe o LLM a cada chamada, pesando qualidade, custo e tokens/s reais medidos. O desenvolvimento do próprio kterminal segue o fluxo SDD do **kspec** (dogfooding).

> Para Claude Code, consulte `CLAUDE.md`. Para OpenAI Codex CLI, consulte `AGENTS.md`.

## Visão Geral

- **Linguagem**: Go 1.27, módulo `kterminal`, binário único auto-contido
- **TUI**: Charmbracelet (Bubble Tea, Lipgloss, Glamour, bubbles)
- **LLM**: cliente HTTP próprio, OpenAI-compatible, streaming
- **Roteamento**: Jev (Typesafe systemone, `api.typesafe.ai/v1/systemone`) com fallback heurístico
- **Config**: TOML (BurntSushi), XDG (`~/.config/kterminal/config.toml`)

## Estrutura do projeto

```
/                                # Raiz do kterminal
├── main.go                      # Entry point: flags, wiring, --doctor
├── main_test.go
├── internal/
│   ├── agent/                   # Loop agêntico, eventos, subagentes, compactação
│   ├── catalog/                 # Catálogo de modelos embutido (go:embed models.yaml)
│   ├── config/                  # Config TOML do usuário (XDG)
│   ├── jev/                     # Cliente do roteador Jev (Typesafe)
│   ├── llm/                     # Cliente OpenAI-compatible com streaming
│   ├── router/                  # Interface Router, JevRouter, HeuristicRouter
│   ├── session/                 # Transcripts JSONL + snapshots para resume
│   ├── telemetry/               # Store de TPS medido por modelo
│   ├── tools/                   # Registry + tools fs/bash/diff/diagnostics
│   └── tui/                     # TUI Bubble Tea, menções, tema, estilos
├── .docs/                       # Briefs de features (01-10)
├── spec/tasks/                  # Artefatos SDD (PRDs, techspecs, tasks, reviews)
├── .agents/                     # kspec: skills, agents, rules, templates (source of truth)
├── .claude/ .codex/ .cursor/    # Camadas de discovery do kspec (symlinks/derivados)
├── CLAUDE.md                    # Guia equivalente para Claude Code
├── AGENTS.md                    # Guia equivalente para Codex CLI
├── skills-lock.json             # Lockfile de skills de terceiros (mattpocock/skills)
├── go.mod / go.sum
└── kterminal                    # Binário compilado (não versionado)
```

## Comandos do projeto

```bash
go build ./...                 # Build
go run .                       # Rodar em desenvolvimento
go vet ./...                   # Análise estática
gofmt -l .                     # Formatação (não deve listar nenhum arquivo)
go test ./...                  # Testes
```

Verificação completa: `go build ./... && go vet ./... && gofmt -l . && go test ./...`

## Idioma

- **Código-fonte**: inglês (identificadores, mensagens de erro, textos da TUI)
- **Specs e documentação de projeto** (briefs `.docs/`, PRD, tech spec, tasks, reviews): português (Brasil)

## Fluxo SDD (dogfooding)

Toda feature nova do kterminal segue o fluxo kspec:

1. Brief da feature em `.docs/NN-<slug>.md` (Contexto / Objetivo / Especificação / Critérios de aceitação / Verificação / Restrições)
2. Skill `kspec-prd` → `spec/tasks/NNN-prd-<slug>/prd.md`
3. Skill `kspec-techspec` → `techspec.md`
4. Skill `kspec-tasks` → `tasks.md` + `N_task.md`
5. Skill `kspec-implement` → implementação sequencial com review por task

As features 001–010 foram implementadas assim.

## Skills Disponíveis

Para invocar uma skill no Cursor Agent, descreva a ação em linguagem natural ou mencione explicitamente o nome da skill (ex.: `kspec-prd`). Os arquivos de skill estão em `.agents/skills/<nome>/SKILL.md`.

| Skill | Invocação Cursor | Função |
| --- | --- | --- |
| `kspec-ideia` | "faça brainstorm de uma ideia" ou `kspec-ideia` | Brainstorm/discovery para decompor ideia em módulos |
| `kspec-prd` | "crie um PRD para..." ou `kspec-prd` | Cria PRD a partir de solicitação de funcionalidade |
| `kspec-techspec` | "crie a tech spec para..." ou `kspec-techspec` | Traduz PRD em especificação técnica |
| `kspec-tasks` | "quebre em tasks..." ou `kspec-tasks` | Quebra Tech Spec em tarefas incrementais |
| `kspec-implement` | "implemente as tasks de..." ou `kspec-implement` | Executa todas as tasks pendentes |
| `kspec-qa` | "execute QA de..." ou `kspec-qa` | Quality Assurance (E2E, acessibilidade) |
| `kspec-pr-review` | "revisão semântica da entrega antes do PR" ou `kspec-pr-review` | Alinhamento spec × implementação e corpo do PR |
| `kspec-bugfix` | "corrija o bug documentado em..." ou `kspec-bugfix` | Corrige bugs documentados pelo QA |
| `kspec-bootstrap` | "configure o kspec neste projeto" ou `kspec-bootstrap` | Gera configuração kspec para projeto existente |
| `kspec-version` | "qual a versão do kspec?" ou `kspec-version` | Exibe versão atual e lista skills/agents |

## Agents

Os agents são acionados automaticamente pelas skills. No Cursor, a delegação ocorre via **Task tool** com `subagent_type` correspondente. Definições canônicas em `.agents/agents/<nome>/AGENT.md`.

| Agent | Acionado por | `subagent_type` | Função |
| --- | --- | --- | --- |
| `kspec-task-runner` | `kspec-implement` | `kspec-task-runner` | Implementa uma task em contexto isolado |
| `kspec-review-runner` | `kspec-implement` | `kspec-review-runner` | Code review contra spec e rules |
| `kspec-qa-runner` | `kspec-qa` | `kspec-qa-runner` | Testa E2E, acessibilidade, visual |

## Rules

`.agents/rules/` contém rules do ecossistema kspec (orientadas a TypeScript/Java), publicadas em `.cursor/rules/*.mdc` (artefatos derivados). Para código Go do kterminal, prevalecem os padrões do próprio código-base:

- **Sem comentários no código**
- Eventos do agente via canal; TUI com receivers por valor; `strings.Builder` sempre por ponteiro
- Tools registradas no construtor do `Registry` (`internal/tools/tools.go`)
- Assets embutidos via `go:embed` (precedentes: `models.yaml`, `markdown.json`)

## Git

- **Fluxo padrão de entrega**: commitar na branch da feature → fast-forward merge em `develop` → `main` → push das três branches → voltar para `develop` (branch padrão para iniciar novos trabalhos e o fluxo SDD)
- **Não execute** `git restore`, `git reset`, `git clean` ou comandos destrutivos **sem permissão explícita do usuário**
- Binário `kterminal` e `*.test` não são versionados (`.gitignore`)
- **Graphify ativo**: knowledge graph em `graphify-out/graph.json` (não versionado, build code-only). Git hooks (post-commit/post-checkout) o mantêm fresco via `graphify update` (sem LLM). Skills kspec o consultam seguindo `.agents/rules/graphify.md`; edges `INFERRED` são hipótese, `EXTRACTED` são fato

## Limitações conhecidas no Cursor

1. **Ausência de slash commands de projeto**: o Cursor não suporta slash commands de projeto (`/kspec-prd`). Use linguagem natural ou menção explícita (`kspec-prd`) para invocar skills.

2. **Delegação via Task tool**: `kspec-implement` e `kspec-qa` delegam agents via Task tool com `subagent_type`. Se a Task tool estiver indisponível, as skills executam inline com aviso.

3. **Ferramenta interativa `AskQuestion`**: no Cursor, use `AskQuestion` para escolhas estruturadas (equivalente ao `AskUserQuestion` do Claude Code). Skills como `kspec-prd`, `kspec-techspec`, `kspec-tasks`, `kspec-implement`, `kspec-bugfix` e `kspec-bootstrap` dependem de modo interativo.

4. **Rules derivadas**: `.cursor/rules/*.mdc` são gerados a partir de `.agents/rules/*.md`. Edite o source of truth em `.agents/rules/` — não edite `.mdc` manualmente.

5. **Symlinks em Windows**: em sistemas Windows, `.cursor/skills/` e `.cursor/agents/` usam cópias em vez de symlinks.
