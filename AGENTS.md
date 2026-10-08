# AGENTS.md

Guia para agentes de IA ao trabalhar com o código deste repositório — plataforma OpenAI Codex CLI.

Este projeto é o **kterminal** — um coding agent de terminal (TUI agêntica) em Go, no estilo Claude Code/opencode. O diferencial: o roteador externo **Jev** (Typesafe) escolhe o LLM a cada chamada, pesando qualidade, custo e tokens/s reais medidos. O desenvolvimento do próprio kterminal segue o fluxo SDD do **kspec** (dogfooding).

> Para Claude Code, consulte `CLAUDE.md`. Para Cursor, consulte `CURSOR.md`.

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
├── CURSOR.md                    # Guia equivalente para Cursor
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

Para invocar uma skill no Codex CLI, use `$kspec-<nome>` ou descreva a ação em linguagem natural. Os arquivos de skill estão em `.agents/skills/<nome>/SKILL.md`.

| Skill | Invocação Codex | Função |
| --- | --- | --- |
| `kspec-ideia` | `$kspec-ideia` ou "faça brainstorm de uma ideia" | Brainstorm/discovery para decompor ideia em módulos |
| `kspec-prd` | `$kspec-prd` ou "crie um PRD para..." | Cria PRD a partir de solicitação de funcionalidade |
| `kspec-techspec` | `$kspec-techspec` ou "crie a tech spec para..." | Traduz PRD em especificação técnica |
| `kspec-tasks` | `$kspec-tasks` ou "quebre em tasks..." | Quebra Tech Spec em tarefas incrementais |
| `kspec-implement` | `$kspec-implement` ou "implemente as tasks de..." | Executa todas as tasks pendentes |
| `kspec-qa` | `$kspec-qa` ou "execute QA de..." | Quality Assurance (E2E, acessibilidade) |
| `kspec-pr-review` | `$kspec-pr-review` ou "revisão semântica da entrega" | Alinhamento spec × implementação e corpo do PR |
| `kspec-bugfix` | `$kspec-bugfix` ou "corrija o bug documentado em..." | Corrige bugs documentados pelo QA |
| `kspec-bootstrap` | `$kspec-bootstrap` ou "configure o kspec neste projeto" | Gera configuração kspec para projeto existente |
| `kspec-version` | `$kspec-version` ou "qual a versão do kspec?" | Exibe versão atual e lista skills/agents |

## Agents

Os agents são acionados automaticamente pelas skills. No Codex CLI, os agents são definidos em `.codex/agents/<nome>.toml`.

| Agent | Acionado por | Sandbox | Função |
| --- | --- | --- | --- |
| `kspec-task-runner` | `$kspec-implement` | `workspace-write` | Implementa uma task em contexto isolado |
| `kspec-review-runner` | `$kspec-implement` | `read-only` | Code review contra spec e rules |
| `kspec-qa-runner` | `$kspec-qa` | `workspace-write` | Testa E2E, acessibilidade, visual |

## Rules

`.agents/rules/` contém rules do ecossistema kspec (orientadas a TypeScript/Java). Para código Go do kterminal, prevalecem os padrões do próprio código-base:

- **Sem comentários no código**
- Eventos do agente via canal; TUI com receivers por valor; `strings.Builder` sempre por ponteiro
- Tools registradas no construtor do `Registry` (`internal/tools/tools.go`)
- Assets embutidos via `go:embed` (precedentes: `models.yaml`, `markdown.json`)

## Git

- **Não execute** `git restore`, `git reset`, `git clean` ou comandos destrutivos **sem permissão explícita do usuário**
- Binário `kterminal` e `*.test` não são versionados (`.gitignore`)

## Limitações conhecidas no Codex

1. **Ausência de slash commands de projeto**: o Codex CLI não suporta slash commands de projeto (`/kspec-prd`). Use `$kspec-<nome>` ou linguagem natural.

2. **Ausência de `AskUserQuestion` em `codex exec`**: o modo não-interativo não suporta perguntas ao usuário. As skills `kspec-prd`, `kspec-techspec`, `kspec-tasks`, `kspec-implement`, `kspec-bugfix` e `kspec-bootstrap` só funcionam em modo interativo.

3. **Sandbox dos agents**: `kspec-task-runner` e `kspec-qa-runner` exigem `sandbox_mode = "workspace-write"`. Execute o Codex com permissões de escrita no workspace.

4. **MCP Opt-in**: MCPs não são descobertos automaticamente. Declare-os em `.codex/config.toml` (projeto) ou `~/.codex/config.toml` (global).

5. **Symlinks em Windows**: em sistemas Windows, `.codex/skills/` usa cópias em vez de symlinks.
